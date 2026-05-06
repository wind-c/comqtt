// dashboard/handlers/overview.go
package handlers

import (
	"html/template"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/dashboard/auth"
)

var osHostname = os.Hostname

// OverviewDeps bundles dependencies for the Overview handlers.
type OverviewDeps struct {
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
	// Sampler is the per-second rate sampler. Optional - if nil, the rate
	// cards report 0. Callers wiring the dashboard at startup should
	// construct one via NewRateSampler and Stop() it at shutdown.
	Sampler *RateSampler
	// Agent powers the cluster status block. Optional. nil + Cluster=false
	// renders the standalone label.
	Agent ClusterAgent
}

type overviewPageData struct {
	Title    string
	User     auth.User
	CSRF     string
	Cluster  bool
	Flash    string
	Cards    []card
	NodeInfo nodeStatus
}

type nodeStatus struct {
	Mode     string // "Standalone" or "Cluster"
	Self     string // local hostname
	Total    int    // node count
	Leader   string // leader node name (empty in standalone)
	Members  []nodeMember
	Topology template.HTML // inline SVG ring topology
}

type nodeMember struct {
	Name     string
	Addr     string
	IsLeader bool
	IsSelf   bool
}

type overviewFragData struct {
	Cards []card
}

type card struct {
	Label string
	Value string
	Unit  string
	Sub   string
	Spark template.HTML
}

// OverviewGet renders the overview shell. The cards inside are rendered
// inline on first load so the page is usable without waiting for htmx.
func OverviewGet(d OverviewDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d.Renderer.Render(w, "overview", overviewPageData{
			Title:    "Overview",
			User:     auth.UserFromContext(r.Context()),
			CSRF:     auth.NewCSRFToken(),
			Cluster:  d.Cluster,
			Cards:    buildCards(d),
			NodeInfo: buildNodeStatus(d),
		})
	}
}

func buildNodeStatus(d OverviewDeps) nodeStatus {
	self := hostnameOrUnknown()
	if !d.Cluster || d.Agent == nil {
		st := nodeStatus{Mode: "Standalone", Self: self, Total: 1, Members: []nodeMember{{Name: self, IsSelf: true}}}
		st.Topology = ringTopologySVG(st.Members, st.Self, st.Leader, false)
		return st
	}
	members := d.Agent.GetMemberList()
	leader := d.Agent.Leader()
	rows := make([]nodeMember, 0, len(members))
	for _, m := range members {
		rows = append(rows, nodeMember{
			Name:     m.Name,
			Addr:     m.Addr,
			IsLeader: m.Name == leader && leader != "",
			IsSelf:   m.Name == self,
		})
	}
	st := nodeStatus{Mode: "Cluster", Self: self, Total: len(members), Leader: leader, Members: rows}
	st.Topology = ringTopologySVG(rows, self, leader, true)
	return st
}

func hostnameOrUnknown() string {
	if h, err := osHostname(); err == nil {
		return h
	}
	return "unknown"
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
	var connHist, subsHist, retHist, inHist, outHist []float64
	if d.Sampler != nil {
		in, out = d.Sampler.Rates()
		connHist, subsHist, retHist, inHist, outHist = d.Sampler.History()
	}
	return []card{
		{Label: "Connections", Value: itoa(atomic.LoadInt64(&info.ClientsConnected)), Spark: Sparkline(connHist)},
		{Label: "Subscriptions", Value: itoa(atomic.LoadInt64(&info.Subscriptions)), Spark: Sparkline(subsHist)},
		{Label: "Retained", Value: itoa(atomic.LoadInt64(&info.Retained)), Spark: Sparkline(retHist)},
		{Label: "Inflight", Value: itoa(atomic.LoadInt64(&info.Inflight))},
		{Label: "Msg In/sec", Value: ftoa(in), Unit: "msg/s", Spark: Sparkline(inHist)},
		{Label: "Msg Out/sec", Value: ftoa(out), Unit: "msg/s", Spark: Sparkline(outHist)},
	}
}

// RateSampler ticks every second, snapshotting cumulative
// MessagesReceived/MessagesSent and computing the delta as the per-second
// rate. Exposes Rates() for read access. Stop() cleanly shuts down the
// background goroutine.
type RateSampler struct {
	server *mqtt.Server
	mu     sync.RWMutex

	in, out float64
	lastIn  int64
	lastOut int64

	histLen  int
	connHist []int64
	subsHist []int64
	retHist  []int64
	inHist   []float64
	outHist  []float64

	stop chan struct{}
}

func NewRateSampler(server *mqtt.Server) *RateSampler {
	s := &RateSampler{
		server:   server,
		stop:     make(chan struct{}),
		histLen:  60,
		connHist: make([]int64, 0, 60),
		subsHist: make([]int64, 0, 60),
		retHist:  make([]int64, 0, 60),
		inHist:   make([]float64, 0, 60),
		outHist:  make([]float64, 0, 60),
	}
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
			conn := atomic.LoadInt64(&info.ClientsConnected)
			subs := atomic.LoadInt64(&info.Subscriptions)
			ret := atomic.LoadInt64(&info.Retained)

			s.mu.Lock()
			if s.lastIn != 0 || s.lastOut != 0 {
				s.in = float64(ci - s.lastIn)
				s.out = float64(co - s.lastOut)
			}
			s.lastIn = ci
			s.lastOut = co
			s.connHist = pushInt64(s.connHist, conn, s.histLen)
			s.subsHist = pushInt64(s.subsHist, subs, s.histLen)
			s.retHist = pushInt64(s.retHist, ret, s.histLen)
			s.inHist = pushFloat64(s.inHist, s.in, s.histLen)
			s.outHist = pushFloat64(s.outHist, s.out, s.histLen)
			s.mu.Unlock()
		}
	}
}

func (s *RateSampler) Rates() (in, out float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.in, s.out
}

// History returns a snapshot of the per-metric ring buffers. Returned slices
// are copies so callers can mutate freely.
func (s *RateSampler) History() (conn, subs, ret, in, out []float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64ToFloat(s.connHist), int64ToFloat(s.subsHist), int64ToFloat(s.retHist),
		copyFloat(s.inHist), copyFloat(s.outHist)
}

func (s *RateSampler) Stop() { close(s.stop) }

func pushInt64(buf []int64, v int64, max int) []int64 {
	if len(buf) >= max {
		buf = buf[1:]
	}
	return append(buf, v)
}

func pushFloat64(buf []float64, v float64, max int) []float64 {
	if len(buf) >= max {
		buf = buf[1:]
	}
	return append(buf, v)
}

func int64ToFloat(in []int64) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

func copyFloat(in []float64) []float64 {
	out := make([]float64, len(in))
	copy(out, in)
	return out
}

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
