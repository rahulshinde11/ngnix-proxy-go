package host

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BasicAuthProcessor handles basic auth configuration
type BasicAuthProcessor struct {
	basicAuthDir string
	cache        map[string]string
}

// NewBasicAuthProcessor creates a new BasicAuthProcessor
func NewBasicAuthProcessor(basicAuthDir string) (*BasicAuthProcessor, error) {
	if basicAuthDir == "" {
		basicAuthDir = "/etc/nginx/basic_auth"
	}

	// Create the basic auth directory if it doesn't exist
	if err := os.MkdirAll(basicAuthDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create basic auth directory: %v", err)
	}

	log.Printf("Basic auth directory created at: %s", basicAuthDir)
	return &BasicAuthProcessor{
		basicAuthDir: basicAuthDir,
		cache:        make(map[string]string),
	}, nil
}

// generateSalt generates a random salt for password hashing
func (p *BasicAuthProcessor) generateSalt() (string, error) {
	salt := make([]byte, 2)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(salt), nil
}

// generateHtpasswdFile generates an htpasswd file for the given credentials
func (p *BasicAuthProcessor) generateHtpasswdFile(hostname, path string, credentials map[string]string) (string, error) {
	// Create host directory if it doesn't exist
	hostDir := filepath.Join(p.basicAuthDir, hostname)
	if err := os.MkdirAll(hostDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create host directory: %v", err)
	}

	// Generate filename based on path
	filename := "_"
	if path != "" && path != "/" {
		filename = strings.ReplaceAll(path, "/", "_")
	}
	authFile := filepath.Join(hostDir, filename+".htpasswd")

	// Create or update the htpasswd file
	for username, password := range credentials {
		// Use htpasswd command to add/update user
		cmd := exec.Command("htpasswd", "-b", authFile, username, password)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("failed to create htpasswd file: %v", err)
		}
		log.Printf("Created htpasswd file for %s at %s with user %s", hostname, authFile, username)
	}

	return authFile, nil
}

// ProcessBasicAuth processes the basic auth configuration for a host
func (p *BasicAuthProcessor) ProcessBasicAuth(host *Host) error {
	// Process host-level basic auth
	if authConfig, ok := host.Extras.Get("security").(map[string]string); ok {
		log.Printf("Processing basic auth for host %s", host.Hostname)
		authFile, err := p.generateHtpasswdFile(host.Hostname, "", authConfig)
		if err != nil {
			return err
		}
		host.SetBasicAuth(true, authFile)
		log.Printf("Basic auth enabled for host %s with file %s", host.Hostname, authFile)
	}

	// Process location-level basic auth
	for path, location := range host.Locations {
		if authConfig, ok := location.Extras.Get("security").(map[string]string); ok {
			log.Printf("Processing basic auth for location %s on host %s", path, host.Hostname)
			authFile, err := p.generateHtpasswdFile(host.Hostname, path, authConfig)
			if err != nil {
				return err
			}
			host.SetLocationBasicAuth(path, true, authFile)
			log.Printf("Basic auth enabled for location %s on host %s with file %s", path, host.Hostname, authFile)
		}
	}

	return nil
}

// ProcessBasicAuthConfig processes the PROXY_BASIC_AUTH environment variable
func ProcessBasicAuthConfig(authConfig string, hostname string) (map[string]string, error) {
	if authConfig == "" {
		return nil, nil
	}

	log.Printf("Processing basic auth config: %s for hostname: %s", authConfig, hostname)

	// Parse the auth configuration
	// Format: "hostname -> user:pass" or "hostname/path -> user:pass"
	parts := strings.Split(authConfig, "->")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid basic auth format: %s", authConfig)
	}

	// Extract the target hostname/path and credentials
	target := strings.TrimSpace(parts[0])
	credentials := strings.TrimSpace(parts[1])

	// Check if the auth is for this hostname
	if !strings.Contains(target, hostname) {
		log.Printf("Basic auth config %s does not match hostname %s", target, hostname)
		return nil, nil
	}

	// Parse credentials
	credParts := strings.Split(credentials, ":")
	if len(credParts) != 2 {
		return nil, fmt.Errorf("invalid credentials format: %s", credentials)
	}

	log.Printf("Basic auth enabled for hostname %s with user %s", hostname, credParts[0])
	return map[string]string{
		credParts[0]: credParts[1],
	}, nil
}
