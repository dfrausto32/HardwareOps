package artifactingest

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

type PullCredentialStatus struct {
	Configured           bool      `json:"configured"`
	StaticConfigured     bool      `json:"staticConfigured"`
	AWSSecretConfigured  bool      `json:"awsSecretConfigured"`
	VaultBacked          bool      `json:"vault_backed"`
	VaultAddr            string    `json:"vault_addr,omitempty"`
	VaultPath            string    `json:"vault_path,omitempty"`
	ResolverAvailable    bool      `json:"resolverAvailable"`
	CredentialRefCount   int       `json:"credentialRefCount"`
	CredentialRefs       []string  `json:"credentialRefs,omitempty"`
	StaticCredentialRefs int       `json:"staticCredentialRefs"`
	AWSCredentialRefs    int       `json:"awsCredentialRefs"`
	VaultCredentialRefs  int       `json:"vaultCredentialRefs"`
	LastLoadedAt         time.Time `json:"lastLoadedAt,omitempty"`
}

type PullCredentialManager struct {
	filePath    string
	inlineJSON  string
	awsSecretID string
	awsRegion   string
	vaultAddr   string
	vaultToken  string
	vaultPath   string

	loadStatic func(filePath, inlineJSON string) (map[string]map[string]string, error)
	loadAWS    func(ctx context.Context, secretID, region string) (map[string]map[string]string, error)
	loadVault  func(ctx context.Context, addr, token, path string) (map[string]map[string]string, error)

	mu       sync.RWMutex
	resolver CredentialResolver
	status   PullCredentialStatus
}

func NewPullCredentialManager(filePath, inlineJSON, awsSecretID, awsRegion, vaultAddr, vaultToken, vaultPath string) (*PullCredentialManager, error) {
	mgr := &PullCredentialManager{
		filePath:    strings.TrimSpace(filePath),
		inlineJSON:  strings.TrimSpace(inlineJSON),
		awsSecretID: strings.TrimSpace(awsSecretID),
		awsRegion:   strings.TrimSpace(awsRegion),
		vaultAddr:   strings.TrimSpace(vaultAddr),
		vaultToken:  strings.TrimSpace(vaultToken),
		vaultPath:   strings.TrimSpace(vaultPath),
		loadStatic:  LoadStaticCredentials,
		loadAWS:     LoadStaticCredentialsFromAWSSecretManager,
		loadVault:   LoadStaticCredentialsFromVault,
		resolver:    NoopCredentialResolver{},
	}
	if _, err := mgr.Reload(context.Background()); err != nil {
		return nil, err
	}
	return mgr, nil
}

func (m *PullCredentialManager) Resolve(ctx context.Context, ref string) (Credentials, error) {
	if m == nil {
		return Credentials{}, ErrCredentialResolverUnavailable
	}
	m.mu.RLock()
	resolver := m.resolver
	m.mu.RUnlock()
	if resolver == nil {
		return Credentials{}, ErrCredentialResolverUnavailable
	}
	return resolver.Resolve(ctx, ref)
}

func (m *PullCredentialManager) Reload(ctx context.Context) (PullCredentialStatus, error) {
	if m == nil {
		return PullCredentialStatus{}, ErrCredentialResolverUnavailable
	}

	vaultConfigured := m.vaultAddr != "" && m.vaultToken != "" && m.vaultPath != ""
	status := PullCredentialStatus{
		StaticConfigured:    m.filePath != "" || m.inlineJSON != "",
		AWSSecretConfigured: m.awsSecretID != "",
		VaultBacked:         vaultConfigured,
	}
	if vaultConfigured {
		status.VaultAddr = m.vaultAddr
		status.VaultPath = m.vaultPath
	}
	status.Configured = status.StaticConfigured || status.AWSSecretConfigured || vaultConfigured

	sets := make([]map[string]map[string]string, 0, 3)
	if status.StaticConfigured {
		values, err := m.loadStatic(m.filePath, m.inlineJSON)
		if err != nil {
			return PullCredentialStatus{}, err
		}
		status.StaticCredentialRefs = len(values)
		if len(values) > 0 {
			sets = append(sets, values)
		}
	}
	if status.AWSSecretConfigured {
		values, err := m.loadAWS(ctx, m.awsSecretID, m.awsRegion)
		if err != nil {
			return PullCredentialStatus{}, err
		}
		status.AWSCredentialRefs = len(values)
		if len(values) > 0 {
			sets = append(sets, values)
		}
	}
	if vaultConfigured {
		values, err := m.loadVault(ctx, m.vaultAddr, m.vaultToken, m.vaultPath)
		if err != nil {
			return PullCredentialStatus{}, err
		}
		status.VaultCredentialRefs = len(values)
		if len(values) > 0 {
			sets = append(sets, values)
		}
	}

	merged := MergeStaticCredentialSets(sets...)
	resolver := CredentialResolver(NoopCredentialResolver{})
	if len(merged) > 0 {
		resolver = NewStaticCredentialResolver(merged)
	}

	status.ResolverAvailable = len(merged) > 0
	status.CredentialRefCount = len(merged)
	if len(merged) > 0 {
		refs := make([]string, 0, len(merged))
		for ref := range merged {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		status.CredentialRefs = refs
	}
	status.LastLoadedAt = time.Now().UTC()

	m.mu.Lock()
	m.resolver = resolver
	m.status = status
	m.mu.Unlock()

	return status, nil
}

func (m *PullCredentialManager) Status() PullCredentialStatus {
	if m == nil {
		return PullCredentialStatus{}
	}
	m.mu.RLock()
	status := m.status
	m.mu.RUnlock()
	return status
}
