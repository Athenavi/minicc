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

	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/id"
)

// 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€
// /admin 鍏ㄦ爤瀹炶锛氱鎴?/ 鍩熷悕 / 鏁版嵁搴?/ Redis / 妯″瀷 / 瀹氭椂浠诲姟
// 鎵€鏈夋暟鎹潎鏉ヨ嚜鐪熷疄瀛樺偍锛堟棤 mock锛夛紝璇诲啓缁?admin 鏉冮檺璺敱锛坅dminReadMW/adminWriteMW锛夈€?// 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// registerOpsRoutes 鎸傝浇杩愮淮绫荤鐞嗙鐐癸紙鍦?adminMux 鍐咃紝缁?StripPrefix /v1/admin锛夈€?func (h *AdminHandler) registerOpsRoutes(r *http.ServeMux) {
	// 绉熸埛绠＄悊
	r.HandleFunc("GET /tenants", h.ListTenants)
	r.HandleFunc("POST /tenants", h.CreateTenant)
	r.HandleFunc("PUT /tenants/{id}", h.UpdateTenant)
	r.HandleFunc("DELETE /tenants/{id}", h.DeleteTenant)
	r.HandleFunc("POST /tenants/{id}/suspend", h.SuspendTenant)
	r.HandleFunc("GET /tenants/{id}/usage", h.TenantUsage)

	// 鍩熷悕绠＄悊
	r.HandleFunc("GET /domains", h.ListDomains)
	r.HandleFunc("POST /domains", h.CreateDomain)
	r.HandleFunc("PUT /domains/{id}", h.UpdateDomain)
	r.HandleFunc("DELETE /domains/{id}", h.DeleteDomain)
	r.HandleFunc("POST /domains/{id}/verify", h.VerifyDomain)
	r.HandleFunc("POST /domains/{id}/renew-ssl", h.RenewDomainSSL)

	// 鏁版嵁搴撶鐞?	r.HandleFunc("GET /database/configs", h.DatabaseConfigs)
	r.HandleFunc("GET /database/backups", h.DatabaseBackups)
	r.HandleFunc("POST /database/backups", h.CreateDatabaseBackup)
	r.HandleFunc("POST /database/backups/{backupId}/restore", h.RestoreDatabaseBackup)
	r.HandleFunc("GET /database/status", h.DatabaseStatus)
	r.HandleFunc("POST /database/query", h.DatabaseQuery)
	r.HandleFunc("POST /database/optimize/{action}", h.DatabaseOptimize)

	// Redis 绠＄悊锛堝崟瀹炰緥鐪熷疄鎿嶄綔锛?	r.HandleFunc("GET /redis/slow-log", h.RedisSlowLog)
	r.HandleFunc("POST /redis/flush-all", h.RedisFlushAll)

	// 妯″瀷娉ㄥ唽琛?	r.HandleFunc("GET /models", h.ListModels)
	r.HandleFunc("POST /models", h.CreateModel)
	r.HandleFunc("PUT /models/{id}", h.UpdateModel)
	r.HandleFunc("DELETE /models/{id}", h.DeleteModel)

	// 瀹氭椂浠诲姟锛圖B 鎸佷箙鍖栵紱鎵ц鐢辫皟搴﹀櫒鎺ュ叆锛?	r.HandleFunc("GET /cron-jobs", h.ListCronJobs)
	r.HandleFunc("POST /cron-jobs", h.CreateCronJob)
	r.HandleFunc("PUT /cron-jobs/{id}", h.UpdateCronJob)
	r.HandleFunc("DELETE /cron-jobs/{id}", h.DeleteCronJob)
	r.HandleFunc("POST /cron-jobs/{id}/trigger", h.HandleCronTrigger)
}

