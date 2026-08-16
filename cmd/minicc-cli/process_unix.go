//go:build !windows

package main

// POSIX 平台实现：进程存活检查与信号发送

import (
	"syscall"
)

const (
	SIGTERM = syscall.SIGTERM
	SIGKILL = syscall.SIGKILL
)

// processAlive 通过 signal 0 判断进程存活
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// sendSignal 发送信号
func sendSignal(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
