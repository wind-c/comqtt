// mqtt/dashboard/handlers/templates.go
package handlers

import (
	"html/template"
	"io/fs"
	"net/http"
	"sync"
)

// Renderer parses templates from an embed.FS once and renders them with
// data. Use NewRenderer(templatesFS) at startup; reuse across requests.
type Renderer struct {
	once  sync.Once
	tpl   *template.Template
	tplFS fs.FS
	err   error
}

func NewRenderer(tplFS fs.FS) *Renderer {
	return &Renderer{tplFS: tplFS}
}

func (r *Renderer) parse() {
	tpl := template.New("")
	err := fs.WalkDir(r.tplFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(r.tplFS, path)
		if err != nil {
			return err
		}
		_, err = tpl.New(path).Parse(string(b))
		return err
	})
	r.tpl = tpl
	r.err = err
}

// Render executes the named template (one of the {{define "name"}} blocks)
// against data and writes the HTML body. Sets text/html content type.
func (r *Renderer) Render(w http.ResponseWriter, name string, data any) {
	r.once.Do(r.parse)
	if r.err != nil {
		http.Error(w, r.err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.tpl.ExecuteTemplate(w, name, data); err != nil {
		_, _ = w.Write([]byte("<!-- template error: " + err.Error() + " -->"))
	}
}
