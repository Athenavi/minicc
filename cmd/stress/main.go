// MiniCC 压测工具 — 并发压测核心 API
// 运行：go run ./cmd/stress -vu 50 -duration 30s
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var (
	totalReq      int64
	totalErr      int64
	totalDur      int64
	maxDur        int64
	minDur        int64 = 999999
	loginErr      int64
	entApiErr     int64
	cookie        []*http.Cookie
)

func main() {
	vu := flag.Int("vu", 50, "并发虚拟用户数")
	duration := flag.Duration("duration", 30*time.Second, "压测持续时间")
	baseURL := flag.String("url", "http://localhost:8080", "目标 URL")
	email := flag.String("email", "admin@minicc.local", "登录邮箱")
	pass := flag.String("pass", "Admin123456", "登录密码")
	flag.Parse()

	// 登录获取 cookie
	loginBody, _ := json.Marshal(map[string]string{"email": *email, "password": *pass})
	resp, err := http.Post(*baseURL+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		fmt.Printf("登录失败: %v\n", err)
		return
	}
	cookie = resp.Cookies()
	resp.Body.Close()
	fmt.Printf("登录成功，cookie: %d 个\n", len(cookie))

	// 压测的 API 端点
	apis := []string{
		"/v1/ent/audit?limit=20",
		"/v1/ent/roles",
		"/v1/ent/groups",
		"/v1/ent/quotas",
		"/v1/ent/privacy",
		"/v1/ent/model-policies",
		"/v1/ent/market/items",
		"/health",
		"/metrics",
	}

	fmt.Printf("压测开始: %d VU, 持续 %v\n", *vu, *duration)
	fmt.Println("═════════════════════════════════════════════════════════")

	stop := make(chan struct{})
	var wg sync.WaitGroup

	startTime := time.Now()
	for i := 0; i < *vu; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					for _, api := range apis {
						req, _ := http.NewRequest("GET", *baseURL+api, nil)
						for _, c := range cookie {
							req.AddCookie(c)
						}
						s := time.Now()
						resp, err := http.DefaultClient.Do(req)
						dur := time.Since(s).Milliseconds()
						atomic.AddInt64(&totalReq, 1)
						atomic.AddInt64(&totalDur, dur)
						if dur > atomic.LoadInt64(&maxDur) {
							atomic.StoreInt64(&maxDur, dur)
						}
						if dur < atomic.LoadInt64(&minDur) {
							atomic.StoreInt64(&minDur, dur)
						}
						if err != nil || resp.StatusCode >= 400 {
							atomic.AddInt64(&totalErr, 1)
							if api != "/health" && api != "/metrics" {
								atomic.AddInt64(&entApiErr, 1)
							}
						}
						if resp != nil {
							io.Copy(io.Discard, resp.Body)
							resp.Body.Close()
						}
					}
					time.Sleep(500 * time.Millisecond) // 思考时间
				}
			}
		}(i)
	}

	time.Sleep(*duration)
	close(stop)
	wg.Wait()
	elapsed := time.Since(startTime)

	// 报告
	reqCount := atomic.LoadInt64(&totalReq)
	errCount := atomic.LoadInt64(&totalErr)
	avgDur := float64(0)
	if reqCount > 0 {
		avgDur = float64(atomic.LoadInt64(&totalDur)) / float64(reqCount)
	}
	rps := float64(reqCount) / elapsed.Seconds()
	errRate := float64(0)
	if reqCount > 0 {
		errRate = float64(errCount) / float64(reqCount) * 100
	}

	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Println("  MiniCC 压测报告")
	fmt.Println("═════════════════════════════════════════════════════════")
	fmt.Printf("  VU 数:        %d\n", *vu)
	fmt.Printf("  持续时间:     %v\n", elapsed.Round(time.Second))
	fmt.Printf("  总请求数:     %d\n", reqCount)
	fmt.Printf("  RPS:          %.1f\n", rps)
	fmt.Printf("  错误数:       %d\n", errCount)
	fmt.Printf("  错误率:       %.2f%%\n", errRate)
	fmt.Printf("  平均响应:     %.0fms\n", avgDur)
	fmt.Printf("  最小响应:     %dms\n", atomic.LoadInt64(&minDur))
	fmt.Printf("  最大响应:     %dms\n", atomic.LoadInt64(&maxDur))
	fmt.Printf("  企业API错误:  %d\n", atomic.LoadInt64(&entApiErr))
	fmt.Println("═════════════════════════════════════════════════════════")
}
