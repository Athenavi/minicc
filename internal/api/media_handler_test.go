package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/athenavi/minicc/internal/auth"
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
	req := httptest.NewRequest("DELETE", "/v1/media/?id=test", nil)
	w := httptest.NewRecorder()
	h.Delete(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated delete, got %d", w.Code)
	}
}

func TestMediaHandler_PresignUpload_LocalBackend(t *testing.T) {
	h, authenticator := newTestMediaHandler(t)
	token, err := authenticator.GenerateToken("user-1", "test@test.com", "user", nil)
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
	token, err := auth.GenerateToken("user-1", "test@test.com", "user", nil)
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
