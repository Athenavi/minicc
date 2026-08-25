package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start Chiron services",
	Long:  `Start Chiron services in monolith or microservices mode (background).`,
	RunE:  runStart,
}

var (
	startMode   string
	startConfig string
)

func init() {
	startCmd.Flags().StringVarP(&startMode, "mode", "m", "monolith", "Service mode: monolith or microservices")
	startCmd.Flags().StringVarP(&startConfig, "config", "c", "", "Configuration file path (sets CONFIG_FILE env)")
}

func runStart(cmd *cobra.Command, args []string) error {
	fmt.Printf("Starting Chiron in %s mode...\n", startMode)

	// Determine executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// chiron 涓荤▼搴忓彧璇荤幆澧冨彉閲忥紙涓嶈В鏋愬懡浠よ flag锛夛紝閰嶇疆缁?env 浼犻€?	env := os.Environ()
	if startConfig != "" {
		env = append(env, "CONFIG_FILE="+startConfig)
	}

	switch startMode {
	case "monolith":
		// Start single gateway service in background
		gatewayPath := filepath.Join(filepath.Dir(exePath), "chiron")
		serviceCmd := exec.Command(gatewayPath)
		serviceCmd.Env = env
		port := defaultPort()
		if err := startBackground(serviceCmd, "gateway", port, "monolith"); err != nil {
			return fmt.Errorf("failed to start gateway service: %w", err)
		}

	case "microservices":
		// Start multiple services
		return startMicroservices(exePath, env)

	default:
		return fmt.Errorf("unknown mode: %s", startMode)
	}

	return nil
}

// defaultPort 璇诲彇鐜 PORT锛堜笌 chiron 涓荤▼搴忎竴鑷达級锛岄粯璁?8080
func defaultPort() int {
	p := os.Getenv("PORT")
	if p == "" {
		return 8080
	}
	var port int
	if _, err := fmt.Sscanf(p, "%d", &port); err != nil || port <= 0 {
		return 8080
	}
	return port
}

// startBackground 鍚庡彴鍚姩杩涚▼锛屾棩蹇楅噸瀹氬悜鍒?logs/{name}.*.log锛屽苟鍐欏叆 .pids/state.json
func startBackground(serviceCmd *exec.Cmd, name string, port int, mode string) error {
	// 闃查噸澶嶅惎鍔細鍚屽悕瀹炰緥宸插湪杩愯鍒欐嫆缁?	state, err := loadState()
	if err != nil {
		return err
	}
	if inst := state.FindInstance(name); inst != nil && inst.PID > 0 && processAlive(inst.PID) {
		return fmt.Errorf("%s 宸插湪杩愯锛圥ID %d锛夛紝璇峰厛 stop 鎴?instance remove", name, inst.PID)
	}

	logDir := filepath.Join("logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("failed to create logs dir: %w", err)
	}
	stdoutFile, err := os.OpenFile(filepath.Join(logDir, name+".stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	stderrFile, err := os.OpenFile(filepath.Join(logDir, name+".stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		stdoutFile.Close()
		return fmt.Errorf("failed to open stderr log file: %w", err)
	}
	serviceCmd.Stdout = stdoutFile
	serviceCmd.Stderr = stderrFile

	if err := serviceCmd.Start(); err != nil {
		stdoutFile.Close()
		stderrFile.Close()
		return err
	}
	// 鏃ュ織鏂囦欢鍙ユ焺鐢卞瓙杩涚▼鎸佹湁锛岀埗杩涚▼涓嶅啀浣跨敤
	_ = stdoutFile.Close()
	_ = stderrFile.Close()

	// 鍐欏叆鐘舵€侊紱澶辫触鍒欏洖婊氾紙缁堟鍒氬惎鍔ㄧ殑杩涚▼锛夛紝閬垮厤 stop 绠′笉鍒扮殑瀛ゅ効杩涚▼
	state.UpsertInstance(newInstance(name, serviceCmd.Process.Pid, port, mode, filepath.Join("logs", name+".stdout.log")))
	if err := saveState(state); err != nil {
		_ = stopProcess(serviceCmd.Process.Pid, name)
		return fmt.Errorf("鍐欏叆鐘舵€佹枃浠跺け璐ワ紝宸插洖婊氱粓姝㈣繘绋? %w", err)
	}

	fmt.Printf("Started %s service (PID: %d, port: %d, mode: %s)\n", name, serviceCmd.Process.Pid, port, mode)
	return nil
}

func startMicroservices(exePath string, baseEnv []string) error {
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
		svcPath := filepath.Join(dir, fmt.Sprintf("chiron-%s", svc.name))
		if _, err := os.Stat(svcPath); err != nil {
			return fmt.Errorf("鏈嶅姟浜岃繘鍒朵笉瀛樺湪: %s锛堝井鏈嶅姟妯″紡闇€鍏堟瀯寤?chiron-%s锛?, svcPath, svc.name)
		}

		serviceCmd := exec.Command(svcPath)
		svcEnv := append(append([]string{}, baseEnv...), fmt.Sprintf("PORT=%d", svc.port))
		serviceCmd.Env = svcEnv
		if err := startBackground(serviceCmd, svc.name, svc.port, "microservices"); err != nil {
			return fmt.Errorf("failed to start %s service: %w", svc.name, err)
		}
	}

	return nil
}

