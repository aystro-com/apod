package engine

import (
	"testing"
)

func TestParseEnvJSON(t *testing.T) {
	envs, err := parseEnvJSON(`{"APP_DEBUG":"false","DB_HOST":"localhost"}`)
	if err != nil {
		t.Fatalf("parseEnvJSON: %v", err)
	}
	if envs["APP_DEBUG"] != "false" {
		t.Errorf("APP_DEBUG = %q, want false", envs["APP_DEBUG"])
	}
	if envs["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST = %q, want localhost", envs["DB_HOST"])
	}
}

func TestParseEnvJSONEmpty(t *testing.T) {
	envs, err := parseEnvJSON("{}")
	if err != nil {
		t.Fatalf("parseEnvJSON: %v", err)
	}
	if len(envs) != 0 {
		t.Errorf("got %d envs, want 0", len(envs))
	}
}

func TestEnvToJSON(t *testing.T) {
	envs := map[string]string{"APP_DEBUG": "false", "DB_HOST": "localhost"}
	jsonStr, err := envToJSON(envs)
	if err != nil {
		t.Fatalf("envToJSON: %v", err)
	}

	// Parse it back to verify
	parsed, _ := parseEnvJSON(jsonStr)
	if parsed["APP_DEBUG"] != "false" {
		t.Errorf("roundtrip failed for APP_DEBUG")
	}
}

func TestEnvToSlice(t *testing.T) {
	envs := map[string]string{"APP_DEBUG": "false", "DB_HOST": "localhost"}
	slice := envToSlice(envs)

	if len(slice) != 2 {
		t.Errorf("got %d items, want 2", len(slice))
	}

	found := map[string]bool{}
	for _, s := range slice {
		found[s] = true
	}
	if !found["APP_DEBUG=false"] {
		t.Error("missing APP_DEBUG=false")
	}
	if !found["DB_HOST=localhost"] {
		t.Error("missing DB_HOST=localhost")
	}
}

func TestMergeEnv(t *testing.T) {
	existing := `{"APP_DEBUG":"true","DB_HOST":"localhost"}`
	result, err := mergeEnv(existing, "APP_DEBUG", "false")
	if err != nil {
		t.Fatalf("mergeEnv: %v", err)
	}

	envs, _ := parseEnvJSON(result)
	if envs["APP_DEBUG"] != "false" {
		t.Errorf("APP_DEBUG = %q, want false", envs["APP_DEBUG"])
	}
	if envs["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST should be unchanged")
	}
}

func TestRemoveEnv(t *testing.T) {
	existing := `{"APP_DEBUG":"true","DB_HOST":"localhost"}`
	result, err := removeEnv(existing, "APP_DEBUG")
	if err != nil {
		t.Fatalf("removeEnv: %v", err)
	}

	envs, _ := parseEnvJSON(result)
	if _, ok := envs["APP_DEBUG"]; ok {
		t.Error("APP_DEBUG should be removed")
	}
	if envs["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST should be unchanged")
	}
}
