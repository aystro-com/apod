package cli

import (
	"github.com/spf13/cobra"
)

var (
	flagRemote string
	flagKey    string
)

var rootCmd = &cobra.Command{
	Use:   "apod",
	Short: "Open-source Docker-based server orchestrator",
	Long:  "apod manages Docker containers to host websites and applications with full isolation, automatic SSL, and a plugin-based driver system.",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagRemote, "remote", "", "Remote server URL (e.g. https://server:8443)")
	rootCmd.PersistentFlags().StringVar(&flagKey, "key", "", "API key for remote access")
}

func Execute() error {
	return rootCmd.Execute()
}
