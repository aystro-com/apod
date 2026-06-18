package engine

import "testing"

func TestGitRemoteHost(t *testing.T) {
	cases := map[string]string{
		"git@github.com:you/app.git":       "github.com",
		"ssh://git@example.com:22/x/y.git": "example.com",
		"git://example.org/x.git":          "example.org",
		"https://github.com/you/app.git":   "", // http handled by validatePublicURL
		"user@10.0.0.5:repo.git":           "10.0.0.5",
	}
	for in, want := range cases {
		if got := gitRemoteHost(in); got != want {
			t.Errorf("gitRemoteHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateRepoEgressBlocksInternal(t *testing.T) {
	// Literal internal targets must be rejected without needing DNS.
	internal := []string{
		"http://169.254.169.254/latest/meta-data/",
		"https://127.0.0.1/x.git",
		"http://10.0.0.5/x.git",
		"https://localhost/x.git",
		"git@192.168.1.10:x.git",
		"ssh://git@[::1]/x.git",
	}
	for _, repo := range internal {
		if err := validateRepoEgress(repo); err == nil {
			t.Errorf("validateRepoEgress(%q) = nil, want SSRF rejection", repo)
		}
	}

	// Empty is allowed (no repo).
	if err := validateRepoEgress(""); err != nil {
		t.Errorf("validateRepoEgress(\"\") = %v, want nil", err)
	}
}
