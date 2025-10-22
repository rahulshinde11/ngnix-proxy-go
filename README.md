# Nginx-Proxy-Go

A Docker container for automatically creating nginx configuration based on active containers in docker host, written in Go.

## ⚠️ Development Warning

**This project is still in active development and should be used at your own risk.** While we strive to maintain stability, breaking changes may occur without notice. Please test thoroughly in a non-production environment before deploying to production systems.

## Features

- Easy server configuration with environment variables
- Map multiple containers to different locations on same server
- Automatic Let's Encrypt SSL certificate registration
- Basic Authorization support
- WebSocket support
- Multiple virtual hosts on same container
- Redirection support
- Default server configuration

## Quick Setup

### Setup nginx-proxy-go

```bash
# Create a network for nginx proxy
docker network create frontend

# Run the nginx-proxy-go container
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
- `DHPARAM_SIZE` (default: 2048) - Size of DH parameter for SSL certificates
- `CLIENT_MAX_BODY_SIZE` (default: 1m) - Default max body size for all servers
- `NGINX_CONF_DIR` (default: /etc/nginx) - Nginx configuration directory
- `CHALLENGE_DIR` (default: /tmp/acme-challenges) - ACME challenge directory
- `SSL_DIR` (default: /etc/ssl/custom) - SSL certificates directory
- `GO_DEBUG_ENABLE` (default: false) - Enable debug mode
- `GO_DEBUG_PORT` (default: 2345) - Debug port for Delve debugger

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

### WebSocket Support

To enable WebSocket support, explicitly configure the WebSocket endpoint in the virtual host:

```bash
-e "VIRTUAL_HOST=wss://ws.example.com -> :8080/websocket"
```

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
```

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

The `dev.sh` script provides optimized commands for different development scenarios:

#### **For Active Development (Stay in Terminal):**
```bash
# Initial setup - builds everything, shows logs
./dev.sh start

# Quick restart without rebuild - for config changes
./dev.sh quick  

# Fast code rebuild - rebuilds only Go binaries (~5-10 seconds)
./dev.sh rebuild-code
```

#### **Background Mode (Free Up Terminal):**
```bash
# Quick restart in background
./dev.sh quick-bg

# Fast code rebuild in background  
./dev.sh rebuild-code-bg
```

#### **Utility Commands:**
```bash
# Follow container logs anytime
./dev.sh logs

# Open shell inside container for debugging
./dev.sh shell

# Full rebuild without cache (for major changes)
./dev.sh rebuild

# Clean up containers and volumes
./dev.sh clean
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

The development container includes debugging support:

- **Delve Debugger:** Port 2345 is exposed for remote debugging
- **Debug Mode:** Set `GO_DEBUG_ENABLE=true` (default in dev mode)
- **Live Logs:** Real-time container logs for immediate feedback

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
│   ├── container/    # Container management
│   ├── debug/        # Debug mode support
│   ├── errors/       # Error handling
│   ├── event/        # Event processing
│   ├── host/         # Host configuration
│   ├── logger/       # Logging
│   ├── nginx/        # Nginx configuration
│   ├── processor/    # Request processing (basic auth, redirects, etc.)
│   ├── server/       # Server management
│   ├── ssl/          # SSL certificate management
│   └── webserver/    # Main web server
├── nginx/            # Nginx configuration files
├── templates/        # Nginx configuration templates
├── Dockerfile        # Multi-stage container definition
├── docker-compose.yml # Development environment
├── docker-compose-prod.yaml # Production environment
├── dev.sh           # Development workflow script
├── build.sh         # Build script
├── run.sh           # Production run script
├── run-debug.sh     # Debug run script
├── publish.sh       # Multi-platform publish script
├── main.go          # Application entry point
└── go.mod           # Go module definition
```

## License

This project is licensed under the MIT License - see the LICENSE file for details. 