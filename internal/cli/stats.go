package cli

import (
	"encoding/json"
	"fmt"

	"github.com/aystro/apod/internal/models"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats [domain]",
	Short: "Show site stats",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s", args[0]))
		if err != nil {
			return err
		}

		var site models.Site
		json.Unmarshal(resp.Data, &site)

		fmt.Printf("Domain:  %s\n", site.Domain)
		fmt.Printf("Driver:  %s\n", site.Driver)
		fmt.Printf("Status:  %s\n", site.Status)
		fmt.Printf("RAM:     %s\n", site.RAM)
		fmt.Printf("CPU:     %s\n", site.CPU)
		fmt.Printf("Created: %s\n", site.CreatedAt.Format("2006-01-02 15:04:05"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
