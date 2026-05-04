// mqtt/rest/client_unsubscribe.go
package rest

import (
	"net/http"
)

// unsubscribeClient handles DELETE /api/v1/mqtt/clients/{id}/subscriptions/{topic}.
// The {topic} path segment must be URL-encoded by the caller because MQTT
// topic filters can contain '/', '+', '#'. The Go 1.22 ServeMux applies
// path-segment-aware matching so the encoded form is decoded automatically.
func (s *Rest) unsubscribeClient(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("id")
	filter := r.PathValue("topic")
	if clientID == "" || filter == "" {
		http.Error(w, "missing client id or topic", http.StatusBadRequest)
		return
	}
	cl, ok := s.server.Clients.Get(clientID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, has := cl.State.Subscriptions.Get(filter); !has {
		http.NotFound(w, r)
		return
	}
	s.server.Topics.Unsubscribe(filter, clientID)
	cl.State.Subscriptions.Delete(filter)
	w.WriteHeader(http.StatusNoContent)
}
