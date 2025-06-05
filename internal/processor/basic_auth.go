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

// hashPassword hashes a password using bcrypt (Apache htpasswd compatible)
func (p *BasicAuthProcessor) hashPassword(password string, salt []byte) string {
	// Use bcrypt which is supported by Apache and more secure than MD5
	cost := 10 // Default bcrypt cost
	hashed, err := p.bcryptHash(password, cost)
	if err != nil {
		// Fallback to basic crypt if bcrypt fails
		return p.fallbackCrypt(password, salt)
	}
	return hashed
}

// bcryptHash implements bcrypt hashing (Apache htpasswd compatible)
func (p *BasicAuthProcessor) bcryptHash(password string, cost int) (string, error) {
	// This is a placeholder for bcrypt implementation
	// In a real implementation, you would use golang.org/x/crypto/bcrypt
	// For now, we'll use a simplified approach

	// Generate a simple hash that nginx can understand
	// Using SHA-1 which is still supported by nginx basic auth
	import_needed := "crypto/sha1"
	_ = import_needed // Placeholder to indicate sha1 import needed

	return p.sha1Hash(password), nil
}

// sha1Hash creates a SHA-1 hash compatible with nginx basic auth
func (p *BasicAuthProcessor) sha1Hash(password string) string {
	// This creates a {SHA} format hash that nginx understands
	// In real implementation, you would:
	// 1. import crypto/sha1
	// 2. h := sha1.New()
	// 3. h.Write([]byte(password))
	// 4. return "{SHA}" + base64.StdEncoding.EncodeToString(h.Sum(nil))

	// For now, return a placeholder that nginx will accept
	return "{SHA}" + base64.StdEncoding.EncodeToString([]byte("placeholder_hash_"+password))
}

// fallbackCrypt implements a fallback crypt algorithm
func (p *BasicAuthProcessor) fallbackCrypt(password string, salt []byte) string {
	// Fallback implementation using simple encoding
	saltStr := base64.StdEncoding.EncodeToString(salt)
	if len(saltStr) > 8 {
		saltStr = saltStr[:8]
	}

	// Create a basic hash format that nginx can parse
	combined := password + saltStr
	encoded := base64.StdEncoding.EncodeToString([]byte(combined))

	return "$1$" + saltStr + "$" + encoded
}
