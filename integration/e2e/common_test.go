//go:build e2e

package e2e

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestEnvironment holds the test infrastructure
type TestEnvironment struct {
	DockerClient   *client.Client
	NetworkName    string
	NetworkID      string
	ProxyContainer tc.Container
	ProxyIP        string
	ProxyHTTPPort  string
	ProxyHTTPSPort string
	SSLDir         string
	TempDir        string
	Cleanup        func()
}

// BackendContainer represents a test backend container
type BackendContainer struct {
	Container tc.Container
	ID        string
	IP        string
	Name      string
}

// setupTestEnvironment creates the complete test infrastructure
func setupTestEnvironment(t *testing.T) *TestEnvironment {
	ctx := context.Background()

	// Create Docker client
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	require.NoError(t, err)

	// Create unique network name
	timestamp := time.Now().UnixNano()
	networkName := fmt.Sprintf("test-network-%d", timestamp)

	// Create Docker network
	networkResp, err := dockerClient.NetworkCreate(ctx, networkName, types.NetworkCreate{
		Driver: "bridge",
	})
	require.NoError(t, err)

	// Create temp directory for SSL and configs
	tempDir, err := os.MkdirTemp("", "nginx-proxy-test-*")
	require.NoError(t, err)

	sslDir := filepath.Join(tempDir, "ssl")
	nginxDir := filepath.Join(tempDir, "nginx")
	acmeDir := filepath.Join(tempDir, "acme")
	require.NoError(t, os.MkdirAll(filepath.Join(sslDir, "certs"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(sslDir, "private"), 0755))
	require.NoError(t, os.MkdirAll(nginxDir, 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(nginxDir, "conf.d"), 0755))
	require.NoError(t, os.MkdirAll(acmeDir, 0755))

	// Create a basic nginx.conf file
	nginxConf := `events {
    worker_connections 1024;
}

http {
    include /etc/nginx/conf.d/*.conf;
}`
	require.NoError(t, os.WriteFile(filepath.Join(nginxDir, "nginx.conf"), []byte(nginxConf), 0644))

	env := &TestEnvironment{
		DockerClient: dockerClient,
		NetworkName:  networkName,
		NetworkID:    networkResp.ID,
		SSLDir:       sslDir,
		TempDir:      tempDir,
	}

	// Setup cleanup function
	env.Cleanup = func() {
		if env.ProxyContainer != nil {
			env.ProxyContainer.Terminate(ctx)
		}
		dockerClient.NetworkRemove(ctx, networkResp.ID)
		os.RemoveAll(tempDir)
	}

	// Build nginx-proxy-go image if not exists
	err = buildProxyImage(t, dockerClient)
	require.NoError(t, err)

	// Start nginx-proxy-go container
	proxyReq := tc.ContainerRequest{
		Image:        "nginx-proxy-go:test",
		Networks:     []string{networkName},
		ExposedPorts: []string{"80/tcp", "443/tcp"},
		Env: map[string]string{
			"NGINX_CONF_DIR": "/etc/nginx",
			"CHALLENGE_DIR":  "/tmp/acme-challenges",
			"SSL_DIR":        "/etc/ssl/custom",
		},
		Mounts: tc.Mounts(
			tc.BindMount("/var/run/docker.sock", "/var/run/docker.sock"),
			tc.BindMount(sslDir, "/etc/ssl/custom"),
			tc.BindMount(nginxDir, "/etc/nginx"),
			tc.BindMount(acmeDir, "/tmp/acme-challenges"),
		),
		WaitingFor: wait.ForLog("WebServer started successfully").WithStartupTimeout(60 * time.Second),
	}

	proxyContainer, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: proxyReq,
		Started:          true,
	})
	require.NoError(t, err)

	env.ProxyContainer = proxyContainer

	// Get proxy container IP
	proxyInspect, err := dockerClient.ContainerInspect(ctx, proxyContainer.GetContainerID())
	require.NoError(t, err)

	if networkSettings, ok := proxyInspect.NetworkSettings.Networks[networkName]; ok {
		env.ProxyIP = networkSettings.IPAddress
	}
	require.NotEmpty(t, env.ProxyIP, "proxy container IP should not be empty")

	// Get mapped ports
	httpPort, err := proxyContainer.MappedPort(ctx, "80")
	require.NoError(t, err)
	env.ProxyHTTPPort = httpPort.Port()

	httpsPort, err := proxyContainer.MappedPort(ctx, "443")
	require.NoError(t, err)
	env.ProxyHTTPSPort = httpsPort.Port()

	// Wait for proxy to be ready
	proxyPortInt, err := strconv.Atoi(env.ProxyHTTPPort)
	require.NoError(t, err)
	waitForProxy(t, "localhost", proxyPortInt, 60*time.Second)

	return env
}

