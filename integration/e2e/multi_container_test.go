//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMultipleVirtualHostVars(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with multiple VIRTUAL_HOST variables
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST1": "site1.example.com",
		"VIRTUAL_HOST2": "site2.example.com",
		"VIRTUAL_HOST3": "site3.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// All three virtual hosts should route to the same backend
	hosts := []string{"site1.example.com", "site2.example.com", "site3.example.com"}
	for _, host := range hosts {
		resp, body := makeHTTPRequestToProxy(t, env, host, "/")
		require.Equal(t, 200, resp.StatusCode, "Failed for host: %s", host)
		assertResponseContains(t, body, "nginx")
	}
}

func TestMultipleContainersSameHost(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create first backend
	backend1 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "loadbalanced.example.com",
	}, "80/tcp")
	defer backend1.Container.Terminate(nil)

	// Create second backend with same host
	backend2 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "loadbalanced.example.com",
	}, "80/tcp")
	defer backend2.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Both backends should be accessible (upstream load balancing)
	// Make multiple requests to verify both backends respond
	successCount := 0
	for i := 0; i < 10; i++ {
		resp, _ := makeHTTPRequestToProxy(t, env, "loadbalanced.example.com", "/")
		if resp.StatusCode == 200 {
			successCount++
		}
		time.Sleep(100 * time.Millisecond)
	}

	// All requests should succeed
	require.Equal(t, 10, successCount, "All requests should succeed with multiple backends")
}

func TestContainerStartEvent(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Initially, the host should not exist
	resp1, _ := makeHTTPRequestToProxy(t, env, "dynamic.example.com", "/")
	require.Equal(t, 503, resp1.StatusCode)

	// Start a new container
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "dynamic.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Now the host should work
	resp2, _ := makeHTTPRequestToProxy(t, env, "dynamic.example.com", "/")
	require.Equal(t, 200, resp2.StatusCode)
}

func TestContainerStopEvent(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "stoppable.example.com",
	}, "80/tcp")

	waitForBackendRegistration(t, 5*time.Second)

	// Host should work
	resp1, _ := makeHTTPRequestToProxy(t, env, "stoppable.example.com", "/")
	require.Equal(t, 200, resp1.StatusCode)

	// Stop the container
	err := backend.Container.Stop(context.Background(), nil)
	require.NoError(t, err)

	// Wait for nginx to detect the stop and reload
	time.Sleep(5 * time.Second)

	// Host should now return 503
	resp2, _ := makeHTTPRequestToProxy(t, env, "stoppable.example.com", "/")
	require.Equal(t, 503, resp2.StatusCode)
}

func TestContainerRestartEvent(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "restartable.example.com",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Host should work
	resp1, _ := makeHTTPRequestToProxy(t, env, "restartable.example.com", "/")
	require.Equal(t, 200, resp1.StatusCode)

	// Restart the container
	timeout := 10 * time.Second
	err := backend.Container.Stop(context.Background(), &timeout)
	require.NoError(t, err)

	err = backend.Container.Start(context.Background())
	require.NoError(t, err)

	// Wait for nginx to detect the restart
	waitForBackendRegistration(t, 5*time.Second)

	// Host should work again
	resp2, _ := makeHTTPRequestToProxy(t, env, "restartable.example.com", "/")
	require.Equal(t, 200, resp2.StatusCode)
}

func TestNetworkConnectEvent(t *testing.T) {
	t.Skip("Network connect/disconnect events require advanced Docker network manipulation")

	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create container on different network initially
	// Then connect it to the proxy network
	// Verify it becomes accessible
}

func TestNetworkDisconnectEvent(t *testing.T) {
	t.Skip("Network connect/disconnect events require advanced Docker network manipulation")

	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create container on proxy network
	// Disconnect it from the network
	// Verify it becomes inaccessible
}

func TestMultiplePathsSameContainer(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create container with multiple paths
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST1": "example.com/api",
		"VIRTUAL_HOST2": "example.com/admin",
		"VIRTUAL_HOST3": "example.com/public",
	}, "80/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// All paths should work
	paths := []string{"/api", "/admin", "/public"}
	for _, path := range paths {
		resp, _ := makeHTTPRequestToProxy(t, env, "example.com", path)
		require.Equal(t, 200, resp.StatusCode, "Failed for path: %s", path)
	}

	// Root should not work
	resp, _ := makeHTTPRequestToProxy(t, env, "example.com", "/")
	require.Equal(t, 503, resp.StatusCode)
}

