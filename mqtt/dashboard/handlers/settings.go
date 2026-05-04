// mqtt/dashboard/handlers/settings.go
package handlers

import (
	"bytes"
	"html/template"
	"net/http"
	"sync"

	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"gopkg.in/yaml.v3"

	"github.com/wind-c/comqtt/v2/mqtt"
	"github.com/wind-c/comqtt/v2/mqtt/dashboard/auth"
)

// SettingsDeps bundles dependencies for the Settings page.
type SettingsDeps struct {
	// Source returns the value to be marshaled as YAML and shown.
	// Defaults to a function that returns Server.Options if nil.
	Source   func() any
	Server   *mqtt.Server
	Renderer *Renderer
	Cluster  bool
}

type settingsPageData struct {
	Title           string
	User            auth.User
	CSRF            string
	Cluster         bool
	Flash           string
	HighlightedYAML template.HTML
	HighlightCSS    template.CSS
}

var (
	chromaOnce      sync.Once
	chromaFormatter *html.Formatter
	chromaStyleCSS  string
	chromaInitErr   error
)

func initChroma() {
	chromaFormatter = html.New(html.WithClasses(true), html.TabWidth(2))
	style := styles.Get("github")
	if style == nil {
		style = styles.Fallback
	}
	var buf bytes.Buffer
	if err := chromaFormatter.WriteCSS(&buf, style); err != nil {
		chromaInitErr = err
		return
	}
	chromaStyleCSS = buf.String()
}

// Settings handles GET /dashboard/settings.
func Settings(d SettingsDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		chromaOnce.Do(initChroma)
		if chromaInitErr != nil {
			http.Error(w, "settings: "+chromaInitErr.Error(), http.StatusInternalServerError)
			return
		}

		var v any
		if d.Source != nil {
			v = d.Source()
		} else if d.Server != nil {
			v = d.Server.Options
		}

		yamlBytes, err := yaml.Marshal(v)
		if err != nil {
			http.Error(w, "yaml: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var highlighted bytes.Buffer
		lexer := lexers.Get("yaml")
		if lexer == nil {
			lexer = lexers.Fallback
		}
		iter, err := lexer.Tokenise(nil, string(yamlBytes))
		if err != nil {
			http.Error(w, "tokenise: "+err.Error(), http.StatusInternalServerError)
			return
		}
		style := styles.Get("github")
		if style == nil {
			style = styles.Fallback
		}
		if err := chromaFormatter.Format(&highlighted, style, iter); err != nil {
			http.Error(w, "format: "+err.Error(), http.StatusInternalServerError)
			return
		}

		d.Renderer.Render(w, "settings", settingsPageData{
			Title:           "Settings",
			User:            auth.UserFromContext(r.Context()),
			CSRF:            auth.NewCSRFToken(),
			Cluster:         d.Cluster,
			HighlightedYAML: template.HTML(highlighted.String()),
			HighlightCSS:    template.CSS(chromaStyleCSS),
		})
	}
}
