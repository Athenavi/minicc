package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/engine"
	"github.com/robfig/cron/v3"
)

// 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
// 瀹氭椂鑷姩鍖栵細cron_jobs 鎵ц鍣?
// - 璋冨害鍣ㄦ瘡 60s 閲嶈浇鍚敤鐨?cron_jobs 骞舵敞鍐屽埌 robfig/cron
// - 浠诲姟 task 瀛楁涓?JSON锛歿"type":"agent","agent_id":..,"prompt":..}
//                        鎴?{"type":"quick","user_input":..,"mode":"auto"}
// - 鎵ц缁撴灉鍐欏洖 last_run_at / last_status
// 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

type cronEntry struct {
	eid      cron.EntryID
	schedule string
}

type CronScheduler struct {
	mu      sync.Mutex
	cron    *cron.Cron
	entries map[string]cronEntry
	python  *engine.PythonClient
}

// cronSchedulerPython 渚?Webhook/鎵嬪姩瑙﹀彂澶嶇敤鎵ц鍣紙StartCronScheduler 鏃舵敞鍏ワ級銆?
var cronSchedulerPython *engine.PythonClient

type jobRow struct {
	ID       string
	Name     string
	Schedule string
	Task     string
	TenantID string
	UserID   string
}

// StartCronScheduler 鍚姩璋冨害鍣紙goroutine 鍐呰繍琛岋級銆?
func StartCronScheduler(ctx context.Context, python *engine.PythonClient) {
	s := &CronScheduler{
		cron:    cron.New(),
		entries: map[string]cronEntry{},
		python:  python,
	}
	cronSchedulerPython = python
	s.cron.Start()
	go s.syncLoop(ctx)
	slog.Info("cron scheduler started")
}

func (s *CronScheduler) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	s.sync()
	for {
		select {
		case <-ctx.Done():
			s.cron.Stop()
			return
		case <-ticker.C:
			s.sync()
		}
	}
}

