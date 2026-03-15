package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hardwareops/agent/internal/artifacts"
	"github.com/hardwareops/agent/internal/client"
	"github.com/hardwareops/agent/internal/config"
	"github.com/hardwareops/agent/internal/logging"
	"github.com/hardwareops/agent/internal/state"
)

func main() {
	cfg := config.FromEnv()
	once := flag.Bool("once", false, "run a single check-in and exit")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exporter := logging.NewExporter(cfg.LogExportAddr)
	if exporter != nil {
		exporter.Start(ctx)
	}
	logger := logging.New(logging.ParseLevel(cfg.LogLevel), "agent", "", exporter)
	if cfg.LogExportAddr != "" {
		logger.Infof("log export enabled addr=%s", cfg.LogExportAddr)
	}

	st, err := state.Load(cfg.StatePath)
	if err != nil {
		logger.Errorf("load state: %v", err)
		os.Exit(1)
	}
	logger.SetDeviceID(st.DeviceID)

	capabilities := buildCapabilities(logger)
	if err := bootstrapApprovalIdentity(cfg, &st, logger, capabilities); err != nil {
		logger.Errorf("approval bootstrap: %v", err)
		os.Exit(1)
	}

	if cfg.DeviceCertPath != "" {
		deviceID, err := deviceIDFromCert(cfg.DeviceCertPath)
		if err != nil {
			logger.Errorf("read device cert: %v", err)
			os.Exit(1)
		}
		if deviceID != "" && st.DeviceID != deviceID {
			st.DeviceID = deviceID
			logger.SetDeviceID(deviceID)
			if err := state.Save(cfg.StatePath, st); err != nil {
				logger.Warnf("save state: %v", err)
			}
		}
	}

	tlsConfig := buildTLSConfig(cfg, logger)
	c := client.NewWithTLS(cfg.ControlPlaneURL, tlsConfig)
	interval := cfg.CheckinInterval
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	if cfg.CheckinStartupJitter > 0 {
		delay := jitterDuration(cfg.CheckinStartupJitter, rng)
		if delay > 0 {
			logger.Infof("startup jitter sleep=%s", delay)
			time.Sleep(delay)
		}
	}
	for {
		resp, err := c.CheckIn(st, capabilities)
		if err != nil {
			if rl, ok := err.(client.RateLimitError); ok {
				wait := rl.RetryAfter
				if wait <= 0 {
					wait = interval
				}
				wait += jitterDuration(jitterMax(interval, cfg), rng)
				logger.Warnf("check-in rate limited; retrying in %s", wait)
				if *once {
					os.Exit(1)
				}
				time.Sleep(wait)
				continue
			}
			logger.Warnf("check-in failed: %v", err)
			if *once {
				os.Exit(1)
			}
		} else {
			if resp != nil && resp.SigningTrust != nil {
				applySigningTrust(&st, resp.SigningTrust)
			}
			if cfg.AutoReenroll && resp != nil && len(resp.PendingActions) > 0 {
				if reenrolled, err := handlePendingActions(resp.PendingActions, cfg, c, &st, logger); err != nil {
					logger.Warnf("handle pending actions: %v", err)
				} else if reenrolled {
					tlsConfig = buildTLSConfig(cfg, logger)
					c = client.NewWithTLS(cfg.ControlPlaneURL, tlsConfig)
				}
			}
			desiredComponents := desiredComponents(resp.Desired)
			desiredSource := desiredSourceForLog(resp.Desired)
			logger.Infof("check-in ok desired=%v components=%d source=%s", len(desiredComponents) > 0, len(desiredComponents), desiredSource)
			if resp.Desired != nil && resp.Desired.CheckinInterval > 0 {
				interval = time.Duration(resp.Desired.CheckinInterval) * time.Second
			}

			if len(desiredComponents) > 0 {
				applyOpts := artifacts.ApplyOptions{
					AllowUnsupported:    cfg.AllowUnsupportedApply,
					SigningPublicKeyPath: cfg.SigningPubKeyPath,
					SigningKeyID:        cfg.SigningKeyID,
					RequireSignature:    cfg.RequireSignature,
					TrustKeys:           trustKeysFromState(st),
					VerificationMode:    cfg.VerificationMode,
				}
				applyErr := applyDesiredComponents(cfg.ArtifactRoot, c, desiredComponents, &st, logger, applyOpts)
				if applyErr != nil {
					if err := state.Save(cfg.StatePath, st); err != nil {
						logger.Warnf("save state: %v", err)
					}
					if *once {
						os.Exit(1)
					}
				} else if err := state.Save(cfg.StatePath, st); err != nil {
					logger.Warnf("save state: %v", err)
				}
			}
		}

		if *once {
			return
		}
		time.Sleep(interval + jitterDuration(jitterMax(interval, cfg), rng))
	}
}

