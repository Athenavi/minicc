package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/athenavi/chiron/internal/broadcast"
)

// 鈹€鈹€ Mode constants 鈹€鈹€

const (
	ModeAsk  = "ask"
	ModeAuto = "auto"
	ModeYOLO = "yolo"
)

var validModes = map[string]bool{ModeAsk: true, ModeAuto: true, ModeYOLO: true}

// 鈹€鈹€ ModeStore 鈹€鈹€

type ModeStore struct {
	mu    sync.RWMutex
	modes map[string]string // session_id 鈫?mode
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

// Delete removes a session's mode setting to prevent memory leaks
func (s *ModeStore) Delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.modes, sessionID)
}

// 鈹€鈹€ Permission Manager 鈹€鈹€

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
	pending map[string]*PermissionResult // task_id 鈫?result
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
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-result.Done:
		return result.Approved, nil
	case <-timer.C:
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

// 鈹€鈹€ HTTP Handlers 鈹€鈹€

type ModeHandler struct {
	store    *ModeStore
	permMgr  *PermissionManager
	hub      *broadcast.Hub
}

func NewModeHandler(store *ModeStore, permMgr *PermissionManager, hub *broadcast.Hub) *ModeHandler {
	return &ModeHandler{store: store, permMgr: permMgr, hub: hub}
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

	// Notify frontend via SSE
	if h.hub != nil {
		data, _ := json.Marshal(map[string]string{"task_id": body.TaskID, "approved": "true"})
		h.hub.Publish(broadcast.Event{Type: "permission_result", Data: json.RawMessage(data)})
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

	// Notify frontend via SSE
	if h.hub != nil {
		data, _ := json.Marshal(map[string]string{"task_id": body.TaskID, "approved": "false"})
		h.hub.Publish(broadcast.Event{Type: "permission_result", Data: json.RawMessage(data)})
	}

	OK(w, map[string]string{"status": "rejected", "task_id": body.TaskID})
}
