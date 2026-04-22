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

		var info struct {
			Domain  string            `json:"domain"`
			Driver  string            `json:"driver"`
			URL     string            `json:"url"`
			Secrets map[string]string `json:"secrets"`
		}
		json.Unmarshal(resp.Data, &info)

		fmt.Printf("Domain:  %s\n", info.Domain)
		fmt.Printf("Driver:  %s\n", info.Driver)
		fmt.Printf("URL:     %s\n", info.URL)

		if len(info.Secrets) > 0 {
			fmt.Println("\nCredentials:")
			for k, v := range info.Secrets {
				fmt.Printf("  %s = %s\n", k, v)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
