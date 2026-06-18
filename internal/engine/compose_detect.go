package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// preferredWebPorts are container ports treated as the most likely HTTP entry
// point when auto-detecting which compose service apod should route to.
var preferredWebPorts = []string{"80", "8080", "443", "8000", "3000", "5000"}

type composePortsDoc struct {
	Services map[string]struct {
		Ports  []yaml.Node `yaml:"ports"`
		Expose []yaml.Node `yaml:"expose"`
	} `yaml:"services"`
}

// parseShortPort returns the container port from a short-form ports entry such
// as "8080:80", "80", "127.0.0.1:9000:80/tcp" or a range "8000-8005:80". UDP
// entries are skipped (not HTTP routable).
func parseShortPort(s string) string {
	s = strings.TrimSpace(strings.Trim(s, `"'`))
	if s == "" {
		return ""
	}
	if i := strings.Index(s, "/"); i >= 0 {
		if strings.EqualFold(s[i+1:], "udp") {
			return ""
		}
		s = s[:i]
	}
	// The container port is the segment after the last ':'.
	parts := strings.Split(s, ":")
	cp := parts[len(parts)-1]
	if i := strings.Index(cp, "-"); i >= 0 { // port range — take the first
		cp = cp[:i]
	}
	if _, err := strconv.Atoi(cp); err != nil {
		return ""
	}
	return cp
}

// containerPortsOf extracts the container-side TCP ports a service publishes,
// from both `ports` (short or long syntax) and `expose`.
func containerPortsOf(ports, expose []yaml.Node) []string {
	var out []string
	for _, n := range ports {
		switch n.Kind {
		case yaml.ScalarNode:
			if p := parseShortPort(n.Value); p != "" {
				out = append(out, p)
			}
		case yaml.MappingNode:
			var m struct {
				Target   int    `yaml:"target"`
				Protocol string `yaml:"protocol"`
			}
			if n.Decode(&m) == nil && m.Target != 0 && !strings.EqualFold(m.Protocol, "udp") {
				out = append(out, strconv.Itoa(m.Target))
			}
		}
	}
	for _, n := range expose {
		if n.Kind == yaml.ScalarNode {
			if v := strings.TrimSpace(strings.Trim(n.Value, `"'`)); v != "" {
				if _, err := strconv.Atoi(v); err == nil {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

func lowestPort(ports []string) string {
	best := ports[0]
	bn, _ := strconv.Atoi(best)
	for _, p := range ports[1:] {
		if n, err := strconv.Atoi(p); err == nil && n < bn {
			best, bn = p, n
		}
	}
	return best
}

// composeWebTarget auto-detects which service and container port apod should
// route HTTP traffic to, from raw compose content. It prefers a service
// exposing a well-known web port; otherwise, when exactly one service publishes
// ports, that one wins; ties break deterministically by service name. This is
// what lets apod ingest a stock docker-compose.yml without a hand-written
// proxy_service/proxy_port wrapper.
func composeWebTarget(content []byte) (service, port string, err error) {
	var doc composePortsDoc
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return "", "", fmt.Errorf("parse compose: %w", err)
	}

	type cand struct {
		svc   string
		ports []string
	}
	var cands []cand
	for name, svc := range doc.Services {
		if ps := containerPortsOf(svc.Ports, svc.Expose); len(ps) > 0 {
			cands = append(cands, cand{name, ps})
		}
	}
	if len(cands) == 0 {
		return "", "", fmt.Errorf("no published ports found in compose; cannot determine the web service automatically")
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].svc < cands[j].svc })

	// 1) A candidate exposing a well-known web port wins.
	for _, want := range preferredWebPorts {
		for _, c := range cands {
			for _, p := range c.ports {
				if p == want {
					return c.svc, p, nil
				}
			}
		}
	}
	// 2) Otherwise the (deterministically) first candidate, its lowest port.
	return cands[0].svc, lowestPort(cands[0].ports), nil
}
