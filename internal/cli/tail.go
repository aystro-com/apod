package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var tailCmd = &cobra.Command{
	Use:   "tail [domain]",
	Short: "Show container logs",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/container-logs", args[0]))
		if err != nil {
			return err
		}
		var result map[string]string
		json.Unmarshal(resp.Data, &result)
		fmt.Print(result["logs"])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tailCmd)
}
