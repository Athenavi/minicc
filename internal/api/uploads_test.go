package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestUploadHandler 构造以临时目录为存储根的分片上传 handler。
func newTestUploadHandler(t *testing.T) *UploadHandler {
	t.Helper()
	return NewUploadHandler(nil, t.TempDir())
}

// ── 认证：所有分片上传端点必须拒绝未认证请求 ──
// 意图：上传链路涉及磁盘写入与 DB 记录，未认证请求必须在任何
// 副作用（建目录 / 查库）之前被 401 拒绝。

func TestUploadInit_NoClaims(t *testing.T) {
	h := newTestUploadHandler(t)
	req := httptest.NewRequest("POST", "/v1/uploads", strings.NewReader(`{"name":"a.txt","size":10}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Init(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated init, got %d", w.Code)
	}
}

func TestUploadPutChunk_NoClaims(t *testing.T) {
	h := newTestUploadHandler(t)
	req := httptest.NewRequest("PUT", "/v1/uploads/up-1/chunks/0", strings.NewReader("data"))
	req.SetPathValue("id", "up-1")
	req.SetPathValue("index", "0")
	w := httptest.NewRecorder()
	h.PutChunk(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated put-chunk, got %d", w.Code)
	}
}

func TestUploadGetProgress_NoClaims(t *testing.T) {
	h := newTestUploadHandler(t)
	req := httptest.NewRequest("GET", "/v1/uploads/up-1", nil)
	req.SetPathValue("id", "up-1")
	w := httptest.NewRecorder()
	h.GetProgress(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated progress query, got %d", w.Code)
	}
}

func TestUploadComplete_NoClaims(t *testing.T) {
	h := newTestUploadHandler(t)
	req := httptest.NewRequest("POST", "/v1/uploads/up-1/complete", nil)
	req.SetPathValue("id", "up-1")
	w := httptest.NewRecorder()
	h.Complete(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated complete, got %d", w.Code)
	}
}

// ── Init：参数校验（必须先于 DB 落单） ──

func TestUploadInit_InvalidJSON(t *testing.T) {
	h := newTestUploadHandler(t)
	req := requestWithClaims("POST", "/v1/uploads", "{bad", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Init(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// TestUploadInit_RequiredFields 意图：文件名与正数大小是分片计划的前提，
// 缺失/非正数必须拒绝（否则 chunk_count 计算无意义）。
func TestUploadInit_RequiredFields(t *testing.T) {
	h := newTestUploadHandler(t)
	for _, body := range []string{
		`{}`,
		`{"name":"","size":10}`,
		`{"name":"  ","size":10}`,
		`{"name":"a.txt","size":0}`,
		`{"name":"a.txt","size":-1}`,
	} {
		req := requestWithClaims("POST", "/v1/uploads", body, userClaims("user-1"))
		w := httptest.NewRecorder()
		h.Init(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: expected 400, got %d", body, w.Code)
		}
	}
}

// TestUploadInit_PurposeWhitelist 意图：purpose 只允许 media / kb_doc / generic
// 三种落库策略，未知值必须拒绝（防止文件被落到未定义的处理分支）。
func TestUploadInit_PurposeWhitelist(t *testing.T) {
	h := newTestUploadHandler(t)
	req := requestWithClaims("POST", "/v1/uploads",
		`{"name":"a.txt","size":10,"purpose":"evil"}`, userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Init(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown purpose, got %d", w.Code)
	}
}

// ── PutChunk：分片序号边界 ──

// TestUploadPutChunk_InvalidIndex 意图：非法分片序号（非数字 / 负数）必须在
// 归属查询与磁盘写入之前拒绝——断言存储根下未产生任何 uploads 目录，
// 证明无副作用。
func TestUploadPutChunk_InvalidIndex(t *testing.T) {
	for _, idx := range []string{"abc", "-1", ""} {
		h := newTestUploadHandler(t)
		req := requestWithClaims("PUT", "/v1/uploads/up-1/chunks/"+idx, "data", userClaims("user-1"))
		req.SetPathValue("id", "up-1")
		req.SetPathValue("index", idx)
		w := httptest.NewRecorder()
		h.PutChunk(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("index %q: expected 400, got %d", idx, w.Code)
		}
		// 校验失败不得产生分片目录
		if _, err := os.Stat(filepath.Join(h.storageRoot, "uploads")); !os.IsNotExist(err) {
			t.Fatalf("index %q: uploads dir should not be created on rejected request", idx)
		}
	}
}

// ── mergeChunks：分片合并纯文件系统逻辑 ──

// TestUploadMergeChunks_Order 意图：合并必须严格按 chunk_index 顺序拼接，
// 乱序会导致文件内容损坏（断点续传的正确性核心）。
func TestUploadMergeChunks_Order(t *testing.T) {
	h := newTestUploadHandler(t)
	dir, err := h.chunkDir("up-1")
	if err != nil {
		t.Fatalf("chunkDir: %v", err)
	}
	parts := map[string]string{"chunk_0": "AAA", "chunk_1": "BBB", "chunk_2": "cc"}
	for name, content := range parts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	merged, err := h.mergeChunks(dir, 3)
	if err != nil {
		t.Fatalf("mergeChunks: %v", err)
	}
	defer func() {
		name := merged.Name()
		merged.Close()
		os.Remove(name)
	}()

	got, err := io.ReadAll(merged)
	if err != nil {
		t.Fatalf("read merged: %v", err)
	}
	if string(got) != "AAABBBcc" {
		t.Fatalf("merged content = %q, want AAABBBcc (strict chunk order)", string(got))
	}
}

// TestUploadMergeChunks_MissingPart 意图：缺分片时合并必须失败，
// 且不得残留临时合并文件（避免磁盘垃圾累积）。
func TestUploadMergeChunks_MissingPart(t *testing.T) {
	h := newTestUploadHandler(t)
	dir, err := h.chunkDir("up-2")
	if err != nil {
		t.Fatalf("chunkDir: %v", err)
	}
	// 只有 chunk_0，声称共 2 片
	if err := os.WriteFile(filepath.Join(dir, "chunk_0"), []byte("AAA"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := h.mergeChunks(dir, 2); err == nil {
		t.Fatal("mergeChunks should fail when a chunk is missing")
	}

	entries, err := os.ReadDir(h.storageRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "merged_") {
			t.Fatalf("temp merged file %q left behind after failed merge", e.Name())
		}
	}
}

// TestUploadChunkDir_AutoCreate 意图：chunkDir 必须幂等地自动创建分片目录
// （断点续传场景下多次 PutChunk 都依赖该行为）。
func TestUploadChunkDir_AutoCreate(t *testing.T) {
	h := newTestUploadHandler(t)
	dir1, err := h.chunkDir("up-3")
	if err != nil {
		t.Fatalf("first chunkDir: %v", err)
	}
	if fi, err := os.Stat(dir1); err != nil || !fi.IsDir() {
		t.Fatalf("chunk dir not created: %v", err)
	}
	dir2, err := h.chunkDir("up-3")
	if err != nil {
		t.Fatalf("second chunkDir (idempotent): %v", err)
	}
	if dir1 != dir2 {
		t.Fatalf("chunkDir not stable: %q vs %q", dir1, dir2)
	}
}
