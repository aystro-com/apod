package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update apod to latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)

		// Check first
		resp, err := client.Get("/api/v1/update/check")
		if err != nil {
			return err
		}
		var check struct {
			Current   string `json:"current"`
			Latest    string `json:"latest"`
			HasUpdate bool   `json:"has_update"`
		}
		json.Unmarshal(resp.Data, &check)

		if !check.HasUpdate {
			fmt.Printf("Already on latest version (v%s)\n", check.Current)
			return nil
		}

		fmt.Printf("Updating from v%s to v%s...\n", check.Current, check.Latest)

		_, err = client.Post("/api/v1/update", nil)
		if err != nil {
			return err
		}
		fmt.Println("Updated successfully. Restart apod server to use the new version:")
		fmt.Println("  systemctl restart apod")
		return nil
	},
}

var updateDriversCmd = &cobra.Command{
	Use:   "drivers",
	Short: "Update built-in drivers to latest",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Post("/api/v1/update/drivers", nil)
		if err != nil {
			return err
		}
		var result struct {
			Updated []string `json:"updated"`
		}
		json.Unmarshal(resp.Data, &result)
		if len(result.Updated) == 0 {
			fmt.Println("All drivers up to date")
		} else {
			fmt.Printf("Updated drivers: %v\n", result.Updated)
		}
		return nil
	},
}

func init() {
	updateCmd.AddCommand(updateDriversCmd)
	rootCmd.AddCommand(updateCmd)
}
