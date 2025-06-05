package acme

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager represents the ACME certificate manager
type Manager struct {
	apiURL       string
	challengeDir string
	renewBefore  time.Duration
	mu           sync.RWMutex
}

// NewManager creates a new ACME manager
func NewManager(apiURL, challengeDir string) *Manager {
	return &Manager{
		apiURL:       apiURL,
		challengeDir: challengeDir,
		renewBefore:  30 * 24 * time.Hour, // Renew 30 days before expiration
	}
}

// ObtainCertificate obtains a certificate for the specified domain
func (m *Manager) ObtainCertificate(domain, certPath, keyPath, accountKeyPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create ACME client for this request
	acme := NewACMEv2(
		m.apiURL,
		accountKeyPath,
		keyPath,
		certPath,
		m.challengeDir,
		[]string{domain},
		false, // debug
		false, // skipReload
		"",    // dnsProvider
	)

	// Get the certificate
	if err := acme.GetCertificate(); err != nil {
		return fmt.Errorf("failed to obtain certificate: %v", err)
	}

	return nil
}

// CertificateManager manages SSL certificates
type CertificateManager struct {
	acme        *ACMEv2
	certDir     string
	renewBefore time.Duration
	mu          sync.RWMutex
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager(acme *ACMEv2, certDir string, renewBefore time.Duration) *CertificateManager {
	return &CertificateManager{
		acme:        acme,
		certDir:     certDir,
		renewBefore: renewBefore,
	}
}

// GetCertificate gets or renews a certificate for the specified domains
func (m *CertificateManager) GetCertificate(domains []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if certificate exists and is valid
	certPath := filepath.Join(m.certDir, domains[0]+".crt")
	if _, err := os.Stat(certPath); err == nil {
		if valid, err := m.isCertificateValid(certPath); err == nil && valid {
			return nil
		}
	}

	// Get new certificate
	m.acme.domains = domains
	m.acme.certPath = certPath
	return m.acme.GetCertificate()
}

// isCertificateValid checks if a certificate is valid and not close to expiration
func (m *CertificateManager) isCertificateValid(certPath string) (bool, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return false, fmt.Errorf("failed to read certificate: %v", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return false, fmt.Errorf("failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false, fmt.Errorf("failed to parse certificate: %v", err)
	}

	// Check if certificate is valid and not close to expiration
	now := time.Now()
	if now.After(cert.NotAfter) {
		return false, nil
	}

	// Check if certificate is close to expiration
	renewTime := cert.NotAfter.Add(-m.renewBefore)
	return now.Before(renewTime), nil
}

// StartRenewalLoop starts a background loop to check and renew certificates
func (m *CertificateManager) StartRenewalLoop(checkInterval time.Duration) {
	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for range ticker.C {
			m.checkAndRenewCertificates()
		}
	}()
}

// checkAndRenewCertificates checks all certificates and renews them if needed
func (m *CertificateManager) checkAndRenewCertificates() {
	entries, err := os.ReadDir(m.certDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".crt" {
			certPath := filepath.Join(m.certDir, entry.Name())
			valid, err := m.isCertificateValid(certPath)
			if err != nil || !valid {
				// Extract domain from filename
				domain := entry.Name()[:len(entry.Name())-4]
				if err := m.GetCertificate([]string{domain}); err != nil {
					fmt.Printf("Failed to renew certificate for %s: %v\n", domain, err)
				}
			}
		}
	}
}
