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
	Long:  `View Chiron service logs (logs/{service}.stdout.log / .stderr.log).`,
	RunE:  runLogs,
}

var (
	logsService string
	logsTail    int
	logsFollow  bool
)

// 鏈嶅姟鍚嶇櫧鍚嶅崟锛氫粎瀛楁瘝鏁板瓧涓庤繛瀛楃/涓嬪垝绾匡紝闃叉璺緞绌胯秺
var serviceNameRe = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func init() {
	logsCmd.Flags().StringVarP(&logsService, "service", "s", "", "Service name (e.g. gateway, python-engine)")
	logsCmd.Flags().IntVarP(&logsTail, "tail", "t", 100, "Number of lines to show")
	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "Follow log output")
}

func runLogs(cmd *cobra.Command, args []string) error {
	if logsService == "" {
		// 鏈寚瀹氭湇鍔★細鍒楀嚭鍙敤鏃ュ織鏂囦欢
		entries, err := os.ReadDir("logs")
		if err != nil {
			return fmt.Errorf("璇诲彇 logs/ 鐩綍澶辫触锛堝彲鐢?-s <service> 鎸囧畾鏈嶅姟锛? %w", err)
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
		return fmt.Errorf("闈炴硶鐨勬湇鍔″悕: %s锛堜粎鍏佽瀛楁瘝鏁板瓧銆?銆乢锛?, logsService)
	}

	logPaths := []string{
		filepath.Join("logs", logsService+".stdout.log"),
		filepath.Join("logs", logsService+".stderr.log"),
	}

	// 鑷冲皯涓€涓棩蹇楁枃浠跺瓨鍦?	existing := false
	for _, p := range logPaths {
		if _, err := os.Stat(p); err == nil {
			existing = true
			break
		}
	}
	if !existing {
		return fmt.Errorf("鏈壘鍒版湇鍔?%s 鐨勬棩蹇楋紙logs/%s.*.log锛?, logsService, logsService)
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

// tailFile 浠庢枃浠舵湯灏惧弽鍚戣鍙?N 琛岋紙閬垮厤澶ф棩蹇楀叏閲忚鍏ュ唴瀛橈級
func tailFile(path string, n int) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Printf("===== %s =====\n", path)
	lines, err := readLastLines(file, n)
	if err != nil {
		return fmt.Errorf("璇诲彇 %s 澶辫触: %w", path, err)
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}

// readLastLines 浠庢枃浠跺熬鍚戝墠璇诲彇鏈€澶?n 琛岋紙绱Н瀛楄妭鍚庣粺涓€鍒囧垎锛岄檺鍐呭瓨銆佷笉璺ㄥ潡鏂锛?func readLastLines(file *os.File, n int) ([]string, error) {
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

	const maxBytes = 1 << 20 // 鍗曟绱Н涓婇檺 1MB锛堥槻姝㈣秴澶ф棩蹇楁墦婊″唴瀛橈級
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
	// 鏈鍒版枃浠跺ご鏃讹紝寮€澶村彲鑳芥槸琛屼腑娈碉紝涓㈠純绗竴涓笉瀹屾暣琛?	if !atHead {
		if idx := strings.Index(content, "\n"); idx >= 0 {
			content = content[idx+1:]
		} else {
			content = ""
		}
	}
	lines := strings.Split(content, "\n")
	// 鏂囦欢浠?\n 缁撳熬 鈫?鏈熬绌轰覆鍏冪礌鍘绘帀
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

// followLogs 杞璇诲彇鏂囦欢鏂板鍐呭锛堢敤 bufio.Reader 鏀寔瓒呴暱琛岋級
func followLogs(paths []string) error {
	// 璁板綍鍚勬枃浠跺綋鍓嶄綅缃紙鏂囦欢灏撅級
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
						break // EOF 鎴栭敊璇?					}
				}
				newOff, _ := file.Seek(0, 1)
				offsets[p] = newOff
			}
			file.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
}

