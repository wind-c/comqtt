// dashboard/handlers/events.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"

	"github.com/wind-c/comqtt/v2/dashboard/sse"
)

// Events returns an http.HandlerFunc that streams events from the given Hub
// as Server-Sent Events. Default payload is HTML for direct htmx-sse swap;
// pass ?as=json for the raw JSON payload.
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

		asJSON := r.URL.Query().Get("as") == "json"

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
				var payload string
				if asJSON {
					b, _ := json.Marshal(ev)
					payload = string(b)
				} else {
					payload = renderEventHTML(ev)
				}
				fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, payload)
				flusher.Flush()
			}
		}
	}
}

// renderEventHTML produces a single-line <li> fragment per event so it can
// flow through htmx-sse's hx-swap="afterbegin" without wrecking newlines
// (SSE data: lines are newline-delimited; the fragment must be one line).
func renderEventHTML(ev sse.Event) string {
	var summary map[string]any
	_ = json.Unmarshal(ev.Payload, &summary)

	icon := eventIcon(ev.Type)
	tone := eventTone(ev.Type)

	headline := html.EscapeString(eventHeadline(ev.Type, summary))
	detail := html.EscapeString(eventDetail(ev.Type, summary))
	node := html.EscapeString(ev.Node)
	ts := fmt.Sprintf("%d", ev.TS)

	return fmt.Sprintf(
		`<li class="px-3 py-2 text-xs flex items-start gap-2 %s">`+
			`<span class="font-mono">%s</span>`+
			`<span class="flex-1"><span class="font-medium">%s</span> <span class="text-slate-500 dark:text-slate-400">%s</span></span>`+
			`<span class="text-slate-400" title="ts=%s node=%s">%s</span>`+
			`</li>`,
		tone, icon, headline, detail, ts, node, node,
	)
}

func eventIcon(t string) string {
	switch t {
	case "client.connected":
		return "+"
	case "client.disconnected":
		return "-"
	case "message.published":
		return ">"
	case "subscription.added":
		return "*"
	case "subscription.removed":
		return "~"
	}
	return "."
}

func eventTone(t string) string {
	switch t {
	case "client.connected", "subscription.added":
		return "text-emerald-700 dark:text-emerald-300"
	case "client.disconnected", "subscription.removed":
		return "text-rose-700 dark:text-rose-300"
	}
	return ""
}

func eventHeadline(t string, p map[string]any) string {
	switch t {
	case "client.connected":
		return strOf(p, "client_id") + " connected"
	case "client.disconnected":
		return strOf(p, "client_id") + " disconnected"
	case "message.published":
		return strOf(p, "client_id") + " -> " + strOf(p, "topic")
	case "subscription.added":
		return strOf(p, "client_id") + " sub " + strOf(p, "topic")
	case "subscription.removed":
		return strOf(p, "client_id") + " unsub " + strOf(p, "topic")
	}
	return t
}

func eventDetail(t string, p map[string]any) string {
	switch t {
	case "client.connected":
		return strOf(p, "remote")
	case "client.disconnected":
		if reason := strOf(p, "reason"); reason != "" {
			return reason
		}
		return ""
	case "message.published":
		size, _ := p["payload_size"].(float64)
		qos, _ := p["qos"].(float64)
		return fmt.Sprintf("qos=%d size=%dB", int(qos), int(size))
	}
	return ""
}

func strOf(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}
