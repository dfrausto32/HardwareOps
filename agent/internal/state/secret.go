package state

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/pem"
	"os"
)

// DeriveStateHMACKey reads the device private key at keyPath and derives a
// 32-byte HMAC key from it. Returns nil, nil if the key file does not exist
// yet (pre-enrollment). Returns an error if the file exists but cannot be
// parsed.
func DeriveStateHMACKey(keyPath string) ([]byte, error) {
	if keyPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(keyPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, nil
	}
	// Use SHA-256 of the raw DER key material as the HMAC key.
	sum := sha256.Sum256(block.Bytes)
	return sum[:], nil
}

func computeHMAC(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}
