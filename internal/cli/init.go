package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

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

		// ACME email
		fmt.Print("\nEmail for SSL certificates (Let's Encrypt): ")
		email, _ := reader.ReadString('\n')
		email = strings.TrimSpace(email)
		if email == "" {
			return fmt.Errorf("email is required for SSL certificates")
		}

		// Data directory
		fmt.Print("Data directory [/var/lib/apod]: ")
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

		// Copy bundled drivers if available
		fmt.Print("Setting up drivers... ")
		exec.Command("apod", "update", "drivers").Run()
		fmt.Println("OK")

		// Create systemd service
		fmt.Print("Creating systemd service... ")
		service := fmt.Sprintf(`[Unit]
Description=apod server orchestrator
After=docker.service
Requires=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/apod server --acme-email %s --data-dir %s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, email, dataDir)

		if werr := os.WriteFile("/etc/systemd/system/apod.service", []byte(service), 0644); werr != nil {
			fmt.Println("FAILED")
			fmt.Printf("  Could not write service file: %v\n", werr)
			fmt.Println("  You can start manually: apod server --acme-email", email)
		} else {
			fmt.Println("OK")
			exec.Command("systemctl", "daemon-reload").Run()
			exec.Command("systemctl", "enable", "apod").Run()
			exec.Command("systemctl", "start", "apod").Run()
			fmt.Println("  Service enabled and started.")
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

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
