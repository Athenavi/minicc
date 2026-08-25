// Chiron 鍘嬫祴宸ュ叿 鈥?骞跺彂鍘嬫祴鏍稿績 API
// 杩愯锛歡o run ./cmd/stress -vu 50 -duration 30s
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
	vu := flag.Int("vu", 50, "骞跺彂铏氭嫙鐢ㄦ埛鏁?)
	duration := flag.Duration("duration", 30*time.Second, "鍘嬫祴鎸佺画鏃堕棿")
	baseURL := flag.String("url", "http://localhost:8080", "鐩爣 URL")
	email := flag.String("email", "admin@chiron.local", "鐧诲綍閭")
	pass := flag.String("pass", "Admin123456", "鐧诲綍瀵嗙爜")
	flag.Parse()

	// 鐧诲綍鑾峰彇 cookie
	loginBody, _ := json.Marshal(map[string]string{"email": *email, "password": *pass})
	resp, err := http.Post(*baseURL+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		fmt.Printf("鐧诲綍澶辫触: %v\n", err)
		return
	}
	cookie = resp.Cookies()
	resp.Body.Close()
	fmt.Printf("鐧诲綍鎴愬姛锛宑ookie: %d 涓猏n", len(cookie))

	// 鍘嬫祴鐨?API 绔偣
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

	fmt.Printf("鍘嬫祴寮€濮? %d VU, 鎸佺画 %v\n", *vu, *duration)
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)

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
					time.Sleep(500 * time.Millisecond) // 鎬濊€冩椂闂?				}
			}
		}(i)
	}

	time.Sleep(*duration)
	close(stop)
	wg.Wait()
	elapsed := time.Since(startTime)

	// 鎶ュ憡
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

	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	fmt.Println("  Chiron 鍘嬫祴鎶ュ憡")
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
	fmt.Printf("  VU 鏁?        %d\n", *vu)
	fmt.Printf("  鎸佺画鏃堕棿:     %v\n", elapsed.Round(time.Second))
	fmt.Printf("  鎬昏姹傛暟:     %d\n", reqCount)
	fmt.Printf("  RPS:          %.1f\n", rps)
	fmt.Printf("  閿欒鏁?       %d\n", errCount)
	fmt.Printf("  閿欒鐜?       %.2f%%\n", errRate)
	fmt.Printf("  骞冲潎鍝嶅簲:     %.0fms\n", avgDur)
	fmt.Printf("  鏈€灏忓搷搴?     %dms\n", atomic.LoadInt64(&minDur))
	fmt.Printf("  鏈€澶у搷搴?     %dms\n", atomic.LoadInt64(&maxDur))
	fmt.Printf("  浼佷笟API閿欒:  %d\n", atomic.LoadInt64(&entApiErr))
	fmt.Println("鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺愨晲鈺?)
}
