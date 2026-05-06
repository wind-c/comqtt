// dashboard/handlers/sparkline.go
package handlers

import (
	"fmt"
	"html/template"
	"math"
	"strings"
)

// Sparkline returns an inline SVG polyline as template.HTML. Empty input or
// all-zero input returns an empty span (no chart). Output is a fixed-size
// 100x24 viewBox so the CSS can size it via width/height.
func Sparkline(samples []float64) template.HTML {
	if len(samples) == 0 {
		return template.HTML(`<span></span>`)
	}
	mn, mx := minMax(samples)
	if mx-mn < 1e-9 {
		var pts strings.Builder
		for i := range samples {
			x := 0.0
			if len(samples) > 1 {
				x = float64(i) * 100.0 / float64(len(samples)-1)
			}
			fmt.Fprintf(&pts, "%.1f,22 ", x)
		}
		return template.HTML(`<svg class="sparkline" viewBox="0 0 100 24" preserveAspectRatio="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><polyline fill="none" stroke="currentColor" stroke-width="1" points="` + strings.TrimSpace(pts.String()) + `"/></svg>`)
	}
	rng := mx - mn
	var pts strings.Builder
	for i, v := range samples {
		x := 0.0
		if len(samples) > 1 {
			x = float64(i) * 100.0 / float64(len(samples)-1)
		}
		y := 22.0 - ((v-mn)/rng)*20.0
		fmt.Fprintf(&pts, "%.1f,%.1f ", x, y)
	}
	return template.HTML(`<svg class="sparkline" viewBox="0 0 100 24" preserveAspectRatio="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true"><polyline fill="none" stroke="currentColor" stroke-width="1" points="` + strings.TrimSpace(pts.String()) + `"/></svg>`)
}

func minMax(s []float64) (float64, float64) {
	mn, mx := math.Inf(1), math.Inf(-1)
	for _, v := range s {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	return mn, mx
}
