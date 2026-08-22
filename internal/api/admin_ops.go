package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/id"
)

// ─────────────────────────────────────────────────────────────
// /admin 全栈实装：租户 / 域名 / 数据库 / Redis / 模型 / 定时任务
// 所有数据均来自真实存储（无 mock），读写经 admin 权限路由（adminReadMW/adminWriteMW）。
// ─────────────────────────────────────────────────────────────

// registerOpsRoutes 挂载运维类管理端点（在 adminMux 内，经 StripPrefix /v1/admin）。
func (h *AdminHandler) registerOpsRoutes(r *http.ServeMux) {
	// 租户管理
	r.HandleFunc("GET /tenants", h.ListTenants)
	r.HandleFunc("POST /tenants", h.CreateTenant)
	r.HandleFunc("PUT /tenants/{id}", h.UpdateTenant)
	r.HandleFunc("DELETE /tenants/{id}", h.DeleteTenant)
	r.HandleFunc("POST /tenants/{id}/suspend", h.SuspendTenant)
	r.HandleFunc("GET /tenants/{id}/usage", h.TenantUsage)

	// 域名管理
	r.HandleFunc("GET /domains", h.ListDomains)
	r.HandleFunc("POST /domains", h.CreateDomain)
	r.HandleFunc("PUT /domains/{id}", h.UpdateDomain)
	r.HandleFunc("DELETE /domains/{id}", h.DeleteDomain)
	r.HandleFunc("POST /domains/{id}/verify", h.VerifyDomain)
	r.HandleFunc("POST /domains/{id}/renew-ssl", h.RenewDomainSSL)

	// 数据库管理
	r.HandleFunc("GET /database/configs", h.DatabaseConfigs)
	r.HandleFunc("GET /database/backups", h.DatabaseBackups)
	r.HandleFunc("POST /database/backups", h.CreateDatabaseBackup)
	r.HandleFunc("POST /database/backups/{backupId}/restore", h.RestoreDatabaseBackup)
	r.HandleFunc("GET /database/status", h.DatabaseStatus)
	r.HandleFunc("POST /database/query", h.DatabaseQuery)
	r.HandleFunc("POST /database/optimize/{action}", h.DatabaseOptimize)

	// Redis 管理（单实例真实操作）
	r.HandleFunc("GET /redis/slow-log", h.RedisSlowLog)
	r.HandleFunc("POST /redis/flush-all", h.RedisFlushAll)

	// 模型注册表
	r.HandleFunc("GET /models", h.ListModels)
	r.HandleFunc("POST /models", h.CreateModel)
	r.HandleFunc("PUT /models/{id}", h.UpdateModel)
	r.HandleFunc("DELETE /models/{id}", h.DeleteModel)

	// 定时任务（DB 持久化；执行由调度器接入）
	r.HandleFunc("GET /cron-jobs", h.ListCronJobs)
	r.HandleFunc("POST /cron-jobs", h.CreateCronJob)
	r.HandleFunc("PUT /cron-jobs/{id}", h.UpdateCronJob)
	r.HandleFunc("DELETE /cron-jobs/{id}", h.DeleteCronJob)
}

// ── 租户管理 ──

type tenantRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *AdminHandler) ListTenants(w http.ResponseWriter, r *http.Request) {
	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT id::text, name, status, created_at FROM tenants ORDER BY created_at DESC`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list tenants failed")
		return
	}
	defer rows.Close()
	out := []tenantRow{}
	for rows.Next() {
		var t tenantRow
		if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.CreatedAt); err == nil {
			out = append(out, t)
		}
	}
	OK(w, map[string]interface{}{"tenants": out, "total": len(out)})
}

func (h *AdminHandler) CreateTenant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := DecodeJSON(w, r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		BadRequest(w, "name is required")
		return
	}
	tenantID, err := id.UUID()
	if err != nil {
		InternalError(w, "generate id failed")
		return
	}
	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO tenants (id, name, status) VALUES ($1, $2, 'active')`, tenantID, strings.TrimSpace(body.Name)); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create tenant failed")
		return
	}
	OK(w, map[string]interface{}{"id": tenantID, "name": body.Name, "status": "active"})
}

