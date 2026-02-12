package handlers

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"time"
)

func certMetaFromJSON(meta []byte) (active *bool, caFingerprint string) {
	if len(meta) == 0 {
		return nil, ""
	}
	var obj map[string]any
	if err := json.Unmarshal(meta, &obj); err != nil {
		return nil, ""
	}
	hwops, ok := obj["hwops"].(map[string]any)
	if !ok {
		return nil, ""
	}
	cert, ok := hwops["cert"].(map[string]any)
	if !ok {
		return nil, ""
	}
	if v, ok := cert["active"].(bool); ok {
		active = &v
	}
	if v, ok := cert["caFingerprint"].(string); ok {
		caFingerprint = v
	}
	return active, caFingerprint
}

func updateCertMeta(meta []byte, updates map[string]any) []byte {
	var obj map[string]any
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &obj)
	}
	if obj == nil {
		obj = map[string]any{}
	}
	hwops, _ := obj["hwops"].(map[string]any)
	if hwops == nil {
		hwops = map[string]any{}
	}
	cert, _ := hwops["cert"].(map[string]any)
	if cert == nil {
		cert = map[string]any{}
	}
	for k, v := range updates {
		cert[k] = v
	}
	hwops["cert"] = cert
	obj["hwops"] = hwops
	merged, err := json.Marshal(obj)
	if err != nil {
		return nil
	}
	return merged
}

func updateCertMetaIfNeeded(meta []byte, active bool, checkedAt time.Time) []byte {
	currentActive, _ := certMetaFromJSON(meta)
	if currentActive != nil && *currentActive == active {
		return nil
	}
	return updateCertMeta(meta, map[string]any{
		"active":    active,
		"checkedAt": checkedAt.UTC().Format(time.RFC3339),
	})
}

func caFingerprintFromPEM(certPEM []byte) (string, *x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", nil, errors.New("invalid cert pem")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:]), cert, nil
}
