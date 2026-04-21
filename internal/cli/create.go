package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var (
	flagDriver  string
	flagRAM     string
	flagCPU     string
	flagStorage string
	flagRepo    string
	flagBranch  string
	flagDeploy  bool
)

var createCmd = &cobra.Command{
	Use:   "create [domain]",
	Short: "Create a new site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		domain := args[0]

		body := map[string]interface{}{
			"domain":  domain,
			"driver":  flagDriver,
			"ram":     flagRAM,
			"cpu":     flagCPU,
			"storage": flagStorage,
			"repo":    flagRepo,
			"branch":  flagBranch,
		}

		resp, err := client.Post("/api/v1/sites", body)
		if err != nil {
			return fmt.Errorf("create site: %w", err)
		}

		var site map[string]interface{}
		json.Unmarshal(resp.Data, &site)
		fmt.Printf("Site %s created successfully\n", domain)

		// Auto-deploy if --deploy flag or repo was provided
		if flagDeploy || flagRepo != "" {
			fmt.Printf("Deploying %s...\n", domain)
			deployBody := map[string]string{"branch": flagBranch}
			_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/deploy", domain), deployBody)
			if err != nil {
				return fmt.Errorf("deploy: %w", err)
			}
			fmt.Printf("Site %s deployed successfully\n", domain)
		}

		return nil
	},
}

func init() {
	createCmd.Flags().StringVar(&flagDriver, "driver", "", "Driver to use (required)")
	createCmd.Flags().StringVar(&flagRAM, "ram", "512M", "Memory limit")
	createCmd.Flags().StringVar(&flagCPU, "cpu", "1", "CPU limit")
	createCmd.Flags().StringVar(&flagStorage, "storage", "0", "Disk storage limit (e.g., 5G, 500M)")
	createCmd.Flags().StringVar(&flagRepo, "repo", "", "Git repository URL")
	createCmd.Flags().StringVar(&flagBranch, "branch", "main", "Git branch")
	createCmd.Flags().BoolVar(&flagDeploy, "deploy", false, "Deploy immediately after creation")
	createCmd.MarkFlagRequired("driver")
	rootCmd.AddCommand(createCmd)
}