func (h *AdminHandler) UpdateTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	if body.Name == "" && body.Status == "" {
		BadRequest(w, "no fields to update")
		return
	}
	if body.Status != "" && body.Status != "active" && body.Status != "suspended" {
		BadRequest(w, "status must be active or suspended")
		return
	}
	sets, args := []string{}, []interface{}{}
	idx := 1
	if body.Name != "" {
		sets = append(sets, fmt.Sprintf("name = $%d", idx))
		args = append(args, body.Name)
		idx++
	}
	if body.Status != "" {
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, body.Status)
		idx++
	}
	args = append(args, id)
	if _, err := db.Pool.Exec(r.Context(),
		fmt.Sprintf("UPDATE tenants SET %s, updated_at = NOW() WHERE id = $%d", strings.Join(sets, ", "), idx), args...); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update tenant failed")
		return
	}
	OK(w, map[string]string{"status": "updated"})
}

func (h *AdminHandler) DeleteTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := db.Pool.Exec(r.Context(), `DELETE FROM tenants WHERE id = $1`, id); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete tenant failed")
		return
	}
	OK(w, map[string]string{"status": "deleted"})
}

func (h *AdminHandler) SuspendTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE tenants SET status = 'suspended', updated_at = NOW() WHERE id = $1`, id); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "suspend tenant failed")
		return
	}
	OK(w, map[string]string{"status": "suspended"})
}

func (h *AdminHandler) TenantUsage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	type usage struct {
		Users        int `json:"users"`
		Sessions     int `json:"sessions"`
		AgentSessions int `json:"agent_sessions"`
		KnowledgeBases int `json:"knowledge_bases"`
		Agents       int `json:"agents"`
		MediaAssets  int `json:"media_assets"`
	}
	var u usage
	ctx := r.Context()
	q := func(sql string, dst *int) {
		if *dst != 0 {
			return
		}
		_ = db.ReadPool().QueryRow(ctx, sql, id).Scan(dst)
	}
	q(`SELECT COUNT(*) FROM users WHERE tenant_id = $1`, &u.Users)
	q(`SELECT COUNT(*) FROM sessions WHERE tenant_id = $1`, &u.Sessions)
	q(`SELECT COUNT(*) FROM agent_sessions WHERE tenant_id = $1`, &u.AgentSessions)
	q(`SELECT COUNT(*) FROM knowledge_bases WHERE tenant_id = $1`, &u.KnowledgeBases)
	q(`SELECT COUNT(*) FROM agents WHERE tenant_id = $1`, &u.Agents)
	q(`SELECT COUNT(*) FROM media_assets WHERE tenant_id = $1`, &u.MediaAssets)
	OK(w, u)
}

// ── 域名管理 ──

type domainRow struct {
	ID        string    `json:"id"`
	Domain    string    `json:"domain"`
	SSLStatus string    `json:"ssl_status"`
	Verified  bool      `json:"verified"`
	CreatedAt time.Time `json:"created_at"`
}

var validDomainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$`)

func (h *AdminHandler) ListDomains(w http.ResponseWriter, r *http.Request) {
	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT id::text, domain, ssl_status, verified, created_at FROM domains ORDER BY created_at DESC`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list domains failed")
		return
	}
	defer rows.Close()
	out := []domainRow{}
	for rows.Next() {
		var d domainRow
		if err := rows.Scan(&d.ID, &d.Domain, &d.SSLStatus, &d.Verified, &d.CreatedAt); err == nil {
			out = append(out, d)
		}
	}
	OK(w, map[string]interface{}{"domains": out, "total": len(out)})
}

func (h *AdminHandler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Domain string `json:"domain"`
	}
	if err := DecodeJSON(w, r, &body); err != nil || !validDomainRe.MatchString(strings.ToLower(body.Domain)) {
		BadRequest(w, "valid domain is required")
		return
	}
	domain := strings.ToLower(strings.TrimSpace(body.Domain))
	claims := authClaims(r)
	tenantID := "00000000-0000-0000-0000-000000000001"
	if claims != nil && claims.TenantID != "" {
		tenantID = claims.TenantID
	}
	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO domains (tenant_id, domain) VALUES ($1, $2) ON CONFLICT (domain) DO NOTHING`,
		tenantID, domain); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create domain failed")
		return
	}
	OK(w, map[string]interface{}{"domain": domain, "ssl_status": "none", "verified": false})
}