// 鈹€鈹€ 绉熸埛绠＄悊 鈹€鈹€

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
	// S 瀹夊叏淇锛氬垪鍚嶇櫧鍚嶅崟锛岄槻姝?SQL 娉ㄥ叆
	tenantColumnMap := map[string]string{
		"name":   "name",
		"status": "status",
	}
	sets, args := []string{}, []interface{}{}
	idx := 1
	tenantFields := []struct {
		field string
		value string
	}{
		{"name", body.Name},
		{"status", body.Status},
	}
	for _, fv := range tenantFields {
		if fv.value != "" {
			col, ok := tenantColumnMap[fv.field]
			if !ok {
				continue
			}
			sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
			args = append(args, fv.value)
			idx++
		}
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

// 鈹€鈹€ 鍩熷悕绠＄悊 鈹€鈹€

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
		`SELECT id::text, domain, ssl_status, verified, created_at FROM domains ORDER BY created_at DESC LIMIT 100`)
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
	tenantID := GetTenantID(r)
	if tenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
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

// VerifyDomain 鐪熷疄 DNS 鏍￠獙锛氳В鏋?A/AAAA 璁板綍纭鍩熷悕鍙揪銆?func (h *AdminHandler) VerifyDomain(w http.ResponseWriter, r *http.Request) {
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
		OK(w, map[string]interface{}{"verified": false, "reason": "DNS 瑙ｆ瀽澶辫触鎴栨棤璁板綍", "addresses": []string{}})
		return
	}
	OK(w, map[string]interface{}{"verified": true, "addresses": addrs})
}

