package acme

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ACME represents an ACME client for Let's Encrypt
type ACME struct {
	apiURL       string
	accountKey   string
	domainKey    string
	certPath     string
	challengeDir string
	domains      []string
	debug        bool
	skipReload   bool
	httpClient   *http.Client
	directory    map[string]interface{}
	accountKid   string
}

// NewACME creates a new ACME client
func NewACME(apiURL, accountKey, domainKey, certPath, challengeDir string, domains []string, debug, skipReload bool) *ACME {
	return &ACME{
		apiURL:       apiURL,
		accountKey:   accountKey,
		domainKey:    domainKey,
		certPath:     certPath,
		challengeDir: challengeDir,
		domains:      domains,
		debug:        debug,
		skipReload:   skipReload,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// createKey creates a new RSA key and saves it to the specified path
func (a *ACME) createKey(keyPath string) (crypto.PrivateKey, error) {
	// Check if key already exists
	if _, err := os.Stat(keyPath); err == nil {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read existing key: %v", err)
		}
		block, _ := pem.Decode(keyData)
		if block == nil {
			return nil, fmt.Errorf("failed to decode PEM block")
		}
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	// Create new key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %v", err)
	}

	// Save key to file
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	if err := os.MkdirAll(filepath.Dir(keyPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %v", err)
	}

	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("failed to write key: %v", err)
	}

	return key, nil
}

// createCSR creates a Certificate Signing Request
func (a *ACME) createCSR() ([]byte, error) {
	keyData, err := os.ReadFile(a.domainKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read domain key: %v", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: a.domains[0],
		},
		DNSNames: a.domains,
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return nil, fmt.Errorf("failed to create CSR: %v", err)
	}

	return csrDER, nil
}

// b64 encodes data in base64url format
func (a *ACME) b64(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

// jws creates a JSON Web Signature header
func (a *ACME) jws() (map[string]interface{}, error) {
	keyData, err := os.ReadFile(a.accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read account key: %v", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Get public key components
	publicKey := key.Public().(*rsa.PublicKey)
	exponent := fmt.Sprintf("%x", publicKey.E)
	if len(exponent)%2 == 1 {
		exponent = "0" + exponent
	}

	header := map[string]interface{}{
		"alg": "RS256",
		"jwk": map[string]interface{}{
			"e":   a.b64([]byte(exponent)),
			"kty": "RSA",
			"n":   a.b64(publicKey.N.Bytes()),
		},
	}

	return header, nil
}

// thumbprint calculates the account key thumbprint
func (a *ACME) thumbprint() (string, error) {
	jws, err := a.jws()
	if err != nil {
		return "", err
	}

	jwk := jws["jwk"].(map[string]interface{})
	jwkJSON, err := json.Marshal(jwk)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWK: %v", err)
	}

	hash := sha256.Sum256(jwkJSON)
	return a.b64(hash[:]), nil
}

// writeChallenge writes the ACME challenge response
func (a *ACME) writeChallenge(token, thumbprint string) error {
	content := fmt.Sprintf("%s.%s", token, thumbprint)
	path := filepath.Join(a.challengeDir, token)
	return os.WriteFile(path, []byte(content), 0644)
}

// cleanup removes challenge files
func (a *ACME) cleanup(files []string) {
	if !a.debug {
		for _, f := range files {
			os.Remove(f)
		}
	}
}

// sendSignedRequest sends a signed request to the ACME server
func (a *ACME) sendSignedRequest(url string, payload interface{}) (int, []byte, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to marshal payload: %v", err)
	}

	payloadB64 := a.b64(payloadJSON)
	protected := map[string]interface{}{
		"alg": "RS256",
		"url": url,
	}

	if a.accountKid != "" {
		protected["kid"] = a.accountKid
	} else {
		jws, err := a.jws()
		if err != nil {
			return 0, nil, err
		}
		protected["jwk"] = jws["jwk"]
	}

	// Get nonce
	req, err := http.NewRequest("HEAD", a.apiURL+"/directory", nil)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/jose+json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get nonce: %v", err)
	}
	resp.Body.Close()

	protected["nonce"] = resp.Header.Get("Replay-Nonce")
	protectedB64 := a.b64([]byte(mustMarshalJSON(protected)))

	// Sign the request
	keyData, err := os.ReadFile(a.accountKey)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read account key: %v", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return 0, nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, []byte(protectedB64+"."+payloadB64))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to sign request: %v", err)
	}

	request := map[string]interface{}{
		"protected": protectedB64,
		"payload":   payloadB64,
		"signature": a.b64(signature),
	}

	req, err = http.NewRequest("POST", url, strings.NewReader(mustMarshalJSON(request)))
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/jose+json")

	resp, err = a.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read response: %v", err)
	}

	return resp.StatusCode, body, nil
}

// mustMarshalJSON is a helper function that panics if JSON marshaling fails
func mustMarshalJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
