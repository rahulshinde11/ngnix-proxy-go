package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/client"
	"github.com/rahulshinde/nginx-proxy-go/internal/host"
)

// WebServer represents the main server that manages nginx configuration
type WebServer struct {
	client             *client.Client
	config             map[string]string
	configData         *host.ProxyConfigData
	basicAuthProcessor *host.BasicAuthProcessor
}

// NewWebServer creates a new WebServer instance
func NewWebServer(dockerClient *client.Client) (*WebServer, error) {
	// Get nginx conf directory
	nginxConfDir := os.Getenv("NGINX_CONF_DIR")
	if nginxConfDir == "" {
		nginxConfDir = "/etc/nginx"
	}

	// Create basic auth directory
	basicAuthDir := filepath.Join(nginxConfDir, "basic_auth")
	if err := os.MkdirAll(basicAuthDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create basic auth directory: %v", err)
	}

	// Create basic auth processor
	basicAuthProcessor, err := host.NewBasicAuthProcessor(basicAuthDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create basic auth processor: %v", err)
	}

	return &WebServer{
		client:             dockerClient,
		config:             make(map[string]string),
		configData:         host.NewProxyConfigData(),
		basicAuthProcessor: basicAuthProcessor,
	}, nil
}

// ProcessBasicAuth processes basic auth for all hosts
func (s *WebServer) ProcessBasicAuth() error {
	for _, host := range s.configData.HostList() {
		if err := s.basicAuthProcessor.ProcessBasicAuth(host); err != nil {
			return fmt.Errorf("failed to process basic auth for host %s: %v", host.Hostname, err)
		}
	}
	return nil
}

// UpdateContainer updates the configuration for a container
func (s *WebServer) UpdateContainer(containerID string) error {
	container, err := s.client.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return fmt.Errorf("failed to inspect container: %v", err)
	}

	// Process environment variables
	env := make(map[string]string)
	for k, v := range container.Config.Labels {
		if strings.HasPrefix(k, "VIRTUAL_HOST") || k == "PROXY_BASIC_AUTH" {
			env[k] = v
		}
	}

	if len(env) == 0 {
		return nil
	}

	// Process virtual hosts
	for key, value := range env {
		if strings.HasPrefix(key, "VIRTUAL_HOST") {
			// Parse virtual host configuration
			vhost, err := host.ParseVirtualHost(value)
			if err != nil {
				return fmt.Errorf("failed to parse virtual host: %v", err)
			}

			// Create or update host
			h := host.NewHost(vhost.Hostname, vhost.ServerPort)
			h.SetSSL(vhost.Scheme == "https", "")

			// Process basic auth if present
			if authConfig, ok := env["PROXY_BASIC_AUTH"]; ok {
				if err := h.ProcessBasicAuthConfig(authConfig); err != nil {
					return fmt.Errorf("failed to process basic auth: %v", err)
				}
			}

			// Add host to configuration
			s.configData.AddHost(h)
		}
	}

	// Process basic auth for all hosts
	if err := s.ProcessBasicAuth(); err != nil {
		return fmt.Errorf("failed to process basic auth: %v", err)
	}

	return nil
}