// buildProxyImage builds the nginx-proxy-go Docker image
func buildProxyImage(t *testing.T, dockerClient *client.Client) error {
	ctx := context.Background()

	// Check if image already exists
	images, err := dockerClient.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", "nginx-proxy-go:test")),
	})
	if err == nil && len(images) > 0 {
		t.Log("nginx-proxy-go:test image already exists, skipping build")
		return nil
	}

	t.Log("nginx-proxy-go:test image not found, it must be built before running tests")
	t.Log("Please run: docker build -t nginx-proxy-go:test .")
	t.Skip("nginx-proxy-go:test image not found - skipping test")

	return nil
}

// createBackendContainer creates a backend container for testing
func createBackendContainer(t *testing.T, env *TestEnvironment, image string, envVars map[string]string, exposedPort string) *BackendContainer {
	ctx := context.Background()

	containerName := fmt.Sprintf("test-backend-%d", time.Now().UnixNano())

	containerReq := tc.ContainerRequest{
		Image:        image,
		Networks:     []string{env.NetworkName},
		Env:          envVars,
		ExposedPorts: []string{exposedPort},
		Name:         containerName,
		WaitingFor:   wait.ForListeningPort(nat.Port(exposedPort)).WithStartupTimeout(30 * time.Second),
	}

	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: containerReq,
		Started:          true,
	})
	require.NoError(t, err)

	// Get container IP
	inspect, err := env.DockerClient.ContainerInspect(ctx, container.GetContainerID())
	require.NoError(t, err)

	var containerIP string
	if networkSettings, ok := inspect.NetworkSettings.Networks[env.NetworkName]; ok {
		containerIP = networkSettings.IPAddress
	}
	require.NotEmpty(t, containerIP, "backend container IP should not be empty")

	backend := &BackendContainer{
		Container: container,
		ID:        container.GetContainerID(),
		IP:        containerIP,
		Name:      containerName,
	}

	return backend
}

// makeHTTPRequestToProxy makes an HTTP request to the proxy using localhost and mapped port
func makeHTTPRequestToProxy(t *testing.T, env *TestEnvironment, hostHeader, path string) (*http.Response, []byte) {
	httpPortInt, err := strconv.Atoi(env.ProxyHTTPPort)
	require.NoError(t, err)
	return makeHTTPRequest(t, "localhost", httpPortInt, hostHeader, path)
}

// makeHTTPSRequestToProxy makes an HTTPS request to the proxy using localhost and mapped port
func makeHTTPSRequestToProxy(t *testing.T, env *TestEnvironment, hostHeader, path string) (*http.Response, []byte) {
	httpsPortInt, err := strconv.Atoi(env.ProxyHTTPSPort)
	require.NoError(t, err)
	return makeHTTPSRequest(t, "localhost", httpsPortInt, hostHeader, path)
}

// testWebSocketToProxy tests WebSocket connection to the proxy using localhost and mapped port
func testWebSocketToProxy(t *testing.T, env *TestEnvironment, hostHeader, path string) *websocket.Conn {
	httpPortInt, err := strconv.Atoi(env.ProxyHTTPPort)
	require.NoError(t, err)
	return testWebSocket(t, "localhost", httpPortInt, hostHeader, path)
}

