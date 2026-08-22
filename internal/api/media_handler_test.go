package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/minicc/internal/auth"
	"github.com/athenavi/minicc/internal/db"
	"github.com/athenavi/minicc/internal/storage"
)

func newTestMediaHandler(t *testing.T) (*MediaHandler, *auth.Authenticator) {
	t.Helper()
	store, err := storage.NewStore("local", t.TempDir(), "", "", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	auth := auth.NewAuthenticator("test-secret-change-in-production-must-be-16-chars", 3600*time.Second)
	return NewMediaHandler(store, auth), auth
}

func TestMediaHandler_New(t *testing.T) {
	h, _ := newTestMediaHandler(t)
	if h == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestMediaHandler_Create_NoAuth(t *testing.T) {
	h, _ := newTestMediaHandler(t)
	body := `{"name":"test.txt","content":"hello"}`
	req := httptest.NewRequest("POST", "/v1/media/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.Create(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated create, got %d", w.Code)
	}
}

func TestMediaHandler_Upload_NoAuth(t *testing.T) {
	h, _ := newTestMediaHandler(t)
	req := httptest.NewRequest("POST", "/v1/media/upload", nil)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	w := httptest.NewRecorder()
	h.Upload(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated upload, got %d", w.Code)
	}
}

func TestMediaHandler_Delete_NoAuth(t *testing.T) {
	h, _ := newTestMediaHandler(t)
	req := httptest.NewRequest("DELETE", "/v1/media/123", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated delete, got %d", w.Code)
	}
}

// ── P1-6 MIME 校验逻辑单元测试 ──
// 覆盖 truncateMIME 与 isExecutableMIME 的关键场景，确保 magic bytes 检测与
// 可执行文件拒绝逻辑正确（前端 P1-2 上传依赖后端这道安全防线）。
func TestTruncateMIME(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"normal", "image/png", "image/png"},
		{"empty", "", ""},
		{"over_64", strings.Repeat("x", 70), strings.Repeat("x", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateMIME(tt.in)
			if got != tt.want {
				t.Errorf("truncateMIME(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsExecutableMIME(t *testing.T) {
	// 安全类型：不应被拒绝（文件名也是安全的）
	safe := []struct{ mime, name string }{
		{"image/png", "photo.png"},
		{"image/jpeg", "photo.jpg"},
		{"image/gif", "anim.gif"},
		{"image/webp", "photo.webp"},
		{"application/pdf", "doc.pdf"},
		{"text/plain", "notes.txt"},
		{"text/markdown", "readme.md"},
		{"application/json", "data.json"},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "doc.docx"},
	}
	for _, tt := range safe {
		if isExecutableMIME(tt.mime, tt.name) {
			t.Errorf("safe MIME %q (%s) should not be executable", tt.mime, tt.name)
		}
	}
	// 危险 MIME：应被拒绝（文件名无关）
	dangerousMIMEs := []string{
		"application/x-msdownload",
		"application/x-msi",
		"application/x-sh",
		"application/x-bat",
		"application/x-elf",
		"application/x-executable",
		"application/x-mach-o-executable",
		"application/x-mach-binary",
		"application/x-msdownload; charset=binary",
	}
	for _, m := range dangerousMIMEs {
		if !isExecutableMIME(m, "upload.bin") {
			t.Errorf("dangerous MIME %q should be detected as executable", m)
		}
	}
	// 大小写不敏感
	if !isExecutableMIME("APPLICATION/X-MSDOWNLOAD", "x.bin") {
		t.Error("uppercase MIME should be detected as executable")
	}
	// octet-stream + 危险扩展名：应被拒绝（PE/ELF magic bytes 兜底）
	dangerousExts := []string{
		"malware.exe", "lib.dll", "installer.msi", "script.sh",
		"batch.bat", "command.cmd", "old.com", "screen.scr",
		"lib.so", "lib.dylib", "app.app", "j.jar", "c.class",
		"p.py", "r.rb", "perl.pl",
	}
	for _, name := range dangerousExts {
		if !isExecutableMIME("application/octet-stream", name) {
			t.Errorf("octet-stream + ext %q should be detected as executable", name)
		}
	}
	// octet-stream + 安全扩展名：不应被拒绝（如 .bin 数据文件）
	if isExecutableMIME("application/octet-stream", "data.bin") {
		t.Error("octet-stream + .bin should not be rejected")
	}
	// text/plain + 危险扩展名：应被拒绝（shell 脚本检测为 text/plain）
	if !isExecutableMIME("text/plain", "script.sh") {
		t.Error("text/plain + .sh should be rejected")
	}
	if !isExecutableMIME("text/plain", "batch.bat") {
		t.Error("text/plain + .bat should be rejected")
	}
	if !isExecutableMIME("text/html", "page.html") {
		t.Error("text/html + .html should be rejected")
	}
	// text/plain + 安全扩展名：不应被拒绝
	if isExecutableMIME("text/plain", "notes.txt") {
		t.Error("text/plain + .txt should not be rejected")
	}
	if isExecutableMIME("text/plain", "readme.md") {
		t.Error("text/plain + .md should not be rejected")
	}
}

func TestMediaHandler_PresignUpload_LocalBackend(t *testing.T) {
	h, authenticator := newTestMediaHandler(t)
	token, err := authenticator.GenerateToken("user-1", "test@test.com", "user", db.DefaultTenantID, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"name":"test.png","type":"image"}`
	req := httptest.NewRequest("POST", "/v1/media/presign", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.PresignUpload(w, req)
	// LocalStore doesn't support presigned URLs
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for local backend presign, got %d", w.Code)
	}
}

func TestMediaHandler_CompleteUpload_NoAuth(t *testing.T) {
	h, _ := newTestMediaHandler(t)
	body := `{"id":"test","name":"test.txt","file_url":"http://example.com/test.txt"}`
	req := httptest.NewRequest("POST", "/v1/media/complete", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.CompleteUpload(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated complete, got %d", w.Code)
	}
}

func TestMediaHandler_Create_NoClaimsInContext(t *testing.T) {
	h, auth := newTestMediaHandler(t)
	// Generate a JWT token
	token, err := auth.GenerateToken("user-1", "test@test.com", "user", db.DefaultTenantID, nil)
	if err != nil {
		t.Fatal(err)
	}

	body := `{"name":"test.txt","content":"hello world"}`
	req := httptest.NewRequest("POST", "/v1/media/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.Create(w, req)
	// Handler is invoked without authMW, so no claims exist in the request
	// context — the defensive nil check must answer 401.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims in context, got %d: %s", w.Code, w.Body.String())
	}
}
