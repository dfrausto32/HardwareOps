package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
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
	for {
		resp, err := c.CheckIn(st)
		if err != nil {
			logger.Warnf("check-in failed: %v", err)
			if *once {
				os.Exit(1)
			}
		} else {
			logger.Infof("check-in ok desired=%v", resp.Desired != nil)
			if resp.Desired != nil && resp.Desired.CheckinInterval > 0 {
				interval = time.Duration(resp.Desired.CheckinInterval) * time.Second
			}

			if resp.Desired != nil && resp.Desired.ArtifactID != "" {
				if resp.Desired.SoftwareVersion == "" || st.CurrentVersion != resp.Desired.SoftwareVersion || st.CurrentConfigRev != resp.Desired.ConfigRev {
					applyErr := applyDesired(cfg.ArtifactRoot, c, resp.Desired, &st, logger, cfg.AllowUnsupportedApply)
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
		}

		if *once {
			return
		}
		time.Sleep(interval)
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

func applyDesired(root string, c *client.Client, desired *client.DesiredState, st *state.State, logger *logging.Logger, allowUnsupported bool) error {
	oldVersion := st.CurrentVersion
	targetVersion := desired.SoftwareVersion

	meta, err := c.GetArtifact(desired.ArtifactID)
	if err != nil {
		return reportApplyError(c, st, fmt.Sprintf("get artifact: %v", err), artifacts.ApplyOutcome{}, logger)
	}

	presign := desired.DownloadURL
	if presign == "" {
		pres, err := c.PresignArtifact(desired.ArtifactID)
		if err != nil {
			return reportApplyError(c, st, fmt.Sprintf("presign artifact: %v", err), artifacts.ApplyOutcome{}, logger)
		}
		presign = pres.DownloadURL
	}

	if targetVersion == "" {
		targetVersion = meta.Version
	}
	if targetVersion == "" {
		return reportApplyError(c, st, "desired version missing and artifact has no version", artifacts.ApplyOutcome{}, logger)
	}
	if st.CurrentVersion == targetVersion && st.CurrentConfigRev == desired.ConfigRev {
		return nil
	}

	logger.Infof("apply start artifact=%s version=%s", desired.ArtifactID, targetVersion)
	outcome, err := artifacts.Apply(root, artifacts.Desired{
		ArtifactID:      desired.ArtifactID,
		SoftwareVersion: targetVersion,
		ConfigRev:       desired.ConfigRev,
		DownloadURL:     presign,
	}, artifacts.ArtifactMeta{
		ArtifactID: desired.ArtifactID,
		SHA256:     meta.SHA256,
		SizeBytes:  meta.SizeBytes,
		Version:    meta.Version,
		Type:       meta.Type,
	}, c.HTTPClient(), logger, artifacts.ApplyOptions{AllowUnsupported: allowUnsupported})
	if err != nil {
		errMsg := fmt.Sprintf("apply artifact: %v", err)
		if oldVersion != "" {
			if rbErr := artifacts.RollbackToVersion(root, oldVersion); rbErr != nil {
				errMsg = fmt.Sprintf("%s; rollback failed: %v", errMsg, rbErr)
			}
		}
		return reportApplyError(c, st, errMsg, outcome, logger)
	}

	st.PreviousVersion = oldVersion
	st.CurrentVersion = targetVersion
	st.CurrentConfigRev = desired.ConfigRev
	st.LastApplyStatus = "success"
	st.LastApplyError = ""
	logger.Infof("apply success version=%s", targetVersion)

	if err := c.PostApplyResult(st.DeviceID, client.ApplyResultRequest{
		Status:           "success",
		AppliedVersion:   st.CurrentVersion,
		AppliedConfigRev: st.CurrentConfigRev,
		PreApplyStatus:   outcome.PreApplyStatus,
		PreApplyError:    outcome.PreApplyError,
	}); err != nil {
		logger.Warnf("post apply result: %v", err)
	}
	return nil
}

func reportApplyError(c *client.Client, st *state.State, errMsg string, outcome artifacts.ApplyOutcome, logger *logging.Logger) error {
	st.LastApplyStatus = "error"
	st.LastApplyError = errMsg
	if err := c.PostApplyResult(st.DeviceID, client.ApplyResultRequest{
		Status:         "error",
		Error:          errMsg,
		PreApplyStatus: outcome.PreApplyStatus,
		PreApplyError:  outcome.PreApplyError,
	}); err != nil {
		logger.Warnf("post apply result: %v", err)
	}
	return errors.New(errMsg)
}
