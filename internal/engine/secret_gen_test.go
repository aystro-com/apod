package engine

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/aystro/apod/internal/models"
)

func TestRandomHex(t *testing.T) {
	h := randomHex(16)
	if len(h) != 32 {
		t.Errorf("randomHex(16) length = %d, want 32", len(h))
	}
	for _, c := range h {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Fatalf("non-hex character %q in %q", c, h)
		}
	}
	if randomHex(16) == h {
		t.Error("two randomHex calls returned identical output")
	}
}

func TestRandomBase64(t *testing.T) {
	s := randomBase64(30)
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("randomBase64 not valid base64: %v", err)
	}
	if len(raw) != 30 {
		t.Errorf("decoded %d bytes, want 30", len(raw))
	}
	if randomBase64(30) == s {
		t.Error("two randomBase64 calls returned identical output")
	}
}

func TestGenerateJWT(t *testing.T) {
	secret := "test-secret"
	token := generateJWT(secret, "anon")

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT should have 3 parts, got %d", len(parts))
	}

	// Payload carries the role claim and issuer.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if !strings.Contains(string(payload), `"role":"anon"`) {
		t.Errorf("payload missing role claim: %s", payload)
	}
	if !strings.Contains(string(payload), `"iss":"apod"`) {
		t.Errorf("payload missing issuer: %s", payload)
	}

	// The signature must verify under the signing secret.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != want {
		t.Error("JWT signature does not verify with the secret")
	}

	// A different secret must NOT verify (tamper/forgery resistance).
	mac2 := hmac.New(sha256.New, []byte("wrong-secret"))
	mac2.Write([]byte(parts[0] + "." + parts[1]))
	if base64.RawURLEncoding.EncodeToString(mac2.Sum(nil)) == parts[2] {
		t.Error("signature verified under the wrong secret")
	}
}

func TestSecretGenerators(t *testing.T) {
	gens := secretGenerators()

	// Every advertised generated secret name must have a generator.
	for _, name := range generatedSecretNames {
		if _, ok := gens[name]; !ok {
			t.Errorf("no generator for advertised secret %q", name)
		}
	}

	vars := map[string]string{}
	vars["jwt_secret"] = gens["jwt_secret"](vars)
	if vars["jwt_secret"] == "" {
		t.Fatal("jwt_secret generator returned empty")
	}

	// anon_key / service_role_key are JWTs signed by jwt_secret.
	anon := gens["anon_key"](vars)
	if strings.Count(anon, ".") != 2 {
		t.Errorf("anon_key should be a JWT, got %q", anon)
	}
	mac := hmac.New(sha256.New, []byte(vars["jwt_secret"]))
	parts := strings.Split(anon, ".")
	mac.Write([]byte(parts[0] + "." + parts[1]))
	if base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) != parts[2] {
		t.Error("anon_key not signed by jwt_secret")
	}

	// Non-JWT secrets are non-empty and distinct across keys.
	seen := map[string]bool{}
	for _, name := range []string{"secret_key_base", "vault_enc_key", "s3_access_key_secret"} {
		v := gens[name](vars)
		if v == "" {
			t.Errorf("%s generator returned empty", name)
		}
		if seen[v] {
			t.Errorf("%s collided with another secret", name)
		}
		seen[v] = true
	}
}

func TestDriverRawText(t *testing.T) {
	d := &models.Driver{Services: map[string]models.DriverService{
		"app": {
			Image:   "myimage:1",
			Command: "run ${jwt_secret}",
			Volumes: []string{"vol:/data"},
		},
	}}
	txt := driverRawText(d)
	for _, want := range []string{"myimage:1", "${jwt_secret}", "vol:/data"} {
		if !strings.Contains(txt, want) {
			t.Errorf("driverRawText missing %q in %q", want, txt)
		}
	}
}

func TestParseEnvFile(t *testing.T) {
	content := `# a comment
APP_ENV=production

DB_PASSWORD=p@ss=with=equals
  SPACED = trimmed
INVALID_LINE
=novalue`
	m := parseEnvFile(content)

	if m["APP_ENV"] != "production" {
		t.Errorf("APP_ENV = %q, want production", m["APP_ENV"])
	}
	// Only the first '=' splits; the rest is part of the value.
	if m["DB_PASSWORD"] != "p@ss=with=equals" {
		t.Errorf("DB_PASSWORD = %q", m["DB_PASSWORD"])
	}
	if _, ok := m["# a comment"]; ok {
		t.Error("comment line should be skipped")
	}
	if _, ok := m["INVALID_LINE"]; ok {
		t.Error("line without '=' should be skipped")
	}
	// A line starting with '=' has an empty key (idx == 0) and is skipped.
	if _, ok := m[""]; ok {
		t.Error("empty-key line should be skipped")
	}
}
