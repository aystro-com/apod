package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var processCmd = &cobra.Command{Use: "process", Short: "Manage app processes (web, workers, scheduler)"}

var processListCmd = &cobra.Command{
	Use:   "list [domain]",
	Short: "List a site's processes and replica counts",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		resp, err := client.Get(fmt.Sprintf("/api/v1/sites/%s/processes", args[0]))
		if err != nil {
			return err
		}
		var procs []struct {
			Service  string `json:"service"`
			Role     string `json:"role"`
			Replicas int    `json:"replicas"`
			Running  int    `json:"running"`
			Scalable bool   `json:"scalable"`
		}
		json.Unmarshal(resp.Data, &procs)
		if len(procs) == 0 {
			fmt.Println("No processes")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "SERVICE\tROLE\tDESIRED\tRUNNING\tSCALABLE")
		for _, p := range procs {
			role := p.Role
			if role == "" {
				role = "service"
			}
			fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%v\n", p.Service, role, p.Replicas, p.Running, p.Scalable)
		}
		w.Flush()
		return nil
	},
}

var processScaleCmd = &cobra.Command{
	Use:   "scale [domain] [service] [replicas]",
	Short: "Set the replica count for a worker process",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		n, err := strconv.Atoi(args[2])
		if err != nil {
			return fmt.Errorf("replicas must be an integer")
		}
		client := NewClient(flagRemote, flagKey)
		if _, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/processes/%s/scale", args[0], args[1]), map[string]int{"replicas": n}); err != nil {
			return err
		}
		fmt.Printf("Scaled %s to %d replica(s)\n", args[1], n)
		return nil
	},
}

var processRestartCmd = &cobra.Command{
	Use:   "restart [domain] [service]",
	Short: "Restart all replicas of a process",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client := NewClient(flagRemote, flagKey)
		if _, err := client.Post(fmt.Sprintf("/api/v1/sites/%s/processes/%s/restart", args[0], args[1]), nil); err != nil {
			return err
		}
		fmt.Printf("Restarted %s\n", args[1])
		return nil
	},
}

func init() {
	processCmd.AddCommand(processListCmd, processScaleCmd, processRestartCmd)
	rootCmd.AddCommand(processCmd)
}
