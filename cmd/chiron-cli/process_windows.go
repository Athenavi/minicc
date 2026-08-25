//go:build windows

package main

// Windows 骞冲彴瀹炵幇锛氳繘绋嬪瓨娲绘鏌ヤ笌淇″彿鍙戦€侊紙tasklist/taskkill锛?
import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// SIGTERM/SIGKILL 鍗犱綅锛圵indows 鐢?taskkill锛屼笉瀹為檯鍙戦€佷俊鍙凤級
const (
	SIGTERM = syscall.Signal(0)
	SIGKILL = syscall.Signal(0)
)

// processAlive 閫氳繃 tasklist 鍒ゆ柇杩涚▼瀛樻椿锛圥ID 绮剧‘鍖归厤锛岄伩鍏?123 璇懡涓?1234锛?func processAlive(pid int) bool {
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

// sendSignal Windows 涓婃棤 POSIX 淇″彿锛岃繑鍥為敊璇紙stopProcess 鐨?POSIX 鍒嗘敮涓嶄細鍦?Windows 璧板埌锛?func sendSignal(pid int, sig syscall.Signal) error {
	return fmt.Errorf("signals not supported on windows")
}
