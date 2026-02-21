# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

nginx-proxy-go is an automated Docker reverse proxy written in Go. It watches Docker events in real-time and dynamically generates Nginx configurations based on active containers. Supports HTTP/HTTPS, WebSocket (`ws://`/`wss://`), gRPC (`grpc://`/`grpcs://`), SSL automation via Let's Encrypt (ACMEv2), basic authentication, and domain redirects.

## Build & Development Commands

### Development (runs inside Docker)
```bash
./dev.sh start              # Build and start with logs
./dev.sh quick              # Restart using cache (no rebuild)
./dev.sh rebuild-code       # Rebuild Go code only (~5-10s), then restart
./dev.sh rebuild            # Full rebuild without cache
./dev.sh logs               # Follow container logs
./dev.sh shell              # Shell into container
./dev.sh clean              # Cleanup containers and volumes
```

### Testing
```bash
./test.sh unit              # Unit tests: go test -v -race ./internal/...
./test.sh integration       # Integration tests (requires Docker): go test -v -tags=integration ./integration/...
./test.sh e2e               # E2E tests (builds Docker image first): go test -v -tags=e2e -timeout 15m ./integration/e2e/...
./test.sh coverage          # Coverage report → coverage.html
```

Run a specific test:
```bash
go test -v -run TestFunctionName ./internal/package/...           # unit
go test -v -tags=e2e -run TestName -timeout 10m ./integration/e2e/  # e2e (must build image first)
```

Focused E2E suites via `test.sh`: `http`, `https`, `websocket`, `auth`, `redirect`, `multi`.

### Building locally (outside Docker)
```bash
go build -o nginx-proxy-go .
go build -o getssl ./cmd/getssl
```

### Linting (CI uses golangci-lint)

### Docker
```bash
docker build -t nginx-proxy-go .                    # Production image
docker build --target dev -t nginx-proxy-go:dev .   # Dev image (includes Delve debugger on :2345)
```

## Architecture

### Entry Point & Startup
`main.go` → creates Docker client → loads/validates config → creates `WebServer` → handles SIGINT/SIGTERM for graceful shutdown.

### Core Orchestrator: `internal/webserver/`
`WebServer` is the central component. It maintains:
- `hosts map[string]map[int]*host.Host` — virtual hosts keyed by domain then port
- `containers map[string]*appcontainer.Container` — tracked Docker containers
- `networks map[string]string` — known Docker networks

All maps are protected by `sync.RWMutex` for concurrent access.

### Package Responsibilities

| Package | Role |
|---------|------|
| `internal/config` | Environment variable loading and validation |
| `internal/constants` | Ports, timeouts, SSL defaults, ACME endpoints |
| `internal/container` | Parses Docker container metadata (env vars, labels, ports, networks) |
| `internal/dockerapi` | Thin wrapper around Docker SDK client |
| `internal/event` | Listens to Docker events, routes container lifecycle changes |
| `internal/host` | Virtual host model and `VIRTUAL_HOST` format parser |
| `internal/nginx` | Renders `templates/nginx.conf.tmpl` via `text/template`, runs `nginx -t` and `nginx -s reload` |
| `internal/processor` | Request processors: virtual host parsing, basic auth (htpasswd), redirects, default server |
| `internal/ssl` | ACME certificate issuance, self-signed fallback, background renewal (7 days before expiry) |
| `internal/acme` | ACMEv2 protocol implementation |
| `internal/errors` | Typed errors (Docker, Nginx, Config, Network, Container, SSL, System) with context |
| `internal/logger` | Structured logging (JSON/text), levels (DEBUG→FATAL), file rotation |
| `internal/health` | Health check library (nginx config validation, Docker connectivity) |
| `internal/server` | Server management |
| `internal/debug` | Delve debugger integration |
| `cmd/getssl` | CLI tool for manual SSL certificate management |

### Virtual Host Format
Containers declare routing via the `VIRTUAL_HOST` environment variable:
- `example.com` — HTTP on port 80
- `https://example.com` — HTTPS on 443
- `example.com:8080` — custom port
- `example.com/api -> :8080` — path-based routing to container port
- `grpc://api.example.com -> :50051` — gRPC proxying
- `wss://example.com/ws -> :8080` — WebSocket

### Nginx Template
`templates/nginx.conf.tmpl` generates full Nginx config from Host/Location models. Base config is at `nginx/nginx.conf`.

### Event-Driven Flow
Docker events (container start/stop/restart, network connect/disconnect) → `internal/event` → WebServer updates host/container maps → re-renders Nginx config → validates with `nginx -t` → reloads with `nginx -s reload`.

## Testing Strategy

- **Unit tests** (`./internal/...`): no build tags, use `testify` for assertions
- **Integration tests** (`./integration/`, tag `integration`): require running Docker daemon
- **E2E tests** (`./integration/e2e/`, tag `e2e`): use `testcontainers-go` to spin up real containers, require building the Docker image first (`docker build -t nginx-proxy-go:test .`)

## Key Dependencies

- Go 1.23, `github.com/docker/docker` v27.1.1, `github.com/gorilla/websocket`, `golang.org/x/crypto`
- Testing: `github.com/stretchr/testify`, `github.com/testcontainers/testcontainers-go`

## CI/CD

GitHub Actions (`.github/workflows/test-and-publish.yml`): runs unit tests (with race detection), integration tests, E2E tests, golangci-lint, multi-platform Docker build (linux/amd64, linux/arm64), and publishes to Docker Hub on main branch.
