# Nginx-Proxy-Go

A Docker container for automatically creating nginx configuration based on active containers in docker host, written in Go.

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
    -v /etc/ssl:/etc/ssl \
    -v /etc/nginx/dhparam:/etc/nginx/dhparam \
    -p 80:80 \
    -p 443:443 \
    -d --restart always \
    shinde11/nginx-proxy
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
- `/etc/ssl` - directory for storing SSL certificates and private keys
- `/var/log/nginx` - nginx logs directory
- `/tmp/acme-challenges` - directory for Let's Encrypt challenges

Environment variables for customizing behavior:
- `DHPARAM_SIZE` (default: 2048) - Size of DH parameter for SSL certificates
- `CLIENT_MAX_BODY_SIZE` (default: 1m) - Default max body size for all servers

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
- Certificate: `/etc/ssl/certs/domain.crt`
- Private key: `/etc/ssl/private/domain.key`

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

For HTTPS connections, consider setting up wildcard certificates to avoid SSL certificate errors.

## Development

### Building from Source

```bash
# Clone the repository
git clone https://github.com/rahulshinde/nginx-proxy-go.git
cd nginx-proxy-go

# Build the container
./build.sh

# Run in development mode
./run-debug.sh
```

### Project Structure

```
nginx-proxy-go/
├── internal/           # Internal packages
│   ├── config/        # Configuration handling
│   ├── container/     # Container management
│   ├── errors/        # Error handling
│   ├── event/         # Event processing
│   ├── host/          # Host configuration
│   ├── logger/        # Logging
│   ├── nginx/         # Nginx configuration
│   └── processor/     # Request processing
├── nginx/             # Nginx configuration files
├── acme-challenges/   # Let's Encrypt challenges
├── ssl/              # SSL certificates and keys
├── Dockerfile        # Container definition
├── docker-compose.yml # Development environment
├── main.go           # Application entry point
└── go.mod            # Go module definition
```

## License

This project is licensed under the MIT License - see the LICENSE file for details. 