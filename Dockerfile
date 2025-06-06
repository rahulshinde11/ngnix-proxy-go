# -------------------
# Build stage
# -------------------
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies and CA certificates
RUN apk update \
    && apk upgrade \
    && apk add --no-cache git ca-certificates \
    && update-ca-certificates 2>/dev/null || true

# Set Go to use system certificates
ENV SSL_CERT_DIR=/etc/ssl/certs
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
ENV GOPROXY=direct
ENV SSL_DIR=/etc/ssl/custom

# Copy go.mod and go.sum first to leverage Docker caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the main application
RUN go build -o nginx-proxy-go .

# Build getssl command
RUN GOOS=linux go build -o getssl ./cmd/getssl

# Install Delve debugger
RUN go install github.com/go-delve/delve/cmd/dlv@latest

# -------------------
# Development stage (for local dev/debug)
# -------------------
FROM builder AS dev

WORKDIR /app/src

# Copy Delve for debugging
COPY --from=builder /go/bin/dlv /usr/local/bin/

# Install runtime dependencies
RUN apk update \
    && apk upgrade \
    && apk add --no-cache nginx openssl ca-certificates \
    && update-ca-certificates 2>/dev/null || true

# Create necessary directories
RUN mkdir -p /etc/nginx/conf.d /var/log/nginx /var/cache/nginx /etc/ssl/custom

# Copy static/config files
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY templates/ ./templates/
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Expose ports
EXPOSE 80 443 2345

# Set environment variables
ENV GO_DEBUG_ENABLE="true" \
    GO_DEBUG_PORT="2345" \
    GO_DEBUG_HOST="" \
    NGINX_CONF_DIR=/etc/nginx \
    CHALLENGE_DIR=/tmp/acme-challenges \
    SSL_DIR=/etc/ssl/custom

# CMD rebuilds the Go app before running, useful for local development
CMD ["sh", "-c", "go build -buildvcs=false -o nginx-proxy-go . && go build -buildvcs=false -o /usr/local/bin/getssl ./cmd/getssl && /docker-entrypoint.sh"]

# -------------------
# Final production stage
# -------------------
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk update \
    && apk upgrade \
    && apk add --no-cache nginx openssl ca-certificates \
    && update-ca-certificates 2>/dev/null || true

# Copy built binaries and scripts from builder
COPY --from=builder /app/nginx-proxy-go .
COPY --from=builder /app/getssl /usr/local/bin/
COPY --from=builder /go/bin/dlv /usr/local/bin/

# Copy nginx configuration and app templates
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY templates/ ./templates/

# Copy entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Create required directories
RUN mkdir -p /etc/nginx/conf.d /var/log/nginx /var/cache/nginx /etc/ssl/custom

# Expose necessary ports
EXPOSE 80 443 2345

# Set environment variables
ENV GO_DEBUG_ENABLE="false" \
    GO_DEBUG_PORT="2345" \
    GO_DEBUG_HOST="" \
    NGINX_CONF_DIR=/etc/nginx \
    CHALLENGE_DIR=/tmp/acme-challenges \
    SSL_DIR=/etc/ssl/custom

# Run the application
ENTRYPOINT ["/docker-entrypoint.sh"]