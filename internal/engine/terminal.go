package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// TerminalToken represents a time-limited token for container exec access.
// The target service is bound into the token at creation and validated then, so
// exec can't be redirected to another container after the fact.
type TerminalToken struct {
	Token     string    `json:"token"`
	Domain    string    `json:"domain"`
	Service   string    `json:"service"` // target service; "" = primary/app container
	ExpiresAt time.Time `json:"expires_at"`
	CmdCount  int       `json:"-"` // number of commands executed
}

const maxCommandsPerToken = 100

var (
	terminalTokens   = make(map[string]*TerminalToken)
	terminalTokensMu sync.RWMutex
)

const terminalTokenTTL = 5 * time.Minute

// CreateTerminalToken generates a short-lived token for container shell access.
// When service is non-empty the token is scoped to that specific container,
// which must belong to this site — validated here so the bound target can't be
// forged or point at another tenant's container.
func (e *Engine) CreateTerminalToken(ctx context.Context, domain, service string) (*TerminalToken, error) {
	// Verify site exists and is running
	site, err := e.db.GetSite(domain)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, NotFound("site %q not found", domain)
	}
	if site.Status != "running" {
		return nil, fmt.Errorf("site is not running")
	}

	// If a specific container was requested, make sure it's actually one of this
	// site's services before minting a token for it.
	if service != "" {
		ok, verr := e.siteHasService(ctx, domain, service)
		if verr != nil {
			return nil, verr
		}
		if !ok {
			return nil, NotFound("service %q not found for site %q", service, domain)
		}
	}

	// Generate secure random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	token := &TerminalToken{
		Token:     "term_" + hex.EncodeToString(tokenBytes),
		Domain:    domain,
		Service:   service,
		ExpiresAt: time.Now().Add(terminalTokenTTL),
	}

	terminalTokensMu.Lock()
	terminalTokens[token.Token] = token
	terminalTokensMu.Unlock()

	// Clean up expired tokens
	go cleanExpiredTokens()

	return token, nil
}

// ValidateTerminalToken checks if a token is valid and returns the domain and
// the service it is bound to ("" for the primary container).
func ValidateTerminalToken(token string) (domain, service string, err error) {
	terminalTokensMu.Lock()
	defer terminalTokensMu.Unlock()

	t, exists := terminalTokens[token]
	if !exists {
		return "", "", fmt.Errorf("invalid token")
	}

	if time.Now().After(t.ExpiresAt) {
		delete(terminalTokens, token)
		return "", "", fmt.Errorf("token expired")
	}

	if t.CmdCount >= maxCommandsPerToken {
		delete(terminalTokens, token)
		return "", "", fmt.Errorf("token command limit reached — refresh the page for a new token")
	}

	t.CmdCount++
	return t.Domain, t.Service, nil
}

// siteHasService reports whether the given service name corresponds to a
// container of this site. Scoped by both the site and service labels so it can
// only ever match the caller's own containers.
func (e *Engine) siteHasService(ctx context.Context, domain, service string) (bool, error) {
	ids, err := e.serviceContainers(ctx, domain, service)
	if err != nil {
		return false, err
	}
	return len(ids) > 0, nil
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

// ExecInSite runs a command inside a site's container. When service is set, the
// command targets that specific service's container (validated to belong to the
// site); otherwise it falls back to the primary app/shell container.
//
// For normal sites: uses apod-<domain>-app container.
// For compose sites: finds the container with apod.shell=true label, or falls back to first labeled container.
func (e *Engine) ExecInSite(ctx context.Context, domain, service, command string) (string, error) {
	// A specific service was requested: resolve its container, scoped by both
	// the site and service labels so it can only ever be one of this site's own.
	if service != "" {
		ids, err := e.serviceContainers(ctx, domain, service)
		if err != nil {
			return "", fmt.Errorf("resolve container: %w", err)
		}
		if len(ids) == 0 {
			return "", NotFound("service %q has no running container", service)
		}
		return e.execContainer(ctx, ids[0], command)
	}

	// Try normal container first
	containerName := e.primaryServiceContainer(domain)
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

	return e.execContainer(ctx, containerName, command)
}

// execContainer runs a shell command in a single container and returns its
// combined output, capped to a sane size.
func (e *Engine) execContainer(ctx context.Context, containerName, command string) (string, error) {
	// Interactive command: a non-zero exit is a normal result, not an error —
	// return the output regardless of exit code.
	output, _, err := e.docker.ExecCombined(ctx, containerName, []string{"sh", "-c", command})
	if err != nil {
		return "", fmt.Errorf("exec: %w", err)
	}
	if len(output) > 65536 {
		output = output[:65536] + "\n... (output truncated)"
	}
	return output, nil
}
