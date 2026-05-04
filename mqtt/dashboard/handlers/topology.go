// mqtt/dashboard/handlers/topology.go
package handlers

import (
	"fmt"
	"html"
	"html/template"
	"math"
	"strings"
)

// ringTopologySVG renders an inline-SVG cluster topology view.
//
// Layout: nodes are placed on the perimeter of a circle. In cluster mode
// every pair of nodes is connected with a faint mesh line; the leader gets
// a filled disc + a small crown indicator; followers get a hollow ring; the
// "this node" gets a thicker stroke and a halo so the operator can find
// themselves at a glance.
//
// In standalone mode (1 node), no mesh lines, just a single centered disc.
//
// The output uses currentColor so it adapts to light/dark theme via the
// `text-foreground` class on the wrapping element.
func ringTopologySVG(members []nodeMember, self, leader string, cluster bool) template.HTML {
	const (
		w, h = 240.0, 160.0
		cx   = w / 2
		cy   = h / 2 - 6
		r    = 52.0
	)
	if len(members) == 0 {
		return template.HTML(`<svg viewBox="0 0 320 240"></svg>`)
	}

	type pos struct {
		x, y float64
		m    nodeMember
	}
	pts := make([]pos, len(members))
	if len(members) == 1 {
		pts[0] = pos{x: cx, y: cy, m: members[0]}
	} else {
		for i, m := range members {
			angle := -math.Pi/2 + 2*math.Pi*float64(i)/float64(len(members))
			pts[i] = pos{x: cx + r*math.Cos(angle), y: cy + r*math.Sin(angle), m: m}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %g %g" xmlns="http://www.w3.org/2000/svg" class="w-full h-full" aria-hidden="true">`, w, h)

	// Mesh: faint lines between every pair (n*(n-1)/2 edges).
	if cluster && len(pts) > 1 {
		for i := 0; i < len(pts); i++ {
			for j := i + 1; j < len(pts); j++ {
				fmt.Fprintf(&b,
					`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="currentColor" stroke-opacity="0.18" stroke-width="1"/>`,
					pts[i].x, pts[i].y, pts[j].x, pts[j].y)
			}
		}
	}

	// Nodes: leader=filled disc, followers=hollow rings, self=thicker stroke + halo.
	for _, p := range pts {
		isLeader := p.m.IsLeader
		isSelf := p.m.IsSelf

		if isSelf {
			// Soft halo behind the self node.
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="16" fill="currentColor" fill-opacity="0.10"/>`, p.x, p.y)
		}

		// Main disc.
		nodeR := 10.0
		stroke := 1.5
		if isSelf {
			stroke = 2.5
		}
		fill := "var(--background, #fff)"
		fillOpacity := "1"
		if isLeader {
			fill = "currentColor"
			fillOpacity = "1"
		}
		fmt.Fprintf(&b,
			`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" fill-opacity="%s" stroke="currentColor" stroke-width="%.1f"/>`,
			p.x, p.y, nodeR, fill, fillOpacity, stroke)

		// Crown for leader (small upward triangle above the node).
		if isLeader {
			fmt.Fprintf(&b,
				`<path d="M %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f L %.1f %.1f Z" fill="currentColor"/>`,
				p.x-7, p.y-13,
				p.x-5, p.y-19,
				p.x-2, p.y-16,
				p.x, p.y-21,
				p.x+2, p.y-16,
				p.x+5, p.y-19,
				p.x+7, p.y-13)
		}

		// Label below.
		label := p.m.Name
		if len(label) > 16 {
			label = label[:13] + "..."
		}
		fmt.Fprintf(&b,
			`<text x="%.1f" y="%.1f" text-anchor="middle" fill="currentColor" font-size="9" font-family="ui-monospace, Menlo, monospace">%s</text>`,
			p.x, p.y+nodeR+10, html.EscapeString(label))

		// Self indicator under label.
		if isSelf {
			fmt.Fprintf(&b,
				`<text x="%.1f" y="%.1f" text-anchor="middle" fill="currentColor" fill-opacity="0.55" font-size="8">(this node)</text>`,
				p.x, p.y+nodeR+20)
		}
	}

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}
