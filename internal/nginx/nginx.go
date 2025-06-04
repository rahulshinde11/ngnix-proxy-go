package nginx

import (
	"bytes"
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
	SSLConfig struct {
		Ciphers string
		// Add other SSL-related fields as needed
	}
	// Add other configuration fields as needed
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
	tmpl, err := template.New("nginx").Parse(`
# WebSocket support
map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}

# Proxy settings
proxy_cache off;
proxy_request_buffering off;
proxy_http_version 1.1;
proxy_buffering off;
proxy_set_header Host $http_host;
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
proxy_set_header X-Forwarded-Proto $proxy_x_forwarded_proto;
proxy_set_header X-Forwarded-Ssl $proxy_x_forwarded_ssl;
proxy_set_header X-Forwarded-Port $proxy_x_forwarded_port;
proxy_set_header Proxy "";

# Forwarded headers mapping
map $http_x_forwarded_proto $proxy_x_forwarded_proto {
    default $http_x_forwarded_proto;
    ''      $scheme;
}

map $http_x_forwarded_port $proxy_x_forwarded_port {
    default $http_x_forwarded_port;
    ''      $server_port;
}

map $scheme $proxy_x_forwarded_ssl {
    default off;
    https on;
}

# Server configuration
server {
    listen 80;
    listen [::]:80;
    server_name _;

    # Redirect all HTTP traffic to HTTPS
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name _;

    # SSL certificate configuration
    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header X-Content-Type-Options "nosniff" always;

    # WebSocket support
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_read_timeout 1h;
    proxy_send_timeout 1h;

    # Default location
    location / {
        proxy_pass http://backend;
    }

    # ACME challenge location
    location /.well-known/acme-challenge/ {
        alias {{ .Config.ChallengeDir }};
        try_files $uri =404;
    }
}
`)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, config); err != nil {
		return err
	}

	// Write the generated configuration to a file
	return os.WriteFile("/etc/nginx/conf.d/default.conf", buf.Bytes(), 0644)
}