func (h *AdminHandler) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Domain string `json:"domain"`
	}
	if err := DecodeJSON(w, r, &body); err != nil || !validDomainRe.MatchString(strings.ToLower(body.Domain)) {
		BadRequest(w, "valid domain is required")
		return
	}
	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE domains SET domain = $1, verified = false, ssl_status = 'none', updated_at = NOW() WHERE id = $2`,
		strings.ToLower(body.Domain), id); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update domain failed")
		return
	}
	OK(w, map[string]string{"status": "updated"})
}

func (h *AdminHandler) DeleteDomain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := db.Pool.Exec(r.Context(), `DELETE FROM domains WHERE id = $1`, id); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete domain failed")
		return
	}
	OK(w, map[string]string{"status": "deleted"})
}

// VerifyDomain 真实 DNS 校验：解析 A/AAAA 记录确认域名可达。
func (h *AdminHandler) VerifyDomain(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var domain string
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT domain FROM domains WHERE id = $1`, id).Scan(&domain); err != nil {
		NotFound(w, "domain not found")
		return
	}
	addrs, err := net.LookupHost(domain)
	verified := err == nil && len(addrs) > 0
	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE domains SET verified = $1, updated_at = NOW() WHERE id = $2`, verified, id); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update domain failed")
		return
	}
	if !verified {
		OK(w, map[string]interface{}{"verified": false, "reason": "DNS 解析失败或无记录", "addresses": []string{}})
		return
	}
	OK(w, map[string]interface{}{"verified": true, "addresses": addrs})
}

// RenewDomainSSL 要求域名已通过验证后置 ssl_status=active（CA 签发由部署侧接入）。
func (h *AdminHandler) RenewDomainSSL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var verified bool
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT verified FROM domains WHERE id = $1`, id).Scan(&verified); err != nil {
		NotFound(w, "domain not found")
		return
	}
	if !verified {
		BadRequest(w, "domain must be verified before SSL renewal")
		return
	}
	if _, err := db.Pool.Exec(r.Context(),
		`UPDATE domains SET ssl_status = 'active', updated_at = NOW() WHERE id = $1`, id); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "renew ssl failed")
		return
	}
	OK(w, map[string]interface{}{"ssl_status": "active", "note": "证书签发由部署侧 CA 接入点处理"})
}

// ── 数据库管理 ──

