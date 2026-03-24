package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"github.com/hardwareops/control-plane/internal/config"
	"github.com/hardwareops/control-plane/internal/store"
)

var (
	ErrLDAPInvalidCredentials = errors.New("invalid credentials")
	ErrLDAPAccountDisabled    = errors.New("account disabled")
)

// LDAPProvider authenticates users against an LDAP/AD directory.
type LDAPProvider struct {
	url         string
	baseDN      string
	bindDN      string
	bindPW      string
	userFilter  string // e.g. "(uid=%s)" or "(sAMAccountName=%s)"
	mailAttr    string
	displayAttr string
	groupAttr   string
	roleMap     map[string]string // LDAP group DN -> hwops role
	defaultRole string
	store       store.Store
	manager     *Manager
}

// NewLDAPProvider creates and returns an LDAPProvider from config.
func NewLDAPProvider(cfg *config.Config, st store.Store, mgr *Manager) (*LDAPProvider, error) {
	if cfg.AuthLDAPURL == "" {
		return nil, errors.New("AUTH_LDAP_URL is required")
	}
	if cfg.AuthLDAPBaseDN == "" {
		return nil, errors.New("AUTH_LDAP_BASE_DN is required")
	}

	roleMap := map[string]string{}
	if cfg.AuthLDAPRoleMap != "" {
		if err := json.Unmarshal([]byte(cfg.AuthLDAPRoleMap), &roleMap); err != nil {
			return nil, fmt.Errorf("AUTH_LDAP_ROLE_MAP invalid JSON: %w", err)
		}
	}

	defaultRole := cfg.AuthLDAPDefaultRole
	if defaultRole == "" {
		defaultRole = "viewer"
	}

	userFilter := cfg.AuthLDAPUserFilter
	if userFilter == "" {
		userFilter = "(uid=%s)"
	}
	mailAttr := cfg.AuthLDAPMailAttr
	if mailAttr == "" {
		mailAttr = "mail"
	}
	displayAttr := cfg.AuthLDAPDisplayAttr
	if displayAttr == "" {
		displayAttr = "displayName"
	}
	groupAttr := cfg.AuthLDAPGroupAttr
	if groupAttr == "" {
		groupAttr = "memberOf"
	}

	return &LDAPProvider{
		url:         cfg.AuthLDAPURL,
		baseDN:      cfg.AuthLDAPBaseDN,
		bindDN:      cfg.AuthLDAPBindDN,
		bindPW:      cfg.AuthLDAPBindPassword,
		userFilter:  userFilter,
		mailAttr:    mailAttr,
		displayAttr: displayAttr,
		groupAttr:   groupAttr,
		roleMap:     roleMap,
		defaultRole: defaultRole,
		store:       st,
		manager:     mgr,
	}, nil
}

// Authenticate validates a username+password against the directory, upserts the
// local user record, and returns a signed JWT.
func (p *LDAPProvider) Authenticate(ctx context.Context, username, password string) (token string, expiresAt time.Time, user *store.User, err error) {
	conn, err := ldap.DialURL(p.url)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("ldap dial: %w", err)
	}
	defer conn.Close()

	// Service-account bind to search for the user.
	if p.bindDN != "" {
		if err := conn.Bind(p.bindDN, p.bindPW); err != nil {
			return "", time.Time{}, nil, fmt.Errorf("ldap service bind: %w", err)
		}
	}

	filter := fmt.Sprintf(p.userFilter, ldap.EscapeFilter(username))
	searchReq := ldap.NewSearchRequest(
		p.baseDN,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases,
		0, 0, false,
		filter,
		[]string{"dn", p.mailAttr, p.displayAttr, p.groupAttr},
		nil,
	)
	result, err := conn.Search(searchReq)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("ldap search: %w", err)
	}
	if len(result.Entries) == 0 {
		return "", time.Time{}, nil, ErrLDAPInvalidCredentials
	}

	entry := result.Entries[0]
	userDN := entry.DN
	mail := strings.ToLower(strings.TrimSpace(entry.GetAttributeValue(p.mailAttr)))
	displayName := strings.TrimSpace(entry.GetAttributeValue(p.displayAttr))
	groups := entry.GetAttributeValues(p.groupAttr)

	// Rebind as the found user to verify the provided password.
	if err := conn.Bind(userDN, password); err != nil {
		return "", time.Time{}, nil, ErrLDAPInvalidCredentials
	}

	role := p.mapRole(groups)
	rolesJSON, _ := json.Marshal([]string{role})
	now := time.Now().UTC()

	// Upsert user: try by LDAP external ID first, then by email.
	u, found, err := p.store.GetUserByExternalID("ldap", userDN)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("ldap user lookup: %w", err)
	}
	if !found && mail != "" {
		u, found, err = p.store.GetUserByEmail(mail)
		if err != nil {
			return "", time.Time{}, nil, fmt.Errorf("ldap email lookup: %w", err)
		}
	}

	if !found {
		u = store.User{
			UserID:       uuid.NewString(),
			Email:        mail,
			DisplayName:  displayName,
			RolesJSON:    rolesJSON,
			AuthProvider: "ldap",
			ExternalID:   userDN,
			CreatedAt:    now,
			UpdatedAt:    now,
			LastLoginAt:  now,
		}
		if err := p.store.CreateUser(u); err != nil {
			return "", time.Time{}, nil, fmt.Errorf("ldap create user: %w", err)
		}
	} else {
		if u.Disabled {
			return "", time.Time{}, nil, ErrLDAPAccountDisabled
		}
		dispName := u.DisplayName
		if displayName != "" {
			dispName = displayName
		}
		update := store.UserUpdate{
			UserID:      u.UserID,
			DisplayName: &dispName,
			RolesJSON:   rolesJSON,
		}
		if err := p.store.UpdateUser(update); err != nil {
			return "", time.Time{}, nil, fmt.Errorf("ldap update user: %w", err)
		}
		u.AuthProvider = "ldap"
		u.ExternalID = userDN
		u.RolesJSON = rolesJSON
		u.DisplayName = dispName
		_ = p.store.SetUserLastLogin(u.UserID, now)
		u.LastLoginAt = now
	}

	signed, exp, err := p.manager.IssueToken(u)
	if err != nil {
		return "", time.Time{}, nil, fmt.Errorf("ldap issue token: %w", err)
	}
	return signed, exp, &u, nil
}

// mapRole returns the highest-precedence role for the given LDAP group DNs.
func (p *LDAPProvider) mapRole(groups []string) string {
	best := ""
	bestRank := 0
	for _, g := range groups {
		role, ok := p.roleMap[g]
		if !ok {
			continue
		}
		rank := roleRank[strings.ToLower(role)]
		if rank > bestRank {
			bestRank = rank
			best = role
		}
	}
	if best == "" {
		return p.defaultRole
	}
	return best
}
