package main

import (
	"crypto"
	"crypto/ed25519"
	crand "crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	agentbootstrap "github.com/parcel/agent/internal/bootstrap"
	"github.com/parcel/agent/internal/client"
	"github.com/parcel/agent/internal/config"
	"github.com/parcel/agent/internal/logging"
	"github.com/parcel/agent/internal/state"
)

func bootstrapApprovalIdentity(cfg config.Config, st *state.State, logger *logging.Logger, capabilities map[string]any) error {
	if strings.TrimSpace(cfg.EnrollmentMode) != "approval" {
		return nil
	}
	if certFileExists(cfg.DeviceCertPath) {
		return nil
	}
	if strings.TrimSpace(cfg.EnrollmentProfileToken) == "" {
		return errors.New("approval bootstrap requires ENROLLMENT_PROFILE_TOKEN")
	}
	if strings.TrimSpace(cfg.DeviceCertPath) == "" || strings.TrimSpace(cfg.DeviceKeyPath) == "" || strings.TrimSpace(cfg.DeviceIDPath) == "" {
		return errors.New("approval bootstrap requires DEVICE_CERT_PATH, DEVICE_KEY_PATH, and DEVICE_ID_PATH")
	}

	if st.DeviceID != "" {
		st.DeviceID = ""
		logger.SetDeviceID("")
		if err := saveState(cfg.StatePath, *st); err != nil {
			logger.Warnf("save state: %v", err)
		}
	}

	bs, ok, err := agentbootstrap.Load(cfg.BootstrapStatePath)
	if err != nil {
		return fmt.Errorf("load bootstrap state: %w", err)
	}
	if !ok {
		bs = agentbootstrap.State{
			Mode:         agentbootstrap.ModeApproval,
			State:        agentbootstrap.StateBootstrapInit,
			KeyPath:      cfg.DeviceKeyPath,
			CertPath:     cfg.DeviceCertPath,
			DeviceIDPath: cfg.DeviceIDPath,
		}
	}
	bs.Mode = agentbootstrap.ModeApproval
	bs.KeyPath = cfg.DeviceKeyPath
	bs.CertPath = cfg.DeviceCertPath
	bs.DeviceIDPath = cfg.DeviceIDPath
	if hardwareID, _ := detectHardwareIdentity(); hardwareID != "" {
		bs.HardwareID = hardwareID
	}

	bootstrapClient := client.NewWithTLS(cfg.ControlPlaneURL, buildBootstrapTLSConfig(cfg, logger))
	for {
		if certFileExists(cfg.DeviceCertPath) {
			if err := syncIssuedIdentity(cfg, st, logger, bs.IssuedDeviceID); err != nil {
				return err
			}
			_ = agentbootstrap.Clear(cfg.BootstrapStatePath)
			return nil
		}
		if bs.State == agentbootstrap.StateMaterializeIdentity && bs.IssuedDeviceID != "" && bs.IssuedCertPEM != "" {
			if err := writeIssuedIdentity(cfg, st, logger, &bs); err != nil {
				bs.State = agentbootstrap.StateBootstrapBackoff
				bs.LastError = err.Error()
				if saveErr := agentbootstrap.Save(cfg.BootstrapStatePath, bs); saveErr != nil {
					logger.Warnf("save bootstrap state: %v", saveErr)
				}
				logger.Warnf("write issued identity failed: %v", err)
				time.Sleep(cfg.BootstrapRetryInterval)
				continue
			}
			return nil
		}
		if requestExpired(bs.ExpiresAt) {
			clearPendingRequest(&bs)
		}
		if bs.RequestID == "" || bs.ClaimToken == "" {
			key, err := ensureDeviceKey(cfg.DeviceKeyPath)
			if err != nil {
				return fmt.Errorf("ensure device key: %w", err)
			}
			csrPEM, err := generateCSR(key, pendingCSRCommonName(capabilities))
			if err != nil {
				return fmt.Errorf("generate bootstrap csr: %w", err)
			}
			bs.State = agentbootstrap.StateRequestSubmit
			bs.LastError = ""
			if err := agentbootstrap.Save(cfg.BootstrapStatePath, bs); err != nil {
				logger.Warnf("save bootstrap state: %v", err)
			}
			resp, err := bootstrapClient.RequestPendingEnrollment(client.PendingEnrollmentRequest{
				ProfileToken: cfg.EnrollmentProfileToken,
				CSR:          string(csrPEM),
				AgentVersion: st.AgentVersion,
				Capabilities: capabilities,
				Metadata:     enrollmentMetadata(),
			})
			if err != nil {
				if statusErr, ok := err.(client.StatusError); ok {
					switch statusErr.StatusCode {
					case 401, 403:
						bs.State = agentbootstrap.StateBootstrapBlocked
						bs.LastError = err.Error()
						_ = agentbootstrap.Save(cfg.BootstrapStatePath, bs)
						return fmt.Errorf("pending enrollment request rejected: %w", err)
					case 409:
						bs.State = agentbootstrap.StateBootstrapConflict
						bs.LastError = err.Error()
						_ = agentbootstrap.Save(cfg.BootstrapStatePath, bs)
						logger.Warnf("pending enrollment request conflict: %v", err)
						time.Sleep(cfg.BootstrapRetryInterval)
						continue
					}
				}
				bs.State = agentbootstrap.StateBootstrapBackoff
				bs.LastError = err.Error()
				if saveErr := agentbootstrap.Save(cfg.BootstrapStatePath, bs); saveErr != nil {
					logger.Warnf("save bootstrap state: %v", saveErr)
				}
				logger.Warnf("pending enrollment request failed: %v", err)
				time.Sleep(cfg.BootstrapRetryInterval)
				continue
			}
			bs.State = agentbootstrap.StatePendingApproval
			bs.RequestID = resp.RequestID
			bs.ClaimToken = resp.ClaimToken
			bs.ExpiresAt = resp.ExpiresAt
			bs.DeniedReason = ""
			bs.LastError = ""
			if err := agentbootstrap.Save(cfg.BootstrapStatePath, bs); err != nil {
				logger.Warnf("save bootstrap state: %v", err)
			}
			logger.Infof("pending enrollment requested request=%s", bs.RequestID)
		}

		resp, err := bootstrapClient.ClaimPendingEnrollment(client.ClaimPendingEnrollmentRequest{
			RequestID:  bs.RequestID,
			ClaimToken: bs.ClaimToken,
		})
		if err != nil {
			if statusErr, ok := err.(client.StatusError); ok && statusErr.StatusCode == 401 {
				clearPendingRequest(&bs)
				bs.State = agentbootstrap.StateKeypairReady
				bs.LastError = err.Error()
				if saveErr := agentbootstrap.Save(cfg.BootstrapStatePath, bs); saveErr != nil {
					logger.Warnf("save bootstrap state: %v", saveErr)
				}
				logger.Warnf("pending enrollment claim rejected; requesting a new enrollment")
				continue
			}
			bs.State = agentbootstrap.StateBootstrapBackoff
			bs.LastError = err.Error()
			if saveErr := agentbootstrap.Save(cfg.BootstrapStatePath, bs); saveErr != nil {
				logger.Warnf("save bootstrap state: %v", saveErr)
			}
			logger.Warnf("pending enrollment claim failed: %v", err)
			time.Sleep(cfg.BootstrapRetryInterval)
			continue
		}

		switch resp.Status {
		case "pending":
			if resp.SigningTrust != nil {
				applySigningTrust(st, resp.SigningTrust)
				if err := saveState(cfg.StatePath, *st); err != nil {
					logger.Warnf("save state: %v", err)
				}
			}
			bs.State = agentbootstrap.StatePendingApproval
			if !resp.ExpiresAt.IsZero() {
				bs.ExpiresAt = resp.ExpiresAt
			}
			bs.LastError = ""
			if err := agentbootstrap.Save(cfg.BootstrapStatePath, bs); err != nil {
				logger.Warnf("save bootstrap state: %v", err)
			}
			logger.Infof("pending enrollment awaiting approval request=%s", bs.RequestID)
			time.Sleep(claimPollInterval(cfg, resp.PollAfterSec))
		case "issued":
			if resp.SigningTrust != nil {
				applySigningTrust(st, resp.SigningTrust)
				if err := saveState(cfg.StatePath, *st); err != nil {
					logger.Warnf("save state: %v", err)
				}
			}
			bs.State = agentbootstrap.StateMaterializeIdentity
			bs.IssuedDeviceID = resp.DeviceID
			bs.IssuedCertPEM = resp.CertPEM
			bs.IssuedCACertPEM = resp.CACertPEM
			bs.LastError = ""
			if err := agentbootstrap.Save(cfg.BootstrapStatePath, bs); err != nil {
				logger.Warnf("save bootstrap state: %v", err)
			}
			if err := writeIssuedIdentity(cfg, st, logger, &bs); err != nil {
				bs.State = agentbootstrap.StateBootstrapBackoff
				bs.LastError = err.Error()
				if saveErr := agentbootstrap.Save(cfg.BootstrapStatePath, bs); saveErr != nil {
					logger.Warnf("save bootstrap state: %v", saveErr)
				}
				logger.Warnf("write issued identity failed: %v", err)
				time.Sleep(cfg.BootstrapRetryInterval)
				continue
			}
			// C8: clear the enrollment token from the process environment after
			// successful enrollment to prevent child processes from inheriting it.
			os.Unsetenv("ENROLLMENT_PROFILE_TOKEN")
			return nil
		case "denied":
			bs.State = agentbootstrap.StateBootstrapDenied
			bs.DeniedReason = resp.Reason
			bs.LastError = resp.Reason
			if err := agentbootstrap.Save(cfg.BootstrapStatePath, bs); err != nil {
				logger.Warnf("save bootstrap state: %v", err)
			}
			logger.Warnf("pending enrollment denied: %s", resp.Reason)
			time.Sleep(cfg.BootstrapRetryInterval)
		case "conflict":
			bs.State = agentbootstrap.StateBootstrapConflict
			bs.DeniedReason = resp.Reason
			bs.LastError = resp.Reason
			if err := agentbootstrap.Save(cfg.BootstrapStatePath, bs); err != nil {
				logger.Warnf("save bootstrap state: %v", err)
			}
			logger.Warnf("pending enrollment conflict: %s", resp.Reason)
			time.Sleep(cfg.BootstrapRetryInterval)
		case "expired":
			clearPendingRequest(&bs)
			bs.State = agentbootstrap.StateKeypairReady
			bs.LastError = "pending enrollment expired"
			if err := agentbootstrap.Save(cfg.BootstrapStatePath, bs); err != nil {
				logger.Warnf("save bootstrap state: %v", err)
			}
		default:
			bs.State = agentbootstrap.StateBootstrapBackoff
			bs.LastError = "unexpected pending enrollment claim status: " + resp.Status
			if err := agentbootstrap.Save(cfg.BootstrapStatePath, bs); err != nil {
				logger.Warnf("save bootstrap state: %v", err)
			}
			logger.Warnf("unexpected pending enrollment claim status=%s", resp.Status)
			time.Sleep(cfg.BootstrapRetryInterval)
		}
	}
}

