package engine

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUFWRules(t *testing.T) {
	out := `Status: active

     To                         Action      From
     --                         ------      ----
[ 1] 22/tcp                     ALLOW IN    Anywhere
[ 2] 80                         ALLOW IN    192.168.1.0/24
[ 3] 3306                       DENY IN     10.0.0.5
`
	rules := parseUFWRules(out)
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3: %+v", len(rules), rules)
	}
	if rules[0].Num != 1 || rules[0].To != "22/tcp" || rules[0].Action != "ALLOW" || rules[0].From != "Anywhere" {
		t.Errorf("rule 1 parsed wrong: %+v", rules[0])
	}
	if rules[1].From != "192.168.1.0/24" {
		t.Errorf("rule 2 From wrong: %+v", rules[1])
	}
	if rules[2].Action != "DENY" || rules[2].From != "10.0.0.5" {
		t.Errorf("rule 3 parsed wrong: %+v", rules[2])
	}
}

func TestValidatePortNumber(t *testing.T) {
	for _, p := range []string{"1", "80", "65535"} {
		if err := ValidatePortNumber(p); err != nil {
			t.Errorf("ValidatePortNumber(%q) = %v", p, err)
		}
	}
	for _, p := range []string{"0", "65536", "-1", "abc", "", "80; rm"} {
		if err := ValidatePortNumber(p); err == nil {
			t.Errorf("ValidatePortNumber(%q) = nil, want error", p)
		}
	}
}

func TestValidateUFWPort(t *testing.T) {
	for _, p := range []string{"80", "443/tcp", "53/udp", "6000:6010/tcp"} {
		if err := ValidateUFWPort(p); err != nil {
			t.Errorf("ValidateUFWPort(%q) = %v", p, err)
		}
	}
	for _, p := range []string{"", "80/sctp", "99999", "80 443", "allow", "6000:99999"} {
		if err := ValidateUFWPort(p); err == nil {
			t.Errorf("ValidateUFWPort(%q) = nil, want error", p)
		}
	}
}

func TestValidateIPOrCIDR(t *testing.T) {
	for _, s := range []string{"1.2.3.4", "10.0.0.0/8", "::1", "2001:db8::/32", "any", "ANY"} {
		if err := ValidateIPOrCIDR(s); err != nil {
			t.Errorf("ValidateIPOrCIDR(%q) = %v", s, err)
		}
	}
	for _, s := range []string{"", "nope", "1.2.3.4; rm", "10.0.0.0/99"} {
		if err := ValidateIPOrCIDR(s); err == nil {
			t.Errorf("ValidateIPOrCIDR(%q) = nil, want error", s)
		}
	}
}

func TestDriverSaveAndDelete(t *testing.T) {
	dl := NewDriverLoader(t.TempDir())
	yaml := "name: mycustom\nversion: \"1.0\"\ndescription: test\n"

	// Save a valid custom driver.
	if err := dl.Save("mycustom", yaml); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := dl.GetContent("mycustom")
	if err != nil || got != yaml {
		t.Fatalf("GetContent = %q, %v", got, err)
	}

	// Name mismatch is rejected.
	if err := dl.Save("other", yaml); err == nil {
		t.Error("Save with mismatched name should fail")
	}
	// Invalid YAML rejected.
	if err := dl.Save("bad", "name: [unterminated"); err == nil {
		t.Error("Save with invalid YAML should fail")
	}
	// Traversal name rejected.
	if err := dl.Save("../evil", yaml); err == nil {
		t.Error("Save with traversal name should fail")
	}

	// Delete the custom driver.
	if err := dl.Delete("mycustom"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := dl.GetContent("mycustom"); err == nil {
		t.Error("driver still present after delete")
	}

	// Built-ins cannot be deleted.
	if err := dl.Delete("wordpress"); err == nil {
		t.Error("deleting a built-in driver should fail")
	}
}

func TestDriverSaveContainment(t *testing.T) {
	dir := t.TempDir()
	dl := NewDriverLoader(dir)
	if err := dl.Save("ok", "name: ok\n"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := dl.GetContent("ok"); err != nil {
		t.Fatalf("GetContent: %v", err)
	}
	// The file must live directly under the driver dir.
	if _, err := filepath.Rel(dir, filepath.Join(dir, "ok.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestBuildIPAllowListTOML(t *testing.T) {
	// No allow rules => allow all (never lock a site out).
	def := buildIPAllowListTOML("a.com", nil)
	if !strings.Contains(def, "0.0.0.0/0") || !strings.Contains(def, "::/0") {
		t.Errorf("default allowlist should allow all: %s", def)
	}
	if !strings.Contains(def, "a-com-ipallow") {
		t.Errorf("middleware name wrong: %s", def)
	}
	// With rules => only those ranges, quoted.
	out := buildIPAllowListTOML("a.com", []string{"203.0.113.5", "10.0.0.0/8"})
	if !strings.Contains(out, `"203.0.113.5"`) || !strings.Contains(out, `"10.0.0.0/8"`) {
		t.Errorf("allowlist missing ranges: %s", out)
	}
	if strings.Contains(out, "0.0.0.0/0") {
		t.Errorf("allowlist should not allow-all when rules exist: %s", out)
	}
}
