package ssl

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rahulshinde/nginx-proxy-go/internal/acme"
)

// Logger interface for certificate manager
type Logger interface {
	Info(format string, args ...interface{})
	Error(format string, args ...interface{})
	Debug(format string, args ...interface{})
	Warn(format string, args ...interface{})
}

// CertificateOptions represents options for certificate operations
type CertificateOptions struct {
	Domain         string
	SkipDNSCheck   bool
	ForceNew       bool
	Force          bool
	CertPath       string
	KeyPath        string
	AccountKeyPath string
}

// CertificateManager manages SSL certificates
type CertificateManager struct {
	sslPath       string
	acmeManager   *acme.Manager
	logger        Logger
	certCache     map[string]time.Time
	blacklist     map[string]time.Time
	selfSigned    map[string]bool
	mu            sync.RWMutex
	renewalCtx    context.Context
	renewalCancel context.CancelFunc
	renewalWG     sync.WaitGroup
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager(sslPath string, acmeManager *acme.Manager, logger Logger) *CertificateManager {
	ctx, cancel := context.WithCancel(context.Background())

	cm := &CertificateManager{
		sslPath:       sslPath,
		acmeManager:   acmeManager,
		logger:        logger,
		certCache:     make(map[string]time.Time),
		blacklist:     make(map[string]time.Time),
		selfSigned:    make(map[string]bool),
		renewalCtx:    ctx,
		renewalCancel: cancel,
	}

	// Create necessary directories
	os.MkdirAll(filepath.Join(sslPath, "certs"), 0755)
	os.MkdirAll(filepath.Join(sslPath, "private"), 0755)
	os.MkdirAll(filepath.Join(sslPath, "accounts"), 0755)

	// Start renewal thread
	cm.startRenewalThread()

	return cm
}

// startRenewalThread starts the certificate renewal background thread
func (cm *CertificateManager) startRenewalThread() {
	cm.renewalWG.Add(1)
	go func() {
		defer cm.renewalWG.Done()
		cm.logger.Info("SSL certificate renewal thread started")

		ticker := time.NewTicker(24 * time.Hour) // Check daily
		defer ticker.Stop()

		for {
			select {
			case <-cm.renewalCtx.Done():
				cm.logger.Info("SSL certificate renewal thread stopped")
				return
			case <-ticker.C:
				cm.checkAndRenewCertificates()
			}
		}
	}()
}

// checkAndRenewCertificates checks and renews certificates that are about to expire
func (cm *CertificateManager) checkAndRenewCertificates() {
	cm.mu.RLock()
	var toRenew []string
	now := time.Now()

	for domain, expiry := range cm.certCache {
		daysRemaining := int(expiry.Sub(now).Hours() / 24)
		cm.logger.Info("Certificate for %s expires in %d days", domain, daysRemaining)

		if daysRemaining <= 7 { // Renew certificates expiring in 7 days or less
			toRenew = append(toRenew, domain)
		}
	}
	cm.mu.RUnlock()

	if len(toRenew) > 0 {
		cm.logger.Info("Renewing certificates for domains: %v", toRenew)
		for _, domain := range toRenew {
			if err := cm.renewCertificate(domain); err != nil {
				cm.logger.Error("Failed to renew certificate for %s: %v", domain, err)
			}
		}
	}
}

// GetCertificate gets or creates a certificate for the given domain
func (cm *CertificateManager) GetCertificate(domain string) (string, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Check if certificate exists and is valid
	if cm.certificateExists(domain) {
		expiry, err := cm.getCertificateExpiry(domain)
		if err == nil {
			daysRemaining := int(expiry.Sub(time.Now()).Hours() / 24)
			if daysRemaining > 2 {
				cm.certCache[domain] = expiry
				return domain, nil
			}
		}
	}

	// Check for wildcard certificate
	if wildcard := cm.getWildcardDomain(domain); wildcard != "" {
		if cm.certificateExists(wildcard) {
			return wildcard, nil
		}
	}

	// Check if domain is blacklisted
	if cm.isBlacklisted(domain) {
		cm.logger.Info(fmt.Sprintf("Domain %s is blacklisted, using self-signed certificate", domain))
		return cm.generateSelfSignedCertificate(domain)
	}

	// Try to get certificate from ACME
	if err := cm.obtainCertificate(domain); err != nil {
		cm.logger.Error(fmt.Sprintf("Failed to obtain certificate for %s: %v", domain, err))
		cm.addToBlacklist(domain, 3*time.Hour) // Blacklist for 3 hours
		return cm.generateSelfSignedCertificate(domain)
	}

	// Update cache
	if expiry, err := cm.getCertificateExpiry(domain); err == nil {
		cm.certCache[domain] = expiry
	}

	return domain, nil
}

// certificateExists checks if a certificate exists for the domain
func (cm *CertificateManager) certificateExists(domain string) bool {
	certPath := filepath.Join(cm.sslPath, "certs", domain+".crt")
	keyPath := filepath.Join(cm.sslPath, "private", domain+".key")

	_, err1 := os.Stat(certPath)
	_, err2 := os.Stat(keyPath)

	return err1 == nil && err2 == nil
}

// getCertificateExpiry gets the expiry time of a certificate
func (cm *CertificateManager) getCertificateExpiry(domain string) (time.Time, error) {
	certPath := filepath.Join(cm.sslPath, "certs", domain+".crt")

	certData, err := os.ReadFile(certPath)
	if err != nil {
		return time.Time{}, err
	}

	block, _ := pem.Decode(certData)
	if block == nil {
		return time.Time{}, fmt.Errorf("failed to decode certificate")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, err
	}

	return cert.NotAfter, nil
}

// getWildcardDomain returns the wildcard domain name if applicable
func (cm *CertificateManager) getWildcardDomain(domain string) string {
	parts := strings.Split(domain, ".")
	if len(parts) > 2 {
		return "*." + strings.Join(parts[1:], ".")
	}
	return ""
}

// isBlacklisted checks if a domain is blacklisted
func (cm *CertificateManager) isBlacklisted(domain string) bool {
	if expiry, exists := cm.blacklist[domain]; exists {
		if time.Now().Before(expiry) {
			return true
		}
		// Remove expired blacklist entry
		delete(cm.blacklist, domain)
	}
	return false
}

// addToBlacklist adds a domain to the blacklist for the specified duration
func (cm *CertificateManager) addToBlacklist(domain string, duration time.Duration) {
	cm.blacklist[domain] = time.Now().Add(duration)
	cm.logger.Info(fmt.Sprintf("Added %s to blacklist for %v", domain, duration))
}

// obtainCertificate obtains a certificate from ACME
func (cm *CertificateManager) obtainCertificate(domain string) error {
	cm.logger.Info(fmt.Sprintf("Obtaining certificate for %s", domain))

	certPath := filepath.Join(cm.sslPath, "certs", domain+".crt")
	keyPath := filepath.Join(cm.sslPath, "private", domain+".key")
	accountKeyPath := filepath.Join(cm.sslPath, "accounts", domain+".account.key")

	// Use ACME manager to obtain certificate
	if err := cm.acmeManager.ObtainCertificate(domain, certPath, keyPath, accountKeyPath); err != nil {
		return fmt.Errorf("ACME certificate request failed: %v", err)
	}

	cm.logger.Info(fmt.Sprintf("Successfully obtained certificate for %s", domain))
	return nil
}

// renewCertificate renews an existing certificate
func (cm *CertificateManager) renewCertificate(domain string) error {
	cm.logger.Info(fmt.Sprintf("Renewing certificate for %s", domain))

	// Remove from cache to force renewal
	cm.mu.Lock()
	delete(cm.certCache, domain)
	cm.mu.Unlock()

	// Obtain new certificate
	if err := cm.obtainCertificate(domain); err != nil {
		return err
	}

	// Update cache with new expiry
	if expiry, err := cm.getCertificateExpiry(domain); err == nil {
		cm.mu.Lock()
		cm.certCache[domain] = expiry
		cm.mu.Unlock()
		cm.logger.Info(fmt.Sprintf("Certificate renewed for %s, expires: %v", domain, expiry))
	}

	return nil
}

// generateSelfSignedCertificate generates a self-signed certificate
func (cm *CertificateManager) generateSelfSignedCertificate(domain string) (string, error) {
	cm.logger.Info(fmt.Sprintf("Generating self-signed certificate for %s", domain))

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country:      []string{"US"},
			Organization: []string{"Nginx-Proxy-Go"},
			CommonName:   domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", err
	}

	// Write certificate
	certPath := filepath.Join(cm.sslPath, "certs", domain+".selfsigned.crt")
	certFile, err := os.Create(certPath)
	if err != nil {
		return "", err
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return "", err
	}

	// Write private key
	keyPath := filepath.Join(cm.sslPath, "private", domain+".selfsigned.key")
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return "", err
	}
	defer keyFile.Close()

	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return "", err
	}

	if err := pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDER}); err != nil {
		return "", err
	}

	cm.selfSigned[domain] = true
	cm.logger.Info(fmt.Sprintf("Generated self-signed certificate for %s", domain))

	return domain + ".selfsigned", nil
}

// GetCertificateStatus returns status information about certificates
func (cm *CertificateManager) GetCertificateStatus() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	status := make(map[string]interface{})
	status["certificates"] = make(map[string]interface{})

	now := time.Now()
	for domain, expiry := range cm.certCache {
		domainStatus := map[string]interface{}{
			"expiry":         expiry,
			"days_remaining": int(expiry.Sub(now).Hours() / 24),
			"self_signed":    cm.selfSigned[domain],
		}
		status["certificates"].(map[string]interface{})[domain] = domainStatus
	}

	status["blacklisted"] = cm.blacklist
	return status
}

// Shutdown gracefully shuts down the certificate manager
func (cm *CertificateManager) Shutdown() {
	cm.logger.Info("Shutting down certificate manager")
	cm.renewalCancel()
	cm.renewalWG.Wait()
	cm.logger.Info("Certificate manager shutdown complete")
}
