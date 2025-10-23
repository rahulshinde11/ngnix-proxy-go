# End-to-End Tests

This directory contains comprehensive end-to-end tests for nginx-proxy-go. These tests create real Docker containers and verify the complete proxy functionality.

## Test Structure

### Test Files

- **`common_test.go`** - Shared test utilities and helper functions
- **`http_routing_test.go`** - HTTP routing, virtual hosts, path-based routing
- **`https_test.go`** - HTTPS support, SSL certificates, redirects
- **`websocket_test.go`** - WebSocket connections and upgrades
- **`basic_auth_test.go`** - Basic authentication (global and path-specific)
- **`redirect_test.go`** - Domain redirection (PROXY_FULL_REDIRECT)
- **`multi_container_test.go`** - Multiple containers, lifecycle events

## Running Tests

### Prerequisites

1. Docker must be running
2. Build the test image:
   ```bash
   docker build -t nginx-proxy-go:test .
   ```

3. Create the test network:
   ```bash
   docker network create nginx-proxy
   ```

### Run All E2E Tests

```bash
go test -v -tags=e2e -timeout 15m ./integration/e2e/...
```

### Run Specific Test Suites

```bash
# HTTP routing tests
go test -v -tags=e2e ./integration/e2e/ -run TestBasicVirtualHostRouting

# HTTPS tests
go test -v -tags=e2e ./integration/e2e/ -run TestHTTPS

# WebSocket tests
go test -v -tags=e2e ./integration/e2e/ -run TestWebSocket

# Basic auth tests
go test -v -tags=e2e ./integration/e2e/ -run TestBasicAuth

# Redirect tests
go test -v -tags=e2e ./integration/e2e/ -run TestRedirect

# Multi-container tests
go test -v -tags=e2e ./integration/e2e/ -run TestMultiple
```

### Using the Test Script

The easiest way to run tests is using the provided test script from the project root:

```bash
# Run all E2E tests
./test.sh e2e

# Run specific suites
./test.sh http
./test.sh https
./test.sh websocket
./test.sh auth
./test.sh redirect
./test.sh multi
```

## Test Infrastructure

### Test Environment

Each test sets up a complete test environment:

1. **Docker Network**: Isolated network for each test run
2. **nginx-proxy-go Container**: The proxy itself
3. **Backend Containers**: Test backends (nginx, echo-server, etc.)
4. **SSL Certificates**: Self-signed certificates for HTTPS tests
5. **Temp Directories**: Isolated storage for configs and certificates

### Test Flow

```
1. Create test environment (network, volumes, proxy container)
2. Start backend container(s) with VIRTUAL_HOST env vars
3. Wait for proxy to detect and configure the backend
4. Make HTTP/HTTPS/WebSocket requests with proper Host headers
5. Verify responses
6. Cleanup all containers and networks
```

### Backend Images Used

- **nginx:alpine** - Simple HTTP server for routing tests
- **jmalloc/echo-server** - WebSocket echo server

## Helper Functions

### Setup Functions

- `setupTestEnvironment(t)` - Creates complete test infrastructure
- `createBackendContainer(t, env, image, envVars, port)` - Creates backend container
- `generateSelfSignedCert(t, sslDir, hostname)` - Generates SSL certificates

### Request Functions

- `makeHTTPRequest(t, ip, port, host, path)` - HTTP request with Host header
- `makeHTTPSRequest(t, ip, port, host, path)` - HTTPS request
- `makeHTTPRequestWithAuth(t, ip, port, host, path, user, pass)` - HTTP with auth
- `testWebSocket(t, ip, port, host, path)` - WebSocket connection
- `testWebSocketSecure(t, ip, port, host, path)` - Secure WebSocket

### Wait Functions

- `waitForProxy(t, host, port, timeout)` - Wait for proxy to be ready
- `waitForBackendRegistration(t, timeout)` - Wait for backend to be registered

### Assertion Functions

- `assertResponseContains(t, body, expected)` - Check response contains string
- `assertResponseEquals(t, body, expected)` - Check exact response match

## Test Coverage

### HTTP Routing Tests

✅ Basic virtual host routing  
✅ Port-based routing  
✅ Path-based routing  
✅ Path rewriting  
✅ Host header validation  
✅ Multiple virtual hosts  
✅ Container port mapping  
✅ Custom nginx directives  
✅ Default server behavior  
✅ Multiple paths on same host  
✅ Container lifecycle  

### HTTPS Tests

