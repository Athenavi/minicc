package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database management",
	Long:  `Manage MiniCC database.`,
}

var dbStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show database status",
	RunE:  runDBStatus,
}

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	RunE:  runDBMigrate,
}

func init() {
	dbCmd.AddCommand(dbStatusCmd)
	dbCmd.AddCommand(dbMigrateCmd)
}

func runDBStatus(cmd *cobra.Command, args []string) error {
	fmt.Printf("Database Status\n")
	fmt.Printf("===============\n")
	fmt.Printf("Primary:   connected\n")
	fmt.Printf("Replicas:  0\n")

	return nil
}

func runDBMigrate(cmd *cobra.Command, args []string) error {
	fmt.Printf("Running database migrations...\n")
	// TODO: Implement migration
	return nil
}
