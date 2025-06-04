package nginx

import (
	"bytes"
	"text/template"

	"github.com/rahulshinde/nginx-proxy-go/internal/config"
	"github.com/rahulshinde/nginx-proxy-go/internal/host"
)

// Template represents an nginx configuration template
type Template struct {
	tmpl *template.Template
}

// NewTemplate creates a new Template instance
func NewTemplate(tmplStr string) (*Template, error) {
	tmpl, err := template.New("nginx").Parse(tmplStr)
	if err != nil {
		return nil, err
	}
	return &Template{tmpl: tmpl}, nil
}

// Render renders the template with the given data
func (t *Template) Render(hosts map[string]*host.Host, cfg *config.Config) (string, error) {
	data := struct {
		Hosts  map[string]*host.Host
		Config *config.Config
	}{
		Hosts:  hosts,
		Config: cfg,
	}

	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
