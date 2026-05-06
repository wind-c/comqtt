// dashboard/handlers/sparkline_test.go
package handlers

import (
	"strings"
	"testing"
)

func TestSparklineEmpty(t *testing.T) {
	if string(Sparkline(nil)) != `<span></span>` {
		t.Fatalf("expected empty span for nil input")
	}
}

func TestSparklineFlatLine(t *testing.T) {
	got := string(Sparkline([]float64{1, 1, 1, 1}))
	if !strings.Contains(got, "<polyline") {
		t.Fatalf("expected polyline: %q", got)
	}
	if !strings.Contains(got, "22") {
		t.Fatalf("flat line should sit at y=22: %q", got)
	}
}

func TestSparklineMonotonic(t *testing.T) {
	got := string(Sparkline([]float64{0, 1, 2, 3, 4}))
	if !strings.Contains(got, "<polyline") {
		t.Fatalf("expected polyline: %q", got)
	}
	if !strings.Contains(got, "0.0,22") {
		t.Fatalf("first point should be at min (y=22): %q", got)
	}
	if !strings.Contains(got, "100.0,2") {
		t.Fatalf("last point should be at max (y=2): %q", got)
	}
}

func TestSparklineHasViewBox(t *testing.T) {
	got := string(Sparkline([]float64{1, 2, 3}))
	if !strings.Contains(got, `viewBox="0 0 100 24"`) {
		t.Fatalf("expected viewBox: %q", got)
	}
	if !strings.Contains(got, `preserveAspectRatio="none"`) {
		t.Fatalf("expected preserveAspectRatio: %q", got)
	}
}
