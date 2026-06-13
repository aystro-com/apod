package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize apod on this server",
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

		fmt.Println("Welcome to apod! Let's set up your server.")
		fmt.Println()

		// Check Docker
		fmt.Print("Checking Docker... ")
		if err := exec.Command("docker", "info").Run(); err != nil {
			fmt.Println("NOT FOUND")
			fmt.Println("Please install Docker first: https://docs.docker.com/engine/install/")
			return fmt.Errorf("docker not installed")
		}
		fmt.Println("OK")

		// TLS strategy. Different network topologies need different ACME
		// challenges, so let the operator choose how certs are issued/served.
		fmt.Println("\nHow should apod handle SSL/TLS?")
		fmt.Println("  1) auto     — Let's Encrypt HTTP-01 (default). The domain must resolve")
		fmt.Println("                straight to this server. Best for plain DNS / grey-cloud.")
		fmt.Println("  2) dns      — Let's Encrypt DNS-01 via your DNS provider's API. Works")
		fmt.Println("                behind Cloudflare proxy / CDNs, and supports wildcards.")
		fmt.Println("  3) external — No Let's Encrypt; an upstream proxy (e.g. Cloudflare 'Full')")
		fmt.Println("                terminates TLS. apod serves a self-signed / origin cert.")
		fmt.Print("Choice [1]: ")
		tlsChoice, _ := reader.ReadString('\n')

		var tlsMode, dnsProvider, email, dnsToken string
		switch strings.TrimSpace(tlsChoice) {
		case "2", "dns":
			tlsMode = "dns"
		case "3", "external":
			tlsMode = "external"
		default:
			tlsMode = "auto"
		}

		// ACME email — required for auto/dns (Let's Encrypt rejects empty and
		// @localhost), unused in external mode.
		if tlsMode != "external" {
			for {
				fmt.Print("\nEmail for Let's Encrypt certificates: ")
				email, _ = reader.ReadString('\n')
				email = strings.TrimSpace(email)
				if email != "" && strings.Contains(email, "@") && !strings.HasSuffix(email, "@localhost") {
					break
				}
				fmt.Println("  A real email is required (Let's Encrypt rejects empty / @localhost).")
			}
		}

		// DNS-01 provider + credentials.
		if tlsMode == "dns" {
			fmt.Print("DNS provider (lego code) [cloudflare]: ")
			dnsProvider, _ = reader.ReadString('\n')
			dnsProvider = strings.TrimSpace(dnsProvider)
			if dnsProvider == "" {
				dnsProvider = "cloudflare"
			}
			if dnsProvider == "cloudflare" {
				fmt.Print("Cloudflare API token (Zone:DNS:Edit — leave blank to add later): ")
				dnsToken, _ = reader.ReadString('\n')
				dnsToken = strings.TrimSpace(dnsToken)
			}
		}

		// Data directory
		fmt.Print("\nData directory [/var/lib/apod]: ")
		dataDir, _ := reader.ReadString('\n')
		dataDir = strings.TrimSpace(dataDir)
		if dataDir == "" {
			dataDir = "/var/lib/apod"
		}

		// Driver directory
		driverDir := "/etc/apod/drivers"

		// Create directories
		fmt.Print("\nCreating directories... ")
		os.MkdirAll(dataDir, 0755)
		os.MkdirAll(driverDir, 0755)
		os.MkdirAll("/etc/apod", 0755)
		fmt.Println("OK")

		// DNS credentials go in an env file the service loads, kept out of the
		// world-readable unit file.
		if tlsMode == "dns" && dnsProvider == "cloudflare" && dnsToken != "" {
			if werr := os.WriteFile("/etc/apod/apod.env", []byte("CF_DNS_API_TOKEN="+dnsToken+"\n"), 0600); werr != nil {
				fmt.Printf("  Warning: could not write /etc/apod/apod.env: %v\n", werr)
			}
		}

		// Copy bundled drivers if available
		fmt.Print("Setting up drivers... ")
		exec.Command("apod", "update", "drivers").Run()
		fmt.Println("OK")

		// Build the daemon command for the chosen TLS mode.
		execStart := "/usr/local/bin/apod server --data-dir " + dataDir
		switch tlsMode {
		case "dns":
			execStart += " --tls-mode dns --acme-dns-provider " + dnsProvider + " --acme-email " + email
		case "external":
			execStart += " --tls-mode external"
		default:
			execStart += " --acme-email " + email
		}

		// Create systemd service
		fmt.Print("Creating systemd service... ")
		service := fmt.Sprintf(`[Unit]
Description=apod server orchestrator
After=docker.service
Requires=docker.service

[Service]
Type=simple
EnvironmentFile=-/etc/apod/apod.env
ExecStart=%s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, execStart)

		if werr := os.WriteFile("/etc/systemd/system/apod.service", []byte(service), 0644); werr != nil {
			fmt.Println("FAILED")
			fmt.Printf("  Could not write service file: %v\n", werr)
			fmt.Println("  You can start manually:", execStart)
		} else {
			fmt.Println("OK")
			exec.Command("systemctl", "daemon-reload").Run()
			exec.Command("systemctl", "enable", "apod").Run()
			exec.Command("systemctl", "start", "apod").Run()
			fmt.Println("  Service enabled and started.")
		}

		// Mode-specific follow-up.
		switch tlsMode {
		case "dns":
			if dnsProvider != "cloudflare" || dnsToken == "" {
				fmt.Println()
				fmt.Printf("  DNS-01 with %q needs credentials. Add them to /etc/apod/apod.env\n", dnsProvider)
				fmt.Println("  (e.g. CF_DNS_API_TOKEN=...), then: systemctl restart apod")
			}
		case "external":
			fmt.Println()
			fmt.Println("  External TLS: set your proxy to terminate HTTPS (e.g. Cloudflare SSL")
			fmt.Println("  mode 'Full'). To present a trusted origin cert, drop cert/key into")
			fmt.Println("  /etc/apod/traefik/dynamic/ (e.g. a Cloudflare Origin certificate).")
		}

		fmt.Println()
		fmt.Println("apod is ready! Try:")
		fmt.Println("  apod create mysite.com --driver php")
		fmt.Println("  apod create myapp.com --driver laravel --repo https://github.com/you/app.git")
		fmt.Println()
		fmt.Println("Available drivers:")
		fmt.Println("  php       PHP + Nginx + MySQL (blank environment)")
		fmt.Println("  laravel   Laravel with Nginx, PHP, Composer, Node")
		fmt.Println("  wordpress WordPress with Apache and MySQL")
		fmt.Println("  node      Node.js + PostgreSQL")
		fmt.Println("  static    Static HTML with Nginx")
		fmt.Println("  odoo      Odoo ERP with PostgreSQL")
		fmt.Println("  unifi     UniFi Network Application with MongoDB")
		fmt.Println("  apod-ui   Web admin panel for apod")

		// Offer to install the web admin panel (apod-ui).
		fmt.Println()
		fmt.Print("Install the web admin panel now? It runs on its own domain. [y/N]: ")
		uiAns, _ := reader.ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(uiAns)); a == "y" || a == "yes" {
			fmt.Println("  The panel needs a domain whose DNS already points at this server (for SSL).")
			fmt.Print("  Panel domain (e.g. panel.example.com): ")
			uiDomain, _ := reader.ReadString('\n')
			uiDomain = strings.TrimSpace(uiDomain)
			if uiDomain == "" {
				fmt.Println("  No domain entered — skipped. Install it later with:")
				fmt.Println("    apod create <domain> --driver apod-ui")
			} else {
				fmt.Printf("  Installing apod-ui on %s ...\n", uiDomain)
				// Give the freshly-started daemon a moment to open its socket.
				for i := 0; i < 10; i++ {
					if _, serr := os.Stat("/run/apod/apod.sock"); serr == nil {
						break
					}
					time.Sleep(time.Second)
				}
				c := exec.Command("apod", "create", uiDomain, "--driver", "apod-ui")
				c.Stdout, c.Stderr = os.Stdout, os.Stderr
				if err := c.Run(); err != nil {
					fmt.Printf("  Could not install the panel (%v). Retry later with:\n", err)
					fmt.Println("    apod create " + uiDomain + " --driver apod-ui")
				} else {
					fmt.Printf("  Panel installed → https://%s\n", uiDomain)
				}
			}
		} else {
			fmt.Println("  Skipped. You can install the web UI any time (you'll need a domain):")
			fmt.Println("    apod create <domain> --driver apod-ui")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
