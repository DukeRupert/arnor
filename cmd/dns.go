package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/dns"
	"github.com/spf13/cobra"
)

var dnsCmd = &cobra.Command{
	Use:   "dns",
	Short: "Manage DNS records (auto-detects Porkbun or Cloudflare)",
}

var dnsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List DNS records for a domain",
	RunE:  runDNSList,
}

var dnsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a DNS record",
	RunE:  runDNSCreate,
}

var dnsDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a DNS record by ID",
	RunE:  runDNSDelete,
}

func init() {
	dnsListCmd.Flags().String("domain", "", "Domain name (e.g. example.com)")

	dnsCreateCmd.Flags().String("domain", "", "Domain name (e.g. example.com)")
	dnsCreateCmd.Flags().String("name", "", "Record name (e.g. www)")
	dnsCreateCmd.Flags().String("type", "", "Record type (A, CNAME, TXT, etc.)")
	dnsCreateCmd.Flags().String("content", "", "Record content (e.g. IP address)")
	dnsCreateCmd.Flags().String("ttl", "600", "Time to live in seconds")

	dnsDeleteCmd.Flags().String("domain", "", "Domain name (e.g. example.com)")
	dnsDeleteCmd.Flags().String("id", "", "Record ID to delete")

	dnsCmd.AddCommand(dnsListCmd)
	dnsCmd.AddCommand(dnsCreateCmd)
	dnsCmd.AddCommand(dnsDeleteCmd)
	rootCmd.AddCommand(dnsCmd)
}

func getProvider(domain string) (dns.Provider, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return dns.ProviderForDomain(domain, cfg)
}

func runDNSList(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	if domain == "" {
		return fmt.Errorf("--domain is required")
	}

	provider, err := getProvider(domain)
	if err != nil {
		return err
	}

	records, err := provider.ListRecords(domain)
	if err != nil {
		return err
	}

	if len(records) == 0 {
		fmt.Printf("No records found for %s\n", domain)
		return nil
	}

	fmt.Printf("Provider: %s\n\n", provider.Name())

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tTYPE\tNAME\tCONTENT\tTTL")
	fmt.Fprintln(w, "──\t────\t────\t───────\t───")
	for _, r := range records {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Type, r.Name, r.Content, r.TTL)
	}
	return w.Flush()
}

func runDNSCreate(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	name, _ := cmd.Flags().GetString("name")
	recordType, _ := cmd.Flags().GetString("type")
	content, _ := cmd.Flags().GetString("content")
	ttl, _ := cmd.Flags().GetString("ttl")

	if domain == "" || recordType == "" || content == "" {
		return fmt.Errorf("--domain, --type, and --content are required")
	}

	provider, err := getProvider(domain)
	if err != nil {
		return err
	}

	id, err := provider.CreateRecord(domain, name, recordType, content, ttl)
	if err != nil {
		return err
	}

	fmt.Printf("Created %s record via %s (ID: %s)\n", recordType, provider.Name(), id)
	return nil
}

func runDNSDelete(cmd *cobra.Command, args []string) error {
	domain, _ := cmd.Flags().GetString("domain")
	id, _ := cmd.Flags().GetString("id")

	if domain == "" || id == "" {
		return fmt.Errorf("--domain and --id are required")
	}

	provider, err := getProvider(domain)
	if err != nil {
		return err
	}

	if err := provider.DeleteRecord(domain, id); err != nil {
		return err
	}

	fmt.Printf("Deleted record %s from %s via %s\n", id, domain, provider.Name())
	return nil
}
