package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage users",
}

var userCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		role, _ := cmd.Flags().GetString("role")

		body := map[string]string{"name": args[0], "role": role}
		resp, err := client.Post("/api/v1/users", body)
		if err != nil {
			return err
		}

		var result struct {
			User struct {
				Name string `json:"name"`
				UID  int    `json:"uid"`
				Role string `json:"role"`
			} `json:"user"`
			APIKey string `json:"api_key"`
		}
		json.Unmarshal(resp.Data, &result)

		fmt.Printf("User created: %s (uid: %d, role: %s)\n", result.User.Name, result.User.UID, result.User.Role)
		fmt.Println()
		fmt.Printf("API Key: %s\n", result.APIKey)
		fmt.Println()
		fmt.Println("Save this key — it will not be shown again.")
		fmt.Printf("\nUsage: apod --remote https://<server>:8443 --key %s list\n", result.APIKey)
		return nil
	},
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/users")
		if err != nil {
			return err
		}

		var users []struct {
			Name      string `json:"name"`
			UID       int    `json:"uid"`
			Role      string `json:"role"`
			CreatedAt string `json:"created_at"`
		}
		json.Unmarshal(resp.Data, &users)

		if len(users) == 0 {
			fmt.Println("No users found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tUID\tROLE\tCREATED")
		for _, u := range users {
			fmt.Fprintf(w, "%s\t%d\t%s\t%s\n", u.Name, u.UID, u.Role, u.CreatedAt[:10])
		}
		w.Flush()
		return nil
	},
}

var userDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Delete(fmt.Sprintf("/api/v1/users/%s", args[0]))
		if err != nil {
			return err
		}
		fmt.Printf("User %s deleted\n", args[0])
		return nil
	},
}

var userResetKeyCmd = &cobra.Command{
	Use:   "reset-key [name]",
	Short: "Reset a user's API key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Post(fmt.Sprintf("/api/v1/users/%s/reset-key", args[0]), nil)
		if err != nil {
			return err
		}

		var result struct {
			APIKey string `json:"api_key"`
		}
		json.Unmarshal(resp.Data, &result)

		fmt.Printf("New API Key for %s: %s\n", args[0], result.APIKey)
		fmt.Println("Save this key — it will not be shown again.")
		return nil
	},
}

var transferCmd = &cobra.Command{
	Use:   "transfer [domain] [new-owner]",
	Short: "Transfer site ownership to another user (use '' to unassign)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"owner": args[1]}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/transfer", args[0]), body)
		if err != nil {
			return err
		}
		if args[1] == "" {
			fmt.Printf("Site %s unassigned (now admin-owned)\n", args[0])
		} else {
			fmt.Printf("Site %s transferred to %s\n", args[0], args[1])
		}
		return nil
	},
}

func init() {
	userCreateCmd.Flags().String("role", "user", "User role (admin or user)")
	userCmd.AddCommand(userCreateCmd)
	userCmd.AddCommand(userListCmd)
	userCmd.AddCommand(userDeleteCmd)
	userCmd.AddCommand(userResetKeyCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(transferCmd)
}
