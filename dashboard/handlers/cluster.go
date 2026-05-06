// dashboard/handlers/cluster.go
package handlers

import (
	"net/http"
	"sort"

	"github.com/wind-c/comqtt/v2/cluster/discovery"
	"github.com/wind-c/comqtt/v2/dashboard/auth"
)

// ClusterAgent is the small surface from *cluster.Agent that the cluster
// page needs. *cluster.Agent satisfies it. Defined here to avoid an
// import cycle (the cluster package imports handlers? It doesn't, but this
// keeps the handler package free of any cluster-specific imports beyond
// the discovery types.)
type ClusterAgent interface {
	GetMemberList() []discovery.Member
	Leader() string
}

type ClusterDeps struct {
	Agent    ClusterAgent
	Renderer *Renderer
	Cluster  bool
}

type clusterMemberRow struct {
	Name     string
	Addr     string
	Port     int
	IsLeader bool
}

type clusterPageData struct {
	Title   string
	User    auth.User
	CSRF    string
	Cluster bool
	Flash   string
	Error   string
	Members []clusterMemberRow
	Leader  string
}

func ClusterPage(d ClusterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		members := d.Agent.GetMemberList()
		leader := d.Agent.Leader()

		rows := make([]clusterMemberRow, 0, len(members))
		for _, m := range members {
			rows = append(rows, clusterMemberRow{
				Name:     m.Name,
				Addr:     m.Addr,
				Port:     m.Port,
				IsLeader: m.Name == leader && leader != "",
			})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

		d.Renderer.Render(w, "cluster", clusterPageData{
			Title:   "Cluster",
			User:    auth.UserFromContext(r.Context()),
			CSRF:    auth.NewCSRFToken(),
			Cluster: d.Cluster,
			Members: rows,
			Leader:  leader,
		})
	}
}
