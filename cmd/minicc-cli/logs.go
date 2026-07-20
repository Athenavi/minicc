package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View logs",
	Long:  `View MiniCC service logs.`,
	RunE:  runLogs,
}

var (
	logsService string
	logsTail    int
	logsFollow  bool
)

func init() {
	logsCmd.Flags().StringVarP(&logsService, "service", "s", "", "Service name")
	logsCmd.Flags().IntVarP(&logsTail, "tail", "t", 100, "Number of lines to show")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
}

func runLogs(cmd *cobra.Command, args []string) error {
	fmt.Printf("Viewing logs...\n")
	if logsService != "" {
		fmt.Printf("Service: %s\n", logsService)
	}
	fmt.Printf("Tail: %d lines\n", logsTail)
	fmt.Printf("Follow: %v\n", logsFollow)

	// TODO: Implement log viewing
	return nil
}
