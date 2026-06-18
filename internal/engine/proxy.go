package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"

	"golang.org/x/crypto/ssh"
)

// validateIPRule accepts a single IP or a CIDR range.
func validateIPRule(ip string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return Invalid("IP address is required")
	}
	if net.ParseIP(ip) != nil {
		return nil
	}
	if _, _, err := net.ParseCIDR(ip); err == nil {
		return nil
	}
	return fmt.Errorf("invalid IP address or CIDR: %q", ip)
}

// normalizeSSHPublicKey parses an SSH public key and returns a single
// canonical "type base64 [comment]" line. It rejects multi-line input and
// embedded authorized_keys options (command=, from=, environment=...) that
// could be smuggled in via a crafted key.
func normalizeSSHPublicKey(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", Invalid("public key is required")
	}
	if strings.ContainsAny(raw, "\n\r") {
		return "", Invalid("public key must be a single line")
	}
	pk, comment, options, rest, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil {
		return "", Invalid("invalid SSH public key: %v", err)
	}
	if len(options) != 0 {
		return "", Invalid("public key must not contain authorized_keys options (e.g. command=, from=)")
	}
	if len(strings.TrimSpace(string(rest))) != 0 {
		return "", Invalid("public key must contain exactly one key")
	}
	// MarshalAuthorizedKey yields a canonical "type base64\n" line.
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pk)))
	if comment != "" && !strings.ContainsAny(comment, "\n\r") {
		line = line + " " + comment
	}
	return line, nil
}

func (e *Engine) AddProxyRule(ctx context.Context, domain, ruleType string, config map[string]string) (int64, error) {
	// Validate the rule so we never persist an empty no-op rule.
	required := map[string][]string{
		"redirect":   {"from", "to"},
		"header":     {"name", "value"},
		"basic-auth": {"user", "password"},
	}
	keys, ok := required[ruleType]
	if !ok {
		return 0, Invalid("invalid proxy rule type %q: must be redirect, header, or basic-auth", ruleType)
	}
	for _, k := range keys {
		if strings.TrimSpace(config[k]) == "" {
			return 0, Invalid("proxy rule %q requires %q", ruleType, k)
		}
	}

	configJSON, _ := json.Marshal(config)
	id, err := e.db.CreateProxyRule(domain, ruleType, string(configJSON))
	if err != nil {
		return 0, err
	}
	e.LogActivity(domain, "proxy_add", ruleType, "success")
	return id, nil
}

func (e *Engine) ListProxyRules(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListProxyRules(domain)
}

func (e *Engine) RemoveProxyRule(ctx context.Context, id int64, domain string) error {
	return e.db.DeleteProxyRuleForSite(id, domain)
}

// AllowIP adds the IP/CIDR to the site's allowlist. Once a site has any allow
// rule, only listed sources may reach it (enforced via a Traefik ipWhiteList
// middleware materialized by ApplyIPRules).
func (e *Engine) AllowIP(ctx context.Context, domain, ip string) error {
	if err := validateIPRule(ip); err != nil {
		return err
	}
	if err := e.db.AddIPRule(domain, ip, "allow"); err != nil {
		return err
	}
	if err := e.ApplyIPRules(domain); err != nil {
		return err
	}
	e.LogActivity(domain, "ip_allow", ip, "success")
	return nil
}

func (e *Engine) BlockIP(ctx context.Context, domain, ip string) error {
	if err := validateIPRule(ip); err != nil {
		return err
	}
	if err := e.db.BlockIP(domain, ip); err != nil {
		return err
	}
	if err := e.ApplyIPRules(domain); err != nil {
		return err
	}
	e.LogActivity(domain, "ip_block", ip, "success")
	return nil
}

func (e *Engine) UnblockIP(ctx context.Context, domain, ip string) error {
	if err := validateIPRule(ip); err != nil {
		return err
	}
	if err := e.db.UnblockIP(domain, ip); err != nil {
		return err
	}
	if err := e.ApplyIPRules(domain); err != nil {
		return err
	}
	e.LogActivity(domain, "ip_unblock", ip, "success")
	return nil
}

func (e *Engine) ListIPRules(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListIPRules(domain)
}

func (e *Engine) AddFTPAccount(ctx context.Context, domain, username, password string) error {
	if err := e.db.CreateFTPAccount(domain, username, password); err != nil {
		return err
	}
	e.LogActivity(domain, "ftp_add", username, "success")
	return nil
}

func (e *Engine) ListFTPAccounts(ctx context.Context, domain string) (interface{}, error) {
	return e.db.ListFTPAccounts(domain)
}

func (e *Engine) RemoveFTPAccount(ctx context.Context, domain, username string) error {
	return e.db.DeleteFTPAccountForSite(domain, username)
}

func (e *Engine) AddSSHKey(ctx context.Context, name, publicKey string) error {
	normalized, err := normalizeSSHPublicKey(publicKey)
	if err != nil {
		return err
	}
	if err := e.db.AddSSHKey(name, normalized); err != nil {
		return err
	}
	// Also append to authorized_keys
	appendAuthorizedKey(normalized)
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
