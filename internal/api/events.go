package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Beardsoft/GoPool/internal/pool"
)

func (a *API) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	b := pool.GetBroadcaster()
	ch := b.Subscribe()
	defer func() {
		b.Unsubscribe(ch)
	}()

	// Send a comment to keep connection alive
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			// Marshal event to JSON for data payload
			dataBytes, _ := json.Marshal(e)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, string(dataBytes))
			flusher.Flush()
		}
	}
}
