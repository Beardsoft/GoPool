package api

import (
	"fmt"
	"net/http"
	"time"
)

// handleOperatorEvents keeps an authenticated SSE channel open so the console
// can distinguish a healthy API connection from stale cached data. Detailed
// state remains authoritative through the JSON endpoints.
func (a *API) handleOperatorEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "streaming_unsupported", "event streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case timestamp := <-keepAlive.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"at\":%q}\n\n", timestamp.UTC().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}
