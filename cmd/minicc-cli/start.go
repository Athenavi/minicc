package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop MiniCC services",
	Long:  `Stop all running MiniCC services.`,
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	fmt.Println("Stopping MiniCC services...")
	// TODO: Implement graceful shutdown of services
	return nil
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start MiniCC services",
	Long:  `Start MiniCC services in monolith or microservices mode.`,
	RunE:  runStart,
}

var (
	startMode   string
	startConfig string
)

func init() {
	startCmd.Flags().StringVarP(&startMode, "mode", "m", "monolith", "Service mode: monolith or microservices")
	startCmd.Flags().StringVarP(&startConfig, "config", "c", "", "Configuration file path")
}

func runStart(cmd *cobra.Command, args []string) error {
	fmt.Printf("Starting MiniCC in %s mode...\n", startMode)

	// Determine executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Build command based on mode
	var cmdArgs []string
	if startConfig != "" {
		cmdArgs = append(cmdArgs, "--config", startConfig)
	}

	var serviceCmd *exec.Cmd
	switch startMode {
	case "monolith":
		// Start single gateway service
		gatewayPath := filepath.Join(filepath.Dir(exePath), "minicc")
		serviceCmd = exec.Command(gatewayPath, cmdArgs...)

	case "microservices":
		// Start multiple services
		return startMicroservices(exePath, cmdArgs)

	default:
		return fmt.Errorf("unknown mode: %s", startMode)
	}

	// Run service
	serviceCmd.Stdout = os.Stdout
	serviceCmd.Stderr = os.Stderr

	if err := serviceCmd.Run(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	return nil
}

func startMicroservices(exePath string, baseArgs []string) error {
	services := []struct {
		name string
		port int
	}{
		{"auth", 50051},
		{"chat", 50052},
		{"agent", 50053},
		{"admin", 50054},
		{"gateway", 8080},
	}

	dir := filepath.Dir(exePath)

	for _, svc := range services {
		svcPath := filepath.Join(dir, fmt.Sprintf("minicc-%s", svc.name))

		args := append(baseArgs, "--port", fmt.Sprintf("%d", svc.port))
		cmd := exec.Command(svcPath, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start %s service: %w", svc.name, err)
		}

		fmt.Printf("Started %s service on port %d (PID: %d)\n", svc.name, svc.port, cmd.Process.Pid)
	}

	return nil
}
