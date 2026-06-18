package engine

import (
	"fmt"
	"regexp"
	"strings"
)

// Centralized input validation for values that flow into shell commands,
// file paths, container/project names, and proxy rules. These validators are
// enforced inside the engine entry points (CreateSite, Clone, AddDomain,
// ImportSite, Deploy) so both the HTTP API and the CLI are covered — the HTTP
// handlers' own checks are defense-in-depth, not the only line of defense.

var (
	domainPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?)*$`)
	ownerPattern  = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	branchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)
	// Acceptable git remotes: https(s):// URLs or scp-like SSH (git@host:path / user@host:path).
	gitHTTPSPattern = regexp.MustCompile(`^https?://[^\s]+$`)
	gitSSHPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+:[^\s]+$`)
)

// ValidateDomain rejects domains that could escape into container names,
// file paths, or Traefik router rules (Host()` injection).
func ValidateDomain(domain string) error {
	d := strings.ToLower(domain)
	if d == "" || len(d) > 253 {
		return fmt.Errorf("invalid domain: must be 1-253 characters")
	}
	if !domainPattern.MatchString(d) {
		return fmt.Errorf("invalid domain %q: only letters, digits, hyphens and dots allowed", domain)
	}
	return nil
}

// ValidateOwner rejects owner values that could traverse the /home/<owner>/...
// path or break shell/chown calls. Empty is allowed (admin-owned sites).
func ValidateOwner(owner string) error {
	if owner == "" {
		return nil
	}
	if !ownerPattern.MatchString(owner) {
		return fmt.Errorf("invalid owner %q: only lowercase letters, digits, '_' and '-' allowed", owner)
	}
	return nil
}

// ValidateBranch rejects branch names that could be parsed as git options or
// inject path/ref separators. Empty is allowed (defaults applied by callers).
func ValidateBranch(branch string) error {
	if branch == "" {
		return nil
	}
	if strings.HasPrefix(branch, "-") || strings.Contains(branch, "..") {
		return fmt.Errorf("invalid branch %q", branch)
	}
	if !branchPattern.MatchString(branch) {
		return fmt.Errorf("invalid branch %q: only letters, digits and ._/- allowed", branch)
	}
	return nil
}

// ValidateRepo rejects git remotes that use dangerous transports. In
// particular it blocks the ext:: transport (arbitrary command execution),
// file paths, and values that would be parsed as git flags. Empty is allowed
// (sites without a git repo). Defense-in-depth: callers also pass
// gitHardeningArgs() and a "--" separator to git.
func ValidateRepo(repo string) error {
	if repo == "" {
		return nil
	}
	if strings.HasPrefix(repo, "-") {
		return fmt.Errorf("invalid repository URL %q", repo)
	}
	// Reject git "smart transport" helpers such as ext::, fd::, etc. which can
	// execute arbitrary commands during clone.
	if strings.Contains(repo, "::") {
		return fmt.Errorf("invalid repository URL: transport helpers are not allowed")
	}
	if gitHTTPSPattern.MatchString(repo) || gitSSHPattern.MatchString(repo) ||
		strings.HasPrefix(repo, "ssh://") || strings.HasPrefix(repo, "git://") {
		return nil
	}
	return fmt.Errorf("invalid repository URL: only http(s) and ssh remotes are allowed")
}

// gitHardeningArgs returns global git config flags that disable dangerous
// protocols (ext/file) regardless of the remote URL. They are prepended to
// every git invocation that touches a user-supplied remote.
func gitHardeningArgs() []string {
	return []string{
		"-c", "protocol.ext.allow=never",
		"-c", "protocol.file.allow=user",
	}
}
