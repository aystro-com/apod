package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Manage site domains",
}

var domainAddCmd = &cobra.Command{
	Use:   "add [site-domain] [new-domain]",
	Short: "Add a domain to a site",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"domain": args[1]}
		_, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/domains", args[0]), body)
		if err != nil {
			return err
		}
		fmt.Printf("Domain %s added to %s\n", args[1], args[0])
		return nil
	},
}

var domainRemoveCmd = &cobra.Command{
	Use:   "remove [site-domain] [domain-to-remove]",
	Short: "Remove a domain from a site",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		_, err := client.Delete(fmt.Sprintf("/api/v1/sites/%s/domains/%s", args[0], args[1]))
		if err != nil {
			return err
		}
		fmt.Printf("Domain %s removed from %s\n", args[1], args[0])
		return nil
	},
}

var domainListCmd = &cobra.Command{
	Use:   "list [site-domain]",
	Short: "List domains for a site",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/domains", args[0]))
		if err != nil {
			return err
		}
		var domains []string
		json.Unmarshal(resp.Data, &domains)
		if len(domains) == 0 {
			fmt.Println("No domains found")
			return nil
		}
		for _, d := range domains {
			fmt.Println(d)
		}
		return nil
	},
}

func init() {
	domainCmd.AddCommand(domainAddCmd)
	domainCmd.AddCommand(domainRemoveCmd)
	domainCmd.AddCommand(domainListCmd)
	rootCmd.AddCommand(domainCmd)
}
