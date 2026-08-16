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
	Short: "Start MiniCC services",
	Long:  `Start MiniCC services in monolith or microservices mode (background).`,
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
	fmt.Printf("Starting MiniCC in %s mode...\n", startMode)

	// Determine executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// minicc 主程序只读环境变量（不解析命令行 flag），配置经 env 传递
	env := os.Environ()
	if startConfig != "" {
		env = append(env, "CONFIG_FILE="+startConfig)
	}

	switch startMode {
	case "monolith":
		// Start single gateway service in background
		gatewayPath := filepath.Join(filepath.Dir(exePath), "minicc")
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

// defaultPort 读取环境 PORT（与 minicc 主程序一致），默认 8080
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

// startBackground 后台启动进程，日志重定向到 logs/{name}.*.log，并写入 .pids/state.json
func startBackground(serviceCmd *exec.Cmd, name string, port int, mode string) error {
	// 防重复启动：同名实例已在运行则拒绝
	state, err := loadState()
	if err != nil {
		return err
	}
	if inst := state.FindInstance(name); inst != nil && inst.PID > 0 && processAlive(inst.PID) {
		return fmt.Errorf("%s 已在运行（PID %d），请先 stop 或 instance remove", name, inst.PID)
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
	// 日志文件句柄由子进程持有，父进程不再使用
	_ = stdoutFile.Close()
	_ = stderrFile.Close()

	// 写入状态；失败则回滚（终止刚启动的进程），避免 stop 管不到的孤儿进程
	state.UpsertInstance(newInstance(name, serviceCmd.Process.Pid, port, mode, filepath.Join("logs", name+".stdout.log")))
	if err := saveState(state); err != nil {
		_ = stopProcess(serviceCmd.Process.Pid, name)
		return fmt.Errorf("写入状态文件失败，已回滚终止进程: %w", err)
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
		svcPath := filepath.Join(dir, fmt.Sprintf("minicc-%s", svc.name))
		if _, err := os.Stat(svcPath); err != nil {
			return fmt.Errorf("服务二进制不存在: %s（微服务模式需先构建 minicc-%s）", svcPath, svc.name)
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
