package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var flagPurge bool

var destroyCmd = &cobra.Command{
	Use:   "destroy [domain]",
	Short: "Destroy a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		domain := args[0]

		path := fmt.Sprintf("/api/v1/sites/%s", domain)
		if flagPurge {
			path += "?purge=true"
		}

		_, err := client.Delete(path)
		if err != nil {
			return fmt.Errorf("destroy site: %w", err)
		}

		fmt.Printf("Site %s destroyed\n", domain)
		return nil
	},
}

func init() {
	destroyCmd.Flags().BoolVar(&flagPurge, "purge", false, "Remove all data including files")
	rootCmd.AddCommand(destroyCmd)
}
