package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var accessCmd = &cobra.Command{
	Use:   "access [domain]",
	Short: "Open a shell inside a site's container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		shell := accessShell

		dockerPath, err := exec.LookPath("docker")
		if err != nil {
			return fmt.Errorf("docker not found in PATH")
		}

		// Try to find the container:
		// 1. Normal site: apod-<domain>-app
		// 2. Compose site: look for apod.shell=true label, fallback to apod.site label
		containerName := fmt.Sprintf("apod-%s-app", domain)

		// Check if normal container exists
		checkCmd := exec.Command("docker", "inspect", containerName)
		if checkCmd.Run() != nil {
			// Not a normal site — try compose: find shell container by label
			out, err := exec.Command("docker", "ps", "-q",
				"--filter", fmt.Sprintf("label=apod.site=%s", domain),
				"--filter", "label=apod.shell=true",
			).Output()
			if err == nil && len(strings.TrimSpace(string(out))) > 0 {
				containerName = strings.TrimSpace(strings.Split(string(out), "\n")[0])
			} else {
				// Fallback: first container with apod.site label
				out, err = exec.Command("docker", "ps", "-q",
					"--filter", fmt.Sprintf("label=apod.site=%s", domain),
				).Output()
				if err != nil || len(strings.TrimSpace(string(out))) == 0 {
					return fmt.Errorf("no containers found for %s", domain)
				}
				containerName = strings.TrimSpace(strings.Split(string(out), "\n")[0])
			}
		}

		argv := []string{"docker", "exec", "-it", containerName, shell}
		env := os.Environ()
		return syscall.Exec(dockerPath, argv, env)
	},
}

var accessShell string

func init() {
	accessCmd.Flags().StringVar(&accessShell, "shell", "bash", "Shell to use (bash, sh, zsh)")
	rootCmd.AddCommand(accessCmd)
}