func writeIssuedIdentity(cfg config.Config, st *state.State, logger *logging.Logger, bs *agentbootstrap.State) error {
	if bs == nil || bs.IssuedDeviceID == "" || bs.IssuedCertPEM == "" {
		return errors.New("issued identity missing from bootstrap state")
	}
	if err := writeTextFile(cfg.DeviceCertPath, bs.IssuedCertPEM, 0o644); err != nil {
		return fmt.Errorf("write device cert: %w", err)
	}
	if err := writeTextFile(cfg.DeviceIDPath, bs.IssuedDeviceID, 0o644); err != nil {
		return fmt.Errorf("write device id: %w", err)
	}
	if cfg.CACertPath != "" && bs.IssuedCACertPEM != "" && !certFileExists(cfg.CACertPath) {
		if err := writeTextFile(cfg.CACertPath, bs.IssuedCACertPEM, 0o644); err != nil {
			return fmt.Errorf("write bootstrap ca cert: %w", err)
		}
	}
	if err := syncIssuedIdentity(cfg, st, logger, bs.IssuedDeviceID); err != nil {
		return err
	}
	if err := agentbootstrap.Clear(cfg.BootstrapStatePath); err != nil {
		logger.Warnf("clear bootstrap state: %v", err)
	}
	return nil
}

