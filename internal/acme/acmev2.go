package acme

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ACMEv2 represents an ACME v2 client for Let's Encrypt
type ACMEv2 struct {
	*ACME
	dnsProvider string
}

// NewACMEv2 creates a new ACME v2 client
func NewACMEv2(apiURL, accountKey, domainKey, certPath, challengeDir string, domains []string, debug, skipReload bool, dnsProvider string) *ACMEv2 {
	return &ACMEv2{
		ACME:        NewACME(apiURL, accountKey, domainKey, certPath, challengeDir, domains, debug, skipReload),
		dnsProvider: dnsProvider,
	}
}

// RegisterAccount registers a new account with the ACME server
func (a *ACMEv2) RegisterAccount() error {
	// Create account key if it doesn't exist
	if _, err := a.createKey(a.accountKey); err != nil {
		return fmt.Errorf("failed to create account key: %v", err)
	}

	// Get directory
	resp, err := a.httpClient.Get(a.apiURL)
	if err != nil {
		return fmt.Errorf("failed to get directory: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get directory: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read directory: %v", err)
	}

	if err := json.Unmarshal(body, &a.directory); err != nil {
		return fmt.Errorf("failed to parse directory: %v", err)
	}

	// Register account
	payload := map[string]interface{}{
		"termsOfServiceAgreed": true,
	}

	code, result, err := a.sendSignedRequest(a.directory["newAccount"].(string), payload)
	if err != nil {
		return fmt.Errorf("failed to register account: %v", err)
	}

	if code != 201 && code != 200 {
		return fmt.Errorf("failed to register account: %d %s", code, string(result))
	}

	// Get account ID from Location header
	a.accountKid = resp.Header.Get("Location")

	// Create domain key if it doesn't exist
	if _, err := a.createKey(a.domainKey); err != nil {
		return fmt.Errorf("failed to create domain key: %v", err)
	}

	return nil
}

// SolveHTTPChallenge solves the HTTP challenge for domain verification
func (a *ACMEv2) SolveHTTPChallenge() error {
	// Create new order
	identifiers := make([]map[string]string, len(a.domains))
	for i, domain := range a.domains {
		identifiers[i] = map[string]string{
			"type":  "dns",
			"value": domain,
		}
	}

	payload := map[string]interface{}{
		"identifiers": identifiers,
	}

	code, result, err := a.sendSignedRequest(a.directory["newOrder"].(string), payload)
	if err != nil {
		return fmt.Errorf("failed to create order: %v", err)
	}

	if code < 200 || code > 299 {
		return fmt.Errorf("failed to create order: %d %s", code, string(result))
	}

	var order map[string]interface{}
	if err := json.Unmarshal(result, &order); err != nil {
		return fmt.Errorf("failed to parse order: %v", err)
	}

	// Process each authorization
	for _, authURL := range order["authorizations"].([]interface{}) {
		resp, err := a.httpClient.Get(authURL.(string))
		if err != nil {
			return fmt.Errorf("failed to get authorization: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read authorization: %v", err)
		}

		var auth map[string]interface{}
		if err := json.Unmarshal(body, &auth); err != nil {
			return fmt.Errorf("failed to parse authorization: %v", err)
		}

		domain := auth["identifier"].(map[string]interface{})["value"].(string)

		// Find HTTP challenge
		var challenge map[string]interface{}
		for _, c := range auth["challenges"].([]interface{}) {
			ch := c.(map[string]interface{})
			if ch["type"].(string) == "http-01" {
				challenge = ch
				break
			}
		}

		if challenge == nil {
			return fmt.Errorf("no HTTP challenge found for %s", domain)
		}

		// Prepare challenge response
		token := regexp.MustCompile(`[^A-Za-z0-9_\-]`).ReplaceAllString(challenge["token"].(string), "_")
		thumbprint, err := a.thumbprint()
		if err != nil {
			return fmt.Errorf("failed to get thumbprint: %v", err)
		}

		// Write challenge response
		if err := a.writeChallenge(token, thumbprint); err != nil {
			return fmt.Errorf("failed to write challenge: %v", err)
		}

		// Notify server
		code, result, err = a.sendSignedRequest(challenge["url"].(string), nil)
		if err != nil {
			return fmt.Errorf("failed to notify server: %v", err)
		}

		if code > 399 {
			return fmt.Errorf("failed to notify server: %d %s", code, string(result))
		}

		// Wait for challenge to be validated
		if err := a.verifyChallenge(challenge["url"].(string), domain); err != nil {
			return fmt.Errorf("failed to verify challenge: %v", err)
		}

		// Cleanup
		a.cleanup([]string{filepath.Join(a.challengeDir, token)})
	}

	// Finalize order
	return a.finalizeOrder(order)
}

