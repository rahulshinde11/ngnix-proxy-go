package processor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rahulshinde/nginx-proxy-go/internal/host"
	"github.com/rahulshinde/nginx-proxy-go/internal/logger"
)

// RedirectProcessor handles PROXY_FULL_REDIRECT processing
type RedirectProcessor struct {
	logger *logger.Logger
}

// NewRedirectProcessor creates a new redirect processor
func NewRedirectProcessor(logger *logger.Logger) *RedirectProcessor {
	return &RedirectProcessor{
		logger: logger,
	}
}

// ProcessRedirection processes PROXY_FULL_REDIRECT environment variables
func (rp *RedirectProcessor) ProcessRedirection(env map[string]string, hosts map[string]*host.Host) error {
	// Find all PROXY_FULL_REDIRECT variables
	var redirectRules []string
	for key, value := range env {
		if strings.HasPrefix(key, "PROXY_FULL_REDIRECT") {
			redirectRules = append(redirectRules, value)
		}
	}

	if len(redirectRules) == 0 {
		return nil
	}

	// Count existing hosts to determine if single host
	singleHost := len(hosts) == 1
	var targetHost *host.Host
	if singleHost {
		for _, h := range hosts {
			targetHost = h
			break
		}
	}

	for _, rule := range redirectRules {
		if err := rp.processRedirectRule(rule, hosts, singleHost, targetHost); err != nil {
			rp.logger.Error("Failed to process redirect rule '%s': %v", rule, err)
		}
	}

	return nil
}

// processRedirectRule processes a single redirect rule
func (rp *RedirectProcessor) processRedirectRule(rule string, hosts map[string]*host.Host, singleHost bool, targetHost *host.Host) error {
	// Remove whitespace
	rule = regexp.MustCompile(`\s+`).ReplaceAllString(rule, "")

	// Split by ->
	parts := strings.Split(rule, "->")
	if len(parts) != 2 {
		return fmt.Errorf("invalid redirect rule format: %s", rule)
	}

	sourcesStr, targetStr := parts[0], parts[1]

	// Parse target
	target, err := rp.parseURL(targetStr, 80)
	if err != nil {
		return fmt.Errorf("invalid target URL: %v", err)
	}

	// If single host and target hostname is empty, use the single host
	if singleHost && target.Hostname == "" && targetHost != nil {
		target.Hostname = targetHost.Hostname
		target.Port = targetHost.Port
	} else if target.Hostname == "" {
		return fmt.Errorf("unknown target to redirect with PROXY_FULL_REDIRECT: %s", rule)
	}

	// Parse sources
	sources := strings.Split(sourcesStr, ",")
	for _, sourceStr := range sources {
		source, err := rp.parseURL(sourceStr, 80)
		if err != nil {
			rp.logger.Error("Invalid source URL '%s': %v", sourceStr, err)
			continue
		}

		if source.Hostname == "" {
			continue
		}

		// Create or update redirect host
		hostKey := fmt.Sprintf("%s:%d", source.Hostname, source.Port)

		if existingHost, exists := hosts[hostKey]; exists {
			// Update existing host with redirect
			existingHost.SetRedirect(target.Hostname)
		} else {
			// Create new redirect host
			redirectHost := host.NewHost(source.Hostname, source.Port)
			redirectHost.SetRedirect(target.Hostname)
			hosts[hostKey] = redirectHost
		}

		// Ensure target host exists
		targetKey := fmt.Sprintf("%s:%d", target.Hostname, target.Port)
		if _, exists := hosts[targetKey]; !exists {
			hosts[targetKey] = host.NewHost(target.Hostname, target.Port)
		}

		rp.logger.Info("Added redirect: %s:%d -> %s:%d",
			source.Hostname, source.Port, target.Hostname, target.Port)
	}

	return nil
}

// URL represents a parsed URL
type URL struct {
	Hostname string
	Port     int
}

// parseURL parses a URL string and returns hostname and port
func (rp *RedirectProcessor) parseURL(urlStr string, defaultPort int) (*URL, error) {
	url := &URL{Port: defaultPort}

	// Remove protocol if present
	if strings.Contains(urlStr, "://") {
		parts := strings.SplitN(urlStr, "://", 2)
		if len(parts) == 2 {
			urlStr = parts[1]
		}
	}

	// Split hostname and port
	if strings.Contains(urlStr, ":") {
		parts := strings.Split(urlStr, ":")
		if len(parts) >= 2 {
			url.Hostname = parts[0]
			if port, err := strconv.Atoi(parts[1]); err == nil {
				url.Port = port
			}
		}
	} else {
		url.Hostname = urlStr
	}

	return url, nil
}
