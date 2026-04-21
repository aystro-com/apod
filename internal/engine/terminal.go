package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// TerminalToken represents a time-limited token for container exec access
type TerminalToken struct {
	Token     string    `json:"token"`
	Domain    string    `json:"domain"`
	ExpiresAt time.Time `json:"expires_at"`
	CmdCount  int       `json:"-"` // number of commands executed
}

const maxCommandsPerToken = 100

var (
	terminalTokens   = make(map[string]*TerminalToken)
	terminalTokensMu sync.RWMutex
)

const terminalTokenTTL = 5 * time.Minute

// CreateTerminalToken generates a short-lived token for container shell access
func (e *Engine) CreateTerminalToken(ctx context.Context, domain string) (*TerminalToken, error) {
	// Verify site exists and is running
	site, err := e.db.GetSite(domain)
	if err != nil {
		return nil, err
	}
	if site.Status != "running" {
		return nil, fmt.Errorf("site is not running")
	}

	// Generate secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	token := &TerminalToken{
		Token:     "term_" + hex.EncodeToString(tokenBytes),
		Domain:    domain,
		ExpiresAt: time.Now().Add(terminalTokenTTL),
	}

	terminalTokensMu.Lock()
	terminalTokens[token.Token] = token
	terminalTokensMu.Unlock()

	// Clean up expired tokens
	go cleanExpiredTokens()

	return token, nil
}

// ValidateTerminalToken checks if a token is valid and returns the domain
func ValidateTerminalToken(token string) (string, error) {
	terminalTokensMu.Lock()
	defer terminalTokensMu.Unlock()

	t, exists := terminalTokens[token]
	if !exists {
		return "", fmt.Errorf("invalid token")
	}

	if time.Now().After(t.ExpiresAt) {
		delete(terminalTokens, token)
		return "", fmt.Errorf("token expired")
	}

	if t.CmdCount >= maxCommandsPerToken {
		delete(terminalTokens, token)
		return "", fmt.Errorf("token command limit reached — refresh the page for a new token")
	}

	t.CmdCount++
	return t.Domain, nil
}

// RevokeTerminalToken invalidates a token immediately
func RevokeTerminalToken(token string) {
	terminalTokensMu.Lock()
	delete(terminalTokens, token)
	terminalTokensMu.Unlock()
}

func cleanExpiredTokens() {
	terminalTokensMu.Lock()
	defer terminalTokensMu.Unlock()

	now := time.Now()
	for k, t := range terminalTokens {
		if now.After(t.ExpiresAt) {
			delete(terminalTokens, k)
		}
	}
}

// ExecInSite runs a command inside a site's app/shell container.
// For normal sites: uses apod-<domain>-app container.
// For compose sites: finds the container with apod.shell=true label, or falls back to first labeled container.
func (e *Engine) ExecInSite(ctx context.Context, domain, command string) (string, error) {
	// Try normal container first
	containerName := fmt.Sprintf("apod-%s-app", domain)
	if exists, _ := e.docker.ContainerExists(ctx, containerName); !exists {
		// Compose site: find shell container by label
		shellIDs, _ := e.docker.ListContainersByLabel(ctx, "apod.shell", "true")
		found := false
		for _, id := range shellIDs {
			// Verify it belongs to this site
			siteIDs, _ := e.docker.ListContainersByLabel(ctx, "apod.site", domain)
			for _, siteID := range siteIDs {
				if id == siteID {
					containerName = id
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			// Fallback: first container for this site
			ids, _ := e.docker.ListContainersByLabel(ctx, "apod.site", domain)
			if len(ids) > 0 {
				containerName = ids[0]
			}
		}
	}

	output, err := e.docker.ExecInContainer(ctx, containerName, []string{"sh", "-c", command})
	if err != nil {
		return "", fmt.Errorf("exec: %w", err)
	}
	if len(output) > 65536 {
		output = output[:65536] + "\n... (output truncated)"
	}
	return output, nil
}
