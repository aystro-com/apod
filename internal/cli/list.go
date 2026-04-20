package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aystro/apod/internal/models"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all sites",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/sites")
		if err != nil {
			return err
		}

		var sites []models.Site
		json.Unmarshal(resp.Data, &sites)

		if len(sites) == 0 {
			fmt.Println("No sites found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DOMAIN\tDRIVER\tSTATUS\tRAM\tCPU")
		for _, s := range sites {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", s.Domain, s.Driver, s.Status, s.RAM, s.CPU)
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
