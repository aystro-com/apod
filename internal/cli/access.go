package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

var accessCmd = &cobra.Command{
	Use:   "access [domain]",
	Short: "Open a shell inside a site's app container",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		domain := args[0]
		containerName := fmt.Sprintf("apod-%s-app", domain)
		shell := accessShell

		// Find docker binary
		dockerPath, err := exec.LookPath("docker")
		if err != nil {
			return fmt.Errorf("docker not found in PATH")
		}

		// Replace current process with docker exec
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
