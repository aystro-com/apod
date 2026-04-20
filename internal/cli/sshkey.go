package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var sshKeyCmd = &cobra.Command{Use: "ssh-key", Short: "Manage SSH keys"}

var sshKeyAddCmd = &cobra.Command{
	Use:  "add [name] [public-key]",
	Short: "Add SSH key",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"name": args[0], "public_key": args[1]}
		_, err := client.Post("/api/v1/ssh-keys", body)
		if err != nil {
			return err
		}
		fmt.Printf("SSH key %s added\n", args[0])
		return nil
	},
}

var sshKeyListCmd = &cobra.Command{
	Use:  "list",
	Short: "List SSH keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/ssh-keys")
		if err != nil {
			return err
		}
		var keys []struct {
			Name      string `json:"name"`
			PublicKey string `json:"public_key"`
		}
		json.Unmarshal(resp.Data, &keys)
		if len(keys) == 0 {
			fmt.Println("No SSH keys")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tKEY")
		for _, k := range keys {
			key := k.PublicKey
			if len(key) > 40 {
				key = key[:40] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\n", k.Name, key)
		}
		w.Flush()
		return nil
	},
}

var sshKeyRemoveCmd = &cobra.Command{
	Use:  "remove [name]",
	Short: "Remove SSH key",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Delete(fmt.Sprintf("/api/v1/ssh-keys/%s", args[0]))
		if err != nil {
			return err
		}
		fmt.Printf("SSH key %s removed\n", args[0])
		return nil
	},
}

func init() {
	sshKeyCmd.AddCommand(sshKeyAddCmd, sshKeyListCmd, sshKeyRemoveCmd)
	rootCmd.AddCommand(sshKeyCmd)
}
