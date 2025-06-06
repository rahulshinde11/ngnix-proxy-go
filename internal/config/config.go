package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
}

// NewConfig creates a new Config instance with values from environment variables
func NewConfig() *Config {
	cfg := &Config{
		// Nginx configuration
		ConfDir:           getEnv("NGINX_CONF_DIR", "./nginx"),
		ChallengeDir:      getEnv("CHALLENGE_DIR", "./acme-challenges"),
		SSLDir:            getEnv("SSL_DIR", "./ssl"),
		ClientMaxBodySize: getEnv("CLIENT_MAX_BODY_SIZE", "1m"),
		DefaultServer:     getEnvBool("DEFAULT_HOST", true),

		// Basic auth configuration
		BasicAuthEnabled: false,
		BasicAuthFile:    filepath.Join(getEnv("NGINX_CONF_DIR", "/etc/nginx"), "basic_auth"),

		// Debug configuration
		DebugEnabled: getEnvBool("GO_DEBUG_ENABLE", false),
		DebugPort:    getEnvInt("GO_DEBUG_PORT", 2345),
		DebugHost:    getEnv("GO_DEBUG_HOST", ""),
	}

	// Ensure directories end with a slash
	cfg.ConfDir = ensureTrailingSlash(cfg.ConfDir)
	cfg.ChallengeDir = ensureTrailingSlash(cfg.ChallengeDir)
	cfg.SSLDir = ensureTrailingSlash(cfg.SSLDir)

	return cfg
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

// ensureTrailingSlash ensures a path ends with a slash
func ensureTrailingSlash(path string) string {
	if !strings.HasSuffix(path, "/") {
		return path + "/"
	}
	return path
}
