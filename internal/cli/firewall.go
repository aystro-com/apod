package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var firewallCmd = &cobra.Command{Use: "firewall", Short: "Manage server firewall"}

var firewallStatusCmd = &cobra.Command{
	Use:  "status",
	Short: "Show firewall status",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/firewall")
		if err != nil {
			return err
		}
		var status struct {
			Active bool     `json:"active"`
			Rules  []string `json:"rules"`
		}
		json.Unmarshal(resp.Data, &status)
		if status.Active {
			fmt.Println("Firewall: ACTIVE")
		} else {
			fmt.Println("Firewall: INACTIVE")
		}
		for _, r := range status.Rules {
			fmt.Println("  " + r)
		}
		return nil
	},
}

var firewallEnableCmd = &cobra.Command{
	Use:  "enable",
	Short: "Enable firewall",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Post("/api/v1/firewall/enable", nil)
		if err != nil {
			return err
		}
		fmt.Println("Firewall enabled")
		return nil
	},
}

var firewallAllowCmd = &cobra.Command{
	Use:  "allow [port]",
	Short: "Allow a port",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"port": args[0]}
		_, err := client.Post("/api/v1/firewall/allow", body)
		if err != nil {
			return err
		}
		fmt.Printf("Port %s allowed\n", args[0])
		return nil
	},
}

var firewallDenyCmd = &cobra.Command{
	Use:  "deny [port]",
	Short: "Deny a port",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"port": args[0]}
		_, err := client.Post("/api/v1/firewall/deny", body)
		if err != nil {
			return err
		}
		fmt.Printf("Port %s denied\n", args[0])
		return nil
	},
}

func init() {
	firewallCmd.AddCommand(firewallStatusCmd, firewallEnableCmd, firewallAllowCmd, firewallDenyCmd)
	rootCmd.AddCommand(firewallCmd)
}