// RenewDomainSSL 瑕佹眰鍩熷悕宸查€氳繃楠岃瘉鍚庣疆 ssl_status=active锛圕A 绛惧彂鐢遍儴缃蹭晶鎺ュ叆锛夈€?func (h *AdminHandler) RenewDomainSSL(w http.ResponseWriter, r *http.Request) {
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
	OK(w, map[string]interface{}{"ssl_status": "active", "note": "璇佷功绛惧彂鐢遍儴缃蹭晶 CA 鎺ュ叆鐐瑰鐞?})
}

// 鈹€鈹€ 鏁版嵁搴撶鐞?鈹€鈹€

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
	// 澶嶇敤 CreateBackup 鐨?pg_dump 鑳藉姏锛岃惤鐩樺埌澶囦唤鐩綍
	if err := os.MkdirAll(backupDir(), 0o755); err != nil {
		InternalError(w, "backup dir create failed")
		return
	}
	name := fmt.Sprintf("chiron_backup_%s.sql", time.Now().Format("20060102_150405"))
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
	err := db.ReadPool().QueryRow(r.Context(), `SELECT version()`).Scan(&version)
	OK(w, map[string]interface{}{"version": version, "connected": err == nil && version != ""})
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
	// 琛ㄥ悕鐧藉悕鍗曟牎楠岋細浠呭厑璁?public schema 鐨勫父瑙勮〃
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

// 鈹€鈹€ Redis 绠＄悊锛堝崟瀹炰緥鐪熷疄鎿嶄綔锛夆攢鈹€

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

// 鈹€鈹€ 妯″瀷娉ㄥ唽琛?鈹€鈹€

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
	// S 瀹夊叏淇锛氬垪鍚嶇櫧鍚嶅崟锛岄槻姝?SQL 娉ㄥ叆
	modelColumnMap := map[string]string{
		"display_name":  "display_name",
		"enabled":       "enabled",
		"context_window": "context_window",
	}
	sets, args := []string{}, []interface{}{}
	idx := 1
	if body.DisplayName != nil {
		if col, ok := modelColumnMap["display_name"]; ok {
			sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
			args = append(args, *body.DisplayName)
			idx++
		}
	}
	if body.Enabled != nil {
		if col, ok := modelColumnMap["enabled"]; ok {
			sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
			args = append(args, *body.Enabled)
			idx++
		}
	}
	if body.ContextWindow != nil {
		if col, ok := modelColumnMap["context_window"]; ok {
			sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
			args = append(args, *body.ContextWindow)
			idx++
		}
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

// ListUserModels 鐢ㄦ埛渚у彲鐢ㄦā鍨嬶紙浠?enabled锛夛細GET /v1/models
func ListUserModels(w http.ResponseWriter, r *http.Request) {
	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT provider, name, display_name, context_window FROM llm_models
		 WHERE enabled = true ORDER BY provider, name`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list models failed")
		return
	}
	defer rows.Close()
	type model struct {
		Provider      string `json:"provider"`
		Name          string `json:"name"`
		DisplayName   string `json:"display_name"`
		ContextWindow int    `json:"context_window"`
	}
	out := []model{}
	for rows.Next() {
		var m model
		if rows.Scan(&m.Provider, &m.Name, &m.DisplayName, &m.ContextWindow) == nil {
			out = append(out, m)
		}
	}
	OK(w, map[string]interface{}{"models": out})
}

// 鈹€鈹€ 瀹氭椂浠诲姟 鈹€鈹€

type cronRow struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Schedule   string     `json:"schedule"`
	Task       string     `json:"task"`
	Enabled    bool       `json:"enabled"`
	LastRunAt  *time.Time `json:"last_run_at"`
	LastStatus string     `json:"last_status"`
	WebhookToken string   `json:"webhook_token"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (h *AdminHandler) ListCronJobs(w http.ResponseWriter, r *http.Request) {
	rows, err := db.ReadPool().Query(r.Context(),
		`SELECT id::text, name, schedule, task, enabled, last_run_at, last_status, webhook_token, created_at FROM cron_jobs ORDER BY created_at DESC`)
	if err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "list cron jobs failed")
		return
	}
	defer rows.Close()
	out := []cronRow{}
	for rows.Next() {
		var c cronRow
		if rows.Scan(&c.ID, &c.Name, &c.Schedule, &c.Task, &c.Enabled, &c.LastRunAt, &c.LastStatus, &c.WebhookToken, &c.CreatedAt) == nil {
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
	jobID, _ := id.UUID()
	token, _ := id.UUID()
	if _, err := db.Pool.Exec(r.Context(),
		`INSERT INTO cron_jobs (id, name, schedule, task, enabled, webhook_token) VALUES ($1, $2, $3, $4, $5, $6)`,
		jobID, body.Name, body.Schedule, body.Task, body.Enabled, token); err != nil {
		logAndRespond(w, err, http.StatusInternalServerError, "create cron job failed")
		return
	}
	OK(w, map[string]interface{}{"id": jobID, "name": body.Name, "schedule": body.Schedule, "webhook_token": token})
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
	// S 瀹夊叏淇锛氬垪鍚嶇櫧鍚嶅崟锛岄槻姝?SQL 娉ㄥ叆
	cronColumnMap := map[string]string{
		"name":     "name",
		"schedule": "schedule",
		"task":     "task",
		"enabled":  "enabled",
	}
	sets, args := []string{}, []interface{}{}
	idx := 1
	apply := func(col string, v *string) {
		if v != nil {
			if _, ok := cronColumnMap[col]; !ok {
				return
			}
			sets = append(sets, fmt.Sprintf("%s = $%d", col, idx))
			args = append(args, *v)
			idx++
		}
	}
	apply("name", body.Name)
	apply("schedule", body.Schedule)
	apply("task", body.Task)
	if body.Enabled != nil {
		if _, ok := cronColumnMap["enabled"]; ok {
			sets = append(sets, fmt.Sprintf("enabled = $%d", idx))
			args = append(args, *body.Enabled)
			idx++
		}
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

// 鈹€鈹€ 宸ュ叿 鈹€鈹€

// runPGDump 钀界洏 pg_dump锛堝鐢?extractDSN锛夈€?func runPGDump(ctx context.Context, target string) error {
	cmd := exec.CommandContext(ctx, "pg_dump", "--dbname="+extractDSN())
	out, err := cmd.Output()
	if err != nil {
		return err
	}
	return os.WriteFile(target, out, 0o644)
}
