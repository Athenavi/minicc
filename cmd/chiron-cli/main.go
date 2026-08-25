package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "chiron",
	Short: "chiron CLI - AI Agent Platform Management Tool",
	Long:  `Chiron CLI provides commands to manage Chiron services, instances, configuration, and health checks.`,
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(healthCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(instanceCmd)
	rootCmd.AddCommand(dbCmd)
	rootCmd.AddCommand(logsCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