func handlePendingActions(actions []client.Action, cfg config.Config, c *client.Client, st *state.State, logger *logging.Logger) (bool, error) {
	for _, action := range actions {
		if action.Type != "device.reenroll" {
			continue
		}
		logger.Infof("reenroll requested action=%s", action.ActionID)
		if err := reenrollDevice(c, cfg, st, logger); err != nil {
			return false, err
		}
		logger.Infof("reenroll complete")
		return true, nil
	}
	return false, nil
}

func reenrollDevice(c *client.Client, cfg config.Config, st *state.State, logger *logging.Logger) error {
	if cfg.DeviceKeyPath == "" || cfg.DeviceCertPath == "" {
		return errors.New("device key/cert path required for reenroll")
	}
	key, err := loadPrivateKey(cfg.DeviceKeyPath)
	if err != nil {
		return fmt.Errorf("load device key: %w", err)
	}
	deviceID := st.DeviceID
	if deviceID == "" {
		return errors.New("device id missing")
	}
	csrPEM, err := generateCSR(key, deviceID)
	if err != nil {
		return fmt.Errorf("generate csr: %w", err)
	}
	resp, err := c.Reenroll(csrPEM)
	if err != nil {
		return fmt.Errorf("reenroll: %w", err)
	}
	if err := os.WriteFile(cfg.DeviceCertPath, []byte(resp.CertPEM), 0o644); err != nil {
		return fmt.Errorf("write device cert: %w", err)
	}
	// Persist the returned CA when the agent is configured with an explicit CA file.
	// This keeps private-CA deployments working across CA rotation after reenroll.
	if err := persistReenrollCACert(cfg, resp.CACertPEM); err != nil {
		return err
	}
	return nil
}

func persistReenrollCACert(cfg config.Config, caPEM string) error {
	if strings.TrimSpace(cfg.CACertPath) == "" || strings.TrimSpace(caPEM) == "" {
		return nil
	}
	if !strings.HasSuffix(caPEM, "\n") {
		caPEM += "\n"
	}
	pool := x509.NewCertPool()
	if ok := pool.AppendCertsFromPEM([]byte(caPEM)); !ok {
		return errors.New("write reenroll ca cert: invalid ca cert pem")
	}
	if err := writeTextFile(cfg.CACertPath, caPEM, 0o644); err != nil {
		return fmt.Errorf("write reenroll ca cert: %w", err)
	}
	return nil
}

func loadPrivateKey(path string) (crypto.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("invalid key pem")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		switch k := key.(type) {
		case *rsa.PrivateKey:
			return k, nil
		case *ecdsa.PrivateKey:
			return k, nil
		case ed25519.PrivateKey:
			return k, nil
		default:
			return nil, errors.New("unsupported pkcs8 key")
		}
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, errors.New("unsupported key format")
}

func generateCSR(key crypto.Signer, commonName string) ([]byte, error) {
	req := &x509.CertificateRequest{Subject: pkix.Name{CommonName: commonName}}
	der, err := x509.CreateCertificateRequest(crand.Reader, req, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), nil
}

