package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Unified error message constants 鈥?single source of truth for all handlers.
const (
	ErrAuthRequired  = "authentication required"
	ErrDBUnavailable = "service temporarily unavailable"
	ErrInvalidReq    = "invalid request body"
	ErrNotFound      = "resource not found"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type Meta struct {
	Total   int `json:"total,omitempty"`
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

func JSON(w http.ResponseWriter, status int, resp APIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}

func OK(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, APIResponse{Success: true, Data: data})
}

func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, APIResponse{Success: true, Data: data})
}

func Accepted(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusAccepted, APIResponse{Success: true, Data: data})
}

func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

func BadRequest(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusBadRequest, APIResponse{Success: false, Error: msg})
}

func NotFound(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusNotFound, APIResponse{Success: false, Error: msg})
}

func InternalError(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusInternalServerError, APIResponse{Success: false, Error: msg})
}

// logAndRespond logs the internal error details and returns a generic message to the client.
func logAndRespond(w http.ResponseWriter, err error, statusCode int, userMsg string) {
	slog.Error(userMsg, "error", err)
	JSON(w, statusCode, APIResponse{Success: false, Error: userMsg})
}

func Unauthorized(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusUnauthorized, APIResponse{Success: false, Error: msg})
}

func Forbidden(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusForbidden, APIResponse{Success: false, Error: msg})
}

func ServiceUnavailable(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusServiceUnavailable, APIResponse{Success: false, Error: msg})
}

func TooManyRequests(w http.ResponseWriter) {
	JSON(w, http.StatusTooManyRequests, APIResponse{Success: false, Error: "rate limit exceeded"})
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
	// Enforce 1MB limit at the read level (not just Content-Length header)
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	return json.NewDecoder(r.Body).Decode(v)
}
