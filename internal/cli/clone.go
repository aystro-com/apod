package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cloneCmd = &cobra.Command{
	Use:   "clone [source-domain] [target-domain]",
	Short: "Clone a site to a new domain",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"target": args[1]}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/clone", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Site %s cloned to %s\n", args[0], args[1])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(cloneCmd)
}
