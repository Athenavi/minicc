package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
	"github.com/athenavi/chiron/internal/storage"
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

// 鈹€鈹€ P1-6 MIME 鏍￠獙閫昏緫鍗曞厓娴嬭瘯 鈹€鈹€
// 瑕嗙洊 truncateMIME 涓?isExecutableMIME 鐨勫叧閿満鏅紝纭繚 magic bytes 妫€娴嬩笌
// 鍙墽琛屾枃浠舵嫆缁濋€昏緫姝ｇ‘锛堝墠绔?P1-2 涓婁紶渚濊禆鍚庣杩欓亾瀹夊叏闃茬嚎锛夈€?
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
	// 瀹夊叏绫诲瀷锛氫笉搴旇鎷掔粷锛堟枃浠跺悕涔熸槸瀹夊叏鐨勶級
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
	// 鍗遍櫓 MIME锛氬簲琚嫆缁濓紙鏂囦欢鍚嶆棤鍏筹級
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
	// 澶у皬鍐欎笉鏁忔劅
	if !isExecutableMIME("APPLICATION/X-MSDOWNLOAD", "x.bin") {
		t.Error("uppercase MIME should be detected as executable")
	}
	// octet-stream + 鍗遍櫓鎵╁睍鍚嶏細搴旇鎷掔粷锛圥E/ELF magic bytes 鍏滃簳锛?
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
	// octet-stream + 瀹夊叏鎵╁睍鍚嶏細涓嶅簲琚嫆缁濓紙濡?.bin 鏁版嵁鏂囦欢锛?
	if isExecutableMIME("application/octet-stream", "data.bin") {
		t.Error("octet-stream + .bin should not be rejected")
	}
	// text/plain + 鍗遍櫓鎵╁睍鍚嶏細搴旇鎷掔粷锛坰hell 鑴氭湰妫€娴嬩负 text/plain锛?
	if !isExecutableMIME("text/plain", "script.sh") {
		t.Error("text/plain + .sh should be rejected")
	}
	if !isExecutableMIME("text/plain", "batch.bat") {
		t.Error("text/plain + .bat should be rejected")
	}
	if !isExecutableMIME("text/html", "page.html") {
		t.Error("text/html + .html should be rejected")
	}
	// text/plain + 瀹夊叏鎵╁睍鍚嶏細涓嶅簲琚嫆缁?
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
	// context 鈥?the defensive nil check must answer 401.
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without claims in context, got %d: %s", w.Code, w.Body.String())
	}
}
