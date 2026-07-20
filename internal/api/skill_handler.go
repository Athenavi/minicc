package api

import (
	"log/slog"
	"net/http"
	"regexp"

	"github.com/athenavi/minicc/internal/engine"
)

// SkillHandler proxies skill requests to the Python AI engine.
type SkillHandler struct {
	python *engine.PythonClient
}

func NewSkillHandler(python *engine.PythonClient) *SkillHandler {
	return &SkillHandler{python: python}
}

var validSkillName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func (h *SkillHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/skills/", h.proxy)
	mux.HandleFunc("POST /v1/skills/install", h.proxy)
	mux.HandleFunc("POST /v1/skills/generate", h.proxy)
	mux.HandleFunc("DELETE /v1/skills/{name}", h.proxyDelete)
	mux.HandleFunc("GET /v1/skills/discover", h.proxy)
}

// proxy forwards the request to the Python engine.
func (h *SkillHandler) proxy(w http.ResponseWriter, r *http.Request) {
	if h.python == nil || !h.python.IsConnected() {
		InternalError(w, "python engine not available")
		return
	}

	var result map[string]interface{}
	var err error

	switch r.Method {
	case "GET":
		err = h.python.GetJSON(r.Context(), r.URL.Path, &result)
	case "POST":
		var body map[string]interface{}
		if err := DecodeJSON(w, r, &body); err != nil {
			BadRequest(w, "invalid request body")
			return
		}
		err = h.python.PostJSON(r.Context(), r.URL.Path, body, &result)
	default:
		BadRequest(w, "unsupported method")
		return
	}

	if err != nil {
		slog.Error("skill proxy error", "path", r.URL.Path, "error", err)
		InternalError(w, "python engine error: "+err.Error())
		return
	}
	OK(w, result)
}

// proxyDelete forwards DELETE requests to the Python engine.
func (h *SkillHandler) proxyDelete(w http.ResponseWriter, r *http.Request) {
	if h.python == nil || !h.python.IsConnected() {
		InternalError(w, "python engine not available")
		return
	}

	name := r.PathValue("name")
	if name == "" {
		BadRequest(w, "name is required")
		return
	}
	if !validSkillName.MatchString(name) {
		BadRequest(w, "invalid skill name")
		return
	}

	var result map[string]interface{}
	if err := h.python.DeleteJSON(r.Context(), "/v1/skills/"+name, &result); err != nil {
		slog.Error("skill proxy delete error", "name", name, "error", err)
		InternalError(w, "python engine error: "+err.Error())
		return
	}
	OK(w, result)
}
