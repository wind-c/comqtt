// dashboard/handlers/templates.go
package handlers

import (
	"bytes"
	"html/template"
	"io/fs"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Renderer reads templates from a filesystem and executes them with data.
//
// IMPORTANT: every page template defines its body as `{{define "content"}}`
// for the shared layout to render via `{{template "content" .}}`. With one
// global tree, only the last-parsed `content` block survives. So this
// renderer parses each page into its OWN tree at first use, mixing in the
// shared partials (anything starting with `_`).
type Renderer struct {
	tplFS fs.FS

	mu      sync.Mutex
	parsed  map[string]*template.Template // tree keyed by page-define name
	loadErr error

	// Initialised lazily on first call; never re-loaded.
	files       map[string][]byte // path -> bytes
	defineToTpl map[string]string // page-define name -> file path
	partials    [][]byte          // shared partials (path-prefixed _ files)
	loadedOnce  sync.Once
}

func NewRenderer(tplFS fs.FS) *Renderer {
	return &Renderer{tplFS: tplFS}
}

// Render finds the file that defines the given block name, builds a fresh
// template tree from {partials + that file alone}, and executes it.
func (r *Renderer) Render(w http.ResponseWriter, name string, data any) {
	r.loadOnce()
	if r.loadErr != nil {
		http.Error(w, r.loadErr.Error(), http.StatusInternalServerError)
		return
	}
	tpl, err := r.lookup(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, name, data); err != nil {
		_, _ = w.Write([]byte("<!-- template error: " + err.Error() + " -->"))
		return
	}
	_, _ = buf.WriteTo(w)
}

func (r *Renderer) loadOnce() {
	r.loadedOnce.Do(func() {
		r.files = map[string][]byte{}
		r.defineToTpl = map[string]string{}
		err := fs.WalkDir(r.tplFS, "templates", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := fs.ReadFile(r.tplFS, path)
			if err != nil {
				return err
			}
			r.files[path] = b
			base := pathBase(path)
			// Shared sources are included in every per-page tree. Two
			// kinds: partial files (basename starts with `_`) and any file
			// under templates/fragments/ which holds reusable blocks (e.g.
			// overview_cards) that pages reference by name.
			if (len(base) > 0 && base[0] == '_') || strings.Contains(path, "/fragments/") {
				r.partials = append(r.partials, b)
				return nil
			}
			// Pages: remember which file owns each block name so Render
			// can find the right one when invoked by name.
			for _, name := range extractDefines(b) {
				r.defineToTpl[name] = path
			}
			return nil
		})
		// Stable iteration of partials.
		sort.Slice(r.partials, func(i, j int) bool {
			return bytes.Compare(r.partials[i], r.partials[j]) < 0
		})
		r.loadErr = err
	})
}

func (r *Renderer) lookup(name string) (*template.Template, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if tpl, ok := r.parsed[name]; ok {
		return tpl, nil
	}
	path, ok := r.defineToTpl[name]
	if !ok {
		return nil, errFoundNoTemplate(name)
	}
	tpl := template.New("")
	for _, body := range r.partials {
		if _, err := tpl.Parse(string(body)); err != nil {
			return nil, err
		}
	}
	if _, err := tpl.Parse(string(r.files[path])); err != nil {
		return nil, err
	}
	if r.parsed == nil {
		r.parsed = map[string]*template.Template{}
	}
	r.parsed[name] = tpl
	return tpl, nil
}

// extractDefines pulls the names from `{{define "name"}}` directives.
var defineRE = regexp.MustCompile(`\{\{[-\s]*define[\s]+"([^"]+)"[-\s]*\}\}`)

func extractDefines(b []byte) []string {
	matches := defineRE.FindAllSubmatch(b, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, string(m[1]))
	}
	return out
}

func pathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

type errFoundNoTemplate string

func (e errFoundNoTemplate) Error() string { return "no template defines block: " + string(e) }
