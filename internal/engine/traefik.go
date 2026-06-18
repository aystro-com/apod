package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const (
	traefikContainerName = "apod-traefik"
	traefikImage         = "traefik:v2.11"
	apodNetwork          = "apod-net"

	// certResolverName is the Traefik ACME resolver attached to site routers.
	certResolverName = "letsencrypt"
)

// TLS strategies. apod has to issue certs across very different network
// topologies, and no single ACME challenge works everywhere — so the strategy
// is selectable:
//
//   - auto:     ACME HTTP-01. Zero-config default. Needs the domain to resolve
//     straight to this server on :80 (direct DNS / grey-cloud).
//   - dns:      ACME DNS-01 via a provider API. Works behind Cloudflare proxy,
//     CDNs, or closed :80, and is the only way to get wildcards.
//   - external: No ACME. An upstream proxy (e.g. Cloudflare "Full") terminates
//     public TLS; Traefik serves its default self-signed cert on :443
//     (or a cert dropped into the file provider, e.g. a Cloudflare
//     Origin certificate) and does not force an HTTP->HTTPS redirect.
const (
	TLSModeAuto     = "auto"
	TLSModeDNS      = "dns"
	TLSModeExternal = "external"
)

// dnsProviderEnvPrefixes are environment-variable name prefixes forwarded from
// the daemon's environment into the Traefik container so lego's DNS-01
// providers can authenticate. Covers the common providers; set the relevant
// vars in the apod service environment (see EnvironmentFile in `apod init`).
var dnsProviderEnvPrefixes = []string{
	"CF_", "CLOUDFLARE_", // Cloudflare
	"AWS_",                 // Route 53
	"DO_", "DIGITALOCEAN_", // DigitalOcean
	"AZURE_",       // Azure
	"GCE_", "GCP_", // Google Cloud
	"HETZNER_",   // Hetzner
	"LINODE_",    // Linode
	"OVH_",       // OVH
	"VULTR_",     // Vultr
	"NAMECHEAP_", // Namecheap
	"GANDIV5_",   // Gandi
	"PORKBUN_",   // Porkbun
	"DNSIMPLE_",  // DNSimple
}

// TLSConfig describes how Traefik should obtain/serve certificates.
type TLSConfig struct {
	Mode        string // one of TLSMode*; empty means auto
	Email       string // ACME account email (auto/dns)
	DNSProvider string // lego provider code for dns mode, e.g. "cloudflare"
}

func (c TLSConfig) mode() string {
	if c.Mode == "" {
		return TLSModeAuto
	}
	return c.Mode
}

// CertResolver returns the Traefik certresolver name to attach to site routers,
// or "" when ACME is not used (external TLS).
func (c TLSConfig) CertResolver() string {
	if c.mode() == TLSModeExternal {
		return ""
	}
	return certResolverName
}

type Traefik struct {
	docker *Docker
	tls    TLSConfig
}

func NewTraefik(docker *Docker, tls TLSConfig) *Traefik {
	return &Traefik{docker: docker, tls: tls}
}

func traefikCommand(cfg TLSConfig) []string {
	args := []string{
		"--api.dashboard=false",
		"--providers.docker=true",
		"--providers.docker.exposedbydefault=false",
		"--providers.docker.network=" + apodNetwork,
		"--entrypoints.web.address=:80",
		"--entrypoints.websecure.address=:443",
		"--serversTransport.insecureSkipVerify=true",
		"--providers.file.directory=/etc/traefik/dynamic",
		"--providers.file.watch=true",
	}

	if cfg.mode() == TLSModeExternal {
		// Upstream terminates TLS. No ACME, and no forced HTTP->HTTPS redirect
		// (that would loop under Cloudflare "Flexible"). Traefik serves its
		// default cert on :443, or a cert from the file provider.
		return args
	}

	email := cfg.Email
	if email == "" {
		email = "admin@localhost" // ACME will reject this; `apod init` validates.
	}
	r := "--certificatesresolvers." + certResolverName + ".acme."
	args = append(args,
		"--entrypoints.web.http.redirections.entrypoint.to=websecure",
		"--entrypoints.web.http.redirections.entrypoint.scheme=https",
		r+"email="+email,
		r+"storage=/letsencrypt/acme.json",
	)
	if cfg.mode() == TLSModeDNS {
		args = append(args,
			r+"dnschallenge=true",
			r+"dnschallenge.provider="+cfg.DNSProvider,
		)
	} else {
		args = append(args, r+"httpchallenge.entrypoint=web")
	}
	return args
}

