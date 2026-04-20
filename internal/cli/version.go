package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show apod version",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/version")
		if err != nil {
			// If daemon isn't running, just show the compiled version
			fmt.Println("apod version (daemon not running, showing compiled version)")
			return nil
		}
		var result struct {
			Version   string `json:"version"`
			DBVersion int    `json:"db_version"`
		}
		json.Unmarshal(resp.Data, &result)
		fmt.Printf("apod v%s (db schema: v%d)\n", result.Version, result.DBVersion)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