func (s *CronScheduler) sync() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.ReadPool().Query(ctx,
		`SELECT id::text, name, schedule, task,
		        COALESCE(tenant_id::text, ''), COALESCE(user_id::text, '')
		 FROM cron_jobs WHERE enabled = true`)
	if err != nil {
		slog.Warn("cron sync failed", "error", err)
		return
	}
	defer rows.Close()

	jobs := map[string]jobRow{}
	for rows.Next() {
		var j jobRow
		if rows.Scan(&j.ID, &j.Name, &j.Schedule, &j.Task, &j.TenantID, &j.UserID) == nil {
			jobs[j.ID] = j
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// 绉婚櫎宸插仠鐢?鍒犻櫎/鏀?schedule 鐨?job
	for id, e := range s.entries {
		if j, ok := jobs[id]; !ok || j.Schedule != e.schedule {
			s.cron.Remove(e.eid)
			delete(s.entries, id)
		}
	}
	// 娉ㄥ唽鏂?job / 鏇存柊 schedule
	for id, j := range jobs {
		if e, ok := s.entries[id]; ok && e.schedule == j.Schedule {
			continue
		}
		j := j // 寰幆鍙橀噺鎷疯礉锛氶棴鍖呮崟鑾风ǔ瀹氬€硷紙Go 1.22 鍓嶈涔夛級
		eid, err := s.cron.AddFunc(j.Schedule, func() { s.execute(context.Background(), j) })
		if err != nil {
			slog.Warn("cron register failed", "job", j.Name, "schedule", j.Schedule, "error", err)
			continue
		}
		s.entries[id] = cronEntry{eid: eid, schedule: j.Schedule}
		slog.Info("cron job registered", "job", j.Name, "schedule", j.Schedule)
	}
}

func (s *CronScheduler) execute(ctx context.Context, j jobRow) {
	// Add a timeout to prevent hanging jobs from blocking the cron executor
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	start := time.Now()
	status := "success"
	errMsg := ""
	if s.python == nil {
		status = "failed"
		errMsg = "python engine unavailable"
	} else {
		switch {
		case strings.Contains(j.Task, `"type":"agent"`), strings.Contains(j.Task, `"type": "agent"`):
			var t struct {
				AgentID string `json:"agent_id"`
				Prompt  string `json:"prompt"`
			}
			_ = json.Unmarshal([]byte(j.Task), &t)
			if t.AgentID == "" {
				status, errMsg = "failed", "agent_id required"
			} else if err := s.runAgent(execCtx, j.TenantID, j.UserID, t.AgentID, t.Prompt); err != nil {
				status, errMsg = "failed", err.Error()
			}
		default: // quick / 閫氱敤缁熶竴浠诲姟
			var t struct {
				UserInput string `json:"user_input"`
				Mode      string `json:"mode"`
			}
			_ = json.Unmarshal([]byte(j.Task), &t)
			if t.UserInput == "" {
				status, errMsg = "failed", "user_input required"
			} else if err := s.runQuick(execCtx, j.TenantID, j.UserID, t.UserInput, t.Mode); err != nil {
				status, errMsg = "failed", err.Error()
			}
		}
	}
	_, _ = db.Pool.Exec(execCtx,
		`UPDATE cron_jobs SET last_run_at = NOW(), last_status = $1 WHERE id = $2`,
		status, j.ID)
	if status != "success" {
		slog.Warn("cron job failed", "job", j.Name, "error", errMsg, "duration", time.Since(start))
	}
}

func (s *CronScheduler) runAgent(ctx context.Context, tenantID, userID, agentID, prompt string) error {
	var name, systemPrompt, tools, llmConfig string
	var maxTurns, timeout int
	if err := db.ReadPool().QueryRow(ctx,
		`SELECT name, COALESCE(system_prompt,''), COALESCE(tools,'[]'::jsonb)::text,
		        COALESCE(llm_config,'{}'::jsonb)::text, max_turns, timeout_seconds
		 FROM agents WHERE id = $1 AND tenant_id = $2 AND user_id = $3`,
		agentID, tenantID, userID).Scan(&name, &systemPrompt, &tools, &llmConfig, &maxTurns, &timeout); err != nil {
		return fmt.Errorf("load agent: %w", err)
	}
	body := map[string]interface{}{
		"task":          prompt,
		"session_id":    fmt.Sprintf("cron_%s_%d", agentID, time.Now().Unix()),
		"agent_name":    name,
		"system_prompt": systemPrompt,
		"tools":         tools,
		"llm_config":    llmConfig,
		"max_turns":     maxTurns,
		"timeout_seconds": timeout,
	}
	var resp map[string]interface{}
	return s.python.PostJSON(ctx,
		"/v1/agents/dispatch?user_id="+userID+"&tenant_id="+tenantID, body, &resp)
}

func (s *CronScheduler) runQuick(ctx context.Context, tenantID, userID, input, mode string) error {
	if mode == "" {
		mode = "auto"
	}
	sessionID := fmt.Sprintf("uni_%d", time.Now().UnixMilli())
	body := map[string]interface{}{
		"user_input": input,
		"mode":       mode,
		"session_id": sessionID,
	}
	var resp map[string]interface{}
	return s.python.PostJSON(ctx,
		"/v1/chat/submit?user_id="+userID+"&tenant_id="+tenantID, body, &resp)
}

// 鈹€鈹€ Webhook 瑙﹀彂锛歅OST /v1/hooks/{jobID}?token=xxx 鈹€鈹€

func HandleCronWebhook(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobID")
	token := r.URL.Query().Get("token")
	if jobID == "" || token == "" {
		BadRequest(w, "jobID and token are required")
		return
	}
	var enabled bool
	var storedToken string
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT enabled, webhook_token FROM cron_jobs WHERE id = $1`, jobID).Scan(&enabled, &storedToken); err != nil {
		NotFound(w, "job not found")
		return
	}
	if !enabled || storedToken == "" || storedToken != token {
		Forbidden(w, "invalid token or job disabled")
		return
	}
	// 寮傛鎵ц锛坵ebhook 灏藉揩杩斿洖锛?
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("cron webhook async panic", "job", jobID, "panic", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		s := &CronScheduler{python: cronSchedulerPython}
		rows, err := db.ReadPool().Query(ctx,
			`SELECT id::text, name, schedule, task, COALESCE(tenant_id::text,''), COALESCE(user_id::text,'')
			 FROM cron_jobs WHERE id = $1`, jobID)
		if err == nil && rows.Next() {
			var j jobRow
			if rows.Scan(&j.ID, &j.Name, &j.Schedule, &j.Task, &j.TenantID, &j.UserID) == nil {
				s.execute(ctx, j)
			}
		}
		if rows != nil {
			rows.Close()
		}
	}()
	OK(w, map[string]interface{}{"status": "triggered"})
}

// HandleCronTrigger 绠＄悊绔墜鍔ㄨЕ鍙戯細POST /v1/admin/cron-jobs/{id}/trigger
func (h *AdminHandler) HandleCronTrigger(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var tenantID, userID string
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT COALESCE(tenant_id::text,''), COALESCE(user_id::text,'') FROM cron_jobs WHERE id = $1`, id).Scan(&tenantID, &userID); err != nil {
		NotFound(w, "job not found")
		return
	}
	var j jobRow
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT id::text, name, schedule, task, COALESCE(tenant_id::text,''), COALESCE(user_id::text,'')
		 FROM cron_jobs WHERE id = $1`, id).Scan(&j.ID, &j.Name, &j.Schedule, &j.Task, &j.TenantID, &j.UserID); err != nil {
		NotFound(w, "job not found")
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("cron trigger async panic", "job", id, "panic", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		s := &CronScheduler{python: cronSchedulerPython}
		s.execute(ctx, j)
	}()
	_ = tenantID
	_ = userID
	OK(w, map[string]interface{}{"status": "triggered"})
}