func TestMultipleContainersDifferentPaths(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create API backend
	backend1 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com/api",
	}, "80/tcp")
	defer backend1.Container.Terminate(nil)

	// Create admin backend
	backend2 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "example.com/admin",
	}, "80/tcp")
	defer backend2.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Both paths should work independently
	resp1, _ := makeHTTPRequestToProxy(t, env, "example.com", "/api")
	require.Equal(t, 200, resp1.StatusCode)

	resp2, _ := makeHTTPRequestToProxy(t, env, "example.com", "/admin")
	require.Equal(t, 200, resp2.StatusCode)
}

func TestContainerWithMixedSchemes(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificates
	generateSelfSignedCert(t, env.SSLDir, "mixed.example.com")

	// Create container with both HTTP and WebSocket
	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST1": "http://mixed.example.com/http",
		"VIRTUAL_HOST2": "ws://mixed.example.com/ws",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// HTTP should work
	resp, _ := makeHTTPRequestToProxy(t, env, "mixed.example.com", "/http")
	require.Equal(t, 200, resp.StatusCode)

	// WebSocket should work
	conn := testWebSocket(t, env.ProxyIP, 80, "mixed.example.com", "/ws")
	defer conn.Close()

	err := conn.WriteMessage(1, []byte("test"))
	require.NoError(t, err)
}

func TestContainerRemoval(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend
	backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "removable.example.com",
	}, "80/tcp")

	waitForBackendRegistration(t, 5*time.Second)

	// Host should work
	resp1, _ := makeHTTPRequestToProxy(t, env, "removable.example.com", "/")
	require.Equal(t, 200, resp1.StatusCode)

	// Remove container (terminate completely)
	err := backend.Container.Terminate(context.Background())
	require.NoError(t, err)

	// Wait for nginx to detect removal
	time.Sleep(5 * time.Second)

	// Host should now return 503
	resp2, _ := makeHTTPRequestToProxy(t, env, "removable.example.com", "/")
	require.Equal(t, 503, resp2.StatusCode)
}

func TestScalingContainers(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Start with one container
	backend1 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "scaled.example.com",
	}, "80/tcp")
	defer backend1.Container.Terminate(nil)

	waitForBackendRegistration(t, 3*time.Second)

	// Should work with one container
	resp1, _ := makeHTTPRequestToProxy(t, env, "scaled.example.com", "/")
	require.Equal(t, 200, resp1.StatusCode)

	// Add second container
	backend2 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "scaled.example.com",
	}, "80/tcp")
	defer backend2.Container.Terminate(nil)

	waitForBackendRegistration(t, 3*time.Second)

	// Should still work with two containers
	resp2, _ := makeHTTPRequestToProxy(t, env, "scaled.example.com", "/")
	require.Equal(t, 200, resp2.StatusCode)

	// Add third container
	backend3 := createBackendContainer(t, env, "nginx:alpine", map[string]string{
		"VIRTUAL_HOST": "scaled.example.com",
	}, "80/tcp")
	defer backend3.Container.Terminate(nil)

	waitForBackendRegistration(t, 3*time.Second)

	// Should work with three containers
	resp3, _ := makeHTTPRequestToProxy(t, env, "scaled.example.com", "/")
	require.Equal(t, 200, resp3.StatusCode)
}

func TestContainerIPChange(t *testing.T) {
	t.Skip("Testing IP change requires container network reconfiguration")

	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create container, stop it, and start again
	// IP might change, proxy should handle it
}

func TestMultipleProxyInstances(t *testing.T) {
	t.Skip("Testing multiple proxy instances requires complex setup")

	// This would test behavior with multiple nginx-proxy-go instances
	// watching the same Docker daemon
}

func TestContainerUpdateWithoutRestart(t *testing.T) {
	t.Skip("Testing container updates without restart requires Docker API manipulation")

	// Test that environment variable changes trigger reconfiguration
	// without requiring container restart
}
