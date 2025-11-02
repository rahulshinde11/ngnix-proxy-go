package constants

import "time"

// Network ports
const (
	DefaultHTTPPort  = 80
	DefaultHTTPSPort = 443
	MinValidPort     = 1
	MaxValidPort     = 65535
)

// SSL/TLS configuration
const (
	DefaultDHParamSize           = 2048
	DefaultKeySize               = 2048
	CertificateRenewalThreshold  = 7 * 24 * time.Hour  // Renew 7 days before expiry
	CertificateCheckInterval     = 24 * time.Hour      // Check certificates daily
	MinCertificateValidityDays   = 2                   // Minimum days before considering invalid
)

// ACME/Let's Encrypt
const (
	LetsEncryptProductionAPI = "https://acme-v02.api.letsencrypt.org/directory"
	LetsEncryptStagingAPI    = "https://acme-staging-v02.api.letsencrypt.org/directory"
	ACMETimeout              = 30 * time.Second
)

// Nginx configuration
const (
	DefaultClientMaxBodySize = "1m"
	NginxReloadTimeout       = 10 * time.Second
	NginxConfigTestTimeout   = 5 * time.Second
)

// Debug configuration
const (
	DefaultDebugPort = 2345
)

// Event processing
const (
	EventChannelBufferSize = 100
	EventProcessTimeout    = 30 * time.Second
)

// Docker
const (
	DefaultDockerTimeout     = 30 * time.Second
	ContainerInspectTimeout  = 10 * time.Second
	NetworkInspectTimeout    = 10 * time.Second
)

// Retry configuration
const (
	DefaultRetryAttempts = 3
	DefaultRetryDelay    = 1 * time.Second
	DefaultMaxRetryDelay = 30 * time.Second
	DefaultBackoffFactor = 2.0
)

// File permissions
const (
	DirPermissions        = 0755
	ConfigFilePermissions = 0644
	PrivateKeyPermissions = 0600
)

// Default network
const (
	DefaultNetworkName = "frontend"
)
