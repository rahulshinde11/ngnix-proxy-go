package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rahulshinde/nginx-proxy-go/internal/constants"
)

// Config represents the application configuration
type Config struct {
	// Nginx configuration
	ConfDir           string
	ChallengeDir      string
	SSLDir            string
	ClientMaxBodySize string
	DefaultServer     bool

	// Basic auth configuration
	BasicAuthEnabled bool
	BasicAuthFile    string

	// Debug configuration
	DebugEnabled bool
	DebugPort    int
	DebugHost    string

	// IP filtering / trusted proxy configuration
	TrustedProxyIPs []string // From TRUSTED_PROXY_IPS
	RealIPHeader    string   // From REAL_IP_HEADER
	RealIPRecursive string   // From REAL_IP_RECURSIVE (default "on")
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation failed for %s: %s", e.Field, e.Message)
}

// NewConfig creates a new Config instance with values from environment variables
func NewConfig() *Config {
	cfg := &Config{
		// Nginx configuration
		ConfDir:           getEnv("NGINX_CONF_DIR", "./nginx"),
		ChallengeDir:      getEnv("CHALLENGE_DIR", "./acme-challenges"),
		SSLDir:            getEnv("SSL_DIR", "./ssl"),
		ClientMaxBodySize: getEnv("CLIENT_MAX_BODY_SIZE", constants.DefaultClientMaxBodySize),
		DefaultServer:     getEnvBool("DEFAULT_HOST", true),

		// Basic auth configuration
		BasicAuthEnabled: false,
		BasicAuthFile:    filepath.Join(getEnv("NGINX_CONF_DIR", "/etc/nginx"), "basic_auth"),

		// Debug configuration
		DebugEnabled: getEnvBool("GO_DEBUG_ENABLE", false),
		DebugPort:    getEnvInt("GO_DEBUG_PORT", constants.DefaultDebugPort),
		DebugHost:    getEnv("GO_DEBUG_HOST", ""),

		// IP filtering / trusted proxy
		TrustedProxyIPs: parseCommaSeparated(os.Getenv("TRUSTED_PROXY_IPS")),
		RealIPHeader:    getEnv("REAL_IP_HEADER", ""),
		RealIPRecursive: getEnv("REAL_IP_RECURSIVE", "on"),
	}

	// Ensure directories end with a slash
	cfg.ConfDir = ensureTrailingSlash(cfg.ConfDir)
	cfg.ChallengeDir = ensureTrailingSlash(cfg.ChallengeDir)
	cfg.SSLDir = ensureTrailingSlash(cfg.SSLDir)

	return cfg
}

// Validate validates the configuration and returns an error if invalid
func (c *Config) Validate() error {
	// Validate debug port
	if c.DebugPort < constants.MinValidPort || c.DebugPort > constants.MaxValidPort {
		return &ValidationError{
			Field:   "DebugPort",
			Message: fmt.Sprintf("must be between %d and %d, got %d", constants.MinValidPort, constants.MaxValidPort, c.DebugPort),
		}
	}

	// Validate directories exist or can be created
	dirs := map[string]string{
		"ConfDir":      c.ConfDir,
		"ChallengeDir": c.ChallengeDir,
		"SSLDir":       c.SSLDir,
	}

	for name, dir := range dirs {
		if err := validateDirectory(dir); err != nil {
			return &ValidationError{
				Field:   name,
				Message: fmt.Sprintf("invalid directory: %v", err),
			}
		}
	}

	// Validate ClientMaxBodySize format (basic check)
	if c.ClientMaxBodySize == "" {
		return &ValidationError{
			Field:   "ClientMaxBodySize",
			Message: "cannot be empty",
		}
	}

	return nil
}

// validateDirectory checks if a directory exists or can be created
func validateDirectory(dir string) error {
	// Remove trailing slash for stat
	dir = strings.TrimSuffix(dir, "/")
	
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Try to create the directory
			if err := os.MkdirAll(dir, constants.DirPermissions); err != nil {
				return fmt.Errorf("cannot create directory: %w", err)
			}
			return nil
		}
		return fmt.Errorf("cannot access directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path exists but is not a directory")
	}

	// Check if directory is writable by trying to create a temp file
	testFile := filepath.Join(dir, ".write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("directory is not writable: %w", err)
	}
	f.Close()
	os.Remove(testFile)

	return nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvBool gets a boolean environment variable or returns a default value
func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		return value == "true"
	}
	return defaultValue
}

// getEnvInt gets an integer environment variable or returns a default value
func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}

// parseCommaSeparated splits a comma-separated string into trimmed, non-empty parts
func parseCommaSeparated(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// ensureTrailingSlash ensures a path ends with a slash
func ensureTrailingSlash(path string) string {
	if !strings.HasSuffix(path, "/") {
		return path + "/"
	}
	return path
}
