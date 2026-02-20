package cmd

import (
	"fmt"
	"os"

	"github.com/dukerupert/arnor/internal/config"
	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	store   config.Store
)

var rootCmd = &cobra.Command{
	Use:   "arnor",
	Short: "Unified infrastructure management CLI",
	Long:  `Arnor manages web project infrastructure across Hetzner Cloud, Porkbun DNS, and Cloudflare DNS.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if store != nil {
			store.Close()
		}
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initStore)
}

func initStore() {
	s, err := config.NewSQLiteStore(config.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not open database: %v\n", err)
		return
	}
	store = s
}
