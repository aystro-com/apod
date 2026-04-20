package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var ftpCmd = &cobra.Command{Use: "ftp", Short: "Manage FTP/SFTP accounts"}

var ftpAddCmd = &cobra.Command{
	Use:  "add [domain]",
	Short: "Add FTP account",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		user, _ := cmd.Flags().GetString("user")
		pass, _ := cmd.Flags().GetString("password")
		if user == "" || pass == "" {
			return fmt.Errorf("--user and --password required")
		}
		body := map[string]string{"username": user, "password": pass}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/ftp", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("FTP account %s created for %s\n", user, args[0])
		return nil
	},
}

var ftpListCmd = &cobra.Command{
	Use:  "list [domain]",
	Short: "List FTP accounts",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/ftp", args[0]))
		if err != nil {
			return err
		}
		var accounts []struct {
			Username string `json:"username"`
		}
		json.Unmarshal(resp.Data, &accounts)
		if len(accounts) == 0 {
			fmt.Println("No FTP accounts")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "USERNAME")
		for _, a := range accounts {
			fmt.Fprintf(w, "%s\n", a.Username)
		}
		w.Flush()
		return nil
	},
}

var ftpRemoveCmd = &cobra.Command{
	Use:  "remove [domain] [username]",
	Short: "Remove FTP account",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Delete(fmt.Sprintf("/api/v1/sites/%s/ftp/%s", args[0], args[1]))
		if err != nil {
			return err
		}
		fmt.Printf("FTP account %s removed\n", args[1])
		return nil
	},
}

func init() {
	ftpAddCmd.Flags().String("user", "", "FTP username")
	ftpAddCmd.Flags().String("password", "", "FTP password")
	ftpCmd.AddCommand(ftpAddCmd, ftpListCmd, ftpRemoveCmd)
	rootCmd.AddCommand(ftpCmd)
}
