# Nginx-Proxy-Go

[![Test and Publish](https://github.com/rahulshinde/nginx-proxy-go/actions/workflows/test-and-publish.yml/badge.svg)](https://github.com/rahulshinde/nginx-proxy-go/actions/workflows/test-and-publish.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/rahulshinde/nginx-proxy-go)](https://goreportcard.com/report/github.com/rahulshinde/nginx-proxy-go)

A Docker container for automatically creating nginx configuration based on active containers in docker host, written in Go.

## ⚠️ Development Warning

**This project is still in active development and should be used at your own risk.** While we strive to maintain stability, breaking changes may occur without notice. Please test thoroughly in a non-production environment before deploying to production systems.

## Features

- **Easy Configuration**: Server configuration with environment variables
- **Multi-container Support**: Map multiple containers to different locations on same server
- **SSL Automation**: Automatic Let's Encrypt SSL certificate registration and renewal
- **Basic Authentication**: Global and path-specific basic auth support
- **WebSocket Support**: Full WebSocket proxy support with proper headers
- **gRPC Support**: Native gRPC and gRPCs (secure gRPC) proxy with HTTP/2
- **Virtual Hosts**: Multiple virtual hosts on same container with VIRTUAL_HOST1, VIRTUAL_HOST2, etc.
- **Redirection**: Domain redirection support with PROXY_FULL_REDIRECT
- **Default Server**: Default server configuration for unmatched requests
- **SSL Management**: Complete SSL certificate lifecycle management with renewal
- **Self-signed Fallback**: Automatic self-signed certificate generation when ACME fails
- **Manual SSL Tools**: `getssl` CLI tool for manual certificate management
- **Debug Support**: Integrated Delve debugger for development
- **Structured Logging**: JSON and text log formats with configurable levels and context
- **Enhanced Error Handling**: Comprehensive error handling with retry logic and detailed diagnostics
- **Multi-platform**: Support for linux/amd64 and linux/arm64 architectures
- **Development Workflow**: Optimized development scripts with hot-reloading

## Quick Setup

### Docker Image

The project provides two ways to run the container:

1. **Development/Testing**: Use the local build
2. **Production**: Use the published image `shinde11/nginx-proxy`

### Setup nginx-proxy-go

```bash
# Create a network for nginx proxy
docker network create frontend

# Option 1: Use published image (recommended for production)
docker run --network frontend \
    --name nginx-proxy-go \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v /etc/ssl:/etc/ssl/custom \
    -v /etc/nginx/dhparam:/etc/nginx/dhparam \
    -p 80:80 \
    -p 443:443 \
    -d --restart always \
    shinde11/nginx-proxy

# Option 2: Build and run locally (for development)
docker build -t nginx-proxy-go .
docker run --network frontend \
    --name nginx-proxy-go \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v /etc/ssl:/etc/ssl/custom \
    -v /etc/nginx/dhparam:/etc/nginx/dhparam \
    -p 80:80 \
    -p 443:443 \
    -d --restart always \
    nginx-proxy-go
```

### Setup your container

The only requirement is that your container shares at least one common network with the nginx-proxy-go container and has the `VIRTUAL_HOST` environment variable set.

Examples:

- **WordPress**
```bash
docker run --network frontend \
    --name wordpress-server \
    -e VIRTUAL_HOST="wordpress.example.com" \
    wordpress
```

- **Docker Registry**
```bash
docker run --network frontend \
    --name docker-registry \
    -e VIRTUAL_HOST='https://registry.example.com/v2 -> /v2; client_max_body_size 2g' \
    -e PROXY_BASIC_AUTH="registry.example.com -> user1:password,user2:password2,user3:password3" \
    registry:2
```

## Configuration Guide

### Configure nginx-proxy-go

The following directories can be mounted as volumes to persist configurations:
- `/etc/nginx/conf.d` - nginx configuration directory
- `/etc/nginx/dhparam` - directory for storing DH parameters for SSL connections
- `/etc/ssl/custom` - directory for storing SSL certificates and private keys
- `/var/log/nginx` - nginx logs directory
- `/tmp/acme-challenges` - directory for Let's Encrypt challenges

Environment variables for customizing behavior:
- `CLIENT_MAX_BODY_SIZE` (default: 1m) - Default max body size for all servers
- `NGINX_CONF_DIR` (default: /etc/nginx in containers, ./nginx for local dev) - Nginx configuration directory
- `CHALLENGE_DIR` (default: /tmp/acme-challenges in containers, ./acme-challenges for local dev) - ACME challenge directory
- `SSL_DIR` (default: /etc/ssl/custom in containers, ./ssl for local dev) - SSL certificates directory
- `DEFAULT_HOST` (default: true) - Enable default server configuration
- `GO_DEBUG_ENABLE` (default: false) - Enable debug mode
- `GO_DEBUG_PORT` (default: 2345) - Debug port for Delve debugger
- `GO_DEBUG_HOST` (default: "") - Debug host binding (empty for all interfaces)
- `LETSENCRYPT_API` (default: https://acme-v02.api.letsencrypt.org/directory) - ACME API URL for SSL certificates

### Virtual Host Configuration

Set the `VIRTUAL_HOST` environment variable to configure your container's domain. The container must:
- Share a network with nginx-proxy-go
- Expose the required port
- Have the correct port binding

Virtual host configuration examples:

| VIRTUAL_HOST | Release Address | Container Path | Container Port |
|--------------|----------------|----------------|----------------|
| example.com | http://example.com | / | exposed port |
| example.com:8080 | http://example.com:8080 | / | exposed port |
| example.com -> :8080 | http://example.com | / | 8080 |
| https://example.com | https://example.com | / | exposed port |
| example.com/api | http://example.com/api | /api | exposed port |
| example.com/api/ -> / | http://example.com/api | / | exposed port |
| wss://example.com/websocket | wss://example.com/websocket | / | exposed port |
| grpc://api.example.com | grpc://api.example.com | / | 50051 (default) |
| grpc://api.example.com -> :50051 | grpc://api.example.com | / | 50051 |
| grpcs://api.example.com | grpcs://api.example.com | / | exposed port |
| grpc://api.example.com/v1 | grpc://api.example.com/v1 | /v1 | 50051 (default) |

### WebSocket Support

To enable WebSocket support, explicitly configure the WebSocket endpoint in the virtual host:

```bash
-e "VIRTUAL_HOST=wss://ws.example.com -> :8080/websocket"
```

### gRPC Support

The proxy supports both gRPC and gRPCs (secure gRPC) with native HTTP/2 proxying. Configure gRPC services using the `grpc://` or `grpcs://` scheme:

```bash
# Basic gRPC
-e "VIRTUAL_HOST=grpc://api.example.com -> :50051"

# Secure gRPC with SSL
-e "VIRTUAL_HOST=grpcs://api.example.com -> :50051"

# gRPC with path routing
-e "VIRTUAL_HOST=grpc://api.example.com/v1/users -> :50051"

# Sharing port 443 with HTTPS and gRPCs
-e "VIRTUAL_HOST1=https://api.example.com/ -> :8080"
-e "VIRTUAL_HOST2=grpcs://api.example.com/grpc -> :50051"

# All protocols on same domain (HTTP, WebSocket, and gRPC)
-e "VIRTUAL_HOST1=https://app.example.com/ -> :3000"
-e "VIRTUAL_HOST2=wss://app.example.com/ws -> :3001"
-e "VIRTUAL_HOST3=grpcs://app.example.com/api -> :50051"
```

**Note**: gRPC requires HTTP/2, which is automatically enabled for all SSL connections. The default port for gRPC is 50051.

### Multiple Virtual Hosts

Use `VIRTUAL_HOST1`, `VIRTUAL_HOST2`, etc. to configure multiple virtual hosts for a single container:

```bash
docker run -d --network frontend \
    -e "VIRTUAL_HOST1=https://ethereum.example.com -> :8545" \
    -e "VIRTUAL_HOST2=wss://ethereum.example.com/ws -> :8546" \
    ethereum/client-go \
    --rpc --rpcaddr "0.0.0.0" --ws --wsaddr 0.0.0.0
```

### Redirection

Use `PROXY_FULL_REDIRECT` to redirect multiple domains to your main domain:

```bash
-e 'VIRTUAL_HOST=https://example.uk -> :7000' \
-e 'PROXY_FULL_REDIRECT=example.com,www.example.com,www.example.uk->example.uk'
```

### SSL Support

The container automatically handles SSL certificate issuance using Let's Encrypt. If domain ownership cannot be verified, a self-signed certificate is generated.

#### Using Your Own SSL Certificate

Place your SSL certificate and private key in the container:
- Certificate: `/etc/ssl/custom/certs/domain.crt`
- Private key: `/etc/ssl/custom/private/domain.key`

Wildcard certificates are supported (e.g., `*.example.com`).

#### Manual Certificate Issuance

Use the following command to manually obtain certificates:

```bash
# Single domain
docker exec nginx-proxy-go getssl www.example.com

# Multiple domains
docker exec nginx-proxy-go getssl www.example.com example.com www.example.com

# With options
docker exec nginx-proxy-go getssl --new --skip-dns-check example.com

# Force renewal
docker exec nginx-proxy-go getssl --force example.com
```

The `getssl` command supports the following options:
- `--skip-dns-check`: Skip DNS validation
- `--new`: Override existing certificates
- `--force`: Force certificate issuance without checks
- `--api=URL`: Specify ACME API URL (default: Let's Encrypt production)
- `--ssl-dir=DIR`: SSL certificate directory (default: /etc/ssl/custom)
- `--challenge-dir=DIR`: ACME challenge directory (default: /tmp/acme-challenges)

### Basic Authorization

Enable basic auth using the `PROXY_BASIC_AUTH` environment variable:

```bash
# Global basic auth
-e "PROXY_BASIC_AUTH=user1:password1,user2:password2"

# Path-specific basic auth
-e "PROXY_BASIC_AUTH=example.com/api/v1/admin -> admin1:password1,admin2:password2"
```

Note: Basic auth is ignored for non-HTTPS connections.

### Default Server

By default, requests to unregistered server names return a 503 error. To forward these requests to a container, add:

```bash
-e "PROXY_DEFAULT_SERVER=true"
```

Note: The default server configuration is controlled by the `DEFAULT_HOST` environment variable (default: true).

For HTTPS connections, consider setting up wildcard certificates to avoid SSL certificate errors.

## Testing

The project includes comprehensive test suites to ensure reliability and correctness.

### Test Categories

1. **Unit Tests** - Test individual components and functions
2. **Integration Tests** - Test Docker integration and container processing
3. **End-to-End Tests** - Test complete workflows with real containers

### Running Tests

The project includes a convenient `test.sh` script for running different test suites:

```bash
# Run all tests
./test.sh all

# Run specific test suites
./test.sh unit          # Unit tests only
./test.sh integration   # Integration tests
./test.sh e2e           # End-to-end tests
./test.sh http          # HTTP routing tests
./test.sh https         # HTTPS tests
./test.sh websocket     # WebSocket tests
./test.sh auth          # Basic auth tests
./test.sh redirect      # Redirect tests
./test.sh multi         # Multi-container tests

# Generate coverage report
./test.sh coverage

# Clean up test artifacts
./test.sh clean
```

#### Unit Tests

Run unit tests for all internal packages:

```bash
go test ./...
```

With coverage:

```bash
go test -v -race -coverprofile=coverage.txt -covermode=atomic ./internal/...
```

#### Integration Tests

Integration tests require Docker to be running:

```bash
go test -tags=integration ./integration/...
```

#### End-to-End Tests

E2E tests create real containers and test the complete proxy functionality:

```bash
# Build the test image first
docker build -t nginx-proxy-go:test .

# Create test network
docker network create nginx-proxy || true

# Run E2E tests
go test -v -tags=e2e -timeout 15m ./integration/e2e/...
```

Run specific E2E test suites:

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

#### Run All Tests

```bash
# Run all tests including integration and e2e
go test -tags=integration,e2e ./...
```

### Test Coverage

The comprehensive test suite provides extensive coverage across all major functionality:

#### **Unit Tests** (`go test ./internal/...`)
- **Configuration Management**: Environment variable parsing, validation, directory setup
- **Container Processing**: Environment parsing, network reachability, port detection
- **Error Handling**: Retry logic, context propagation, error categorization
- **Virtual Host Processing**: VIRTUAL_HOST parsing, scheme detection, extras handling
- **Basic Authentication**: Credential processing, htpasswd compatibility, user validation
- **SSL Certificate Management**: Certificate validation, expiry checking, renewal logic

#### **Integration Tests** (`go test -tags=integration ./integration/...`)
- **Docker Integration**: Container inspection, network connectivity, event processing
- **Nginx Configuration**: Template rendering, configuration validation, reload handling
- **ACME Operations**: Certificate issuance, challenge handling, DNS validation

#### **End-to-End Tests** (`go test -tags=e2e ./integration/e2e/...`)
- ✅ **HTTP Routing**: Virtual hosts, path routing, host headers, port mapping
- ✅ **HTTPS**: SSL certificates, HTTP to HTTPS redirects, SNI support, certificate validation
- ✅ **WebSocket**: WebSocket upgrades, bidirectional communication, secure WebSocket (WSS)
- ✅ **Basic Authentication**: Global and path-specific auth, HTTPS enforcement, multiple users
- ✅ **Domain Redirects**: Simple and multiple source redirects, path preservation, PROXY_FULL_REDIRECT
- ✅ **Multi-Container**: Multiple virtual hosts, container lifecycle, upstream load balancing
- ✅ **Container Events**: Start, stop, restart, network connect/disconnect, event processing
- ✅ **SSL Lifecycle**: Certificate issuance, renewal, self-signed fallback, manual tools
- ✅ **Configuration Options**: Environment variables, default server, custom nginx directives

#### **Test Statistics**
- **Test Categories**: 8 distinct test suites with specialized focus areas
- **Container Scenarios**: 15+ real container configurations tested
- **SSL Scenarios**: Certificate issuance, renewal, and error handling
- **Network Scenarios**: Multiple network configurations and connectivity patterns

### Continuous Integration

All tests run automatically on:
- Every push to `main`, `develop`, and `tests` branches
- All pull requests to `main` and `develop`

The CI pipeline:
1. Runs unit tests with race detection
2. Runs integration tests
3. Builds Docker image
4. Runs end-to-end tests
5. Publishes Docker image (on main branch only)

View test results and coverage in [GitHub Actions](https://github.com/rahulshinde/nginx-proxy-go/actions).

## Development

### Quick Start Development Setup

The project includes an optimized development workflow using the `dev.sh` script that significantly reduces build times through Docker layer caching and bind mounts.

```bash
# Clone the repository
git clone https://github.com/rahulshinde/nginx-proxy-go.git
cd nginx-proxy-go

# Create the required network (if it doesn't exist)
docker network create nginx-proxy

# Start development environment (first time - downloads dependencies)
./dev.sh start
```

> **Note:** The development environment uses a multi-stage Docker build with a development stage that includes Delve debugger support and hot-reloading capabilities.

> **Note:** The project includes a comprehensive `.gitignore` file that excludes binaries, build artifacts, SSL certificates, and other development files from version control.

### Development Workflow Commands

The `dev.sh` script provides optimized commands for different development scenarios with Docker layer caching and bind mounts for fast iteration:

#### **Core Development Commands:**

```bash
# Initial setup - builds everything, shows logs (first time setup)
./dev.sh start

# Quick restart without rebuild - for config changes
./dev.sh quick

# Fast code rebuild - rebuilds only Go binaries (~5-10 seconds)
./dev.sh rebuild-code

# Quick restart in background (free up terminal)
./dev.sh quick-bg

# Fast code rebuild in background
./dev.sh rebuild-code-bg
```

#### **Testing Commands:**

```bash
# Run unit tests only
go test ./...

# Run integration tests (requires Docker)
go test -tags=integration ./...

# Run end-to-end tests (requires Docker)
go test -tags=e2e ./...

# Use convenience test script with categories
./test.sh all          # Run all tests
./test.sh unit         # Unit tests only
./test.sh integration  # Integration tests
./test.sh e2e          # End-to-end tests
./test.sh http         # HTTP routing tests
./test.sh https        # HTTPS tests
./test.sh websocket    # WebSocket tests
./test.sh auth         # Basic auth tests
./test.sh redirect     # Redirect tests
./test.sh multi        # Multi-container tests
```

#### **Debugging & Monitoring:**

```bash
# Follow container logs in real-time
./dev.sh logs

# Open interactive shell inside container
./dev.sh shell

# Enable debug mode with Delve debugger (port 2345)
-e GO_DEBUG_ENABLE=true

# View structured logs with different levels
# Logs support JSON and text formats with context
```

#### **Maintenance Commands:**

```bash
# Full rebuild without cache (for major changes)
./dev.sh rebuild

# Clean up containers, volumes, and networks
./dev.sh clean

# Build and publish multi-platform Docker images
./publish.sh

# Manual SSL certificate management
docker exec nginx-proxy-go getssl example.com
```

### Typical Development Workflow

1. **Initial Setup:**
   ```bash
   ./dev.sh start  # Watch it start up, downloads dependencies once
   ```

2. **Make Code Changes:**
   - Edit your Go files
   - Modify configurations
   - Update templates

3. **Test Changes (Super Fast):**
   ```bash
   ./dev.sh rebuild-code  # Rebuilds + shows logs immediately
   ```

4. **Iterate:** Repeat steps 2-3 for rapid development

### Performance Optimizations

The development setup includes several optimizations:

- **Go Module Caching:** Dependencies are cached in Docker volumes and only downloaded once
- **Build Cache:** Go build cache is persisted across container restarts
- **Bind Mounts:** Source code is mounted directly, eliminating copy operations
- **Multi-stage Build:** Development stage optimized for fast iteration
- **Layer Caching:** Docker layers are efficiently cached to minimize rebuild time

### Speed Comparison

| Operation | Before | After |
|-----------|--------|--------|
| **Code Changes** | 2-3 minutes | 5-10 seconds |
| **Dependency Changes** | 2-3 minutes | 30-60 seconds |
| **Full Rebuild** | 3-4 minutes | 1-2 minutes |

### Debugging

The development container includes comprehensive debugging support:

- **Delve Debugger:** Port 2345 is exposed for remote debugging
- **Debug Mode:** Set `GO_DEBUG_ENABLE=true` (default in dev mode)
- **Live Logs:** Real-time container logs for immediate feedback
- **Debug Configuration:** Environment variables for debug host and port

### Logging & Error Handling

The application features advanced logging and error handling capabilities:

- **Structured Logging:** JSON and text formats with configurable log levels (DEBUG, INFO, WARN, ERROR)
- **Context-Aware:** All log entries include relevant context information
- **Error Recovery:** Automatic retry logic with exponential backoff for transient failures
- **Detailed Diagnostics:** Comprehensive error messages with operation context
- **Configurable Output:** Logs can be written to files or stdout

### Health Monitoring

The project includes a comprehensive health check library in `internal/health/`:

- **Health Check Components:** Nginx configuration validation, Docker daemon connectivity
- **Status Management:** Healthy, degraded, and unhealthy status levels
- **Metrics Support:** Extensible metrics collection and reporting

**Note:** The health check library is currently not exposed as HTTP endpoints. Integration is planned for future releases. The library provides the foundation for `/health`, `/ready`, and `/live` endpoints when implemented.

#### Debug Environment Variables

- `GO_DEBUG_ENABLE`: Enable/disable debug mode (default: false)
- `GO_DEBUG_PORT`: Debug port for Delve (default: 2345)
- `GO_DEBUG_HOST`: Debug host binding (default: empty for all interfaces)

### Traditional Building (Alternative)

If you prefer the traditional approach:

```bash
# Build the container
docker build -t nginx-proxy-go .

# Run in production mode
./run.sh

# Run in debug mode
./run-debug.sh

# Or manually run with debug support
docker run --rm -it \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v nginx-proxy-go-nginx:/etc/nginx \
  -v nginx-proxy-go-ssl:/etc/ssl/custom \
  -v nginx-proxy-go-acme:/tmp/acme-challenges \
  -e GO_DEBUG_ENABLE=true \
  -p 80:80 -p 443:443 -p 2345:2345 \
  nginx-proxy-go
```

### Project Structure

```
nginx-proxy-go/
├── cmd/
│   └── getssl/        # Manual SSL certificate management CLI
├── internal/          # Internal packages
│   ├── acme/         # ACME/Let's Encrypt integration
│   ├── config/       # Configuration handling
│   ├── constants/    # Application constants
│   ├── container/    # Container management
│   ├── debug/        # Debug mode support
│   ├── dockerapi/    # Docker API client wrapper
│   ├── errors/       # Error handling
│   ├── event/        # Event processing
│   ├── health/       # Health check library (not yet exposed as endpoints)
│   ├── host/         # Host configuration
│   ├── logger/       # Logging
│   ├── nginx/        # Nginx configuration
│   ├── processor/    # Request processing (basic auth, redirects, etc.)
│   ├── server/       # Server management
│   ├── ssl/          # SSL certificate management
│   └── webserver/    # Main web server
├── integration/      # Integration and E2E tests
│   └── e2e/         # End-to-end test suites
├── nginx/            # Nginx configuration files
├── templates/        # Nginx configuration templates
├── Dockerfile        # Multi-stage container definition
├── docker-compose.yml # Development environment
├── docker-compose-prod.yaml # Production environment
├── docker-compose.override.yml # Local overrides
├── dev.sh           # Development workflow script
├── test.sh          # Test runner script
├── build.sh         # Build script
├── run.sh           # Production run script
├── run-debug.sh     # Debug run script
├── publish.sh       # Multi-platform publish script
├── docker-entrypoint.sh # Container entrypoint
├── main.go          # Application entry point
├── go.mod           # Go module definition
└── go.sum           # Go module checksums
```

### Key Components

- **Main Application** (`main.go`): Entry point with graceful shutdown and signal handling
- **WebServer** (`internal/webserver/`): Core nginx proxy server with Docker integration
- **SSL Manager** (`internal/ssl/`): Complete SSL certificate lifecycle management
- **ACME Integration** (`internal/acme/`): Let's Encrypt certificate automation
- **Processors** (`internal/processor/`): Basic auth, redirects, default server handling
- **Configuration** (`internal/config/`): Environment variable and configuration management
- **Docker API** (`internal/dockerapi/`): Docker client wrapper for container operations
- **Event Processing** (`internal/event/`): Docker event handling and processing
- **Health Checks** (`internal/health/`): Health check library (not yet exposed as HTTP endpoints)
- **Structured Logging** (`internal/logger/`): Advanced logging with multiple formats and levels
- **Error Handling** (`internal/errors/`): Comprehensive error handling with retry logic
- **Debug Support** (`internal/debug/`): Delve debugger integration
- **CLI Tools** (`cmd/getssl/`): Manual SSL certificate management

## Docker Image

### Published Images

The project maintains a published Docker image on Docker Hub:

- **Image**: `shinde11/nginx-proxy`
- **Tags**: `latest`, version tags (e.g., `v1.0.0`)
- **Platforms**: linux/amd64, linux/arm64
- **Multi-platform**: Built using Docker buildx for cross-platform compatibility

### Building and Publishing

The project includes a comprehensive publishing script (`publish.sh`) that supports:

```bash
# Build and publish multi-platform image
./publish.sh

# Build with specific version
./publish.sh v1.2.3

# Build only (don't push)
./publish.sh --build-only

# Single platform build (faster for testing)
./publish.sh --single-platform

# Build without cache
./publish.sh --no-cache
```

### Local Development

For local development, use the development scripts:

```bash
# Start development environment
./dev.sh start

# Quick restart
./dev.sh quick

# Rebuild code only
./dev.sh rebuild-code

# Follow logs
./dev.sh logs
```

## License

This project is licensed under the MIT License - see the LICENSE file for details. 