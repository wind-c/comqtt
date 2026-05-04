// mqtt/dashboard/handlers/overview.go
package handlers

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
)

// OverviewDeps bundles dependencies for the Overview handlers.
type OverviewDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
	// Sampler is the per-second rate sampler. Optional - if nil, the rate
	// cards report 0. Callers wiring the dashboard at startup should
	// construct one via NewRateSampler and Stop() it at shutdown.
	Sampler *RateSampler
}

type overviewPageData struct {
	Title   string
	User    auth.User
	CSRF    string
	Cluster bool
	Flash   string
	Cards   []card
}

type overviewFragData struct {
	Cards []card
}

type card struct {
	Label string
	Value string
	Unit  string
	Sub   string
}

// OverviewGet renders the overview shell. The cards inside are rendered
// inline on first load so the page is usable without waiting for htmx.
func OverviewGet(d OverviewDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d.Renderer.Render(w, "overview", overviewPageData{
			Title:   "Overview",
			User:    auth.UserFromContext(r.Context()),
			CSRF:    auth.NewCSRFToken(),
			Cluster: d.Cluster,
			Cards:   buildCards(d),
		})
	}
}

// OverviewCards renders the cards fragment. Used by htmx polling.
func OverviewCards(d OverviewDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d.Renderer.Render(w, "overview_cards", overviewFragData{
			Cards: buildCards(d),
		})
	}
}

func buildCards(d OverviewDeps) []card {
	info := d.Server.Info
	in, out := 0.0, 0.0
	if d.Sampler != nil {
		in, out = d.Sampler.Rates()
	}
	return []card{
		{Label: "Connections", Value: itoa(atomic.LoadInt64(&info.ClientsConnected))},
		{Label: "Subscriptions", Value: itoa(atomic.LoadInt64(&info.Subscriptions))},
		{Label: "Retained", Value: itoa(atomic.LoadInt64(&info.Retained))},
		{Label: "Inflight", Value: itoa(atomic.LoadInt64(&info.Inflight))},
		{Label: "Msg In/sec", Value: ftoa(in), Unit: "msg/s"},
		{Label: "Msg Out/sec", Value: ftoa(out), Unit: "msg/s"},
	}
}

// RateSampler ticks every second, snapshotting cumulative
// MessagesReceived/MessagesSent and computing the delta as the per-second
// rate. Exposes Rates() for read access. Stop() cleanly shuts down the
// background goroutine.
type RateSampler struct {
	server  *mqtt.Server
	mu      sync.RWMutex
	in, out float64
	lastIn  int64
	lastOut int64
	stop    chan struct{}
}

func NewRateSampler(server *mqtt.Server) *RateSampler {
	s := &RateSampler{server: server, stop: make(chan struct{})}
	go s.run()
	return s
}

func (s *RateSampler) run() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			info := s.server.Info
			ci := atomic.LoadInt64(&info.MessagesReceived)
			co := atomic.LoadInt64(&info.MessagesSent)
			s.mu.Lock()
			if s.lastIn != 0 || s.lastOut != 0 {
				s.in = float64(ci - s.lastIn)
				s.out = float64(co - s.lastOut)
			}
			s.lastIn = ci
			s.lastOut = co
			s.mu.Unlock()
		}
	}
}

func (s *RateSampler) Rates() (in, out float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.in, s.out
}

func (s *RateSampler) Stop() { close(s.stop) }

func itoa(v int64) string {
	return formatInt(v)
}

func ftoa(v float64) string {
	if v == float64(int64(v)) {
		return formatInt(int64(v))
	}
	whole := int64(v)
	frac := int64((v - float64(whole)) * 10)
	if frac < 0 {
		frac = -frac
	}
	return formatInt(whole) + "." + string(rune('0'+frac))
}

func formatInt(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [24]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
