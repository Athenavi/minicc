package api

import (
	"net/http"
	"time"

	"github.com/athenavi/minicc/internal/broadcast"
)

// handleSSE manages a Server-Sent Events connection for real-time streaming.
// It subscribes to the event hub and writes events to the response writer.
func handleSSE(w http.ResponseWriter, r *http.Request, hub *broadcast.Hub, subID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		InternalError(w, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := hub.Subscribe(subID)
	defer hub.Unsubscribe(subID)

	// Send initial connected event
	w.Write([]byte(broadcast.FormatSSE(broadcast.Event{Type: "connected", Data: map[string]string{"id": subID}})))
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			w.Write([]byte(broadcast.FormatSSE(event)))
			flusher.Flush()
		case <-time.After(15 * time.Second):
			// Keep-alive ping
			w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

// SSEHandler returns an http.HandlerFunc for SSE connections.
func SSEHandler(hub *broadcast.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subID := r.URL.Query().Get("client_id")
		if subID == "" {
			subID = "anon"
		}
		handleSSE(w, r, hub, subID)
	}
}
