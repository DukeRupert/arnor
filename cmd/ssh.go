package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Manage Hetzner Cloud SSH keys",
}

var sshListCmd = &cobra.Command{
	Use:   "list",
	Short: "List SSH keys across all Hetzner projects",
	RunE:  runSSHList,
}

var sshAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Upload an SSH key to a Hetzner project",
	RunE:  runSSHAdd,
}

func init() {
	sshAddCmd.Flags().String("name", "", "Name for the key in Hetzner")
	sshAddCmd.Flags().String("key", "", "Path to public key file")
	sshAddCmd.Flags().String("project", "", "Hetzner project alias")

	sshCmd.AddCommand(sshListCmd)
	sshCmd.AddCommand(sshAddCmd)
	rootCmd.AddCommand(sshCmd)
}

func runSSHList(cmd *cobra.Command, args []string) error {
	mgr, err := newHetznerManager()
	if err != nil {
		return err
	}

	keys, err := mgr.ListAllSSHKeys()
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		fmt.Println("No SSH keys found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tFINGERPRINT\tPROJECT")
	fmt.Fprintln(w, "──\t────\t───────────\t───────")
	for _, k := range keys {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", k.ID, k.Name, k.Fingerprint, k.ProjectAlias)
	}
	return w.Flush()
}

func runSSHAdd(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	keyPath, _ := cmd.Flags().GetString("key")
	project, _ := cmd.Flags().GetString("project")

	if name == "" || keyPath == "" || project == "" {
		return fmt.Errorf("--name, --key, and --project are required")
	}

	mgr, err := newHetznerManager()
	if err != nil {
		return err
	}

	client, err := mgr.Client(project)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("reading key file: %w", err)
	}

	key, err := client.AddSSHKey(name, strings.TrimSpace(string(data)))
	if err != nil {
		return err
	}

	fmt.Printf("Created SSH key %q (ID: %d, Fingerprint: %s) in project %s\n", key.Name, key.ID, key.Fingerprint, project)
	return nil
}
