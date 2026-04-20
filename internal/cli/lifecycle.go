package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [domain]",
	Short: "Start a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/start", args[0]), nil)
		if err != nil {
			return err
		}
		fmt.Printf("Site %s started\n", args[0])
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop [domain]",
	Short: "Stop a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/stop", args[0]), nil)
		if err != nil {
			return err
		}
		fmt.Printf("Site %s stopped\n", args[0])
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart [domain]",
	Short: "Restart a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/restart", args[0]), nil)
		if err != nil {
			return err
		}
		fmt.Printf("Site %s restarted\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
}