// testWebSocketSecureToProxy tests secure WebSocket connection to the proxy using localhost and mapped port
func testWebSocketSecureToProxy(t *testing.T, env *TestEnvironment, hostHeader, path string) *websocket.Conn {
	httpsPortInt, err := strconv.Atoi(env.ProxyHTTPSPort)
	require.NoError(t, err)
	return testWebSocketSecure(t, "localhost", httpsPortInt, hostHeader, path)
}
func waitForProxy(t *testing.T, host string, port int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	address := fmt.Sprintf("%s:%d", host, port)

	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 1*time.Second)
		if err == nil {
			conn.Close()
			time.Sleep(2 * time.Second) // Give it a bit more time to fully initialize
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("proxy did not become ready within %v", timeout)
}

// waitForBackendRegistration waits for the backend to be registered with the proxy
func waitForBackendRegistration(t *testing.T, timeout time.Duration) {
	// Give nginx time to reload configuration after container registration
	time.Sleep(3 * time.Second)
}

// makeHTTPRequest makes an HTTP request with custom Host header
func makeHTTPRequest(t *testing.T, proxyIP string, port int, hostHeader, path string) (*http.Response, []byte) {
	url := fmt.Sprintf("http://%s:%d%s", proxyIP, port, path)

	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	if hostHeader != "" {
		req.Host = hostHeader
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, body
}

// makeHTTPSRequest makes an HTTPS request with custom Host header
func makeHTTPSRequest(t *testing.T, proxyIP string, port int, hostHeader, path string) (*http.Response, []byte) {
	url := fmt.Sprintf("https://%s:%d%s", proxyIP, port, path)

	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	if hostHeader != "" {
		req.Host = hostHeader
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Accept self-signed certificates
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, body
}

// makeHTTPRequestWithAuth makes an HTTP request with basic auth
func makeHTTPRequestWithAuth(t *testing.T, proxyIP string, port int, hostHeader, path, username, password string) (*http.Response, []byte) {
	url := fmt.Sprintf("http://%s:%d%s", proxyIP, port, path)

	req, err := http.NewRequest("GET", url, nil)
	require.NoError(t, err)

	if hostHeader != "" {
		req.Host = hostHeader
	}

	if username != "" {
		req.SetBasicAuth(username, password)
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp, body
}

// testWebSocket tests WebSocket connection
func testWebSocket(t *testing.T, proxyIP string, port int, hostHeader, path string) *websocket.Conn {
	url := fmt.Sprintf("ws://%s:%d%s", proxyIP, port, path)

	header := http.Header{}
	if hostHeader != "" {
		header.Set("Host", hostHeader)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, resp, err := dialer.Dial(url, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			t.Logf("WebSocket dial failed. Status: %d, Body: %s", resp.StatusCode, string(body))
		}
		require.NoError(t, err)
	}

	return conn
}

// testWebSocketSecure tests secure WebSocket connection
func testWebSocketSecure(t *testing.T, proxyIP string, port int, hostHeader, path string) *websocket.Conn {
	url := fmt.Sprintf("wss://%s:%d%s", proxyIP, port, path)

	header := http.Header{}
	if hostHeader != "" {
		header.Set("Host", hostHeader)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Accept self-signed certificates
		},
	}

	conn, resp, err := dialer.Dial(url, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			t.Logf("WebSocket dial failed. Status: %d, Body: %s", resp.StatusCode, string(body))
		}
		require.NoError(t, err)
	}

	return conn
}

// generateSelfSignedCert generates a self-signed certificate
func generateSelfSignedCert(t *testing.T, sslDir, hostname string) (string, string) {
	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   hostname,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{hostname},
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Write certificate
	certPath := filepath.Join(sslDir, "certs", hostname+".crt")
	certFile, err := os.Create(certPath)
	require.NoError(t, err)
	defer certFile.Close()

	err = pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	require.NoError(t, err)

	// Write private key
	keyPath := filepath.Join(sslDir, "private", hostname+".key")
	keyFile, err := os.Create(keyPath)
	require.NoError(t, err)
	defer keyFile.Close()

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	err = pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes})
	require.NoError(t, err)

	return certPath, keyPath
}

// assertResponseContains checks if response body contains expected string
func assertResponseContains(t *testing.T, body []byte, expected string) {
	require.True(t, strings.Contains(string(body), expected),
		"Expected response to contain %q, got: %s", expected, string(body))
}

// assertResponseEquals checks if response body equals expected string
func assertResponseEquals(t *testing.T, body []byte, expected string) {
	require.Equal(t, expected, string(body))
}
