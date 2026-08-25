package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Health check",
	Long:  `Check the health of Chiron services.`,
	RunE:  runHealth,
}

var healthAddr string
var healthServices []string

func init() {
	healthCmd.Flags().StringVarP(&healthAddr, "addr", "a", "http://localhost:8080", "Service address")
	healthCmd.Flags().StringSliceVarP(&healthServices, "services", "s", []string{}, "Specific services to check (comma-separated)")
}

func runHealth(cmd *cobra.Command, args []string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	// Check main service
	resp, err := client.Get(fmt.Sprintf("%s/health", healthAddr))
	if err != nil {
		fmt.Printf("鉂?Service is unhealthy: %v\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("鉁?Service is healthy\n")
	} else {
		fmt.Printf("鉂?Service returned status %d\n", resp.StatusCode)
	}

	// Check specific services if requested
	if len(healthServices) > 0 {
		for _, svc := range healthServices {
			checkServiceHealth(client, healthAddr, svc)
		}
	}

	return nil
}

func checkServiceHealth(client *http.Client, addr, service string) {
	url := fmt.Sprintf("%s/health/%s", addr, service)
	resp, err := client.Get(url)
	if err != nil {
		fmt.Printf("鉂?%s: %v\n", service, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("鉁?%s: healthy\n", service)
	} else {
		fmt.Printf("鉂?%s: status %d\n", service, resp.StatusCode)
	}
}

