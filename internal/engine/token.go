package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/aystro/apod/internal/db"
	"github.com/aystro/apod/internal/models"
)

const apiTokenPrefix = "apod_pat_"

// validAbilities are the only grants a personal access token may hold.
// Deliberately excludes any "admin" or "manage tokens/auth" ability — PATs
// can never escalate or mint more credentials.
var validAbilities = map[string]bool{
	"read":   true,
	"write":  true,
	"deploy": true,
}

// TokenInfo describes a validated PAT's grants for the request context.
type TokenInfo struct {
	ID        int64
	Name      string
	abilities map[string]bool
	Sensitive bool
}

func (t *TokenInfo) HasAbility(a string) bool { return t.abilities[a] }

// IsAPIToken reports whether a bearer credential is a scoped PAT.
func IsAPIToken(token string) bool { return strings.HasPrefix(token, apiTokenPrefix) }

// CreateAPIToken issues a scoped token for a user. ttlDays of 0 means no
// expiry. The raw token is returned once and never stored.
func (e *Engine) CreateAPIToken(userName, name string, abilities []string, sensitive bool, ttlDays int) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("token name is required")
	}
	if len(abilities) == 0 {
		return "", fmt.Errorf("at least one ability is required")
	}
	seen := map[string]bool{}
	for _, a := range abilities {
		if !validAbilities[a] {
			return "", fmt.Errorf("invalid ability %q (allowed: read, write, deploy)", a)
		}
		seen[a] = true
	}
	if _, err := e.db.GetUserByName(userName); err != nil {
		return "", err
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	raw := apiTokenPrefix + hex.EncodeToString(buf)

	// Normalize abilities to a stable comma list.
	var abilityList []string
	for _, a := range []string{"read", "write", "deploy"} {
		if seen[a] {
			abilityList = append(abilityList, a)
		}
	}

	var expires *time.Time
	if ttlDays > 0 {
		t := time.Now().Add(time.Duration(ttlDays) * 24 * time.Hour)
		expires = &t
	}

	if err := e.db.CreateAPIToken(userName, name, HashAPIKey(raw), strings.Join(abilityList, ","), sensitive, expires); err != nil {
		return "", err
	}
	e.LogActivity("server", "token_create", fmt.Sprintf("created API token %q for %s", name, userName), "success")
	return raw, nil
}

// ValidateAPIToken resolves a PAT to its user and grants, or (nil, nil, nil).
func (e *Engine) ValidateAPIToken(raw string) (*models.User, *TokenInfo, error) {
	if !IsAPIToken(raw) {
		return nil, nil, nil
	}
	tok, err := e.db.GetAPITokenByHash(HashAPIKey(raw))
	if err != nil {
		return nil, nil, err
	}
	if tok == nil {
		return nil, nil, nil
	}
	user, err := e.db.GetUserByName(tok.UserName)
	if err != nil {
		// Token outlived its user — treat as invalid.
		return nil, nil, nil
	}
	abilities := map[string]bool{}
	for _, a := range strings.Split(tok.Abilities, ",") {
		if a != "" {
			abilities[a] = true
		}
	}
	return user, &TokenInfo{
		ID:        tok.ID,
		Name:      tok.Name,
		abilities: abilities,
		Sensitive: tok.Sensitive,
	}, nil
}

func (e *Engine) ListAPITokens(userName string) ([]db.APIToken, error) {
	return e.db.ListAPITokens(userName)
}

func (e *Engine) RevokeAPIToken(userName string, id int64) error {
	if err := e.db.DeleteAPIToken(userName, id); err != nil {
		return err
	}
	e.LogActivity("server", "token_revoke", fmt.Sprintf("revoked API token %d for %s", id, userName), "success")
	return nil
}
