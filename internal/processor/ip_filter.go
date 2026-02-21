package processor

import (
	"fmt"
	"net"
	"strings"

	"github.com/rahulshinde/nginx-proxy-go/internal/config"
	"github.com/rahulshinde/nginx-proxy-go/internal/host"
	"github.com/rahulshinde/nginx-proxy-go/internal/logger"
)

// IPFilterProcessor handles IP filtering and trusted proxy configuration
type IPFilterProcessor struct {
	globalIPs       []string
	globalHeader    string
	globalRecursive string
	log             *logger.Logger
}

// NewIPFilterProcessor creates a new IP filter processor from global config
func NewIPFilterProcessor(cfg *config.Config, log *logger.Logger) *IPFilterProcessor {
	validIPs, _ := ParseAndValidateCIDRs(strings.Join(cfg.TrustedProxyIPs, ","))

	return &IPFilterProcessor{
		globalIPs:       validIPs,
		globalHeader:    cfg.RealIPHeader,
		globalRecursive: cfg.RealIPRecursive,
		log:             log,
	}
}

// ProcessIPFilter applies IP filtering to hosts based on per-container or global config.
// Per-container config fully overrides global (not additive).
func (p *IPFilterProcessor) ProcessIPFilter(env map[string]string, hosts map[string]map[int]*host.Host) {
	// Determine effective IPs: per-container overrides global
	var effectiveIPs []string
	var effectiveHeader string
	effectiveRecursive := p.globalRecursive

	if perContainerIPs, ok := env["PROXY_TRUSTED_IPS"]; ok && perContainerIPs != "" {
		parsed, err := ParseAndValidateCIDRs(perContainerIPs)
		if err != nil {
			p.log.Warn("Invalid PROXY_TRUSTED_IPS: %v", err)
		}
		effectiveIPs = parsed
	} else {
		effectiveIPs = p.globalIPs
	}

	if perContainerHeader, ok := env["PROXY_REAL_IP_HEADER"]; ok && perContainerHeader != "" {
		effectiveHeader = perContainerHeader
	} else {
		effectiveHeader = p.globalHeader
	}

	// If no IPs configured, nothing to do
	if len(effectiveIPs) == 0 {
		return
	}

	// RealIPHeader requires TrustedProxyIPs — if no header, we still set allow/deny
	// but skip set_real_ip_from/real_ip_header directives
	realIPHeader := ""
	if effectiveHeader != "" {
		realIPHeader = effectiveHeader
	}

	// Apply to all hosts
	for _, portMap := range hosts {
		for _, h := range portMap {
			h.SetIPFilter(effectiveIPs, true, realIPHeader, effectiveRecursive)
		}
	}
}

// ParseAndValidateCIDRs parses a comma-separated list of CIDR ranges.
// Bare IPs (without mask) are auto-converted to /32 (IPv4) or /128 (IPv6).
// Returns the list of validated CIDR strings and any error encountered.
func ParseAndValidateCIDRs(cidrList string) ([]string, error) {
	if strings.TrimSpace(cidrList) == "" {
		return nil, nil
	}

	parts := strings.Split(cidrList, ",")
	result := make([]string, 0, len(parts))
	var errs []string

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// If it contains a /, try parsing as CIDR
		if strings.Contains(part, "/") {
			_, _, err := net.ParseCIDR(part)
			if err != nil {
				errs = append(errs, fmt.Sprintf("invalid CIDR %q: %v", part, err))
				continue
			}
			result = append(result, part)
		} else {
			// Bare IP — convert to /32 or /128
			ip := net.ParseIP(part)
			if ip == nil {
				errs = append(errs, fmt.Sprintf("invalid IP %q", part))
				continue
			}
			if ip.To4() != nil {
				result = append(result, part+"/32")
			} else {
				result = append(result, part+"/128")
			}
		}
	}

	if len(errs) > 0 && len(result) == 0 {
		return nil, fmt.Errorf("all entries invalid: %s", strings.Join(errs, "; "))
	}

	return result, nil
}
