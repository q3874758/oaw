package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "1.0.0"
	commit  = "dev"
	date    = "2026-02-26"
)

var rootCmd = &cobra.Command{
	Use:   "oaw",
	Short: "OAW - OpenClaw Agent Work Proof",
	Long: `OAW (OpenClaw Agent Work Proof)

让 AI 的工作变得可量化、可验证、可追溯。`,
	Version: version,
}

func main() {
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
