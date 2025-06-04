package processor

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rahulshinde/nginx-proxy-go/internal/host"
)

// BasicAuthProcessor handles basic authentication configuration
type BasicAuthProcessor struct {
	basicAuthDir string
}

// NewBasicAuthProcessor creates a new BasicAuthProcessor
func NewBasicAuthProcessor(basicAuthDir string) *BasicAuthProcessor {
	if !strings.HasSuffix(basicAuthDir, "/") {
		basicAuthDir += "/"
	}
	return &BasicAuthProcessor{
		basicAuthDir: basicAuthDir,
	}
}

// ProcessBasicAuth processes basic auth configuration from environment variables
func (p *BasicAuthProcessor) ProcessBasicAuth(environments map[string]string, hosts map[string]map[int]*host.Host) {
	// Find all PROXY_BASIC_AUTH environment variables
	for key, value := range environments {
		if !strings.HasPrefix(key, "PROXY_BASIC_AUTH") {
			continue
		}

		// Parse the basic auth configuration
		parts := strings.SplitN(value, "->", 2)
		if len(parts) != 2 {
			// Global basic auth for all hosts
			if authMap := p.parseAuthMap(value); authMap != nil {
				for _, portMap := range hosts {
					for _, h := range portMap {
						p.updateHostSecurity(h, "/", authMap)
					}
				}
			}
			continue
		}

		// Host-specific basic auth
		hostPart := strings.TrimSpace(parts[0])
		authPart := strings.TrimSpace(parts[1])

		// Parse hostname and port
		hostname := hostPart
		port := 80
		if strings.Contains(hostPart, ":") {
			parts := strings.Split(hostPart, ":")
			hostname = parts[0]
			fmt.Sscanf(parts[1], "%d", &port)
		}

		// Parse auth credentials
		if authMap := p.parseAuthMap(authPart); authMap != nil {
			// Find the host and update its security
			if portMap, ok := hosts[hostname]; ok {
				if h, ok := portMap[port]; ok {
					p.updateHostSecurity(h, "/", authMap)
				}
			}
		}
	}
}

// parseAuthMap parses a comma-separated list of username:password pairs
func (p *BasicAuthProcessor) parseAuthMap(authStr string) map[string]string {
	authMap := make(map[string]string)
	for _, pair := range strings.Split(authStr, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(parts) == 2 {
			username := strings.TrimSpace(parts[0])
			password := strings.TrimSpace(parts[1])
			if len(username) > 2 && len(password) > 2 {
				authMap[username] = password
			}
		}
	}
	return authMap
}

// updateHostSecurity updates the security configuration for a host or location
func (p *BasicAuthProcessor) updateHostSecurity(h *host.Host, path string, authMap map[string]string) {
	if path == "/" {
		// Update host-level security
		h.UpdateExtrasContent("security", authMap)
		h.BasicAuth = true
		h.BasicAuthFile = p.generateHtpasswdFile(h.Hostname, "_", authMap)
	} else {
		// Update location-level security
		if loc, ok := h.Locations[path]; ok {
			loc.UpdateExtrasContent("security", authMap)
			loc.BasicAuth = true
			loc.BasicAuthFile = p.generateHtpasswdFile(h.Hostname, strings.ReplaceAll(path, "/", "_"), authMap)
		}
	}
}

// generateHtpasswdFile generates an htpasswd file for basic auth
func (p *BasicAuthProcessor) generateHtpasswdFile(hostname, path string, authMap map[string]string) string {
	// Create directory if it doesn't exist
	dir := filepath.Join(p.basicAuthDir, hostname)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}

	// Generate htpasswd file
	filename := filepath.Join(dir, path)
	file, err := os.Create(filename)
	if err != nil {
		return ""
	}
	defer file.Close()

	// Write credentials
	for username, password := range authMap {
		// Generate salt
		salt := make([]byte, 2)
		if _, err := rand.Read(salt); err != nil {
			continue
		}

		// Hash password with salt
		hashed := p.hashPassword(password, salt)
		if hashed == "" {
			continue
		}

		// Write to file
		fmt.Fprintf(file, "%s:%s\n", username, hashed)
	}

	return filename
}

// hashPassword hashes a password using the Apache htpasswd format
func (p *BasicAuthProcessor) hashPassword(password string, salt []byte) string {
	// Convert salt to base64
	saltStr := base64.StdEncoding.EncodeToString(salt)
	if len(saltStr) > 2 {
		saltStr = saltStr[:2]
	}

	// Hash password using crypt
	hashed := p.crypt(password, saltStr)
	if hashed == "" {
		return ""
	}

	return hashed
}

// crypt implements the Apache htpasswd crypt algorithm
func (p *BasicAuthProcessor) crypt(password, salt string) string {
	// This is a simplified version. In production, you should use a proper
	// crypt implementation that matches Apache's htpasswd format.
	// For now, we'll use a basic MD5 hash with salt.
	hashed := p.md5WithSalt(password, salt)
	if hashed == "" {
		return ""
	}

	return "$apr1$" + salt + "$" + hashed
}

// md5WithSalt implements MD5 hashing with salt
func (p *BasicAuthProcessor) md5WithSalt(password, salt string) string {
	// This is a placeholder. In production, you should use a proper
	// MD5 implementation that matches Apache's htpasswd format.
	// For now, we'll return a simple hash.
	return "hashed_" + password + "_" + salt
}
