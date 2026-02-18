package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/hetzner"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Manage Hetzner Cloud servers",
}

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all servers across all Hetzner projects",
	RunE:  runServerList,
}

var serverViewCmd = &cobra.Command{
	Use:   "view [name]",
	Short: "Show server details",
	Args:  cobra.ExactArgs(1),
	RunE:  runServerView,
}

func init() {
	serverCmd.AddCommand(serverListCmd)
	serverCmd.AddCommand(serverViewCmd)
	rootCmd.AddCommand(serverCmd)
}

func newHetznerManager() (*hetzner.Manager, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if len(cfg.HetznerProjects) == 0 {
		return nil, fmt.Errorf("no Hetzner projects configured — run 'arnor config init' first")
	}
	return hetzner.NewManager(cfg.HetznerProjects)
}

func runServerList(cmd *cobra.Command, args []string) error {
	mgr, err := newHetznerManager()
	if err != nil {
		return err
	}

	servers, err := mgr.ListAllServers()
	if err != nil {
		return err
	}

	if len(servers) == 0 {
		fmt.Println("No servers found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tIP\tSTATUS\tTYPE\tLOCATION\tPROJECT")
	fmt.Fprintln(w, "────\t──\t──────\t────\t────────\t───────")
	for _, s := range servers {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Name,
			s.PublicNet.IPv4.IP,
			s.Status,
			s.ServerType.Name,
			s.Datacenter.Location.Name,
			s.ProjectAlias,
		)
	}
	return w.Flush()
}

func runServerView(cmd *cobra.Command, args []string) error {
	mgr, err := newHetznerManager()
	if err != nil {
		return err
	}

	s, err := mgr.GetServer(args[0])
	if err != nil {
		return err
	}

	fmt.Printf("Name:       %s\n", s.Name)
	fmt.Printf("ID:         %d\n", s.ID)
	fmt.Printf("Status:     %s\n", s.Status)
	fmt.Printf("IPv4:       %s\n", s.PublicNet.IPv4.IP)
	fmt.Printf("Type:       %s\n", s.ServerType.Name)
	fmt.Printf("Datacenter: %s\n", s.Datacenter.Name)
	fmt.Printf("Location:   %s (%s)\n", s.Datacenter.Location.Name, s.Datacenter.Location.City)
	fmt.Printf("Project:    %s\n", s.ProjectAlias)
	fmt.Printf("Created:    %s\n", s.Created)
	return nil
}
