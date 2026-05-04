// mqtt/dashboard/handlers/events.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/wind-c/comqtt/v2/mqtt/dashboard/sse"
)

// Events returns an http.HandlerFunc that streams events from the given Hub
// as Server-Sent Events. The connection stays open until the client
// disconnects or the hub closes.
func Events(hub *sse.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := hub.Subscribe()
		defer hub.Unsubscribe(ch)

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		fmt.Fprintf(w, ": comqtt dashboard events\n\n")
		flusher.Flush()

		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				body, _ := json.Marshal(ev)
				fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, body)
				flusher.Flush()
			}
		}
	}
}
