package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hardwareops/agent/internal/artifacts"
	"github.com/hardwareops/agent/internal/client"
	"github.com/hardwareops/agent/internal/config"
	"github.com/hardwareops/agent/internal/state"
)

func main() {
	cfg := config.FromEnv()
	once := flag.Bool("once", false, "run a single check-in and exit")
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags)

	st, err := state.Load(cfg.StatePath)
	if err != nil {
		logger.Fatalf("load state: %v", err)
	}

	if cfg.DeviceCertPath != "" {
		deviceID, err := deviceIDFromCert(cfg.DeviceCertPath)
		if err != nil {
			logger.Fatalf("read device cert: %v", err)
		}
		if deviceID != "" && st.DeviceID != deviceID {
			st.DeviceID = deviceID
			if err := state.Save(cfg.StatePath, st); err != nil {
				logger.Printf("save state: %v", err)
			}
		}
	}

	tlsConfig := buildTLSConfig(cfg, logger)
	c := client.NewWithTLS(cfg.ControlPlaneURL, tlsConfig)
	interval := cfg.CheckinInterval
	for {
		resp, err := c.CheckIn(st)
		if err != nil {
			logger.Printf("check-in failed: %v", err)
			if *once {
				os.Exit(1)
			}
		} else {
			if resp.Desired != nil && resp.Desired.CheckinInterval > 0 {
				interval = time.Duration(resp.Desired.CheckinInterval) * time.Second
			}

			if resp.Desired != nil && resp.Desired.ArtifactID != "" {
				if resp.Desired.SoftwareVersion == "" || st.CurrentVersion != resp.Desired.SoftwareVersion || st.CurrentConfigRev != resp.Desired.ConfigRev {
					applyErr := applyDesired(cfg.ArtifactRoot, c, resp.Desired, &st, logger)
					if applyErr != nil {
						if err := state.Save(cfg.StatePath, st); err != nil {
							logger.Printf("save state: %v", err)
						}
						if *once {
							os.Exit(1)
						}
					} else if err := state.Save(cfg.StatePath, st); err != nil {
						logger.Printf("save state: %v", err)
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

func buildTLSConfig(cfg config.Config, logger *log.Logger) *tls.Config {
	if cfg.CACertPath == "" && cfg.DeviceCertPath == "" && cfg.DeviceKeyPath == "" {
		return nil
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}

	if cfg.CACertPath != "" {
		caPEM, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			logger.Fatalf("read control-plane CA: %v", err)
		}
		pool := x509.NewCertPool()
		if ok := pool.AppendCertsFromPEM(caPEM); !ok {
			logger.Fatal("invalid control-plane CA cert")
		}
		tlsConfig.RootCAs = pool
	}

	if cfg.DeviceCertPath != "" || cfg.DeviceKeyPath != "" {
		if cfg.DeviceCertPath == "" || cfg.DeviceKeyPath == "" {
			logger.Fatal("DEVICE_CERT_PATH and DEVICE_KEY_PATH are required for mTLS")
		}
		cert, err := tls.LoadX509KeyPair(cfg.DeviceCertPath, cfg.DeviceKeyPath)
		if err != nil {
			logger.Fatalf("load device cert: %v", err)
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

func applyDesired(root string, c *client.Client, desired *client.DesiredState, st *state.State, logger *log.Logger) error {
	oldVersion := st.CurrentVersion
	targetVersion := desired.SoftwareVersion

	meta, err := c.GetArtifact(desired.ArtifactID)
	if err != nil {
		return reportApplyError(c, st, fmt.Sprintf("get artifact: %v", err), logger)
	}

	presign := desired.DownloadURL
	if presign == "" {
		pres, err := c.PresignArtifact(desired.ArtifactID)
		if err != nil {
			return reportApplyError(c, st, fmt.Sprintf("presign artifact: %v", err), logger)
		}
		presign = pres.DownloadURL
	}

	if targetVersion == "" {
		targetVersion = meta.Version
	}
	if targetVersion == "" {
		return reportApplyError(c, st, "desired version missing and artifact has no version", logger)
	}
	if st.CurrentVersion == targetVersion && st.CurrentConfigRev == desired.ConfigRev {
		return nil
	}

	err = artifacts.Apply(root, artifacts.Desired{
		ArtifactID:      desired.ArtifactID,
		SoftwareVersion: targetVersion,
		ConfigRev:       desired.ConfigRev,
		DownloadURL:     presign,
	}, artifacts.ArtifactMeta{
		ArtifactID: desired.ArtifactID,
		SHA256:     meta.SHA256,
		SizeBytes:  meta.SizeBytes,
		Version:    meta.Version,
	}, c.HTTPClient())
	if err != nil {
		errMsg := fmt.Sprintf("apply artifact: %v", err)
		if oldVersion != "" {
			if rbErr := artifacts.RollbackToVersion(root, oldVersion); rbErr != nil {
				errMsg = fmt.Sprintf("%s; rollback failed: %v", errMsg, rbErr)
			}
		}
		return reportApplyError(c, st, errMsg, logger)
	}

	st.PreviousVersion = oldVersion
	st.CurrentVersion = targetVersion
	st.CurrentConfigRev = desired.ConfigRev
	st.LastApplyStatus = "success"
	st.LastApplyError = ""

	if err := c.PostApplyResult(st.DeviceID, client.ApplyResultRequest{
		Status:           "success",
		AppliedVersion:   st.CurrentVersion,
		AppliedConfigRev: st.CurrentConfigRev,
	}); err != nil {
		logger.Printf("post apply result: %v", err)
	}
	return nil
}

func reportApplyError(c *client.Client, st *state.State, errMsg string, logger *log.Logger) error {
	st.LastApplyStatus = "error"
	st.LastApplyError = errMsg
	if err := c.PostApplyResult(st.DeviceID, client.ApplyResultRequest{
		Status: "error",
		Error:  errMsg,
	}); err != nil {
		logger.Printf("post apply result: %v", err)
	}
	return errors.New(errMsg)
}
