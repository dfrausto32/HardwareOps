package auth

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

const defaultRecoveryCodeCount = 8

func GenerateRecoveryCodes(count int) ([]string, []string, error) {
	if count <= 0 {
		count = defaultRecoveryCodeCount
	}
	plain := make([]string, 0, count)
	hashes := make([]string, 0, count)
	encoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	for i := 0; i < count; i++ {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		encoded := strings.ToUpper(encoder.EncodeToString(raw))
		code := strings.Join([]string{
			encoded[0:4],
			encoded[4:8],
			encoded[8:12],
			encoded[12:16],
		}, "-")
		plain = append(plain, code)
		hashes = append(hashes, HashToken(NormalizeRecoveryCode(code)))
	}
	return plain, hashes, nil
}

func NormalizeRecoveryCode(code string) string {
	code = strings.TrimSpace(strings.ToUpper(code))
	replacer := strings.NewReplacer("-", "", " ", "", "\t", "", "\n", "", "\r", "")
	return replacer.Replace(code)
}