func (h *AdminHandler) DatabaseConfigs(w http.ResponseWriter, r *http.Request) {
	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT name, setting FROM pg_settings
		 WHERE name IN ('max_connections','shared_buffers','work_mem','maintenance_work_mem','effective_cache_size',
		   'wal_level','max_worker_processes','max_parallel_workers','statement_timeout','idle_in_transaction_session_timeout')
		 ORDER BY name`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "read pg settings failed")
		return
	}
	defer rows.Close()
	configs := map[string]string{}
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			configs[k] = v
		}
	}
	OK(w, map[string]interface{}{"configs": configs})
}

func backupDir() string {
	d := os.Getenv("BACKUP_DIR")
	if d == "" {
		d = filepath.Join(os.Getenv("STORAGE_ROOT"), "backups")
	}
	if d == "" {
		d = "./data/backups"
	}
	return d
}

func (h *AdminHandler) DatabaseBackups(w http.ResponseWriter, r *http.Request) {
	dir := backupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		OK(w, map[string]interface{}{"backups": []interface{}{}, "total": 0})
		return
	}
	type backupInfo struct {
		Name string    `json:"name"`
		Size int64     `json:"size"`
		Time time.Time `json:"time"`
	}
	out := []backupInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		info, _ := e.Info()
		out = append(out, backupInfo{Name: e.Name(), Size: info.Size(), Time: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	OK(w, map[string]interface{}{"backups": out, "total": len(out)})
}

func (h *AdminHandler) CreateDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	// 复用 CreateBackup 的 pg_dump 能力，落盘到备份目录
	if err := os.MkdirAll(backupDir(), 0o755); err != nil {
		InternalError(w, "backup dir create failed")
		return
	}
	name := fmt.Sprintf("minicc_backup_%s.sql", time.Now().Format("20060102_150405"))
	target := filepath.Join(backupDir(), name)
	if err := runPGDump(r.Context(), target); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "backup failed")
		return
	}
	OK(w, map[string]interface{}{"name": name, "status": "completed"})
}

func (h *AdminHandler) RestoreDatabaseBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("backupId")
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		BadRequest(w, "invalid backup name")
		return
	}
	target := filepath.Join(backupDir(), name)
	if _, err := os.Stat(target); err != nil {
		NotFound(w, "backup not found")
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		InternalError(w, "read backup failed")
		return
	}
	if _, err := db.Pool.Exec(r.Context(), string(data)); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "restore failed")
		return
	}
	OK(w, map[string]interface{}{"status": "restored", "backup": name})
}

func (h *AdminHandler) DatabaseStatus(w http.ResponseWriter, r *http.Request) {
	var version string
	_ = db.ReadPool().QueryRow(r.Context(), `SELECT version()`).Scan(&version)
	OK(w, map[string]interface{}{"version": version, "connected": version != ""})
}

var selectOnlyRe = regexp.MustCompile(`(?i)^\s*select\b`)

func (h *AdminHandler) DatabaseQuery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string `json:"query"`
	}
	if err := DecodeJSON(w, r, &body); err != nil || strings.TrimSpace(body.Query) == "" {
		BadRequest(w, "query is required")
		return
	}
	if !selectOnlyRe.MatchString(body.Query) {
		BadRequest(w, "only SELECT queries are allowed")
		return
	}
	if strings.Contains(strings.ToLower(body.Query), "pg_read_file") || strings.Contains(strings.ToLower(body.Query), "lo_") {
		BadRequest(w, "query not allowed")
		return
	}
	rows, err := db.ReadPool().Query(r.Context(), body.Query)
	if err != nil {
		BadRequest(w, "query failed: "+err.Error())
		return
	}
	defer rows.Close()
	cols := make([]string, 0)
	for _, fd := range rows.FieldDescriptions() {
		cols = append(cols, string(fd.Name))
	}
	results := []map[string]interface{}{}
	count := 0
	for rows.Next() && count < 200 {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if rows.Scan(ptrs...) != nil {
			break
		}
		row := map[string]interface{}{}
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				row[c] = string(v)
			default:
				row[c] = v
			}
		}
		results = append(results, row)
		count++
	}
	OK(w, map[string]interface{}{"columns": cols, "rows": results, "count": count, "truncated": count >= 200})
}

func (h *AdminHandler) DatabaseOptimize(w http.ResponseWriter, r *http.Request) {
	action := strings.ToLower(r.PathValue("action"))
	if action != "analyze" && action != "vacuum" {
		BadRequest(w, "action must be analyze or vacuum")
		return
	}
	var body struct {
		Table string `json:"table"`
	}
	if err := DecodeJSON(w, r, &body); err != nil || body.Table == "" {
		BadRequest(w, "table is required")
		return
	}
	// 表名白名单校验：仅允许 public schema 的常规表
	var exists bool
	if err := db.ReadPool().QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1 AND table_type='BASE TABLE')`,
		body.Table).Scan(&exists); err != nil || !exists {
		BadRequest(w, "unknown table: "+body.Table)
		return
	}
	stmt := "ANALYZE " + quoteIdent(body.Table)
	if action == "vacuum" {
		stmt = "VACUUM " + quoteIdent(body.Table)
	}
	if _, err := db.Pool.Exec(r.Context(), stmt); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, action+" failed")
		return
	}
	OK(w, map[string]interface{}{"status": "completed", "action": action, "table": body.Table})
}

