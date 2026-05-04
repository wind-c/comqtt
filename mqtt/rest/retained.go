// mqtt/rest/retained.go
package rest

import (
	"net/http"
	"sort"
	"strings"
)

const retainedPayloadCap = 4096

type retainedSummary struct {
	Topic    string `json:"topic"`
	QoS      byte   `json:"qos"`
	Size     int    `json:"size"`
	StoredAt int64  `json:"stored_at"`
	Payload  []byte `json:"payload,omitempty"`
}

// listRetained handles GET /api/v1/mqtt/retained.
// Optional ?topic= substring filter on the topic name. ?payload=true includes
// the raw payload (truncated to 4096 bytes). Default omits payloads to keep
// list responses small.
func (s *Rest) listRetained(w http.ResponseWriter, r *http.Request) {
	page := ParsePage(r.URL.Query())
	topicFilter := strings.ToLower(r.URL.Query().Get("topic"))
	includePayload := r.URL.Query().Get("payload") == "true"

	all := make([]retainedSummary, 0, 64)
	for topic, pk := range s.server.Topics.Retained.GetAll() {
		if topicFilter != "" && !strings.Contains(strings.ToLower(topic), topicFilter) {
			continue
		}
		row := retainedSummary{
			Topic:    topic,
			QoS:      pk.FixedHeader.Qos,
			Size:     len(pk.Payload),
			StoredAt: pk.Created,
		}
		if includePayload {
			if len(pk.Payload) > retainedPayloadCap {
				row.Payload = pk.Payload[:retainedPayloadCap]
			} else {
				row.Payload = pk.Payload
			}
		}
		all = append(all, row)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Topic < all[j].Topic })

	resp := Page[retainedSummary]{
		Page:  page.Page,
		Size:  page.Size,
		Total: len(all),
		Items: ApplyPagination(all, page),
	}
	Ok(w, resp)
}

// clearRetained handles DELETE /api/v1/mqtt/retained/{topic}.
// 404 if no retained message exists at the given topic.
func (s *Rest) clearRetained(w http.ResponseWriter, r *http.Request) {
	topic := r.PathValue("topic")
	if topic == "" {
		http.Error(w, "missing topic", http.StatusBadRequest)
		return
	}
	if _, ok := s.server.Topics.Retained.Get(topic); !ok {
		http.NotFound(w, r)
		return
	}
	s.server.Topics.Retained.Delete(topic)
	w.WriteHeader(http.StatusNoContent)
}
