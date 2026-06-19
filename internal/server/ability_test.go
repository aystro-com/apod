package server

import (
	"net/http"
	"strings"
	"testing"
)

// TestClassify pins the scoped-token route policy exactly — this is the
// security boundary for personal access tokens, so each branch is asserted.
func TestClassify(t *testing.T) {
	segs := func(p string) []string { return strings.Split(strings.Trim(p, "/"), "/") }

	cases := []struct {
		name       string
		method     string
		path       string
		management bool
		deploy     bool
		sensitive  bool
	}{
		{"token mgmt", "POST", "tokens", true, false, false},
		{"2fa is mgmt", "POST", "auth/2fa/enable", true, false, false},
		{"login not mgmt", "POST", "auth/login", false, false, false},
		{"user password mgmt", "POST", "users/bob/password", true, false, false},
		{"user reset-key mgmt", "POST", "users/bob/reset-key", true, false, false},
		{"user delete mgmt", "DELETE", "users/bob", true, false, false},
		{"user get not mgmt", "GET", "users", false, false, false},
		{"deploy action", "POST", "sites/ex.com/deploy", false, true, false},
		{"restart action", "POST", "sites/ex.com/restart", false, true, false},
		{"stop action", "POST", "sites/ex.com/stop", false, true, false},
		{"env sensitive", "GET", "sites/ex.com/env", false, false, true},
		{"db sensitive", "POST", "sites/ex.com/db/export", false, false, true},
		{"info sensitive", "GET", "sites/ex.com/info", false, false, true},
		{"webhook sensitive", "GET", "sites/ex.com/webhook", false, false, true},
		{"ftp sensitive", "GET", "sites/ex.com/ftp", false, false, true},
		{"backup download sensitive", "POST", "sites/ex.com/backups/download", false, false, true},
		{"export sensitive", "POST", "sites/ex.com/export", false, false, true},
		{"clone sensitive", "POST", "sites/ex.com/clone", false, false, true},
		{"terminal mint sensitive", "POST", "sites/ex.com/terminal", false, false, true},
		{"terminal exec sensitive", "POST", "terminal/exec", false, false, true},
		{"user permissions mgmt", "POST", "users/bob/permissions", true, false, false},
		{"plain site read", "GET", "sites/ex.com", false, false, false},
		{"sites list", "GET", "sites", false, false, false},
		{"empty path", "GET", "", false, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := classify(c.method, segs(c.path))
			if p.management != c.management || p.deploy != c.deploy || p.sensitive != c.sensitive {
				t.Errorf("classify(%s %s) = %+v, want mgmt=%v deploy=%v sensitive=%v",
					c.method, c.path, p, c.management, c.deploy, c.sensitive)
			}
		})
	}
}

// A non-management, non-deploy mutating action on a site must require 'write'
// (not be silently allowed) — guarded here at the classify level: such a path
// yields an all-false policy, so AbilityMiddleware falls through to the write
// check.
func TestClassifyMutatingSiteActionNeedsWrite(t *testing.T) {
	p := classify(http.MethodPost, []string{"sites", "ex.com", "domains"})
	if p.management || p.deploy || p.sensitive {
		t.Errorf("adding a domain should be a plain write, got %+v", p)
	}
}
