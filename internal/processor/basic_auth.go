package processor

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rahulshinde/nginx-proxy-go/internal/host"
	"golang.org/x/crypto/bcrypt"
)

// Credential represents basic auth credentials
type Credential struct {
	Username string
	Password string
	Path     string
}

// BasicAuthProcessor handles basic auth configuration
type BasicAuthProcessor struct {
	basicAuthDir string
	credentials  map[string]map[string]Credential // host -> path -> credential
}

// NewBasicAuthProcessor creates a new basic auth processor
func NewBasicAuthProcessor(basicAuthDir string) *BasicAuthProcessor {
	log.Printf("Initializing basic auth processor with directory: %s", basicAuthDir)
	return &BasicAuthProcessor{
		basicAuthDir: basicAuthDir,
		credentials:  make(map[string]map[string]Credential),
	}
}

// ProcessBasicAuth processes basic auth configuration from environment variables
func (p *BasicAuthProcessor) ProcessBasicAuth(environments map[string]string, hosts map[string]map[int]*host.Host) error {
	// Find basic auth config
	authConfig, ok := environments["PROXY_BASIC_AUTH"]
	if !ok {
		log.Println("No basic auth configuration found")
		return nil
	}

	// Remove surrounding quotes if present
	authConfig = strings.Trim(authConfig, `"`)
	log.Printf("Processing basic auth: %s", authConfig)

	// Parse URL and credentials
	parts := strings.Split(authConfig, " -> ")
	if len(parts) != 2 {
		return fmt.Errorf("invalid basic auth format: %s", authConfig)
	}

	// Parse URL
	urlStr := strings.TrimSpace(parts[0])
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL in basic auth config: %v", err)
	}

	hostname := parsedURL.Hostname()
	path := parsedURL.Path
	if path == "" {
		path = "/"
	}

	// Parse credentials
	credentials := strings.TrimSpace(parts[1])
	credParts := strings.Split(credentials, ":")
	if len(credParts) != 2 {
		return fmt.Errorf("invalid credentials format: %s", credentials)
	}

	username := strings.TrimSpace(credParts[0])
	password := strings.TrimSpace(credParts[1])

	// Validate credentials
	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters long")
	}
	if len(password) < 3 {
		return fmt.Errorf("password must be at least 3 characters long")
	}

	log.Printf("Configuring basic auth for %s: user=%s", hostname, username)

	// Store credentials
	if _, ok := p.credentials[hostname]; !ok {
		p.credentials[hostname] = make(map[string]Credential)
	}
	p.credentials[hostname][path] = Credential{
		Username: username,
		Password: password,
		Path:     path,
	}

	// Update host security
	return p.updateHostSecurity(hostname, path, hosts)
}

// updateHostSecurity updates the security configuration for a host
func (p *BasicAuthProcessor) updateHostSecurity(hostname string, path string, hosts map[string]map[int]*host.Host) error {
	log.Printf("Updating security for %s at %s", hostname, path)

	// Get host from hosts map
	hostMap, ok := hosts[hostname]
	if !ok {
		return fmt.Errorf("host not found: %s", hostname)
	}

	// Get the first host (they should all have the same configuration)
	var host *host.Host
	for _, h := range hostMap {
		host = h
		break
	}

	if host == nil {
		return fmt.Errorf("no host configuration found for: %s", hostname)
	}

	// Create basic auth directory if it doesn't exist
	if err := os.MkdirAll(p.basicAuthDir, 0755); err != nil {
		return fmt.Errorf("failed to create basic auth directory: %v", err)
	}

	// Generate htpasswd file
	htpasswdFile := filepath.Join(p.basicAuthDir, hostname+".htpasswd")
	authMap := make(map[string]string)
	for _, cred := range p.credentials[hostname] {
		authMap[cred.Username] = cred.Password
	}
	if err := p.generateHtpasswdFile(htpasswdFile, authMap); err != nil {
		return fmt.Errorf("failed to generate htpasswd file: %v", err)
	}

	log.Printf("Added credentials to %s", htpasswdFile)

	// Update host configuration
	if path == "/" {
		host.SetBasicAuth(true, htpasswdFile)
		log.Printf("Enabled basic auth for %s", hostname)
	} else {
		host.SetLocationBasicAuth(path, true, htpasswdFile)
		log.Printf("Enabled basic auth for %s at %s", hostname, path)
	}

	return nil
}

// randomSalt generates a random salt string of a given length
func randomSalt(length int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./"
	result := make([]byte, length)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		result[i] = letters[n.Int64()]
	}
	return string(result)
}

// generateHtpasswdFile generates an htpasswd file for basic auth
func (p *BasicAuthProcessor) generateHtpasswdFile(filename string, authMap map[string]string) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Printf("Failed to create directory %s: %v", dir, err)
		return err
	}
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Failed to create htpasswd file %s: %v", filename, err)
		return err
	}
	defer file.Close()
	for username, password := range authMap {
		// Generate bcrypt hash with cost 10 (good balance between security and performance)
		hash, err := bcrypt.GenerateFromPassword([]byte(password), 10)
		if err != nil {
			log.Printf("Failed to hash password for user %s: %v", username, err)
			continue
		}
		// Format: username:$2y$10$hash
		if _, err := fmt.Fprintf(file, "%s:%s\n", username, string(hash)); err != nil {
			log.Printf("Failed to write credentials for user %s: %v", username, err)
			return err
		}
		log.Printf("Added credentials for user %s to file %s", username, filename)
	}
	return nil
}