func buildTLSConfig(cfg config.Config, logger *logging.Logger) *tls.Config {
	// If no TLS config at all, allow it (unauthenticated dev mode).
	if cfg.CACertPath == "" && cfg.DeviceCertPath == "" && cfg.DeviceKeyPath == "" {
		return nil
	}
	// Once any TLS option is set, require the CA cert to prevent system CA fallback.
	if cfg.CACertPath == "" {
		logger.Errorf("CONTROL_PLANE_CA_CERT_PATH is required when using mTLS; refusing to fall back to system CA store")
		os.Exit(1)
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if cfg.CACertPath != "" {
		caPEM, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			logger.Errorf("read control-plane CA: %v", err)
			os.Exit(1)
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(caPEM); !ok {
			logger.Errorf("invalid control-plane CA cert")
			os.Exit(1)
		}
		tlsConfig.RootCAs = pool
	}

	if cfg.DeviceCertPath != "" || cfg.DeviceKeyPath != "" {
		if cfg.DeviceCertPath == "" || cfg.DeviceKeyPath == "" {
			logger.Errorf("DEVICE_CERT_PATH and DEVICE_KEY_PATH are required for mTLS")
			os.Exit(1)
		}
		cert, err := tls.LoadX509KeyPair(cfg.DeviceCertPath, cfg.DeviceKeyPath)
		if err != nil {
			logger.Errorf("load device cert: %v", err)
			os.Exit(1)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig
}

func deviceIDFromCert(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("invalid cert pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", err
	}
	return cert.Subject.CommonName, nil
}

func jitterMax(interval time.Duration, cfg config.Config) time.Duration {
	maxJitter := time.Duration(float64(interval) * cfg.CheckinJitterPercent)
	if cfg.CheckinJitterMax > 0 && maxJitter > cfg.CheckinJitterMax {
		maxJitter = cfg.CheckinJitterMax
	}
	return maxJitter
}

func jitterDuration(max time.Duration, rng *rand.Rand) time.Duration {
	if max <= 0 {
		return 0
	}
	if rng == nil {
		return time.Duration(rand.Int63n(int64(max)))
	}
	return time.Duration(rng.Int63n(int64(max)))
}

func signatureKeyIDFromMetadata(meta *client.ArtifactResponse) string {
	if meta == nil {
		return ""
	}
	if strings.TrimSpace(meta.SignatureKeyID) != "" {
		return strings.TrimSpace(meta.SignatureKeyID)
	}
	if len(meta.Metadata) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(meta.Metadata, &obj); err != nil {
		return ""
	}
	raw, ok := obj["signatureKeyId"]
	if !ok {
		return ""
	}
	val, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func desiredComponents(desired *client.DesiredState) map[string]client.DesiredComponent {
	if desired == nil {
		return nil
	}
	if len(desired.Components) > 0 {
		return desired.Components
	}
	if desired.ArtifactID == "" {
		return nil
	}
	return map[string]client.DesiredComponent{
		"app_bundle": {
			ArtifactID:      desired.ArtifactID,
			SoftwareVersion: desired.SoftwareVersion,
			ConfigRev:       desired.ConfigRev,
			DownloadURL:     desired.DownloadURL,
			ApplyPolicy:     desired.ApplyPolicy,
			Source:          desired.Source,
		},
	}
}

func desiredSourceForLog(desired *client.DesiredState) string {
	if desired == nil {
		return "none"
	}
	if len(desired.Components) == 0 {
		if desired.Source != "" {
			return desired.Source
		}
		return "unknown"
	}
	source := ""
	for _, comp := range desired.Components {
		if comp.Source == "" {
			source = "mixed"
			break
		}
		if source == "" {
			source = comp.Source
		} else if source != comp.Source {
			source = "mixed"
			break
		}
	}
	if source == "" {
		return "unknown"
	}
	return source
}

func applyDesiredComponents(root string, c *client.Client, desired map[string]client.DesiredComponent, st *state.State, logger *logging.Logger, applyOpts artifacts.ApplyOptions) error {
	if len(desired) == 0 {
		return nil
	}
	keys := make([]string, 0, len(desired))
	for key := range desired {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		component := normalizeComponentKey(key)
		if component == "" {
			continue
		}
		compDesired := desired[key]
		if compDesired.ArtifactID == "" {
			continue
		}
		if err := applyDesiredComponent(component, root, c, compDesired, st, logger, applyOpts); err != nil {
			return err
		}
	}
	return nil
}

func applyDesiredComponent(component, root string, c *client.Client, desired client.DesiredComponent, st *state.State, logger *logging.Logger, applyOpts artifacts.ApplyOptions) error {
	st.EnsureComponents()
	compState := st.Components[component]
	oldVersion := compState.CurrentVersion
	targetVersion := desired.SoftwareVersion

	meta, err := c.GetArtifact(desired.ArtifactID)
	if err != nil {
		return reportApplyError(c, st, component, fmt.Sprintf("get artifact: %v", err), artifacts.ApplyOutcome{}, logger, desired.ArtifactID)
	}

	presign := desired.DownloadURL
	if presign == "" {
		pres, err := c.PresignArtifact(desired.ArtifactID)
		if err != nil {
			return reportApplyError(c, st, component, fmt.Sprintf("presign artifact: %v", err), artifacts.ApplyOutcome{}, logger, desired.ArtifactID)
		}
		presign = pres.DownloadURL
	}

	if targetVersion == "" {
		targetVersion = meta.Version
	}
	if targetVersion == "" {
		return reportApplyError(c, st, component, "desired version missing and artifact has no version", artifacts.ApplyOutcome{}, logger, desired.ArtifactID)
	}
	if compState.CurrentVersion == targetVersion && compState.CurrentConfigRev == desired.ConfigRev {
		return nil
	}
	componentOpts := mergeApplyPolicyOptions(applyOpts, desired.ApplyPolicy)

	componentRoot := componentRootPath(root, component)
	logger.Infof("apply start component=%s artifact=%s version=%s", component, desired.ArtifactID, targetVersion)
	outcome, err := artifacts.Apply(componentRoot, artifacts.Desired{
		ArtifactID:      desired.ArtifactID,
		SoftwareVersion: targetVersion,
		ConfigRev:       desired.ConfigRev,
		DownloadURL:     presign,
	}, artifacts.ArtifactMeta{
		ArtifactID:         desired.ArtifactID,
		SHA256:             meta.SHA256,
		SizeBytes:          meta.SizeBytes,
		Version:            meta.Version,
		Type:               meta.Type,
		Signature:          meta.Signature,
		SignatureType:      signatureTypeFromMetadata(meta),
		SignatureKeyID:     signatureKeyIDFromMetadata(meta),
		VerificationStatus: meta.VerificationStatus,
		VerificationError:  meta.VerificationError,
	}, c.HTTPClient(), logger, componentOpts)
	if err != nil {
		errMsg := fmt.Sprintf("apply artifact: %v", err)
		if oldVersion != "" {
			if rbErr := artifacts.RollbackToVersion(componentRoot, oldVersion); rbErr != nil {
				errMsg = fmt.Sprintf("%s; rollback failed: %v", errMsg, rbErr)
			}
		}
		return reportApplyError(c, st, component, errMsg, outcome, logger, desired.ArtifactID)
	}

	compState.PreviousVersion = oldVersion
	compState.CurrentVersion = targetVersion
	compState.CurrentConfigRev = desired.ConfigRev
	compState.LastApplyStatus = "success"
	compState.LastApplyError = ""
	compState.LastApplyAt = time.Now().UTC()
	compState.LastApplyArtifactID = desired.ArtifactID
	if outcome.PreApplyStatus != "" {
		compState.LastPreApplyStatus = outcome.PreApplyStatus
		compState.LastPreApplyError = outcome.PreApplyError
		compState.LastPreApplyAt = time.Now().UTC()
	}
	st.Components[component] = compState
	if component == "app_bundle" {
		syncTopLevelFromComponent(st, compState)
	}
	logger.Infof("apply success component=%s version=%s", component, targetVersion)

	if err := c.PostApplyResult(st.DeviceID, client.ApplyResultRequest{
		Status:           "success",
		ArtifactID:       desired.ArtifactID,
		Component:        component,
		AppliedVersion:   compState.CurrentVersion,
		AppliedConfigRev: compState.CurrentConfigRev,
		PreApplyStatus:   outcome.PreApplyStatus,
		PreApplyError:    outcome.PreApplyError,
	}); err != nil {
		logger.Warnf("post apply result: %v", err)
	}
	return nil
}

type applyPolicy struct {
	VerificationMode      string   `json:"verificationMode,omitempty"`
	AllowedSigningKeyIDs  []string `json:"allowedSigningKeyIds,omitempty"`
	AllowedSignatureTypes []string `json:"allowedSignatureTypes,omitempty"`
	RequireSignature      *bool    `json:"requireSignature,omitempty"`
	SigningKeyID          string   `json:"signingKeyId,omitempty"`
}

func mergeApplyPolicyOptions(base artifacts.ApplyOptions, raw json.RawMessage) artifacts.ApplyOptions {
	if len(raw) == 0 || string(raw) == "null" {
		return base
	}
	var policy applyPolicy
	if err := json.Unmarshal(raw, &policy); err != nil {
		return base
	}
	out := base
	if mode := strings.TrimSpace(policy.VerificationMode); mode != "" {
		out.VerificationMode = mode
	}
	if len(policy.AllowedSigningKeyIDs) > 0 {
		out.AllowedSigningKeyIDs = policy.AllowedSigningKeyIDs
	}
	if len(policy.AllowedSignatureTypes) > 0 {
		out.AllowedSignatureTypes = policy.AllowedSignatureTypes
	}
	if policy.RequireSignature != nil {
		out.RequireSignature = *policy.RequireSignature
	}
	if keyID := strings.TrimSpace(policy.SigningKeyID); keyID != "" {
		out.SigningKeyID = keyID
	}
	return out
}

func applySigningTrust(st *state.State, bundle *client.SigningTrustBundle) {
	if st == nil || bundle == nil {
		return
	}
	keys := make([]state.SigningTrustKey, 0, len(bundle.Keys))
	for _, key := range bundle.Keys {
		if strings.TrimSpace(key.KeyID) == "" || strings.TrimSpace(key.PublicKeyPEM) == "" {
			continue
		}
		keys = append(keys, state.SigningTrustKey{
			KeyID:        strings.TrimSpace(key.KeyID),
			Algorithm:    strings.TrimSpace(key.Algorithm),
			PublicKeyPEM: key.PublicKeyPEM,
		})
	}
	st.SigningTrustUpdatedAt = bundle.UpdatedAt
	st.SigningTrustKeys = keys
}

func trustKeysFromState(st state.State) []artifacts.TrustKey {
	out := make([]artifacts.TrustKey, 0, len(st.SigningTrustKeys))
	for _, key := range st.SigningTrustKeys {
		out = append(out, artifacts.TrustKey{
			KeyID:        key.KeyID,
			Algorithm:    key.Algorithm,
			PublicKeyPEM: key.PublicKeyPEM,
		})
	}
	return out
}

func signatureTypeFromMetadata(meta *client.ArtifactResponse) string {
	if meta == nil {
		return ""
	}
	if strings.TrimSpace(meta.SignatureType) != "" {
		return strings.TrimSpace(meta.SignatureType)
	}
	if len(meta.Metadata) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(meta.Metadata, &obj); err != nil {
		return ""
	}
	raw, ok := obj["signatureType"]
	if !ok {
		return ""
	}
	val, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(val)
}

func reportApplyError(c *client.Client, st *state.State, component string, errMsg string, outcome artifacts.ApplyOutcome, logger *logging.Logger, artifactID string) error {
	st.EnsureComponents()
	compState := st.Components[component]
	compState.LastApplyStatus = "error"
	compState.LastApplyError = errMsg
	compState.LastApplyAt = time.Now().UTC()
	compState.LastApplyArtifactID = artifactID
	if outcome.PreApplyStatus != "" {
		compState.LastPreApplyStatus = outcome.PreApplyStatus
		compState.LastPreApplyError = outcome.PreApplyError
		compState.LastPreApplyAt = time.Now().UTC()
	}
	st.Components[component] = compState
	if component == "app_bundle" {
		syncTopLevelFromComponent(st, compState)
	}
	if err := c.PostApplyResult(st.DeviceID, client.ApplyResultRequest{
		Status:         "error",
		ArtifactID:     artifactID,
		Component:      component,
		Error:          errMsg,
		PreApplyStatus: outcome.PreApplyStatus,
		PreApplyError:  outcome.PreApplyError,
	}); err != nil {
		logger.Warnf("post apply result: %v", err)
	}
	return errors.New(errMsg)
}

func normalizeComponentKey(val string) string {
	clean := strings.TrimSpace(strings.ToLower(val))
	return clean
}

func componentRootPath(root, component string) string {
	if component == "" || component == "app_bundle" {
		return root
	}
	return filepath.Join(root, "components", component)
}

func syncTopLevelFromComponent(st *state.State, comp state.ComponentState) {
	st.PreviousVersion = comp.PreviousVersion
	st.CurrentVersion = comp.CurrentVersion
	st.CurrentConfigRev = comp.CurrentConfigRev
	st.LastApplyStatus = comp.LastApplyStatus
	st.LastApplyError = comp.LastApplyError
	st.LastApplyAt = comp.LastApplyAt
	st.LastApplyArtifactID = comp.LastApplyArtifactID
	st.LastPreApplyStatus = comp.LastPreApplyStatus
	st.LastPreApplyError = comp.LastPreApplyError
	st.LastPreApplyAt = comp.LastPreApplyAt
}
