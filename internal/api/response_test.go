package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	// We need a config and minimal dependencies to test the router.
	// For now, test the response helpers directly.

	t.Run("health response format", func(t *testing.T) {
		w := httptest.NewRecorder()
		JSON(w, http.StatusOK, APIResponse{
			Success: true,
			Data:    map[string]string{"status": "ok"},
		})

		var resp APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if !resp.Success {
			t.Error("expected success=true")
		}
		if resp.Error != "" {
			t.Errorf("expected no error, got: %s", resp.Error)
		}
	})

	t.Run("error response format", func(t *testing.T) {
		w := httptest.NewRecorder()
		BadRequest(w, "invalid input")

		var resp APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if resp.Success {
			t.Error("expected success=false")
		}
		if resp.Error != "invalid input" {
			t.Errorf("expected 'invalid input', got: %s", resp.Error)
		}
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got: %d", w.Code)
		}
	})

	t.Run("unauthorized response format", func(t *testing.T) {
		w := httptest.NewRecorder()
		Unauthorized(w, "invalid token")

		var resp APIResponse
		json.Unmarshal(w.Body.Bytes(), &resp)
		if !resp.Success {
			t.Log("unauthorized correctly returns success=false")
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got: %d", w.Code)
		}
	})
}
