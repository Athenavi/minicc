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

// newTestUploadHandler 鏋勯€犱互涓存椂鐩綍涓哄瓨鍌ㄦ牴鐨勫垎鐗囦笂浼?handler銆?
func newTestUploadHandler(t *testing.T) *UploadHandler {
	t.Helper()
	return NewUploadHandler(nil, t.TempDir())
}

// 鈹€鈹€ 璁よ瘉锛氭墍鏈夊垎鐗囦笂浼犵鐐瑰繀椤绘嫆缁濇湭璁よ瘉璇锋眰 鈹€鈹€
// 鎰忓浘锛氫笂浼犻摼璺秹鍙婄鐩樺啓鍏ヤ笌 DB 璁板綍锛屾湭璁よ瘉璇锋眰蹇呴』鍦ㄤ换浣?
// 鍓綔鐢紙寤虹洰褰?/ 鏌ュ簱锛変箣鍓嶈 401 鎷掔粷銆?

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

// 鈹€鈹€ Init锛氬弬鏁版牎楠岋紙蹇呴』鍏堜簬 DB 钀藉崟锛?鈹€鈹€

func TestUploadInit_InvalidJSON(t *testing.T) {
	h := newTestUploadHandler(t)
	req := requestWithClaims("POST", "/v1/uploads", "{bad", userClaims("user-1"))
	w := httptest.NewRecorder()
	h.Init(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

// TestUploadInit_RequiredFields 鎰忓浘锛氭枃浠跺悕涓庢鏁板ぇ灏忔槸鍒嗙墖璁″垝鐨勫墠鎻愶紝
// 缂哄け/闈炴鏁板繀椤绘嫆缁濓紙鍚﹀垯 chunk_count 璁＄畻鏃犳剰涔夛級銆?
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

// TestUploadInit_PurposeWhitelist 鎰忓浘锛歱urpose 鍙厑璁?media / kb_doc / generic
// 涓夌钀藉簱绛栫暐锛屾湭鐭ュ€煎繀椤绘嫆缁濓紙闃叉鏂囦欢琚惤鍒版湭瀹氫箟鐨勫鐞嗗垎鏀級銆?
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

// 鈹€鈹€ PutChunk锛氬垎鐗囧簭鍙疯竟鐣?鈹€鈹€

// TestUploadPutChunk_InvalidIndex 鎰忓浘锛氶潪娉曞垎鐗囧簭鍙凤紙闈炴暟瀛?/ 璐熸暟锛夊繀椤诲湪
// 褰掑睘鏌ヨ涓庣鐩樺啓鍏ヤ箣鍓嶆嫆缁濃€斺€旀柇瑷€瀛樺偍鏍逛笅鏈骇鐢熶换浣?uploads 鐩綍锛?
// 璇佹槑鏃犲壇浣滅敤銆?
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
		// 鏍￠獙澶辫触涓嶅緱浜х敓鍒嗙墖鐩綍
		if _, err := os.Stat(filepath.Join(h.storageRoot, "uploads")); !os.IsNotExist(err) {
			t.Fatalf("index %q: uploads dir should not be created on rejected request", idx)
		}
	}
}

// 鈹€鈹€ mergeChunks锛氬垎鐗囧悎骞剁函鏂囦欢绯荤粺閫昏緫 鈹€鈹€

// TestUploadMergeChunks_Order 鎰忓浘锛氬悎骞跺繀椤讳弗鏍兼寜 chunk_index 椤哄簭鎷兼帴锛?
// 涔卞簭浼氬鑷存枃浠跺唴瀹规崯鍧忥紙鏂偣缁紶鐨勬纭€ф牳蹇冿級銆?
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

// TestUploadMergeChunks_MissingPart 鎰忓浘锛氱己鍒嗙墖鏃跺悎骞跺繀椤诲け璐ワ紝
// 涓斾笉寰楁畫鐣欎复鏃跺悎骞舵枃浠讹紙閬垮厤纾佺洏鍨冨溇绱Н锛夈€?
func TestUploadMergeChunks_MissingPart(t *testing.T) {
	h := newTestUploadHandler(t)
	dir, err := h.chunkDir("up-2")
	if err != nil {
		t.Fatalf("chunkDir: %v", err)
	}
	// 鍙湁 chunk_0锛屽０绉板叡 2 鐗?
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

// TestUploadChunkDir_AutoCreate 鎰忓浘锛歝hunkDir 蹇呴』骞傜瓑鍦拌嚜鍔ㄥ垱寤哄垎鐗囩洰褰?
// 锛堟柇鐐圭画浼犲満鏅笅澶氭 PutChunk 閮戒緷璧栬琛屼负锛夈€?
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