func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// ── Redis 管理（单实例真实操作）──

func (h *AdminHandler) redisDo(ctx context.Context, args ...interface{}) (interface{}, error) {
	if db.Redis == nil {
		return nil, fmt.Errorf("redis unavailable")
	}
	cmd := db.Redis.Do(ctx, args...)
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	return cmd.Result()
}

func (h *AdminHandler) RedisSlowLog(w http.ResponseWriter, r *http.Request) {
	res, err := h.redisDo(r.Context(), "SLOWLOG", "GET", 20)
	if err != nil {
		OK(w, map[string]interface{}{"slow_log": []interface{}{}, "error": err.Error()})
		return
	}
	OK(w, map[string]interface{}{"slow_log": res})
}

func (h *AdminHandler) RedisFlushAll(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm bool `json:"confirm"`
	}
	_ = DecodeJSON(w, r, &body)
	if !body.Confirm {
		BadRequest(w, "confirm=true is required to flush redis")
		return
	}
	if _, err := h.redisDo(r.Context(), "FLUSHDB"); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "flush failed")
		return
	}
	OK(w, map[string]interface{}{"status": "flushed"})
}

// ── 模型注册表 ──

type modelRow struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	Name          string    `json:"name"`
	DisplayName   string    `json:"display_name"`
	Enabled       bool      `json:"enabled"`
	ContextWindow int       `json:"context_window"`
	CreatedAt     time.Time `json:"created_at"`
}

func (h *AdminHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT id::text, provider, name, display_name, enabled, context_window, created_at FROM llm_models ORDER BY provider, name`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list models failed")
		return
	}
	defer rows.Close()
	out := []modelRow{}
	for rows.Next() {
		var m modelRow
		if rows.Scan(&m.ID, &m.Provider, &m.Name, &m.DisplayName, &m.Enabled, &m.ContextWindow, &m.CreatedAt) == nil {
			out = append(out, m)
		}
	}
	OK(w, map[string]interface{}{"models": out, "total": len(out)})
}

func (h *AdminHandler) CreateModel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider      string `json:"provider"`
		Name          string `json:"name"`
		DisplayName   string `json:"display_name"`
		Enabled       bool   `json:"enabled"`
		ContextWindow int    `json:"context_window"`
	}
	if err := DecodeJSON(w, r, &body); err != nil || body.Provider == "" || body.Name == "" {
		BadRequest(w, "provider and name are required")
		return
	}
	if body.ContextWindow <= 0 {
		body.ContextWindow = 8192
	}
	id, _ := id.UUID()
	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO llm_models (id, provider, name, display_name, enabled, context_window)
		 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (provider, name) DO UPDATE
		 SET display_name = EXCLUDED.display_name, enabled = EXCLUDED.enabled, context_window = EXCLUDED.context_window, updated_at = NOW()`,
		id, body.Provider, body.Name, body.DisplayName, body.Enabled, body.ContextWindow); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create model failed")
		return
	}
	OK(w, map[string]interface{}{"id": id, "provider": body.Provider, "name": body.Name})
}