✅ HTTPS virtual host  
✅ HTTP to HTTPS redirect  
✅ Self-signed certificate loading  
✅ Mixed HTTP and HTTPS  
✅ Multiple HTTPS hosts (SNI)  
✅ HTTPS with path routing  

### WebSocket Tests

✅ WebSocket upgrade  
✅ WebSocket with path  
✅ Bidirectional communication  
✅ Connection persistence  
✅ Secure WebSocket (WSS)  
✅ Path rewriting  
✅ Multiple WebSocket endpoints  
✅ Host header routing  
✅ Binary messages  
✅ Close handshake  

### Basic Auth Tests

✅ Global basic auth  
✅ Path-specific auth  
✅ HTTPS-only enforcement  
✅ Unauthorized access rejection  
✅ Different paths with auth  
✅ Header validation  

### Redirect Tests

✅ Simple redirect  
✅ Multiple source redirects  
✅ Port-based redirects  
✅ HTTPS redirects  
✅ Path preservation  
✅ Query string preservation  
✅ Multiple redirect rules  
✅ Different schemes  
✅ Subdomain redirects  

### Multi-Container Tests

✅ Multiple VIRTUAL_HOST variables  
✅ Multiple containers on same host  
✅ Container start event  
✅ Container stop event  
✅ Container restart event  
✅ Multiple paths same container  
✅ Multiple containers different paths  
✅ Mixed schemes (HTTP + WebSocket)  
✅ Container removal  
✅ Scaling containers  

## Writing New Tests

### Test Template

```go
//go:build e2e

package e2e

import (
    "testing"
    "time"
    
    "github.com/stretchr/testify/require"
)

func TestYourFeature(t *testing.T) {
    // Setup
    env := setupTestEnvironment(t)
    defer env.Cleanup()
    
    // Create backend with configuration
    backend := createBackendContainer(t, env, "nginx:alpine", map[string]string{
        "VIRTUAL_HOST": "test.example.com",
    }, "80/tcp")
    defer backend.Container.Terminate(nil)
    
    // Wait for registration
    waitForBackendRegistration(t, 5*time.Second)
    
    // Test
    resp, body := makeHTTPRequest(t, env.ProxyIP, 80, "test.example.com", "/")
    
    // Assert
    require.Equal(t, 200, resp.StatusCode)
    assertResponseContains(t, body, "expected content")
}
```

### Best Practices

1. **Isolation**: Each test should be independent
2. **Cleanup**: Always defer cleanup functions
3. **Timeouts**: Use reasonable timeouts for waits
4. **Host Headers**: Always set proper Host header for routing
5. **Assertions**: Use clear assertion messages
6. **Skip Long Tests**: Use `t.Skip()` for tests that take too long

## Debugging Tests

### Enable Verbose Output

```bash
go test -v -tags=e2e ./integration/e2e/ -run TestYourTest
```

### Check Container Logs

```bash
# List test containers
docker ps -a | grep test-

# View logs
docker logs <container-id>
```

### Inspect Test Environment

Tests create temporary directories. Check them before cleanup:

```bash
# Temporary SSL certs and configs are in /tmp/nginx-proxy-test-*
ls -la /tmp/nginx-proxy-test-*
```

### Common Issues

**Issue**: Tests timeout waiting for proxy
- **Solution**: Ensure Docker has enough resources
- Check: `docker logs <proxy-container>`

**Issue**: Backend not registered
- **Solution**: Wait longer (increase timeout)
- Check: Proxy container logs for "Valid configuration" messages

**Issue**: Connection refused
- **Solution**: Verify network connectivity
- Check: `docker network inspect <test-network>`

**Issue**: Certificate errors
- **Solution**: Verify SSL certificates are generated
- Check: Temp directory for `.crt` and `.key` files

## Cleanup

Tests automatically clean up after themselves, but if interrupted:

```bash
# Clean all test artifacts
./test.sh clean

# Or manually:
docker ps -a | grep "test-" | awk '{print $1}' | xargs docker rm -f
docker network ls | grep "test-network-" | awk '{print $1}' | xargs docker network rm
```

## Contributing

When adding new tests:

1. Follow the existing test structure
2. Use helper functions from `common_test.go`
3. Add meaningful test names
4. Include cleanup in defer statements
5. Document any special requirements
6. Update this README with new test coverage

## Resources

- [testcontainers-go Documentation](https://golang.testcontainers.org/)
- [Docker SDK for Go](https://docs.docker.com/engine/api/sdk/)
- [Gorilla WebSocket](https://github.com/gorilla/websocket)
- [Testify](https://github.com/stretchr/testify)

