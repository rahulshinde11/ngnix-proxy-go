package processor

import (
	"strings"

	"github.com/rahulshinde/nginx-proxy-go/internal/host"
	"github.com/rahulshinde/nginx-proxy-go/internal/logger"
)

// DefaultServerProcessor handles PROXY_DEFAULT_SERVER processing
type DefaultServerProcessor struct {
	logger *logger.Logger
}

// NewDefaultServerProcessor creates a new default server processor
func NewDefaultServerProcessor(logger *logger.Logger) *DefaultServerProcessor {
	return &DefaultServerProcessor{
		logger: logger,
	}
}

// ProcessDefaultServer processes PROXY_DEFAULT_SERVER environment variables
func (dsp *DefaultServerProcessor) ProcessDefaultServer(env map[string]string, hosts map[string]*host.Host) error {
	// Check for PROXY_DEFAULT_SERVER environment variable
	var hasDefaultServer bool
	for key, value := range env {
		if key == "PROXY_DEFAULT_SERVER" && strings.ToLower(value) == "true" {
			hasDefaultServer = true
			break
		}
	}

	if !hasDefaultServer {
		return nil
	}

	// Find the first host to make it the default server
	// In the Python version, only one host should be marked as default
	var defaultFound bool
	for _, h := range hosts {
		if !defaultFound {
			h.IsDefaultServer = true
			defaultFound = true
			dsp.logger.Info("Set default server: %s", h.Hostname)
		} else {
			// Ensure only one default server
			h.IsDefaultServer = false
		}
	}

	return nil
}
