package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var networkCmd = &cobra.Command{
	Use:   "network",
	Short: "Manage shared networks that connect sites privately",
}

var networkCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a shared network",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		if _, err := client.Post("/api/v1/networks", map[string]string{"name": args[0]}); err != nil {
			return err
		}
		fmt.Printf("Created shared network %q.\n", args[0])
		return nil
	},
}

var networkListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List shared networks",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/networks")
		if err != nil {
			return err
		}
		var nets []struct {
			Name    string   `json:"name"`
			Owner   string   `json:"owner"`
			Members []string `json:"members"`
		}
		json.Unmarshal(resp.Data, &nets)
		if len(nets) == 0 {
			fmt.Println("No shared networks.")
			return nil
		}
		for _, n := range nets {
			owner := n.Owner
			if owner == "" {
				owner = "admin"
			}
			fmt.Printf("%s (owner: %s)\n", n.Name, owner)
			for _, m := range n.Members {
				fmt.Printf("  - %s\n", m)
			}
		}
		return nil
	},
}

var networkDeleteCmd = &cobra.Command{
	Use:   "rm [name]",
	Short: "Delete a shared network",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		if _, err := client.Delete("/api/v1/networks/" + args[0]); err != nil {
			return err
		}
		fmt.Printf("Deleted shared network %q.\n", args[0])
		return nil
	},
}

var networkAddCmd = &cobra.Command{
	Use:   "add [name] [domain]",
	Short: "Attach a site to a shared network",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		if _, err := client.Post("/api/v1/networks/"+args[0]+"/members", map[string]string{"domain": args[1]}); err != nil {
			return err
		}
		fmt.Printf("Added %s to network %q.\n", args[1], args[0])
		return nil
	},
}

var networkRemoveCmd = &cobra.Command{
	Use:   "remove [name] [domain]",
	Short: "Detach a site from a shared network",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		if _, err := client.Delete("/api/v1/networks/" + args[0] + "/members/" + args[1]); err != nil {
			return err
		}
		fmt.Printf("Removed %s from network %q.\n", args[1], args[0])
		return nil
	},
}

func init() {
	networkCmd.AddCommand(networkCreateCmd)
	networkCmd.AddCommand(networkListCmd)
	networkCmd.AddCommand(networkDeleteCmd)
	networkCmd.AddCommand(networkAddCmd)
	networkCmd.AddCommand(networkRemoveCmd)
	rootCmd.AddCommand(networkCmd)
}
