package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func (e *Engine) CreateWebhook(ctx context.Context, domain string) (string, error) {
	// Generate random token
	b := make([]byte, 20)
	rand.Read(b)
	token := "whk_" + hex.EncodeToString(b)

	if err := e.db.CreateWebhook(domain, token); err != nil {
		return "", fmt.Errorf("create webhook: %w", err)
	}

	e.LogActivity(domain, "webhook_create", "", "success")
	return token, nil
}

func (e *Engine) HandleWebhook(ctx context.Context, token string) error {
	wh, err := e.db.GetWebhookByToken(token)
	if err != nil {
		return fmt.Errorf("invalid webhook token")
	}
	if !wh.Active {
		return fmt.Errorf("webhook is inactive")
	}

	return e.Deploy(ctx, wh.SiteDomain, "")
}

func (e *Engine) ListWebhooks(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListWebhooks(domain)
}

func (e *Engine) DeleteWebhook(ctx context.Context, domain string) error {
	e.LogActivity(domain, "webhook_delete", "", "success")
	return e.db.DeleteWebhook(domain)
}
