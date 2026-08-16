package main

// stop 命令 — 按 .pids/state.json 优雅停止本地服务实例

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop MiniCC services",
	Long:  `Stop all running MiniCC services (from .pids/state.json).`,
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	return stopInstances()
}

// stopInstances 终止所有状态文件中的实例，等待退出后清理状态
func stopInstances() error {
	state, err := loadState()
	if err != nil {
		return err
	}
	if len(state.Instances) == 0 {
		fmt.Println("No running instances in state file")
		return nil
	}

	var failed []string
	for _, inst := range state.Instances {
		if err := stopProcess(inst.PID, inst.Name); err != nil {
			fmt.Printf("Failed to stop %s (PID %d): %v\n", inst.Name, inst.PID, err)
			failed = append(failed, inst.Name)
			continue
		}
		fmt.Printf("Stopped %s (PID %d)\n", inst.Name, inst.PID)
	}

	// 清理状态（只移除成功停止的；失败保留记录便于重试）
	if len(failed) == 0 {
		if err := clearState(); err != nil {
			return fmt.Errorf("failed to clear state file: %w", err)
		}
		fmt.Println("All instances stopped, state cleared")
	} else {
		fmt.Printf("Instances with stop failure kept in state: %s\n", strings.Join(failed, ", "))
	}
	return nil
}

// stopProcess 优雅终止进程：先 SIGTERM/taskkill，等待退出，超时强杀
func stopProcess(pid int, name string) error {
	if !processAlive(pid) {
		return nil // 已退出，无需处理
	}

	if runtime.GOOS == "windows" {
		// taskkill 不带 /F 尝试优雅关闭
		if err := exec.Command("taskkill", "/PID", strconv.Itoa(pid)).Run(); err != nil {
			fmt.Printf("warning: taskkill graceful failed for PID %d: %v\n", pid, err)
		}
	} else {
		if err := sendSignal(pid, SIGTERM); err != nil {
			fmt.Printf("warning: SIGTERM failed for PID %d: %v\n", pid, err)
		}
	}

	// 等待退出（最多 10s），超时强杀
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}

	if runtime.GOOS == "windows" {
		if err := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run(); err != nil {
			fmt.Printf("warning: taskkill force failed for PID %d: %v\n", pid, err)
		}
	} else {
		if err := sendSignal(pid, SIGKILL); err != nil {
			fmt.Printf("warning: SIGKILL failed for PID %d: %v\n", pid, err)
		}
	}
	time.Sleep(500 * time.Millisecond)
	if processAlive(pid) {
		return fmt.Errorf("process %d still alive after force kill", pid)
	}
	return nil
}

func clearState() error {
	state := &State{Instances: []InstanceState{}}
	return saveState(state)
}
