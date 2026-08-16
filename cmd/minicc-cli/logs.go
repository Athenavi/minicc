package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View logs",
	Long:  `View MiniCC service logs (logs/{service}.stdout.log / .stderr.log).`,
	RunE:  runLogs,
}

var (
	logsService string
	logsTail    int
	logsFollow  bool
)

// 服务名白名单：仅字母数字与连字符/下划线，防止路径穿越
var serviceNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func init() {
	logsCmd.Flags().StringVarP(&logsService, "service", "s", "", "Service name (e.g. gateway, python-engine)")
	logsCmd.Flags().IntVarP(&logsTail, "tail", "t", 100, "Number of lines to show")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
}

func runLogs(cmd *cobra.Command, args []string) error {
	if logsService == "" {
		// 未指定服务：列出可用日志文件
		entries, err := os.ReadDir("logs")
		if err != nil {
			return fmt.Errorf("读取 logs/ 目录失败（可用 -s <service> 指定服务）: %w", err)
		}
		fmt.Println("Available log files in logs/:")
		for _, e := range entries {
			if !e.IsDir() {
				fmt.Printf("  %s\n", e.Name())
			}
		}
		return nil
	}

	if !serviceNameRe.MatchString(logsService) {
		return fmt.Errorf("非法的服务名: %s（仅允许字母数字、-、_）", logsService)
	}

	logPaths := []string{
		filepath.Join("logs", logsService+".stdout.log"),
		filepath.Join("logs", logsService+".stderr.log"),
	}

	// 至少一个日志文件存在
	existing := false
	for _, p := range logPaths {
		if _, err := os.Stat(p); err == nil {
			existing = true
			break
		}
	}
	if !existing {
		return fmt.Errorf("未找到服务 %s 的日志（logs/%s.*.log）", logsService, logsService)
	}

	for _, p := range logPaths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := tailFile(p, logsTail); err != nil {
			return err
		}
	}

	if logsFollow {
		return followLogs(logPaths)
	}
	return nil
}

// tailFile 从文件末尾反向读取 N 行（避免大日志全量读入内存）
func tailFile(path string, n int) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Printf("===== %s =====\n", path)
	lines, err := readLastLines(file, n)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}

// readLastLines 从文件尾向前读取最多 n 行（累积字节后统一切分，限内存、不跨块断裂）
func readLastLines(file *os.File, n int) ([]string, error) {
	if n <= 0 {
		n = 100
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}

	const maxBytes = 1 << 20 // 单次累积上限 1MB（防止超大日志打满内存）
	var data []byte
	pos := size
	newlines := 0
	atHead := false
	for pos > 0 && len(data) < maxBytes && newlines <= n {
		readSize := int64(4096)
		if pos < readSize {
			readSize = pos
			atHead = true
		}
		pos -= readSize
		chunk := make([]byte, readSize)
		if _, err := file.ReadAt(chunk, pos); err != nil && err != io.EOF {
			return nil, err
		}
		data = append(chunk, data...)
		newlines = bytes.Count(data, []byte{'\n'})
	}

	content := string(data)
	// 未读到文件头时，开头可能是行中段，丢弃第一个不完整行
	if !atHead {
		if idx := strings.Index(content, "\n"); idx >= 0 {
			content = content[idx+1:]
		} else {
			content = ""
		}
	}
	lines := strings.Split(content, "\n")
	// 文件以 \n 结尾 → 末尾空串元素去掉
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// followLogs 轮询读取文件新增内容（用 bufio.Reader 支持超长行）
func followLogs(paths []string) error {
	// 记录各文件当前位置（文件尾）
	offsets := map[string]int64{}
	for _, p := range paths {
		file, err := os.Open(p)
		if err != nil {
			continue
		}
		off, _ := file.Seek(0, 2)
		offsets[p] = off
		file.Close()
	}

	fmt.Println("Following logs... (Ctrl+C to stop)")
	for {
		for _, p := range paths {
			file, err := os.Open(p)
			if err != nil {
				continue
			}
			off := offsets[p]
			if _, err := file.Seek(off, 0); err == nil {
				reader := bufio.NewReader(file)
				for {
					line, err := reader.ReadString('\n')
					if line != "" {
						fmt.Print(line)
					}
					if err != nil {
						break // EOF 或错误
					}
				}
				newOff, _ := file.Seek(0, 1)
				offsets[p] = newOff
			}
			file.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
}
