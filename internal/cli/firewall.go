package cli

import (
	"encoding/json"
	"fmt"
	"strconv"

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

var firewallRulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "List numbered firewall rules",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/firewall/rules")
		if err != nil {
			return err
		}
		var rules []struct {
			Num    int    `json:"num"`
			To     string `json:"to"`
			Action string `json:"action"`
			From   string `json:"from"`
		}
		json.Unmarshal(resp.Data, &rules)
		if len(rules) == 0 {
			fmt.Println("No rules")
			return nil
		}
		for _, r := range rules {
			fmt.Printf("[%d] %-28s %-8s %s\n", r.Num, r.To, r.Action, r.From)
		}
		return nil
	},
}

var fwAllowFromSource, fwAllowFromPort, fwAllowFromProto string

var firewallAllowFromCmd = &cobra.Command{
	Use:   "allow-from",
	Short: "Whitelist a source IP/CIDR (optionally to a port)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"source": fwAllowFromSource, "port": fwAllowFromPort, "proto": fwAllowFromProto}
		if _, err := client.Post("/api/v1/firewall/allow-from", body); err != nil {
			return err
		}
		fmt.Printf("Allowed from %s\n", fwAllowFromSource)
		return nil
	},
}

var firewallDeleteCmd = &cobra.Command{
	Use:   "delete [num]",
	Short: "Delete a rule by its number (see 'firewall rules')",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		num, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("rule number must be an integer")
		}
		client := NewClient(flagRemote, flagKey)
		if _, err := client.Post("/api/v1/firewall/delete", map[string]int{"num": num}); err != nil {
			return err
		}
		fmt.Printf("Rule %d deleted\n", num)
		return nil
	},
}

func init() {
	firewallAllowFromCmd.Flags().StringVar(&fwAllowFromSource, "source", "", "Source IP or CIDR (required)")
	firewallAllowFromCmd.Flags().StringVar(&fwAllowFromPort, "port", "", "Destination port (optional)")
	firewallAllowFromCmd.Flags().StringVar(&fwAllowFromProto, "proto", "", "Protocol: tcp or udp (optional)")
	firewallAllowFromCmd.MarkFlagRequired("source")

	firewallCmd.AddCommand(firewallStatusCmd, firewallRulesCmd, firewallEnableCmd,
		firewallAllowCmd, firewallAllowFromCmd, firewallDenyCmd, firewallDeleteCmd)
	rootCmd.AddCommand(firewallCmd)
}
