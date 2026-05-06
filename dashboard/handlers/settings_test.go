// dashboard/handlers/settings_test.go
package handlers

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func newSettingsRenderer(t *testing.T) *Renderer {
	t.Helper()
	return NewRenderer(fs.FS(fstest.MapFS{
		"templates/settings.html": &fstest.MapFile{Data: []byte(`{{define "layout"}}<html><head><style>{{.HighlightCSS}}</style></head><body>{{template "content" .}}</body></html>{{end}}{{define "settings"}}{{template "layout" .}}{{end}}{{define "content"}}<div class="yaml">{{.HighlightedYAML}}</div>{{end}}`)},
	}))
}

func TestSettingsRendersHighlightedYAML(t *testing.T) {
	deps := SettingsDeps{
		Source: func() any {
			return map[string]any{
				"hello": "world",
				"port":  1883,
				"sub": map[string]any{
					"enabled": true,
					"items":   []string{"a", "b"},
				},
			}
		},
		Renderer: newSettingsRenderer(t),
	}
	rr := httptest.NewRecorder()
	Settings(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/settings", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "<span") {
		t.Fatalf("expected chroma <span> output: %q", body[:min(len(body), 400)])
	}
	if !strings.Contains(body, "hello") {
		t.Fatalf("expected hello key in YAML output: %q", body[:min(len(body), 400)])
	}
	if !strings.Contains(body, ".chroma") && !strings.Contains(body, "color:") {
		t.Fatalf("expected chroma CSS in output: %q", body[:min(len(body), 400)])
	}
}

func TestSettingsHandlesNilSource(t *testing.T) {
	deps := SettingsDeps{
		Source:   func() any { return nil },
		Renderer: newSettingsRenderer(t),
	}
	rr := httptest.NewRecorder()
	Settings(deps)(rr, httptest.NewRequest(http.MethodGet, "/dashboard/settings", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status: %d body: %s", rr.Code, rr.Body.String())
	}
}
