package engine

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func (e *Engine) CreateWebhook(ctx context.Context, domain string) (string, error) {
	// Generate random token (160 bits).
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate webhook token: %w", err)
	}
	token := "whk_" + hex.EncodeToString(b)

	if err := e.db.CreateWebhook(domain, token); err != nil {
		return "", fmt.Errorf("create webhook: %w", err)
	}

	e.LogActivity(domain, "webhook_create", "", "success")
	return token, nil
}

// HandleWebhook validates the webhook token and, when the caller supplies a
// signature header, verifies an HMAC-SHA256 over the request body keyed by the
// webhook token. This rejects forged payloads from anyone who does not also
// know the token. Callers that send no signature fall back to token-only auth
// for backward compatibility.
func (e *Engine) HandleWebhook(ctx context.Context, token string, body []byte, signature string) error {
	wh, err := e.db.GetWebhookByToken(token)
	if err != nil {
		return fmt.Errorf("invalid webhook token")
	}
	if !wh.Active {
		return fmt.Errorf("webhook is inactive")
	}

	if signature != "" {
		if !verifyWebhookSignature(token, body, signature) {
			return fmt.Errorf("invalid webhook signature")
		}
	}

	return e.Deploy(ctx, wh.SiteDomain, "")
}

// verifyWebhookSignature checks an HMAC-SHA256 signature (hex, optionally
// prefixed with "sha256=") of body keyed by token, using a constant-time
// comparison.
func verifyWebhookSignature(token string, body []byte, signature string) bool {
	signature = strings.TrimSpace(signature)
	signature = strings.TrimPrefix(signature, "sha256=")
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write(body)
	return hmac.Equal(sigBytes, mac.Sum(nil))
}

func (e *Engine) ListWebhooks(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListWebhooks(domain)
}

func (e *Engine) DeleteWebhook(ctx context.Context, domain string) error {
	e.LogActivity(domain, "webhook_delete", "", "success")
	return e.db.DeleteWebhook(domain)
}
