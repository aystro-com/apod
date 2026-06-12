package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage scoped API tokens (personal access tokens)",
}

var tokenCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a scoped API token",
	Long: `Create a personal access token with a limited set of abilities.

Abilities (comma-separated via --abilities): read, write, deploy.
Use --sensitive to allow reading secrets (env values, DB credentials).
Scoped tokens can never manage users, passwords, 2FA, or other tokens.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		abilitiesStr, _ := cmd.Flags().GetString("abilities")
		sensitive, _ := cmd.Flags().GetBool("sensitive")
		ttl, _ := cmd.Flags().GetInt("ttl-days")

		var abilities []string
		for _, a := range strings.Split(abilitiesStr, ",") {
			if a = strings.TrimSpace(a); a != "" {
				abilities = append(abilities, a)
			}
		}

		body := map[string]interface{}{
			"name":      args[0],
			"abilities": abilities,
			"sensitive": sensitive,
			"ttl_days":  ttl,
		}
		resp, err := client.Post("/api/v1/tokens", body)
		if err != nil {
			return err
		}
		var result struct {
			Token string `json:"token"`
		}
		json.Unmarshal(resp.Data, &result)
		fmt.Printf("API token %q created:\n\n  %s\n\n", args[0], result.Token)
		fmt.Println("Save it now — it will not be shown again.")
		return nil
	},
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List your scoped API tokens",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/tokens")
		if err != nil {
			return err
		}
		var result struct {
			Tokens []struct {
				ID        int64      `json:"id"`
				Name      string     `json:"name"`
				Abilities string     `json:"abilities"`
				Sensitive bool       `json:"sensitive"`
				ExpiresAt *time.Time `json:"expires_at"`
				CreatedAt time.Time  `json:"created_at"`
			} `json:"tokens"`
		}
		json.Unmarshal(resp.Data, &result)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tABILITIES\tSENSITIVE\tEXPIRES")
		for _, tok := range result.Tokens {
			exp := "never"
			if tok.ExpiresAt != nil {
				exp = tok.ExpiresAt.Format("2006-01-02")
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%v\t%s\n", tok.ID, tok.Name, tok.Abilities, tok.Sensitive, exp)
		}
		w.Flush()
		return nil
	},
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke [id]",
	Short: "Revoke a scoped API token by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		var id int64
		fmt.Sscanf(args[0], "%d", &id)
		_, err := client.Delete2("/api/v1/tokens", map[string]interface{}{"id": id})
		if err != nil {
			return err
		}
		fmt.Printf("Token %d revoked.\n", id)
		return nil
	},
}

func init() {
	tokenCreateCmd.Flags().String("abilities", "read", "comma-separated abilities: read,write,deploy")
	tokenCreateCmd.Flags().Bool("sensitive", false, "allow reading secrets (env, DB credentials)")
	tokenCreateCmd.Flags().Int("ttl-days", 0, "expiry in days (0 = never)")
	tokenCmd.AddCommand(tokenCreateCmd)
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)
	rootCmd.AddCommand(tokenCmd)
}
