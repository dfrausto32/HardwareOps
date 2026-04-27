package certs

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"sync/atomic"
	"time"

	cpcrypto "github.com/parcel/control-plane/internal/crypto"
)

type Signer interface {
	SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) (certPEM []byte, fingerprint string, err error)
	CACertPEM() []byte
}

type State struct {
	Signer               Signer
	ActivePool           *x509.CertPool
	ActiveCert           *x509.Certificate
	ActiveFingerprint    string
	ActiveCertPEM        []byte
	ActiveCertPath       string
	ActiveKeyPath        string
	ClientCAPath         string
	ClientPool           *x509.CertPool
	ClientCerts          []*x509.Certificate
	ClientContainsActive bool
}

type Manager struct {
	state atomic.Value // *State
}

func NewManager() *Manager {
	m := &Manager{}
	m.state.Store(&State{})
	return m
}

func (m *Manager) State() *State {
	if m == nil {
		return &State{}
	}
	if v := m.state.Load(); v != nil {
		return v.(*State)
	}
	return &State{}
}

func (m *Manager) Load(activeCertPath, activeKeyPath, clientCAPath string) (*State, error) {
	state := &State{
		ActiveCertPath: activeCertPath,
		ActiveKeyPath:  activeKeyPath,
		ClientCAPath:   clientCAPath,
	}

	if clientCAPath != "" {
		certs, pool, err := loadCerts(clientCAPath)
		if err != nil {
			return nil, err
		}
		state.ClientCerts = certs
		state.ClientPool = pool
	}

	if activeCertPath != "" {
		certs, pool, err := loadCerts(activeCertPath)
		if err != nil {
			return nil, err
		}
		state.ActiveCert = certs[0]
		state.ActiveFingerprint = certFingerprint(state.ActiveCert)
		state.ActiveCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: state.ActiveCert.Raw})

		if activeKeyPath != "" {
			keyPEM, err := os.ReadFile(activeKeyPath)
			if err != nil {
				return nil, err
			}
			ca, err := cpcrypto.LoadCA(state.ActiveCertPEM, keyPEM)
			if err != nil {
				return nil, err
			}
			state.Signer = &cpcrypto.CASigner{CA: ca, CAPEM: state.ActiveCertPEM}
			state.ActivePool = pool
		}
		if state.ClientPool != nil {
			state.ClientContainsActive = containsCert(state.ClientCerts, state.ActiveFingerprint)
		}
	}

	m.state.Store(state)
	return state, nil
}

func (m *Manager) Reload() (*State, error) {
	state := m.State()
	return m.Load(state.ActiveCertPath, state.ActiveKeyPath, state.ClientCAPath)
}

func (m *Manager) SignDeviceCert(csrPEM []byte, deviceID string, validity time.Duration) ([]byte, string, error) {
	signer := m.State().Signer
	if signer == nil {
		return nil, "", errors.New("signer not configured")
	}
	return signer.SignDeviceCert(csrPEM, deviceID, validity)
}

func (m *Manager) CACertPEM() []byte {
	state := m.State()
	if state.Signer != nil {
		return state.Signer.CACertPEM()
	}
	return state.ActiveCertPEM
}

func (m *Manager) ActivePool() *x509.CertPool {
	return m.State().ActivePool
}

func (m *Manager) ClientPool() *x509.CertPool {
	return m.State().ClientPool
}

func loadCerts(path string) ([]*x509.Certificate, *x509.CertPool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var certs []*x509.Certificate
	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			data = rest
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, err
		}
		certs = append(certs, cert)
		data = rest
	}
	if len(certs) == 0 {
		return nil, nil, errors.New("no certificates found")
	}
	pool := x509.NewCertPool()
	for _, cert := range certs {
		pool.AddCert(cert)
	}
	return certs, pool, nil
}

func containsCert(certs []*x509.Certificate, fingerprint string) bool {
	for _, cert := range certs {
		if certFingerprint(cert) == fingerprint {
			return true
		}
	}
	return false
}

func certFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}
