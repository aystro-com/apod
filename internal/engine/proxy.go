package engine

import (
	"context"
	"encoding/json"
)

func (e *Engine) AddProxyRule(ctx context.Context, domain, ruleType string, config map[string]string) (int64, error) {
	configJSON, _ := json.Marshal(config)
	id, err := e.db.CreateProxyRule(domain, ruleType, string(configJSON))
	if err != nil { return 0, err }
	e.LogActivity(domain, "proxy_add", ruleType, "success")
	return id, nil
}

func (e *Engine) ListProxyRules(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListProxyRules(domain)
}

func (e *Engine) RemoveProxyRule(ctx context.Context, id int64) error {
	return e.db.DeleteProxyRule(id)
}

func (e *Engine) BlockIP(ctx context.Context, domain, ip string) error {
	if err := e.db.BlockIP(domain, ip); err != nil { return err }
	e.LogActivity(domain, "ip_block", ip, "success")
	return nil
}

func (e *Engine) UnblockIP(ctx context.Context, domain, ip string) error {
	if err := e.db.UnblockIP(domain, ip); err != nil { return err }
	e.LogActivity(domain, "ip_unblock", ip, "success")
	return nil
}

func (e *Engine) ListIPRules(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListIPRules(domain)
}

func (e *Engine) AddFTPAccount(ctx context.Context, domain, username, password string) error {
	if err := e.db.CreateFTPAccount(domain, username, password); err != nil { return err }
	e.LogActivity(domain, "ftp_add", username, "success")
	return nil
}

func (e *Engine) ListFTPAccounts(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListFTPAccounts(domain)
}

func (e *Engine) RemoveFTPAccount(ctx context.Context, username string) error {
	return e.db.DeleteFTPAccount(username)
}

func (e *Engine) AddSSHKey(ctx context.Context, name, publicKey string) error {
	if err := e.db.AddSSHKey(name, publicKey); err != nil { return err }
	// Also append to authorized_keys
	appendAuthorizedKey(publicKey)
	e.LogActivity("server", "ssh_key_add", name, "success")
	return nil
}

func (e *Engine) ListSSHKeys(ctx context.Context) (interface{}, error) {
	return e.db.ListSSHKeys()
}

func (e *Engine) RemoveSSHKey(ctx context.Context, name string) error {
	e.db.DeleteSSHKey(name)
	e.LogActivity("server", "ssh_key_remove", name, "success")
	return nil
}

func appendAuthorizedKey(key string) {
	// Placeholder — would append to /root/.ssh/authorized_keys
	// Actual implementation writes the file
}
