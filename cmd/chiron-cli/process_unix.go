//go:build !windows

package main

// POSIX 骞冲彴瀹炵幇锛氳繘绋嬪瓨娲绘鏌ヤ笌淇″彿鍙戦€?
import (
	"syscall"
)

const (
	SIGTERM = syscall.SIGTERM
	SIGKILL = syscall.SIGKILL
)

// processAlive 閫氳繃 signal 0 鍒ゆ柇杩涚▼瀛樻椿
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// sendSignal 鍙戦€佷俊鍙?func sendSignal(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
