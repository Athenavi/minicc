package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var instanceCmd = &cobra.Command{
	Use:   "instance",
	Short: "Manage instances",
	Long:  `Manage MiniCC service instances.`,
}

var instanceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List instances",
	RunE:  runInstanceList,
}

var instanceAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new instance",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceAdd,
}

var instanceRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an instance",
	Args:  cobra.ExactArgs(1),
	RunE:  runInstanceRemove,
}

func init() {
	instanceCmd.AddCommand(instanceListCmd)
	instanceCmd.AddCommand(instanceAddCmd)
	instanceCmd.AddCommand(instanceRemoveCmd)
}

func runInstanceList(cmd *cobra.Command, args []string) error {
	fmt.Printf("MiniCC Instances\n")
	fmt.Printf("================\n")
	fmt.Printf("NAME\t\tSTATUS\t\tPORT\t\tMODE\n")
	fmt.Printf("----\t\t------\t\t----\t\t----\n")
	fmt.Printf("gateway\t\trunning\t\t8080\t\tmonolith\n")

	return nil
}

func runInstanceAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	fmt.Printf("Adding instance: %s\n", name)
	// TODO: Implement instance addition
	return nil
}

func runInstanceRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	fmt.Printf("Removing instance: %s\n", name)
	// TODO: Implement instance removal
	return nil
}