// SolveDNSChallenge solves the DNS challenge for domain verification
func (a *ACMEv2) SolveDNSChallenge(dnsClient DNSProvider) error {
	// Create new order
	identifiers := make([]map[string]string, len(a.domains))
	for i, domain := range a.domains {
		identifiers[i] = map[string]string{
			"type":  "dns",
			"value": domain,
		}
	}

	payload := map[string]interface{}{
		"identifiers": identifiers,
	}

	code, result, err := a.sendSignedRequest(a.directory["newOrder"].(string), payload)
	if err != nil {
		return fmt.Errorf("failed to create order: %v", err)
	}

	if code < 200 || code > 299 {
		return fmt.Errorf("failed to create order: %d %s", code, string(result))
	}

	var order map[string]interface{}
	if err := json.Unmarshal(result, &order); err != nil {
		return fmt.Errorf("failed to parse order: %v", err)
	}

	// Process each authorization
	for _, authURL := range order["authorizations"].([]interface{}) {
		resp, err := a.httpClient.Get(authURL.(string))
		if err != nil {
			return fmt.Errorf("failed to get authorization: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read authorization: %v", err)
		}

		var auth map[string]interface{}
		if err := json.Unmarshal(body, &auth); err != nil {
			return fmt.Errorf("failed to parse authorization: %v", err)
		}

		domain := auth["identifier"].(map[string]interface{})["value"].(string)

		// Find DNS challenge
		var challenge map[string]interface{}
		for _, c := range auth["challenges"].([]interface{}) {
			ch := c.(map[string]interface{})
			if ch["type"].(string) == "dns-01" {
				challenge = ch
				break
			}
		}

		if challenge == nil {
			return fmt.Errorf("no DNS challenge found for %s", domain)
		}

		// Prepare challenge response
		token := regexp.MustCompile(`[^A-Za-z0-9_\-]`).ReplaceAllString(challenge["token"].(string), "_")
		thumbprint, err := a.thumbprint()
		if err != nil {
			return fmt.Errorf("failed to get thumbprint: %v", err)
		}

		keyAuth := fmt.Sprintf("%s.%s", token, thumbprint)
		txtRecord := a.b64([]byte(keyAuth))

		// Create DNS record
		record, err := dnsClient.CreateRecord(domain, "_acme-challenge."+strings.TrimPrefix(strings.TrimSuffix(domain, "."), "*.")+".", txtRecord)
		if err != nil {
			return fmt.Errorf("failed to create DNS record: %v", err)
		}

		// Notify server
		code, result, err = a.sendSignedRequest(challenge["url"].(string), map[string]string{
			"keyAuthorization": keyAuth,
		})
		if err != nil {
			return fmt.Errorf("failed to notify server: %v", err)
		}

		if code > 399 {
			return fmt.Errorf("failed to notify server: %d %s", code, string(result))
		}

		// Wait for challenge to be validated
		if err := a.verifyChallenge(challenge["url"].(string), domain); err != nil {
			return fmt.Errorf("failed to verify challenge: %v", err)
		}

		// Cleanup DNS record
		if !a.debug {
			if err := dnsClient.DeleteRecord(domain, record); err != nil {
				return fmt.Errorf("failed to delete DNS record: %v", err)
			}
		}
	}

	// Finalize order
	return a.finalizeOrder(order)
}

// verifyChallenge waits for the challenge to be validated
func (a *ACMEv2) verifyChallenge(url, domain string) error {
	for i := 0; i < 60; i++ {
		resp, err := a.httpClient.Get(url)
		if err != nil {
			return fmt.Errorf("failed to check challenge status: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read challenge status: %v", err)
		}

		var status map[string]interface{}
		if err := json.Unmarshal(body, &status); err != nil {
			return fmt.Errorf("failed to parse challenge status: %v", err)
		}

		switch status["status"].(string) {
		case "valid":
			return nil
		case "invalid":
			return fmt.Errorf("challenge failed for %s: %v", domain, status["error"])
		case "pending":
			time.Sleep(5 * time.Second)
			continue
		default:
			return fmt.Errorf("unexpected challenge status: %s", status["status"])
		}
	}

	return fmt.Errorf("challenge verification timed out for %s", domain)
}

// finalizeOrder finalizes the order and downloads the certificate
func (a *ACMEv2) finalizeOrder(order map[string]interface{}) error {
	// Create CSR
	csr, err := a.createCSR()
	if err != nil {
		return fmt.Errorf("failed to create CSR: %v", err)
	}

	// Finalize order
	payload := map[string]string{
		"csr": a.b64(csr),
	}

	code, result, err := a.sendSignedRequest(order["finalize"].(string), payload)
	if err != nil {
		return fmt.Errorf("failed to finalize order: %v", err)
	}

	if code > 399 {
		return fmt.Errorf("failed to finalize order: %d %s", code, string(result))
	}

	// Download certificate
	resp, err := a.httpClient.Get(order["certificate"].(string))
	if err != nil {
		return fmt.Errorf("failed to download certificate: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read certificate: %v", err)
	}

	// Save certificate
	if err := os.WriteFile(a.certPath, body, 0644); err != nil {
		return fmt.Errorf("failed to save certificate: %v", err)
	}

	return nil
}

// GetCertificate obtains a certificate for the specified domains
func (a *ACMEv2) GetCertificate() error {
	if err := a.RegisterAccount(); err != nil {
		return fmt.Errorf("failed to register account: %v", err)
	}

	if a.dnsProvider != "" {
		var dnsClient DNSProvider
		switch a.dnsProvider {
		case "digitalocean":
			dnsClient = NewDigitalOcean()
		default:
			return fmt.Errorf("unsupported DNS provider: %s", a.dnsProvider)
		}
		return a.SolveDNSChallenge(dnsClient)
	}

	return a.SolveHTTPChallenge()
}
