//go:build e2e

package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBasicVirtualHostRouting(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend container with VIRTUAL_HOST
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	// Wait for backend to be registered
	waitForBackendRegistration(t, 5*time.Second)

	// Make request with Host header
	resp, body := makeHTTPRequestToProxy(t, env, "example.com", "/")

	// Verify response
	require.Equal(t, 200, resp.StatusCode)
	assertResponseContains(t, body, "Welcome to nginx")
}

func TestPortBasedRouting(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend container with port-based VIRTUAL_HOST
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com:8080",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Request should work on specified port
	resp, body := makeHTTPRequestToProxy(t, env, "example.com:8080", "/")
	require.Equal(t, 200, resp.StatusCode)
	assertResponseContains(t, body, "Welcome to nginx")

	// Request without port should fail (503)
	resp2, _ := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 503, resp2.StatusCode)
}

func TestPathBasedRouting(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend container with path-based VIRTUAL_HOST
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com/api",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Request to /api should work
	resp, body := makeHTTPRequestToProxy(t, env, "example.com", "/api")
	require.Equal(t, 200, resp.StatusCode)
	assertResponseContains(t, body, "nginx")

	// Request to / should fail (503)
	resp2, _ := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 503, resp2.StatusCode)
}

func TestPathRewriting(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with path rewriting: /api -> /
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com/api -> /",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Request to /api should proxy to backend's /
	resp, body := makeHTTPRequestToProxy(t, env, "example.com", "/api")
	require.Equal(t, 200, resp.StatusCode)
	assertResponseContains(t, body, "nginx")
}

func TestHostHeaderValidation(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create two backends with different virtual hosts
	backend1 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "site1.com",
	}, "80/tcp")
	defer backend1.Container.Terminate(nil)

	backend2 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "site2.com",
	}, "80/tcp")
	defer backend2.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Request with site1.com Host header
	resp1, _ := makeHTTPRequestToProxy(t, env, "site1.com", "/")
	require.Equal(t, 200, resp1.StatusCode)

	// Request with site2.com Host header
	resp2, _ := makeHTTPRequestToProxy(t, env, "site2.com", "/")
	require.Equal(t, 200, resp2.StatusCode)

	// Request with unknown host should fail (503)
	resp3, _ := makeHTTPRequestToProxy(t, env, "unknown.com", "/")
	require.Equal(t, 503, resp3.StatusCode)
}

func TestMultipleVirtualHosts(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with multiple VIRTUAL_HOST entries
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST1": "site1.com",
		"VIRTUAL_HOST2": "site2.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Both hosts should work
	resp1, body1 := makeHTTPRequestToProxy(t, env, "site1.com", "/")
	require.Equal(t, 200, resp1.StatusCode)
	assertResponseContains(t, body1, "nginx")

	resp2, body2 := makeHTTPRequestToProxy(t, env, "site2.com", "/")
	require.Equal(t, 200, resp2.StatusCode)
	assertResponseContains(t, body2, "nginx")
}

func TestContainerPortMapping(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with specific container port
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com -> :80",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Request should be proxied to container port 80
	resp, body := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 200, resp.StatusCode)
	assertResponseContains(t, body, "nginx")
}

func TestCustomNginxDirectives(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with custom nginx directive
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com; client_max_body_size 50m",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Basic request should still work
	resp, body := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 200, resp.StatusCode)
	assertResponseContains(t, body, "nginx")
}

func TestDefaultServerBehavior(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Without any backends, unknown hosts should return 503
	resp, _ := makeHTTPRequestToProxy(t, env, "unknown.example.com", "/")
	require.Equal(t, 503, resp.StatusCode)

	// Create a backend
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "known.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Known host should work
	resp2, _ := makeHTTPRequestToProxy(t, env, "known.example.com", "/")
	require.Equal(t, 200, resp2.StatusCode)

	// Unknown host should still return 503
	resp3, _ := makeHTTPRequestToProxy(t, env, "unknown.example.com", "/")
	require.Equal(t, 503, resp3.StatusCode)
}

func TestMultiplePathsOnSameHost(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend for /api
	backend1 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com/api",
	}, "80/tcp")
	defer backend1.Container.Terminate(nil)

	// Create backend for /admin
	backend2 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com/admin",
	}, "80/tcp")
	defer backend2.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Both paths should work
	resp1, _ := makeHTTPRequestToProxy(t, env, "example.com", "/api")
	require.Equal(t, 200, resp1.StatusCode)

	resp2, _ := makeHTTPRequestToProxy(t, env, "example.com", "/admin")
	require.Equal(t, 200, resp2.StatusCode)

	// Root path should fail (503)
	resp3, _ := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 503, resp3.StatusCode)
}

func TestContainerLifecycle(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com",
	}, "80/tcp")

	waitForBackendRegistration(t, 5*time.Second)

	// Request should work
	resp1, _ := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 200, resp1.StatusCode)

	// Stop container
	err := backend.Container.Stop(nil, nil)
	require.NoError(t, err)

	// Wait for nginx to reload
	time.Sleep(3 * time.Second)

	// Request should now fail (503)
	resp2, _ := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 503, resp2.StatusCode)
}

func TestVirtualHostWithQueryString(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Request with query string
	resp, _ := makeHTTPRequestToProxy(t, env, "example.com", "/index.html?param=value")
	require.Equal(t, 200, resp.StatusCode)
}

func TestMultipleContainersLoadBalancing(t *testing.T) {
	t.Skip("Load balancing test requires multiple containers on same host - implement after basic tests work")

	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create multiple backends with same VIRTUAL_HOST
	backend1 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com",
	}, "80/tcp")
	defer backend1.Container.Terminate(nil)

	backend2 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com",
	}, "80/tcp")
	defer backend2.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Make multiple requests - should distribute across backends
	successCount := 0
	for i := 0; i < 10; i++ {
		resp, _ := makeHTTPRequestToProxy(t, env, "example.com", fmt.Sprintf("/test%d", i))
		if resp.StatusCode == 200 {
			successCount++
		}
	}

	// At least some requests should succeed
	require.Greater(t, successCount, 0, "Expected at least some requests to succeed with load balancing")
}
