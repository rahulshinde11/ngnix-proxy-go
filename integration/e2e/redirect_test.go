//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSimpleRedirect(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create main backend
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "new.example.com",
		"PROXY_FULL_REDIRECT": "old.example.com->new.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Request to old.example.com should redirect to new.example.com
	resp, _ := makeHTTPRequestToProxy(t, env, "old.example.com", "/test")
	require.Equal(t, 301, resp.StatusCode)

	location := resp.Header.Get("Location")
	require.Contains(t, location, "new.example.com")
	require.Contains(t, location, "/test")

	// Request to new.example.com should work normally
	resp2, _ := makeHTTPRequestToProxy(t, env, "new.example.com", "/")
	require.Equal(t, 200, resp2.StatusCode)
}

func TestMultipleSourceRedirects(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create main backend with multiple redirect sources
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "main.example.com",
		"PROXY_FULL_REDIRECT": "a.example.com,b.example.com,c.example.com->main.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// All source domains should redirect to main
	sources := []string{"a.example.com", "b.example.com", "c.example.com"}
	for _, source := range sources {
		resp, _ := makeHTTPRequestToProxy(t, env, source, "/page")
		require.Equal(t, 301, resp.StatusCode, "Failed for source: %s", source)

		location := resp.Header.Get("Location")
		require.Contains(t, location, "main.example.com")
		require.Contains(t, location, "/page")
	}

	// Main domain should work normally
	resp, _ := makeHTTPRequestToProxy(t, env, "main.example.com", "/")
	require.Equal(t, 200, resp.StatusCode)
}

func TestRedirectWithPorts(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with port-based redirect
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "example.com:8080",
		"PROXY_FULL_REDIRECT": "example.com:9090->example.com:8080",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Request to port 9090 should redirect
	resp, _ := makeHTTPRequestToProxy(t, env, "example.com:9090", "/")
	require.Equal(t, 301, resp.StatusCode)

	location := resp.Header.Get("Location")
	require.Contains(t, location, "example.com")
}

func TestHTTPSRedirect(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificates
	generateSelfSignedCert(t, env.SSLDir, "secure.example.com")
	generateSelfSignedCert(t, env.SSLDir, "old-secure.example.com")

	// Create HTTPS backend with redirect
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "https://secure.example.com",
		"PROXY_FULL_REDIRECT": "old-secure.example.com->secure.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// HTTP request to old domain should redirect
	resp, _ := makeHTTPRequestToProxy(t, env, "old-secure.example.com", "/")
	require.Equal(t, 301, resp.StatusCode)

	// HTTPS request to main domain should work
	resp2, _ := makeHTTPSRequestToProxy(t, env, "secure.example.com", "/")
	require.Equal(t, 200, resp2.StatusCode)
}

func TestRedirectPreservesPath(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with redirect
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "target.example.com",
		"PROXY_FULL_REDIRECT": "source.example.com->target.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Redirect should preserve the full path
	paths := []string{"/", "/page", "/api/v1/users", "/path/with/multiple/segments"}
	for _, path := range paths {
		resp, _ := makeHTTPRequestToProxy(t, env, "source.example.com", path)
		require.Equal(t, 301, resp.StatusCode, "Failed for path: %s", path)

		location := resp.Header.Get("Location")
		require.Contains(t, location, path, "Redirect should preserve path: %s", path)
	}
}

func TestRedirectPreservesQueryString(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with redirect
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "target.example.com",
		"PROXY_FULL_REDIRECT": "source.example.com->target.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Redirect should preserve query string
	resp, _ := makeHTTPRequestToProxy(t, env, "source.example.com", "/page?param1=value1&param2=value2")
	require.Equal(t, 301, resp.StatusCode)

	location := resp.Header.Get("Location")
	require.Contains(t, location, "param1=value1")
	require.Contains(t, location, "param2=value2")
}

func TestMultipleRedirectRules(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with multiple redirect rules
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":         "main.example.com",
		"PROXY_FULL_REDIRECT":  "old1.example.com->main.example.com",
		"PROXY_FULL_REDIRECT2": "old2.example.com->main.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Both redirect rules should work
	resp1, _ := makeHTTPRequestToProxy(t, env, "old1.example.com", "/")
	require.Equal(t, 301, resp1.StatusCode)

	resp2, _ := makeHTTPRequestToProxy(t, env, "old2.example.com", "/")
	require.Equal(t, 301, resp2.StatusCode)

	// Main domain should work
	resp3, _ := makeHTTPRequestToProxy(t, env, "main.example.com", "/")
	require.Equal(t, 200, resp3.StatusCode)
}

func TestRedirectWithDifferentSchemes(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificate for HTTPS target
	generateSelfSignedCert(t, env.SSLDir, "https-target.example.com")

	// Create HTTPS backend with HTTP redirect source
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "https://https-target.example.com",
		"PROXY_FULL_REDIRECT": "http-source.example.com->https-target.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// HTTP request to source should redirect
	resp, _ := makeHTTPRequestToProxy(t, env, "http-source.example.com", "/")
	require.Equal(t, 301, resp.StatusCode)

	location := resp.Header.Get("Location")
	require.Contains(t, location, "https-target.example.com")
}

func TestRedirectWithSubdomains(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with subdomain redirects
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "www.example.com",
		"PROXY_FULL_REDIRECT": "example.com,app.example.com->www.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// All subdomains should redirect to www
	resp1, _ := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 301, resp1.StatusCode)

	resp2, _ := makeHTTPRequestToProxy(t, env, "app.example.com", "/")
	require.Equal(t, 301, resp2.StatusCode)

	// www should work normally
	resp3, _ := makeHTTPRequestToProxy(t, env, "www.example.com", "/")
	require.Equal(t, 200, resp3.StatusCode)
}

func TestRedirectLoopPrevention(t *testing.T) {
	t.Skip("Redirect loop detection - requires specific configuration")

	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// This would test that circular redirects are prevented
	// e.g., A->B, B->A should not create an infinite loop
}

func TestRedirectWithTrailingSlash(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "target.example.com",
		"PROXY_FULL_REDIRECT": "source.example.com->target.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Test with and without trailing slash
	resp1, _ := makeHTTPRequestToProxy(t, env, "source.example.com", "/path")
	require.Equal(t, 301, resp1.StatusCode)

	resp2, _ := makeHTTPRequestToProxy(t, env, "source.example.com", "/path/")
	require.Equal(t, 301, resp2.StatusCode)
}

func TestRedirectEmptyPath(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST":        "target.example.com",
		"PROXY_FULL_REDIRECT": "source.example.com->target.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Root path should also redirect
	resp, _ := makeHTTPRequestToProxy(t, env, "source.example.com", "/")
	require.Equal(t, 301, resp.StatusCode)

	location := resp.Header.Get("Location")
	require.Contains(t, location, "target.example.com")
}
