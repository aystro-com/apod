package engine

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func genTestSSHKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ed25519 key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey: %v", err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " user@host"
}

func TestValidateRepo(t *testing.T) {
	good := []string{
		"",
		"https://github.com/aystro-com/apod.git",
		"http://example.com/repo.git",
		"git@github.com:aystro-com/apod.git",
		"ssh://git@example.com/repo.git",
	}
	for _, r := range good {
		if err := ValidateRepo(r); err != nil {
			t.Errorf("ValidateRepo(%q) = %v, want nil", r, err)
		}
	}
	bad := []string{
		"ext::sh -c 'id'",
		"-oProxyCommand=evil",
		"fd::17/foo",
		"file:///etc/passwd",
		"--upload-pack=evil",
		"javascript:alert(1)",
	}
	for _, r := range bad {
		if err := ValidateRepo(r); err == nil {
			t.Errorf("ValidateRepo(%q) = nil, want error", r)
		}
	}
}

func TestValidateDomain(t *testing.T) {
	good := []string{"example.com", "a.b.c.example.io", "site-1.example.com"}
	for _, d := range good {
		if err := ValidateDomain(d); err != nil {
			t.Errorf("ValidateDomain(%q) = %v, want nil", d, err)
		}
	}
	bad := []string{"", "../../etc", "evil`whoami`", "a||b", "foo .com", "../x"}
	for _, d := range bad {
		if err := ValidateDomain(d); err == nil {
			t.Errorf("ValidateDomain(%q) = nil, want error", d)
		}
	}
}

func TestValidateBranch(t *testing.T) {
	if err := ValidateBranch("main"); err != nil {
		t.Errorf("ValidateBranch(main) = %v", err)
	}
	for _, b := range []string{"--upload-pack=x", "a..b", "-x"} {
		if err := ValidateBranch(b); err == nil {
			t.Errorf("ValidateBranch(%q) = nil, want error", b)
		}
	}
}

func TestValidateOwner(t *testing.T) {
	if err := ValidateOwner(""); err != nil {
		t.Errorf("empty owner should be allowed: %v", err)
	}
	if err := ValidateOwner("alice"); err != nil {
		t.Errorf("ValidateOwner(alice) = %v", err)
	}
	for _, o := range []string{"../root", "a b", "Alice!", "/etc"} {
		if err := ValidateOwner(o); err == nil {
			t.Errorf("ValidateOwner(%q) = nil, want error", o)
		}
	}
}

func TestRedactStorageSecrets(t *testing.T) {
	in := `{"access_key":"AKIA123","secret_key":"shhh","region":"us-east-1","password":"pw"}`
	out := redactStorageSecrets(in)
	for _, leaked := range []string{"AKIA123", "shhh", "pw"} {
		if strings.Contains(out, leaked) {
			t.Errorf("redactStorageSecrets leaked %q: %s", leaked, out)
		}
	}
	if !strings.Contains(out, "us-east-1") {
		t.Errorf("redactStorageSecrets dropped non-secret field: %s", out)
	}
	// Unparseable input must not pass through.
	if got := redactStorageSecrets("not json"); got != "{}" {
		t.Errorf("redactStorageSecrets(bad) = %q, want {}", got)
	}
}

func TestNormalizeSSHPublicKey(t *testing.T) {
	valid := genTestSSHKey(t)
	got, err := normalizeSSHPublicKey(valid)
	if err != nil {
		t.Fatalf("normalizeSSHPublicKey(valid) error: %v", err)
	}
	if got == "" {
		t.Fatal("normalizeSSHPublicKey returned empty")
	}
	// Multi-line / injected option lines must be rejected.
	for _, bad := range []string{
		valid + "\nssh-rsa AAAAB3Nz second@key",
		"command=\"evil\" " + valid,
		"not a key",
		"",
	} {
		if _, err := normalizeSSHPublicKey(bad); err == nil {
			t.Errorf("normalizeSSHPublicKey(%q) = nil error, want error", bad)
		}
	}
}

func TestDriverLoadRejectsTraversal(t *testing.T) {
	dl := NewDriverLoader(t.TempDir())
	for _, name := range []string{"../../etc/passwd", "..", "a/b", "evil$(x)", "", "UPPER", "a.b"} {
		if _, err := dl.Load(name); err == nil {
			t.Errorf("Load(%q) = nil error, want rejection", name)
		}
	}
}

func TestImageRepoName(t *testing.T) {
	cases := map[string]string{
		"postgres:16":                 "postgres",
		"docker.io/library/mysql:8":   "mysql",
		"attacker/evil-mysql":         "evil-mysql",
		"ghcr.io/x/redis@sha256:abcd": "redis",
		"mariadb":                     "mariadb",
	}
	for in, want := range cases {
		if got := imageRepoName(in); got != want {
			t.Errorf("imageRepoName(%q) = %q, want %q", in, got, want)
		}
	}
	// The exemption must only fire on exact repo names, not substrings.
	if containerSecurityOpt("attacker/evil-mysql") == nil {
		t.Error("no-new-privileges wrongly disabled for non-official image containing 'mysql'")
	}
	if containerSecurityOpt("postgres:16") != nil {
		t.Error("no-new-privileges should be disabled for official postgres image")
	}
}

func TestIsOfficialDBImage(t *testing.T) {
	official := []string{
		"mysql", "mysql:8.0", "mariadb", "postgres:16-alpine",
		"library/redis", "docker.io/library/mongo:7", "index.docker.io/library/mysql",
	}
	for _, img := range official {
		if !isOfficialDBImage(img) {
			t.Errorf("isOfficialDBImage(%q) = false, want true", img)
		}
	}
	// Third-party registry/namespace whose basename is a db name must NOT be
	// exempted from no-new-privileges.
	hostile := []string{
		"evil.io/x/mysql", "evil.io/x/mysql:8", "attacker/postgres",
		"attacker/mysql@sha256:dead", "registry.example.com:5000/team/redis",
		"evil-mysql", "mysql-evil", "notadb",
	}
	for _, img := range hostile {
		if isOfficialDBImage(img) {
			t.Errorf("isOfficialDBImage(%q) = true, want false (exemption bypass)", img)
		}
		if containerSecurityOpt(img) == nil {
			t.Errorf("no-new-privileges wrongly disabled for %q", img)
		}
	}
}

func TestValidateIPRule(t *testing.T) {
	for _, ip := range []string{"1.2.3.4", "10.0.0.0/8", "::1", "2001:db8::/32"} {
		if err := validateIPRule(ip); err != nil {
			t.Errorf("validateIPRule(%q) = %v", ip, err)
		}
	}
	for _, ip := range []string{"", "not-an-ip", "1.2.3.4; rm -rf /", "evil`x`"} {
		if err := validateIPRule(ip); err == nil {
			t.Errorf("validateIPRule(%q) = nil, want error", ip)
		}
	}
}