func syncIssuedIdentity(cfg config.Config, st *state.State, logger *logging.Logger, issuedDeviceID string) error {
	deviceID := strings.TrimSpace(issuedDeviceID)
	if deviceID == "" && cfg.DeviceIDPath != "" {
		deviceID = readFileTrimmed(cfg.DeviceIDPath)
	}
	if deviceID == "" && cfg.DeviceCertPath != "" {
		certDeviceID, err := deviceIDFromCert(cfg.DeviceCertPath)
		if err != nil {
			return fmt.Errorf("read device cert: %w", err)
		}
		deviceID = certDeviceID
	}
	if deviceID == "" {
		return errors.New("issued device id missing")
	}
	st.DeviceID = deviceID
	logger.SetDeviceID(deviceID)
	if err := saveState(cfg.StatePath, *st); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	logger.Infof("approval bootstrap complete device=%s", deviceID)
	return nil
}

func ensureDeviceKey(path string) (crypto.Signer, error) {
	if certFileExists(path) {
		return loadPrivateKey(path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	_, key, err := ed25519.GenerateKey(crand.Reader)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(path, keyPEM, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func pendingCSRCommonName(capabilities map[string]any) string {
	if hw, ok := capabilities["hw"].(map[string]any); ok {
		if ident, ok := hw["identity"].(map[string]any); ok {
			if raw, ok := ident["id"].(string); ok {
				cn := strings.TrimSpace(raw)
				if cn != "" {
					return truncateCSRCommonName(cn)
				}
			}
		}
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		return truncateCSRCommonName(host)
	}
	return "parcel-pending"
}

func truncateCSRCommonName(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 128 {
		return v
	}
	return v[:128]
}

func enrollmentMetadata() map[string]any {
	meta := map[string]any{
		"os": goruntime.GOOS,
	}
	if host, err := os.Hostname(); err == nil && strings.TrimSpace(host) != "" {
		meta["hostname"] = strings.TrimSpace(host)
	}
	return meta
}

func requestExpired(expiresAt time.Time) bool {
	return !expiresAt.IsZero() && time.Now().UTC().After(expiresAt)
}

func clearPendingRequest(bs *agentbootstrap.State) {
	if bs == nil {
		return
	}
	bs.RequestID = ""
	bs.ClaimToken = ""
	bs.ExpiresAt = time.Time{}
	bs.IssuedDeviceID = ""
	bs.IssuedCertPEM = ""
	bs.IssuedCACertPEM = ""
	bs.DeniedReason = ""
}

func claimPollInterval(cfg config.Config, pollAfterSec int) time.Duration {
	if pollAfterSec > 0 {
		return time.Duration(pollAfterSec) * time.Second
	}
	if cfg.BootstrapPollInterval > 0 {
		return cfg.BootstrapPollInterval
	}
	return 5 * time.Second
}

func writeTextFile(path, contents string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(contents), mode); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func certFileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func buildBootstrapTLSConfig(cfg config.Config, logger *logging.Logger) *tls.Config {
	if cfg.CACertPath == "" {
		return nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	pool, err := x509.SystemCertPool()
	if err != nil {
		logger.Errorf("load system CA pool: %v", err)
		os.Exit(1)
	}
	if pool == nil {
		pool = x509.NewCertPool()
	}
	caPEM, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			tlsConfig.RootCAs = pool
			return tlsConfig
		}
		logger.Errorf("read control-plane CA: %v", err)
		os.Exit(1)
	}
	if ok := pool.AppendCertsFromPEM(caPEM); !ok {
		logger.Errorf("invalid control-plane CA cert")
		os.Exit(1)
	}
	tlsConfig.RootCAs = pool
	return tlsConfig
}
