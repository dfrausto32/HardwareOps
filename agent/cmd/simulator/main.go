package main

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	mrand "math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

type client struct {
	baseURL string
	http    *http.Client
}

type enrollmentTokenResponse struct {
	Token string `json:"token"`
}

type enrollResponse struct {
	DeviceID  string `json:"deviceId"`
	CertPEM   string `json:"certPem"`
	CACertPEM string `json:"caCertPem"`
}

type checkinRequest struct {
	DeviceID     string         `json:"deviceId"`
	AgentVersion string         `json:"agentVersion"`
	Current      checkinCurrent `json:"current"`
}

type checkinCurrent struct {
	SoftwareVersion string `json:"softwareVersion"`
	ConfigRev       string `json:"configRev"`
}

func main() {
	baseURL := flag.String("base-url", "http://localhost:8080", "control-plane base URL")
	devices := flag.Int("devices", 5, "number of simulated devices")
	interval := flag.Duration("interval", 10*time.Second, "check-in interval")
	once := flag.Bool("once", false, "perform a single check-in per device and exit")
	skipEnroll := flag.Bool("skip-enroll", false, "skip enrollment and use random device IDs")
	caCertPath := flag.String("ca-cert", "", "path to control-plane CA cert (for https)")
	insecure := flag.Bool("insecure", false, "skip TLS verification (dev only)")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags)
	mrand.Seed(time.Now().UnixNano())
	httpClient, err := buildHTTPClient(*baseURL, *caCertPath, *insecure)
	if err != nil {
		logger.Fatalf("http client: %v", err)
	}
	c := &client{
		baseURL: *baseURL,
		http:    httpClient,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	wg.Add(*devices)
	for i := 0; i < *devices; i++ {
		idx := i
		go func() {
			defer wg.Done()
			if err := runDevice(ctx, c, idx, *interval, *once, *skipEnroll, logger); err != nil {
				logger.Printf("device %d error: %v", idx, err)
			}
		}()
	}

	wg.Wait()
}

func runDevice(ctx context.Context, c *client, idx int, interval time.Duration, once bool, skipEnroll bool, logger *log.Logger) error {
	deviceID := uuid.NewString()
	checkinClient := c.http
	if !skipEnroll {
		token, err := c.createEnrollmentToken(ctx)
		if err != nil {
			return err
		}
		csr, key, err := generateCSR(fmt.Sprintf("parcel-device-%d", idx))
		if err != nil {
			return err
		}
		resp, err := c.enroll(ctx, token, csr)
		if err != nil {
			return err
		}
		deviceID = resp.DeviceID

		if strings.HasPrefix(c.baseURL, "https://") && resp.CertPEM != "" && resp.CACertPEM != "" {
			mtlsClient, err := buildMTLSClient([]byte(resp.CertPEM), key, []byte(resp.CACertPEM))
			if err != nil {
				return err
			}
			checkinClient = mtlsClient
		}
	}

	agentVersion := "sim-0.1.0"
	softwareVersion := fmt.Sprintf("v1.%d", idx%3)
	configRev := fmt.Sprintf("c%d", idx%5)

	checkin := func() error {
		return c.checkIn(ctx, checkinRequest{
			DeviceID:     deviceID,
			AgentVersion: agentVersion,
			Current: checkinCurrent{
				SoftwareVersion: softwareVersion,
				ConfigRev:       configRev,
			},
		}, checkinClient)
	}

	if err := checkin(); err != nil {
		return err
	}
	if once {
		return nil
	}

	jitter := time.Duration(mrand.Intn(1000)) * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval + jitter):
			if err := checkin(); err != nil {
				logger.Printf("device %s check-in error: %v", deviceID, err)
			}
			jitter = time.Duration(mrand.Intn(1000)) * time.Millisecond
		}
	}
}

func (c *client) createEnrollmentToken(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, http.MethodPost, "/api/v1/enrollments", map[string]any{"expiresInSec": 3600})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("enrollments status=%d body=%s", resp.StatusCode, string(body))
	}
	var out enrollmentTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}
	return out.Token, nil
}

func (c *client) enroll(ctx context.Context, token string, csrPEM []byte) (enrollResponse, error) {
	payload := map[string]any{
		"token": token,
		"csr":   string(csrPEM),
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/v1/devices/enroll", payload)
	if err != nil {
		return enrollResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return enrollResponse{}, fmt.Errorf("devices/enroll status=%d body=%s", resp.StatusCode, string(body))
	}
	var out enrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return enrollResponse{}, err
	}
	if out.DeviceID == "" {
		return enrollResponse{}, fmt.Errorf("empty deviceId in response")
	}
	return out, nil
}

func (c *client) checkIn(ctx context.Context, req checkinRequest, httpClient *http.Client) error {
	if httpClient == nil {
		httpClient = c.http
	}
	resp, err := c.doWithClient(ctx, httpClient, http.MethodPost, "/api/v1/devices/checkin", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("checkin status=%d body=%s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *client) do(ctx context.Context, method, path string, payload any) (*http.Response, error) {
	return c.doWithClient(ctx, c.http, method, path, payload)
}

func (c *client) doWithClient(ctx context.Context, httpClient *http.Client, method, path string, payload any) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return httpClient.Do(req)
}

func generateCSR(commonName string) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	csrDER, err := x509.CreateCertificateRequest(crand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{CommonName: commonName},
	}, key)
	if err != nil {
		return nil, nil, err
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return csrPEM, keyPEM, nil
}

func buildHTTPClient(baseURL, caCertPath string, insecure bool) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.HasPrefix(baseURL, "https://") {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
		if caCertPath != "" {
			caPEM, err := os.ReadFile(caCertPath)
			if err != nil {
				return nil, err
			}
			pool := x509.NewCertPool()
			if ok := pool.AppendCertsFromPEM(caPEM); !ok {
				return nil, fmt.Errorf("invalid ca cert")
			}
			tlsConfig.RootCAs = pool
		}
		if insecure {
			tlsConfig.InsecureSkipVerify = true
		}
		transport.TLSClientConfig = tlsConfig
	}
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}, nil
}

func buildMTLSClient(certPEM, keyPEM, caPEM []byte) (*http.Client, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf("invalid ca cert")
	}
	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Timeout: 10 * time.Second, Transport: transport}, nil
}
