package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info [domain]",
	Short: "Show site credentials and connection details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/info", args[0]))
		if err != nil {
			return err
		}

		// Flat map: domain/driver/url plus each credential as its own key.
		var info map[string]string
		json.Unmarshal(resp.Data, &info)

		fmt.Printf("Domain:  %s\n", info["domain"])
		fmt.Printf("Driver:  %s\n", info["driver"])
		fmt.Printf("URL:     %s\n", info["url"])

		first := true
		for k, v := range info {
			if k == "domain" || k == "driver" || k == "url" {
				continue
			}
			if first {
				fmt.Println("\nCredentials:")
				first = false
			}
			fmt.Printf("  %s = %s\n", k, v)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
