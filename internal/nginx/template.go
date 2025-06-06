package nginx

import (
	"bytes"
	"fmt"
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

// BasicAuthDirectives returns the basic auth directives for a host
func BasicAuthDirectives(host *host.Host) string {
	if !host.BasicAuth {
		return ""
	}

	authFile := host.Extras.Get("auth_file")
	if authFile == nil {
		return ""
	}

	return generateBasicAuthConfig(authFile.(string))
}

// LocationBasicAuthDirectives returns the basic auth directives for a location
func LocationBasicAuthDirectives(location *host.Location) string {
	if !location.BasicAuth {
		return ""
	}

	authFile := location.Extras.Get("auth_file")
	if authFile == nil {
		return ""
	}

	return generateLocationBasicAuthConfig(location.Path, authFile.(string))
}

func generateBasicAuthConfig(authFile string) string {
	return fmt.Sprintf(`
auth_basic "Restricted Access";
auth_basic_user_file %s;
auth_basic_hash_type bcrypt;`, authFile)
}

func generateLocationBasicAuthConfig(path string, authFile string) string {
	return fmt.Sprintf(`
location %s {
    auth_basic "Restricted Access";
    auth_basic_user_file %s;
    auth_basic_hash_type bcrypt;
}`, path, authFile)
}
