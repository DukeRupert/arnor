package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/dukerupert/arnor/internal/domain"
	"github.com/spf13/cobra"
)

var domainCmd = &cobra.Command{
	Use:   "domain",
	Short: "Domain diagnostics and management",
}

var domainCheckCmd = &cobra.Command{
	Use:   "check [domain]",
	Short: "Verify DNS is pointing to the right place",
	Args:  cobra.ExactArgs(1),
	RunE:  runDomainCheck,
}

func init() {
	domainCheckCmd.Flags().Bool("all-records", false, "Show all A/CNAME records, not just those matching the domain")

	domainCmd.AddCommand(domainCheckCmd)
	rootCmd.AddCommand(domainCmd)
}

func runDomainCheck(cmd *cobra.Command, args []string) error {
	domainName := args[0]
	allRecords, _ := cmd.Flags().GetBool("all-records")

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	result, err := domain.Check(cfg, domainName)
	if err != nil {
		return err
	}

	// Header
	fmt.Printf("Domain:   %s\n", result.Domain)
	if result.ProviderName != "" {
		fmt.Printf("Provider: %s\n", result.ProviderName)
	}
	if result.Context != nil {
		fmt.Printf("Project:  %s (%s)\n", result.Context.ProjectName, result.Context.EnvName)
		fmt.Printf("Server:   %s\n", result.Context.ServerName)
	}

	// DNS Resolution
	fmt.Println()
	fmt.Println("DNS Resolution")
	res := result.Resolution
	if res.Error != "" {
		fmt.Printf("  Error: %s\n", res.Error)
	} else if len(res.ResolvedIPs) > 0 {
		fmt.Printf("  A record resolves to: %s\n", strings.Join(res.ResolvedIPs, ", "))
	}
	if res.ExpectedIP != "" {
		fmt.Printf("  Expected server IP:   %s\n", res.ExpectedIP)
	}
	fmt.Printf("  Status: %s\n", res.Status)

	// DNS Records
	records := result.Records
	if allRecords && result.RecordsError == "" && result.ProviderName != "" {
		// Re-fetch unfiltered — reuse the provider
		provider, err := getProvider(result.RootDomain)
		if err == nil {
			allRecs, err := provider.ListRecords(result.RootDomain)
			if err == nil {
				records = nil
				for _, r := range allRecs {
					if r.Type == "A" || r.Type == "CNAME" {
						records = append(records, r)
					}
				}
			}
		}
	}

	if result.RecordsError != "" {
		fmt.Printf("\nDNS Records\n  Error: %s\n", result.RecordsError)
	} else if len(records) > 0 {
		fmt.Println()
		fmt.Println("DNS Records")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "  ID\tTYPE\tNAME\tCONTENT\tTTL")
		fmt.Fprintln(w, "  ──\t────\t────\t───────\t───")
		for _, r := range records {
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", r.ID, r.Type, r.Name, r.Content, r.TTL)
		}
		w.Flush()
	}

	// Summary
	fmt.Println()
	switch result.Summary {
	case domain.StatusPass:
		fmt.Println("Summary: All checks passed")
	case domain.StatusFail:
		fmt.Println("Summary: DNS check failed")
	case domain.StatusWarn:
		if result.Context == nil {
			fmt.Println("Summary: Domain not found in config — skipped IP comparison")
		} else {
			fmt.Println("Summary: Check completed with warnings")
		}
	}

	return nil
}