func (h *AdminHandler) UpdateModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		DisplayName   *string `json:"display_name"`
		Enabled       *bool   `json:"enabled"`
		ContextWindow *int    `json:"context_window"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	sets, args := []string{}, []interface{}{}
	idx := 1
	if body.DisplayName != nil {
		sets = append(sets, fmt.Sprintf("display_name = $%d", idx))
		args = append(args, *body.DisplayName)
		idx++
	}
	if body.Enabled != nil {
		sets = append(sets, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *body.Enabled)
		idx++
	}
	if body.ContextWindow != nil {
		sets = append(sets, fmt.Sprintf("context_window = $%d", idx))
		args = append(args, *body.ContextWindow)
		idx++
	}
	if len(sets) == 0 {
		BadRequest(w, "nothing to update")
		return
	}
	args = append(args, id)
	if _, err := db.Pool.Exec(r.Context(),
		fmt.Sprintf("UPDATE llm_models SET %s, updated_at = NOW() WHERE id = $%d", strings.Join(sets, ", "), idx), args...); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update model failed")
		return
	}
	OK(w, map[string]string{"status": "updated"})
}

func (h *AdminHandler) DeleteModel(w http.ResponseWriter, r *http.Request) {
	if _, err := db.Pool.Exec(r.Context(), `DELETE FROM llm_models WHERE id = $1`, r.PathValue("id")); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete model failed")
		return
	}
	OK(w, map[string]string{"status": "deleted"})
}

// ── 定时任务 ──

type cronRow struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Schedule   string     `json:"schedule"`
	Task       string     `json:"task"`
	Enabled    bool       `json:"enabled"`
	LastRunAt  *time.Time `json:"last_run_at"`
	LastStatus string     `json:"last_status"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (h *AdminHandler) ListCronJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT id::text, name, schedule, task, enabled, last_run_at, last_status, created_at FROM cron_jobs ORDER BY created_at DESC`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list cron jobs failed")
		return
	}
	defer rows.Close()
	out := []cronRow{}
	for rows.Next() {
		var c cronRow
		if rows.Scan(&c.ID, &c.Name, &c.Schedule, &c.Task, &c.Enabled, &c.LastRunAt, &c.LastStatus, &c.CreatedAt) == nil {
			out = append(out, c)
		}
	}
	OK(w, map[string]interface{}{"jobs": out, "total": len(out)})
}

func (h *AdminHandler) CreateCronJob(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		Schedule string `json:"schedule"`
		Task     string `json:"task"`
		Enabled  bool   `json:"enabled"`
	}
	if err := DecodeJSON(w, r, &body); err != nil || body.Name == "" || body.Schedule == "" || body.Task == "" {
		BadRequest(w, "name, schedule and task are required")
		return
	}
	id, _ := id.UUID()
	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO cron_jobs (id, name, schedule, task, enabled) VALUES ($1, $2, $3, $4, $5)`,
		id, body.Name, body.Schedule, body.Task, body.Enabled); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create cron job failed")
		return
	}
	OK(w, map[string]interface{}{"id": id, "name": body.Name, "schedule": body.Schedule})
}

func (h *AdminHandler) UpdateCronJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Name     *string `json:"name"`
		Schedule *string `json:"schedule"`
		Task     *string `json:"task"`
		Enabled  *bool   `json:"enabled"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, ErrInvalidReq)
		return
	}
	sets, args := []string{}, []interface{}{}
	idx := 1
	apply := func(col string, v *string) {
		if v != nil {
			sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
			args = append(args, *v)
			idx++
		}
	}
	apply("name", body.Name)
	apply("schedule", body.Schedule)
	apply("task", body.Task)
	if body.Enabled != nil {
		sets = append(sets, fmt.Sprintf("enabled = $%d", idx))
		args = append(args, *body.Enabled)
		idx++
	}
	if len(sets) == 0 {
		BadRequest(w, "nothing to update")
		return
	}
	args = append(args, id)
	if _, err := db.Pool.Exec(r.Context(),
		fmt.Sprintf("UPDATE cron_jobs SET %s, updated_at = NOW() WHERE id = $%d", strings.Join(sets, ", "), idx), args...); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "update cron job failed")
		return
	}
	OK(w, map[string]string{"status": "updated"})
}

func (h *AdminHandler) DeleteCronJob(w http.ResponseWriter, r *http.Request) {
	if _, err := db.Pool.Exec(r.Context(), `DELETE FROM cron_jobs WHERE id = $1`, r.PathValue("id")); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "delete cron job failed")
		return
	}
	OK(w, map[string]string{"status": "deleted"})
}

// ── 工具 ──

func authClaims(r *http.Request) *auth.Claims {
	return auth.GetClaims(r.Context())
}

// runPGDump 落盘 pg_dump（复用 extractDSN）。
func runPGDump(ctx context.Context, target string) error {
	cmd := exec.CommandContext(ctx, "pg_dump", "--dbname="+extractDSN())
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	return os.WriteFile(target, out, 0o644)
}
