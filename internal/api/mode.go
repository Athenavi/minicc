package api

import (
	"net/http"
	"sync"
	"time"
)

// ── Mode constants ──

const (
	ModeAsk  = "ask"
	ModeAuto = "auto"
	ModeYOLO = "yolo"
)

var validModes = map[string]bool{ModeAsk: true, ModeAuto: true, ModeYOLO: true}

// ── ModeStore ──

type ModeStore struct {
	mu    sync.RWMutex
	modes map[string]string // session_id → mode
}

func NewModeStore() *ModeStore {
	return &ModeStore{modes: make(map[string]string)}
}

func (s *ModeStore) Get(sessionID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	mode, ok := s.modes[sessionID]
	if !ok {
		return ModeAuto // default
	}
	return mode
}

func (s *ModeStore) Set(sessionID, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modes[sessionID] = mode
}

// ── Permission Manager ──

type PermissionRequest struct {
	SessionID string `json:"session_id"`
	ToolName  string `json:"tool_name"`
	TaskName  string `json:"task_name"`
	TaskID    string `json:"task_id"`
}

type PermissionResult struct {
	Approved bool
	Done     chan struct{}
}

type PermissionManager struct {
	mu      sync.Mutex
	pending map[string]*PermissionResult // task_id → result
}

func NewPermissionManager() *PermissionManager {
	return &PermissionManager{pending: make(map[string]*PermissionResult)}
}

// WaitForApproval blocks until the user approves or rejects.
// Returns true if approved, false if rejected.
func (pm *PermissionManager) WaitForApproval(taskID string, timeout time.Duration) (bool, error) {
	result := &PermissionResult{Done: make(chan struct{})}

	pm.mu.Lock()
	pm.pending[taskID] = result
	pm.mu.Unlock()

	// Wait for approval/rejection or timeout
	select {
	case <-result.Done:
		return result.Approved, nil
	case <-time.After(timeout):
		pm.mu.Lock()
		delete(pm.pending, taskID)
		pm.mu.Unlock()
		return false, nil // timeout = reject
	}
}

// Approve approves a pending permission request.
func (pm *PermissionManager) Approve(taskID string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	result, ok := pm.pending[taskID]
	if !ok {
		return false
	}
	result.Approved = true
	close(result.Done)
	delete(pm.pending, taskID)
	return true
}

// Reject rejects a pending permission request.
func (pm *PermissionManager) Reject(taskID string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	result, ok := pm.pending[taskID]
	if !ok {
		return false
	}
	result.Approved = false
	close(result.Done)
	delete(pm.pending, taskID)
	return true
}

// ── HTTP Handlers ──

type ModeHandler struct {
	store    *ModeStore
	permMgr  *PermissionManager
}

func NewModeHandler(store *ModeStore, permMgr *PermissionManager) *ModeHandler {
	return &ModeHandler{store: store, permMgr: permMgr}
}

// GetMode returns the current mode for a session.
func (h *ModeHandler) GetMode(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("session_id")
	mode := h.store.Get(sessionID)
	OK(w, map[string]string{"mode": mode, "session_id": sessionID})
}

// SetMode changes the mode for a session.
func (h *ModeHandler) SetMode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SessionID string `json:"session_id"`
		Mode      string `json:"mode"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.SessionID == "" {
		BadRequest(w, "session_id is required")
		return
	}
	if !validModes[body.Mode] {
		BadRequest(w, "invalid mode: must be ask/auto/yolo")
		return
	}

	h.store.Set(body.SessionID, body.Mode)
	OK(w, map[string]string{"mode": body.Mode, "session_id": body.SessionID})
}

// ApprovePermission approves a pending permission request.
func (h *ModeHandler) ApprovePermission(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TaskID string `json:"task_id"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.TaskID == "" {
		BadRequest(w, "task_id is required")
		return
	}

	if ok := h.permMgr.Approve(body.TaskID); !ok {
		NotFound(w, "permission request not found or already handled")
		return
	}

	OK(w, map[string]string{"status": "approved", "task_id": body.TaskID})
}

// RejectPermission rejects a pending permission request.
func (h *ModeHandler) RejectPermission(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TaskID string `json:"task_id"`
	}
	if err := DecodeJSON(w, r, &body); err != nil {
		BadRequest(w, "invalid request")
		return
	}
	if body.TaskID == "" {
		BadRequest(w, "task_id is required")
		return
	}

	if ok := h.permMgr.Reject(body.TaskID); !ok {
		NotFound(w, "permission request not found or already handled")
		return
	}

	OK(w, map[string]string{"status": "rejected", "task_id": body.TaskID})
}
