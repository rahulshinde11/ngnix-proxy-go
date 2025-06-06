package nginx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// Nginx represents an nginx server instance
type Nginx struct {
	confFile     string
	challengeDir string
	lastConfig   string
}

// NginxConfig represents the configuration for nginx.
type NginxConfig struct {
	ChallengeDir string
}

// NewNginx creates a new Nginx instance
func NewNginx(confFile, challengeDir string) *Nginx {
	return &Nginx{
		confFile:     confFile,
		challengeDir: challengeDir,
	}
}

// UpdateConfig updates the nginx configuration and reloads the server
func (n *Nginx) UpdateConfig(config string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(n.confFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Write configuration
	if err := os.WriteFile(n.confFile, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	// Test configuration
	if err := n.configTest(); err != nil {
		return fmt.Errorf("nginx config test failed: %v", err)
	}

	// Reload nginx
	if err := n.reload(); err != nil {
		return fmt.Errorf("failed to reload nginx: %v", err)
	}

	n.lastConfig = config
	return nil
}

// ForceStart forces nginx to start with the given configuration
func (n *Nginx) ForceStart(config string) bool {
	// Create directory if it doesn't exist
	dir := filepath.Dir(n.confFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false
	}

	// Write configuration
	if err := os.WriteFile(n.confFile, []byte(config), 0644); err != nil {
		return false
	}

	// Start nginx
	cmd := exec.Command("nginx", "-g", "daemon off;")
	if err := cmd.Start(); err != nil {
		return false
	}

	n.lastConfig = config
	return true
}

// configTest tests the nginx configuration
func (n *Nginx) configTest() error {
	cmd := exec.Command("nginx", "-t")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nginx config test failed: %s", string(output))
	}
	return nil
}

// reload reloads the nginx configuration
func (n *Nginx) reload() error {
	cmd := exec.Command("nginx", "-s", "reload")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reload nginx: %s", string(output))
	}
	return nil
}

// RenderTemplate renders an nginx configuration template
func (n *Nginx) RenderTemplate(tmpl *template.Template, data interface{}) (string, error) {
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render template: %v", err)
	}
	return buf.String(), nil
}

// GenerateConfig generates the nginx configuration using a template.
func (n *Nginx) GenerateConfig(config NginxConfig) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(n.confFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Create a minimal default configuration
	defaultConfig := fmt.Sprintf(`
server {
    listen 80 default_server;
    server_name _;
    location /.well-known/acme-challenge/ {
        alias %s;
        try_files $uri =404;
    }
    location / {
        return 503;
    }
}
`, config.ChallengeDir)

	// Write the configuration
	return os.WriteFile(n.confFile, []byte(defaultConfig), 0644)
}
