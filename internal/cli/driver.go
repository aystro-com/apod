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

var driverGetCmd = &cobra.Command{
	Use:   "get [name]",
	Short: "Print a driver's YAML definition",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get("/api/v1/drivers/" + args[0])
		if err != nil {
			return err
		}
		var d struct {
			YAML    string `json:"yaml"`
			Builtin bool   `json:"builtin"`
		}
		json.Unmarshal(resp.Data, &d)
		fmt.Print(d.YAML)
		return nil
	},
}

var driverAddFile string

var driverAddCmd = &cobra.Command{
	Use:   "add [name]",
	Short: "Create or update a custom driver from a YAML file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if driverAddFile == "" {
			return fmt.Errorf("--file is required")
		}
		content, err := os.ReadFile(driverAddFile)
		if err != nil {
			return fmt.Errorf("read %s: %w", driverAddFile, err)
		}
		client := NewClient(flagRemote, flagKey)
		body := map[string]string{"name": args[0], "yaml": string(content)}
		if _, err := client.Post("/api/v1/drivers", body); err != nil {
			return err
		}
		fmt.Printf("Driver %q saved\n", args[0])
		return nil
	},
}

var driverRemoveCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Delete a custom driver (built-ins are protected)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		if _, err := client.Delete("/api/v1/drivers/" + args[0]); err != nil {
			return err
		}
		fmt.Printf("Driver %q removed\n", args[0])
		return nil
	},
}

func init() {
	driverAddCmd.Flags().StringVarP(&driverAddFile, "file", "f", "", "Path to the driver YAML file")
	driverCmd.AddCommand(driverListCmd, driverGetCmd, driverAddCmd, driverRemoveCmd)
	rootCmd.AddCommand(driverCmd)
}
