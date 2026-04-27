package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const totpEncryptedPrefix = "aes-gcm:"

// TOTPPendingClaims is the JWT payload for a short-lived intermediate token
// issued after successful password verification when TOTP is enabled.
// It is NOT a valid session token — the auth middleware rejects it for
// any route that requires authentication.
type TOTPPendingClaims struct {
	TOTPPending bool `json:"totp_pending"`
	jwt.RegisteredClaims
}

// GenerateTOTPKey creates a new TOTP key for the given account email.
// Returns the *otp.Key which carries both the raw secret and the otpauth URI
// for QR code generation.
func GenerateTOTPKey(issuer, email string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: email,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
}

// ValidateTOTPCode checks a 6-digit TOTP code against the raw (unencrypted) secret.
// Accepts one period of clock skew in either direction.
func ValidateTOTPCode(code, rawSecret string) bool {
	return totp.Validate(code, rawSecret)
}

// EncryptTOTPSecret encrypts the raw TOTP secret with AES-256-GCM.
// key must be exactly 32 bytes.
func EncryptTOTPSecret(key []byte, plaintext string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("TOTP encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return totpEncryptedPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptTOTPSecret reverses EncryptTOTPSecret.
func DecryptTOTPSecret(key []byte, encoded string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("TOTP encryption key must be 32 bytes")
	}
	encoded = strings.TrimPrefix(encoded, totpEncryptedPrefix)
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// IssueTOTPPendingToken issues a short-lived JWT that signals the client to
// complete TOTP verification. It does NOT grant access to any protected route.
func (m *Manager) IssueTOTPPendingToken(userID string) (string, time.Time, error) {
	now := time.Now().UTC()
	exp := now.Add(5 * time.Minute)
	claims := TOTPPendingClaims{
		TOTPPending: true,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, exp, nil
}

// VerifyTOTPPendingToken parses and validates a totp_pending JWT.
// Returns the userID embedded as the subject claim.
func (m *Manager) VerifyTOTPPendingToken(tokenStr string) (string, error) {
	claims := TOTPPendingClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, &claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return m.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return "", errors.New("invalid token")
	}
	if !claims.TOTPPending {
		return "", errors.New("not a totp_pending token")
	}
	if claims.Subject == "" {
		return "", errors.New("missing subject")
	}
	return claims.Subject, nil
}
