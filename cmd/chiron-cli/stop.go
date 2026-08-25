package main

// stop 鍛戒护 鈥?鎸?.pids/state.json 浼橀泤鍋滄鏈湴鏈嶅姟瀹炰緥

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
	Short: "Stop Chiron services",
	Long:  `Stop all running Chiron services (from .pids/state.json).`,
	RunE:  runStop,
}

func runStop(cmd *cobra.Command, args []string) error {
	return stopInstances()
}

// stopInstances 缁堟鎵€鏈夌姸鎬佹枃浠朵腑鐨勫疄渚嬶紝绛夊緟閫€鍑哄悗娓呯悊鐘舵€?func stopInstances() error {
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

	// 娓呯悊鐘舵€侊紙鍙Щ闄ゆ垚鍔熷仠姝㈢殑锛涘け璐ヤ繚鐣欒褰曚究浜庨噸璇曪級
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

// stopProcess 浼橀泤缁堟杩涚▼锛氬厛 SIGTERM/taskkill锛岀瓑寰呴€€鍑猴紝瓒呮椂寮烘潃
func stopProcess(pid int, name string) error {
	if !processAlive(pid) {
		return nil // 宸查€€鍑猴紝鏃犻渶澶勭悊
	}

	if runtime.GOOS == "windows" {
		// taskkill 涓嶅甫 /F 灏濊瘯浼橀泤鍏抽棴
		if err := exec.Command("taskkill", "/PID", strconv.Itoa(pid)).Run(); err != nil {
			fmt.Printf("warning: taskkill graceful failed for PID %d: %v\n", pid, err)
		}
	} else {
		if err := sendSignal(pid, SIGTERM); err != nil {
			fmt.Printf("warning: SIGTERM failed for PID %d: %v\n", pid, err)
		}
	}

	// 绛夊緟閫€鍑猴紙鏈€澶?10s锛夛紝瓒呮椂寮烘潃
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

