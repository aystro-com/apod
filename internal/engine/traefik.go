package engine

import (
	"context"
	"fmt"
	"strings"
)

const (
	traefikContainerName = "apod-traefik"
	traefikImage         = "traefik:v3.0"
	apodNetwork          = "apod-net"
)

type Traefik struct {
	docker    *Docker
	acmeEmail string
}

func NewTraefik(docker *Docker, acmeEmail string) *Traefik {
	return &Traefik{docker: docker, acmeEmail: acmeEmail}
}

func traefikCommand(email string) []string {
	if email == "" {
		email = "admin@localhost"
	}
	return []string{
		"--api.dashboard=false",
		"--providers.docker=true",
		"--providers.docker.exposedbydefault=false",
		"--providers.docker.network=" + apodNetwork,
		"--entrypoints.web.address=:80",
		"--entrypoints.websecure.address=:443",
		"--entrypoints.web.http.redirections.entrypoint.to=websecure",
		"--entrypoints.web.http.redirections.entrypoint.scheme=https",
		"--certificatesresolvers.letsencrypt.acme.email=" + email,
		"--certificatesresolvers.letsencrypt.acme.storage=/letsencrypt/acme.json",
		"--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=web",
	}
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

	cmd := traefikCommand(t.acmeEmail)

	id, err := t.docker.CreateContainer(ctx, ContainerConfig{
		Name:  traefikContainerName,
		Image: traefikImage,
		Labels: map[string]string{
			"apod.managed": "true",
			"apod.role":    "proxy",
		},
		Volumes: map[string]string{
			"/var/run/docker.sock": "/var/run/docker.sock",
			"apod-letsencrypt":     "/letsencrypt",
		},
		Ports: map[string]string{
			"80":  "80",
			"443": "443",
		},
		Command: strings.Join(cmd, " "),
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

func TraefikLabels(siteDomain string, domains []string, servicePort string) map[string]string {
	routerName := strings.ReplaceAll(siteDomain, ".", "-")

	var hostRules []string
	for _, d := range domains {
		hostRules = append(hostRules, fmt.Sprintf("Host(`%s`)", d))
	}
	rule := strings.Join(hostRules, " || ")

	return map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.routers.%s.rule", routerName):                      rule,
		fmt.Sprintf("traefik.http.routers.%s.tls", routerName):                       "true",
		fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", routerName):          "letsencrypt",
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", routerName): servicePort,
		labelPrefix + "site":    siteDomain,
		labelPrefix + "managed": "true",
	}
}
