package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	flagDriver string
	flagRAM    string
	flagCPU    string
	flagRepo   string
	flagBranch string
)

var createCmd = &cobra.Command{
	Use:   "create [domain]",
	Short: "Create a new site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		domain := args[0]

		body := map[string]interface{}{
			"domain": domain,
			"driver": flagDriver,
			"ram":    flagRAM,
			"cpu":    flagCPU,
			"repo":   flagRepo,
			"branch": flagBranch,
		}

		resp, err := client.Post("/api/v1/sites", body)
		if err != nil {
			return fmt.Errorf("create site: %w", err)
		}

		var site map[string]interface{}
		json.Unmarshal(resp.Data, &site)
		fmt.Printf("Site %s created successfully\n", domain)
		return nil
	},
}

func init() {
	createCmd.Flags().StringVar(&flagDriver, "driver", "", "Driver to use (required)")
	createCmd.Flags().StringVar(&flagRAM, "ram", "256M", "Memory limit")
	createCmd.Flags().StringVar(&flagCPU, "cpu", "1", "CPU limit")
	createCmd.Flags().StringVar(&flagRepo, "repo", "", "Git repository URL")
	createCmd.Flags().StringVar(&flagBranch, "branch", "main", "Git branch")
	createCmd.MarkFlagRequired("driver")
	rootCmd.AddCommand(createCmd)
}
