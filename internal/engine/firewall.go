package engine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

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
	cmd := exec.CommandContext(ctx, "ufw", "allow", port)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("allow port %s: %w", port, err)
	}
	e.LogActivity("server", "firewall_allow", port, "success")
	return nil
}

func (e *Engine) FirewallDeny(ctx context.Context, port string) error {
	cmd := exec.CommandContext(ctx, "ufw", "deny", port)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deny port %s: %w", port, err)
	}
	e.LogActivity("server", "firewall_deny", port, "success")
	return nil
}
