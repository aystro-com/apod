package engine

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// FirewallRule is a single parsed `ufw status numbered` entry.
type FirewallRule struct {
	Num    int    `json:"num"`
	To     string `json:"to"`
	Action string `json:"action"`
	From   string `json:"from"`
}

var ufwNumberedLine = regexp.MustCompile(`^\[\s*(\d+)\]\s+(.*\S)\s+(ALLOW|DENY|REJECT|LIMIT)\s+(?:IN|OUT)?\s*(.*)$`)

// parseUFWRules parses the output of `ufw status numbered` into rules.
func parseUFWRules(output string) []FirewallRule {
	var rules []FirewallRule
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		m := ufwNumberedLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[1])
		rules = append(rules, FirewallRule{
			Num:    num,
			To:     strings.TrimSpace(m[2]),
			Action: strings.TrimSpace(m[3]),
			From:   strings.TrimSpace(m[4]),
		})
	}
	return rules
}

// FirewallRules returns the numbered firewall rules.
func (e *Engine) FirewallRules(ctx context.Context) ([]FirewallRule, error) {
	out, err := exec.CommandContext(ctx, "ufw", "status", "numbered").Output()
	if err != nil {
		// Inactive/uninstalled firewall has no rules.
		return []FirewallRule{}, nil
	}
	return parseUFWRules(string(out)), nil
}

// FirewallAllowFrom whitelists a source IP/CIDR, optionally restricted to a
// destination port and protocol. With no port it allows all traffic from the
// source. All inputs are validated and passed as separate argv elements (no
// shell), so they cannot inject ufw subcommands.
func (e *Engine) FirewallAllowFrom(ctx context.Context, source, port, proto string) error {
	if err := ValidateIPOrCIDR(source); err != nil {
		return err
	}
	if err := ValidateProto(proto); err != nil {
		return err
	}
	args := []string{"allow", "from", source}
	if port != "" {
		if err := ValidatePortNumber(port); err != nil {
			return err
		}
		args = append(args, "to", "any", "port", port)
		if proto != "" {
			args = append(args, "proto", proto)
		}
	}
	if out, err := exec.CommandContext(ctx, "ufw", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("allow from %s: %s: %w", source, strings.TrimSpace(string(out)), err)
	}
	e.LogActivity("server", "firewall_allow_from", fmt.Sprintf("%s port=%s proto=%s", source, port, proto), "success")
	return nil
}

// FirewallDelete removes a rule by its number (from FirewallRules).
func (e *Engine) FirewallDelete(ctx context.Context, num int) error {
	if num < 1 {
		return fmt.Errorf("invalid rule number %d", num)
	}
	cmd := exec.CommandContext(ctx, "ufw", "--force", "delete", strconv.Itoa(num))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("delete rule %d: %s: %w", num, strings.TrimSpace(string(out)), err)
	}
	e.LogActivity("server", "firewall_delete", strconv.Itoa(num), "success")
	return nil
}

type FirewallStatus struct {
	Active bool     `json:"active"`
	Rules  []string `json:"rules"`
}

func (e *Engine) FirewallStatus(ctx context.Context) (*FirewallStatus, error) {
	out, err := exec.CommandContext(ctx, "ufw", "status").Output()
	if err != nil {
		return &FirewallStatus{Active: false}, nil
	}
	output := string(out)
	active := strings.Contains(output, "Status: active")
	var rules []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "Status:") && !strings.HasPrefix(line, "To") && !strings.HasPrefix(line, "--") {
			rules = append(rules, line)
		}
	}
	return &FirewallStatus{Active: active, Rules: rules}, nil
}

func (e *Engine) FirewallEnable(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ufw", "--force", "enable")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("enable firewall: %w", err)
	}
	e.LogActivity("server", "firewall_enable", "", "success")
	return nil
}

func (e *Engine) FirewallAllow(ctx context.Context, port string) error {
	if err := ValidateUFWPort(port); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "ufw", "allow", port)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("allow port %s: %w", port, err)
	}
	e.LogActivity("server", "firewall_allow", port, "success")
	return nil
}

func (e *Engine) FirewallDeny(ctx context.Context, port string) error {
	if err := ValidateUFWPort(port); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "ufw", "deny", port)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deny port %s: %w", port, err)
	}
	e.LogActivity("server", "firewall_deny", port, "success")
	return nil
}
