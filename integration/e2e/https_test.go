//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPSVirtualHost(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate self-signed certificate
	generateSelfSignedCert(t, env.SSLDir, "secure.example.com")

	// Create backend with HTTPS virtual host
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://secure.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Make HTTPS request
	resp, body := makeHTTPSRequestToProxy(t, env, "secure.example.com", "/")
	require.Equal(t, 200, resp.StatusCode)
	assertResponseContains(t, body, "nginx")
}

func TestHTTPToHTTPSRedirect(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate self-signed certificate
	generateSelfSignedCert(t, env.SSLDir, "redirect.example.com")

	// Create backend with HTTPS virtual host
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://redirect.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Make HTTP request (should get redirected)
	resp, _ := makeHTTPRequestToProxy(t, env, "redirect.example.com", "/")
	require.Equal(t, 301, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Location"), "https://")
}

func TestSelfSignedCertificateLoading(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	hostname := "ssl.example.com"

	// Generate self-signed certificate before creating container
	certPath, keyPath := generateSelfSignedCert(t, env.SSLDir, hostname)
	require.FileExists(t, certPath)
	require.FileExists(t, keyPath)

	// Create backend with HTTPS
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://" + hostname,
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// HTTPS request should work with the certificate
	resp, _ := makeHTTPSRequestToProxy(t, env, hostname, "/")
	require.Equal(t, 200, resp.StatusCode)
}

func TestMixedHTTPAndHTTPS(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificate for HTTPS host
	generateSelfSignedCert(t, env.SSLDir, "mixed.example.com")

	// Create backend with both HTTP and HTTPS
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST1": "http://mixed.example.com/http",
		"VIRTUAL_HOST2": "https://mixed.example.com/https",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// HTTP request should work
	resp1, _ := makeHTTPRequestToProxy(t, env, "mixed.example.com", "/http")
	require.Equal(t, 200, resp1.StatusCode)

	// HTTPS request should work
	resp2, _ := makeHTTPSRequestToProxy(t, env, "mixed.example.com", "/https")
	require.Equal(t, 200, resp2.StatusCode)
}

func TestMultipleHTTPSHosts(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificates for both hosts
	generateSelfSignedCert(t, env.SSLDir, "ssl1.example.com")
	generateSelfSignedCert(t, env.SSLDir, "ssl2.example.com")

	// Create backend 1
	backend1 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://ssl1.example.com",
	}, "80/tcp")
	defer backend1.Container.Terminate(nil)

	// Create backend 2
	backend2 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://ssl2.example.com",
	}, "80/tcp")
	defer backend2.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Both HTTPS hosts should work (SNI support)
	resp1, _ := makeHTTPSRequestToProxy(t, env, "ssl1.example.com", "/")
	require.Equal(t, 200, resp1.StatusCode)

	resp2, _ := makeHTTPSRequestToProxy(t, env, "ssl2.example.com", "/")
	require.Equal(t, 200, resp2.StatusCode)
}

func TestHTTPSWithPathRouting(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificate
	generateSelfSignedCert(t, env.SSLDir, "path.example.com")

	// Create backend with HTTPS path routing
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://path.example.com/api",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// HTTPS request to /api should work
	resp, _ := makeHTTPSRequestToProxy(t, env, "path.example.com", "/api")
	require.Equal(t, 200, resp.StatusCode)
}

func TestHTTPSWithCustomPort(t *testing.T) {
	t.Skip("Custom HTTPS ports require port mapping configuration")

	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificate
	generateSelfSignedCert(t, env.SSLDir, "custom.example.com")

	// Create backend with custom HTTPS port
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://custom.example.com:8443",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// HTTPS request on custom port
	// Note: This might fail if port 8443 is not exposed by the proxy
	resp, _ := makeHTTPSRequest(t, "localhost", 8443, "custom.example.com", "/")
	require.Equal(t, 200, resp.StatusCode)
}

func TestHTTPSWithoutCertificate(t *testing.T) {
	t.Skip("Testing self-signed fallback behavior - requires checking specific certificate")

	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with HTTPS but no certificate
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://nocert.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Should fallback to self-signed certificate or disable SSL
	// This behavior needs to be defined in requirements
}

func TestHTTPSRedirectWithMultipleDomains(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificate
	generateSelfSignedCert(t, env.SSLDir, "main.example.com")

	// Create backend with HTTPS
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "https://main.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// HTTP request should redirect to HTTPS
	resp, _ := makeHTTPRequestToProxy(t, env, "main.example.com", "/test")
	require.Equal(t, 301, resp.StatusCode)
	location := resp.Header.Get("Location")
	require.Contains(t, location, "https://")
	require.Contains(t, location, "/test")
}

func TestHTTPSWithLetsEncryptEnv(t *testing.T) {
	t.Skip("Let's Encrypt integration excluded from scope")
}
