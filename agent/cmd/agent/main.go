package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
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
		resp, err := c.CheckIn(st)
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
			desiredComponents := desiredComponents(resp.Desired)
			desiredSource := desiredSourceForLog(resp.Desired)
			logger.Infof("check-in ok desired=%v components=%d source=%s", len(desiredComponents) > 0, len(desiredComponents), desiredSource)
			if resp.Desired != nil && resp.Desired.CheckinInterval > 0 {
				interval = time.Duration(resp.Desired.CheckinInterval) * time.Second
			}

			if len(desiredComponents) > 0 {
				applyOpts := artifacts.ApplyOptions{
					AllowUnsupported:     cfg.AllowUnsupportedApply,
					SigningPublicKeyPath: cfg.SigningPubKeyPath,
					SigningKeyID:         cfg.SigningKeyID,
					RequireSignature:     cfg.RequireSignature,
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

func buildTLSConfig(cfg config.Config, logger *logging.Logger) *tls.Config {
	if cfg.CACertPath == "" && cfg.DeviceCertPath == "" && cfg.DeviceKeyPath == "" {
		return nil
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

func signatureKeyIDFromMetadata(meta json.RawMessage) string {
	if len(meta) == 0 {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(meta, &obj); err != nil {
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

	componentRoot := componentRootPath(root, component)
	logger.Infof("apply start component=%s artifact=%s version=%s", component, desired.ArtifactID, targetVersion)
	outcome, err := artifacts.Apply(componentRoot, artifacts.Desired{
		ArtifactID:      desired.ArtifactID,
		SoftwareVersion: targetVersion,
		ConfigRev:       desired.ConfigRev,
		DownloadURL:     presign,
	}, artifacts.ArtifactMeta{
		ArtifactID:     desired.ArtifactID,
		SHA256:         meta.SHA256,
		SizeBytes:      meta.SizeBytes,
		Version:        meta.Version,
		Type:           meta.Type,
		Signature:      meta.Signature,
		SignatureKeyID: signatureKeyIDFromMetadata(meta.Metadata),
	}, c.HTTPClient(), logger, applyOpts)
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
