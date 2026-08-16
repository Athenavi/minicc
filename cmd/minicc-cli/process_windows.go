//go:build windows

package main

// Windows 平台实现：进程存活检查与信号发送（tasklist/taskkill）

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// SIGTERM/SIGKILL 占位（Windows 用 taskkill，不实际发送信号）
const (
	SIGTERM = syscall.Signal(0)
	SIGKILL = syscall.Signal(0)
)

// processAlive 通过 tasklist 判断进程存活（PID 精确匹配，避免 123 误命中 1234）
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
	if err != nil {
		return false
	}
	want := strconv.Itoa(pid)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == want {
			return true
		}
	}
	return false
}

// sendSignal Windows 上无 POSIX 信号，返回错误（stopProcess 的 POSIX 分支不会在 Windows 走到）
func sendSignal(pid int, sig syscall.Signal) error {
	return fmt.Errorf("signals not supported on windows")
}
