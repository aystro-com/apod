package engine

import "testing"

func TestBuildTraefikRule(t *testing.T) {
	domains := []string{"example.com", "www.example.com", "shop.example.com"}
	rule := buildTraefikRule(domains)
	expected := "Host(`example.com`) || Host(`www.example.com`) || Host(`shop.example.com`)"
	if rule != expected {
		t.Errorf("got %q, want %q", rule, expected)
	}
}

func TestBuildTraefikRuleSingle(t *testing.T) {
	domains := []string{"example.com"}
	rule := buildTraefikRule(domains)
	expected := "Host(`example.com`)"
	if rule != expected {
		t.Errorf("got %q, want %q", rule, expected)
	}
}
