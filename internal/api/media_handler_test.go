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

func TestMediaHandler_List_NoDB(t *testing.T) {
	h, authenticator := newTestMediaHandler(t)
	token, err := authenticator.GenerateToken("user-1", "test@test.com", "user", nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/media/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h.List(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
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

func TestMediaHandler_Create_WithAuth(t *testing.T) {
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
	// With auth but no DB, should fail at DB insert (which is optional now)
	// Actually it writes to store first, then DB — so with LocalStore it should
	// succeed the write but fail at DB (which is nil Pool)
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Fatalf("expected 200 or 404, got %d: %s", w.Code, w.Body.String())
	}
}
