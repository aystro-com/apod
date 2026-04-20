package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aystro/apod/internal/models"
	"github.com/spf13/cobra"
)

var driverCmd = &cobra.Command{
	Use:   "driver",
	Short: "Manage drivers",
}

var driverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available drivers",
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/drivers")
		if err != nil {
			return err
		}

		var drivers []models.Driver
		json.Unmarshal(resp.Data, &drivers)

		if len(drivers) == 0 {
			fmt.Println("No drivers found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION\tDESCRIPTION")
		for _, d := range drivers {
			fmt.Fprintf(w, "%s\t%s\t%s\n", d.Name, d.Version, d.Description)
		}
		w.Flush()
		return nil
	},
}

func init() {
	driverCmd.AddCommand(driverListCmd)
	rootCmd.AddCommand(driverCmd)
}
