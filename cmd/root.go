package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "arnor",
	Short: "Unified infrastructure management CLI",
	Long:  `Arnor manages web project infrastructure across Hetzner Cloud, Porkbun DNS, and Cloudflare DNS.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initEnv, initConfig)
}

func initEnv() {
	dotfilePath := filepath.Join(os.Getenv("HOME"), ".dotfiles", ".env")
	if _, err := os.Stat(dotfilePath); err == nil {
		_ = godotenv.Load(dotfilePath)
	} else {
		_ = godotenv.Load(".env")
	}
}

func initConfig() {
	cfgDir := filepath.Join(os.Getenv("HOME"), ".config", "arnor")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(cfgDir)
	_ = viper.ReadInConfig()
}