// dnsProviderEnv collects credential env vars (KEY=VALUE) from the daemon's own
// environment to forward to the Traefik container for DNS-01.
func dnsProviderEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		for _, p := range dnsProviderEnvPrefixes {
			if strings.HasPrefix(name, p) {
				env = append(env, kv)
				break
			}
		}
	}
	return env
}

func (t *Traefik) EnsureRunning(ctx context.Context) error {
	exists, err := t.docker.ContainerExists(ctx, traefikContainerName)
	if err != nil {
		return fmt.Errorf("check traefik: %w", err)
	}
	if exists {
		return nil
	}

	if err := t.docker.EnsureNetwork(ctx, apodNetwork); err != nil {
		return fmt.Errorf("ensure network: %w", err)
	}

	if err := t.docker.PullImage(ctx, traefikImage); err != nil {
		return fmt.Errorf("pull traefik image: %w", err)
	}

	var env []string
	if t.tls.mode() == TLSModeDNS {
		env = dnsProviderEnv()
	}

	id, err := t.docker.CreateContainer(ctx, ContainerConfig{
		Name:  traefikContainerName,
		Image: traefikImage,
		Env:   env,
		Labels: map[string]string{
			"apod.managed": "true",
			"apod.role":    "proxy",
		},
		Volumes: map[string]string{
			// Read-only: Traefik only needs to watch the Docker API, never
			// write to it. A writable socket is a direct host-root escape.
			"/var/run/docker.sock":      "/var/run/docker.sock:ro",
			"apod-letsencrypt":          "/letsencrypt",
			"/etc/apod/traefik/dynamic": "/etc/traefik/dynamic:ro",
		},
		Ports: map[string]string{
			"80":  "80",
			"443": "443",
		},
		Args: traefikCommand(t.tls),
	})
	if err != nil {
		return fmt.Errorf("create traefik container: %w", err)
	}

	if err := t.docker.ConnectNetwork(ctx, apodNetwork, id); err != nil {
		return fmt.Errorf("connect traefik to network: %w", err)
	}

	if err := t.docker.StartContainer(ctx, id); err != nil {
		return fmt.Errorf("start traefik: %w", err)
	}

	return nil
}

func TraefikLabels(siteDomain string, domains []string, servicePort string, backendScheme string, certResolver string) map[string]string {
	routerName := strings.ReplaceAll(siteDomain, ".", "-")

	var hostRules []string
	for _, d := range domains {
		hostRules = append(hostRules, fmt.Sprintf("Host(`%s`)", d))
	}
	rule := strings.Join(hostRules, " || ")

	labels := map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.routers.%s.rule", routerName):                      rule,
		fmt.Sprintf("traefik.http.routers.%s.tls", routerName):                       "true",
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", routerName): servicePort,
		labelPrefix + "site":    siteDomain,
		labelPrefix + "managed": "true",
	}

	// Attach an ACME resolver only when apod is managing certs (auto/dns).
	// In external mode certResolver is empty and Traefik serves the default
	// cert (or a file-provider cert) for the router.
	if certResolver != "" {
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", routerName)] = certResolver
	}

	if backendScheme != "" {
		labels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.scheme", routerName)] = backendScheme
	}

	// Attach the per-site IP allowlist middleware (file provider) when
	// enforcement is enabled. ApplyIPRules always writes the referenced file,
	// defaulting to allow-all, so the reference never dangles.
	if ipRulesEnforced() {
		labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", routerName)] = ipAllowMiddlewareName(siteDomain) + "@file"
	}

	return labels
}
