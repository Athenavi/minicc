package api

import (
	"log/slog"
	"net/http"

	"github.com/athenavi/chiron/internal/auth"
	"github.com/athenavi/chiron/internal/db"
)

// SearchHandler provides a unified search endpoint across conversations, files, and media.
type SearchHandler struct{}

func NewSearchHandler() *SearchHandler {
	return &SearchHandler{}
}

// SearchResult holds a single search hit.
type SearchResult struct {
	Type      string  `json:"type"` // message, file, media
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Snippet   string  `json:"snippet"`
	SessionID string  `json:"session_id,omitempty"`
	Path      string  `json:"path,omitempty"`
	URL       string  `json:"url,omitempty"`
	Score     float64 `json:"score"`
}

func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		OK(w, map[string]interface{}{"results": []interface{}{}})
		return
	}
	// 澶氱鎴烽殧绂讳慨澶嶏紙P0-S4锛夛細蹇呴』鎸夊綋鍓嶇鎴?鐢ㄦ埛杩囨护锛屽惁鍒欎换涓€绉熸埛鐢ㄦ埛鍙叏鏂囨悳绱㈠叏搴撱€?
	claims := auth.GetClaims(r.Context())
	if claims == nil || claims.TenantID == "" {
		Unauthorized(w, "missing tenant context")
		return
	}

	var results []SearchResult

	// 1. Search messages (conversations)
	msgRows, err := db.ReadPool().Query(r.Context(),
		`SELECT m.id, m.session_id, m.content, s.title,
			ts_rank(to_tsvector('simple', m.content), plainto_tsquery('simple', $1)) as rank
		 FROM messages m
		 LEFT JOIN sessions s ON s.id = m.session_id
		 WHERE to_tsvector('simple', m.content) @@ plainto_tsquery('simple', $1)
		   AND s.tenant_id = $2 AND s.user_id = $3
		 ORDER BY rank DESC LIMIT 20`, q, claims.TenantID, claims.UserID)
	if err == nil {
		defer msgRows.Close()
		for msgRows.Next() {
			var result SearchResult
			var content, sessionID string
			var title *string
			if err := msgRows.Scan(&result.ID, &sessionID, &content, &title, &result.Score); err != nil {
				continue
			}
			result.Type = "message"
			result.SessionID = sessionID
			result.Snippet = truncateText(content, 150)
			if title != nil {
				result.Title = *title
			} else {
				result.Title = truncateText(content, 60)
			}
			results = append(results, result)
		}
		if err := msgRows.Err(); err != nil {
			slog.Warn("search messages iteration error", "error", err)
		}
	}

	// 2. Search files (from editor file list)
	fileRows, err := db.ReadPool().Query(r.Context(),
		`SELECT id, name, file_path,
			ts_rank(to_tsvector('simple', COALESCE(name,'')), plainto_tsquery('simple', $1)) as rank
		 FROM media_assets
		 WHERE to_tsvector('simple', COALESCE(name,'')) @@ plainto_tsquery('simple', $1)
		   AND tenant_id = $2 AND user_id = $3
		 ORDER BY rank DESC LIMIT 10`, q, claims.TenantID, claims.UserID)
	if err == nil {
		defer fileRows.Close()
		for fileRows.Next() {
			var result SearchResult
			var name, path string
			if err := fileRows.Scan(&result.ID, &name, &path, &result.Score); err != nil {
				continue
			}
			result.Type = "file"
			result.Title = name
			result.Path = path
			result.Snippet = path
			results = append(results, result)
		}
		if err := fileRows.Err(); err != nil {
			slog.Warn("search files iteration error", "error", err)
		}
	}

	OK(w, map[string]interface{}{
		"query":   q,
		"results": results,
		"count":   len(results),
	})
}

func truncateText(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
