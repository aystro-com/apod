package engine

import (
	"strings"
	"testing"
)

func TestTraefikLabels(t *testing.T) {
	labels := TraefikLabels("example.com", []string{"example.com", "www.example.com"}, "80", "", certResolverName)

	if labels["traefik.enable"] != "true" {
		t.Error("expected traefik.enable to be true")
	}

	expectedRule := "Host(`example.com`) || Host(`www.example.com`)"
	routerKey := "traefik.http.routers.example-com.rule"
	if labels[routerKey] != expectedRule {
		t.Errorf("got rule %q, want %q", labels[routerKey], expectedRule)
	}

	portKey := "traefik.http.services.example-com.loadbalancer.server.port"
	if labels[portKey] != "80" {
		t.Errorf("got port %q, want 80", labels[portKey])
	}

	if labels["traefik.http.routers.example-com.tls.certresolver"] != "letsencrypt" {
		t.Error("expected certresolver label for ACME mode")
	}

	if labels["apod.site"] != "example.com" {
		t.Error("expected apod.site label")
	}
}

func TestTraefikLabelsSingleDomain(t *testing.T) {
	labels := TraefikLabels("test.com", []string{"test.com"}, "8080", "", certResolverName)

	routerKey := "traefik.http.routers.test-com.rule"
	expected := "Host(`test.com`)"
	if labels[routerKey] != expected {
		t.Errorf("got rule %q, want %q", labels[routerKey], expected)
	}
}

// External TLS: no ACME resolver attached to the router (Traefik serves a
// default / file-provider cert), but TLS is still enabled.
func TestTraefikLabelsExternalNoResolver(t *testing.T) {
	labels := TraefikLabels("ext.com", []string{"ext.com"}, "80", "", "")

	if _, ok := labels["traefik.http.routers.ext-com.tls.certresolver"]; ok {
		t.Error("did not expect a certresolver label in external mode")
	}
	if labels["traefik.http.routers.ext-com.tls"] != "true" {
		t.Error("expected tls to still be enabled")
	}
}

func hasArg(cmd []string, arg string) bool {
	for _, a := range cmd {
		if a == arg {
			return true
		}
	}
	return false
}

func hasArgWithPrefix(cmd []string, prefix string) bool {
	for _, a := range cmd {
		if strings.HasPrefix(a, prefix) {
			return true
		}
	}
	return false
}

func TestTraefikCommandAuto(t *testing.T) {
	cmd := traefikCommand(TLSConfig{Email: "admin@example.com"})

	for _, want := range []string{
		"--api.dashboard=false",
		"--providers.docker=true",
		"--entrypoints.web.address=:80",
		"--entrypoints.websecure.address=:443",
		"--entrypoints.web.http.redirections.entrypoint.to=websecure",
		"--certificatesresolvers.letsencrypt.acme.email=admin@example.com",
		"--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json",
		"--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web",
	} {
		if !hasArg(cmd, want) {
			t.Errorf("auto mode missing flag: %s", want)
		}
	}
	if hasArgWithPrefix(cmd, "--certificatesresolvers.letsencrypt.acme.dnschallenge") {
		t.Error("auto mode should not configure dnschallenge")
	}
}

func TestTraefikCommandDefaultEmail(t *testing.T) {
	cmd := traefikCommand(TLSConfig{})
	if !hasArg(cmd, "--certificatesresolvers.letsencrypt.acme.email=admin@localhost") {
		t.Error("expected default email when empty")
	}
}

func TestTraefikCommandDNS(t *testing.T) {
	cmd := traefikCommand(TLSConfig{Mode: TLSModeDNS, Email: "a@b.com", DNSProvider: "cloudflare"})

	for _, want := range []string{
		"--certificatesresolvers.letsencrypt.acme.dnschallenge=true",
		"--certificatesresolvers.letsencrypt.acme.dnschallenge.provider=cloudflare",
		"--certificatesresolvers.letsencrypt.acme.email=a@b.com",
	} {
		if !hasArg(cmd, want) {
			t.Errorf("dns mode missing flag: %s", want)
		}
	}
	if hasArgWithPrefix(cmd, "--certificatesresolvers.letsencrypt.acme.httpchallenge") {
		t.Error("dns mode should not configure httpchallenge")
	}
}

func TestTraefikCommandExternal(t *testing.T) {
	cmd := traefikCommand(TLSConfig{Mode: TLSModeExternal})

	if hasArgWithPrefix(cmd, "--certificatesresolvers.") {
		t.Error("external mode should not configure any ACME resolver")
	}
	if hasArgWithPrefix(cmd, "--entrypoints.web.http.redirections") {
		t.Error("external mode should not force an HTTP->HTTPS redirect")
	}
	if !hasArg(cmd, "--entrypoints.websecure.address=:443") {
		t.Error("external mode should still define the websecure entrypoint")
	}
}

func TestCertResolver(t *testing.T) {
	if got := (TLSConfig{}).CertResolver(); got != "letsencrypt" {
		t.Errorf("auto mode resolver = %q, want letsencrypt", got)
	}
	if got := (TLSConfig{Mode: TLSModeDNS}).CertResolver(); got != "letsencrypt" {
		t.Errorf("dns mode resolver = %q, want letsencrypt", got)
	}
	if got := (TLSConfig{Mode: TLSModeExternal}).CertResolver(); got != "" {
		t.Errorf("external mode resolver = %q, want empty", got)
	}
}
