# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o nginx-proxy-go .

# Build getssl command
RUN CGO_ENABLED=0 GOOS=linux go build -o getssl ./cmd/getssl

# Install Delve
RUN go install github.com/go-delve/delve/cmd/dlv@latest

# Development stage - for faster iteration
FROM builder AS dev

WORKDIR /app/src

# Development dependencies installed in builder stage

# Copy Delve for debugging
COPY --from=builder /go/bin/dlv /usr/local/bin/

# Install runtime dependencies
RUN apk add --no-cache nginx openssl

# Create necessary directories
RUN mkdir -p /etc/nginx/conf.d /var/log/nginx /var/cache/nginx

# Copy static files that don't change often
COPY nginx/nginx.conf /etc/nginx/nginx.conf
COPY templates/ ./templates/
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Expose ports
EXPOSE 80 443 2345

# Set environment variables
ENV GO_DEBUG_ENABLE="true"
ENV GO_DEBUG_PORT="2345"
ENV GO_DEBUG_HOST=""
ENV NGINX_CONF_DIR=/etc/nginx
ENV CHALLENGE_DIR=/tmp/acme-challenges
ENV SSL_DIR=/etc/ssl

# Development entrypoint that rebuilds on start
CMD ["sh", "-c", "go build -buildvcs=false -o nginx-proxy-go . && go build -buildvcs=false -o /usr/local/bin/getssl ./cmd/getssl && /docker-entrypoint.sh"]

# Final stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache nginx openssl

ENV NGINX_CONF_DIR=/etc/nginx
ENV CHALLENGE_DIR=/tmp/acme-challenges
ENV SSL_DIR=/etc/ssl
# Copy the binary from builder
COPY --from=builder /app/nginx-proxy-go .
COPY --from=builder /app/getssl /usr/local/bin/
COPY --from=builder /go/bin/dlv /usr/local/bin/

# Copy nginx configuration
COPY nginx/nginx.conf /etc/nginx/nginx.conf

# Copy nginx template
COPY templates/ ./templates/

# Copy entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Create necessary directories
RUN mkdir -p /etc/nginx/conf.d /var/log/nginx /var/cache/nginx

# Expose ports
EXPOSE 80 443 2345

# Set environment variables
ENV GO_DEBUG_ENABLE="false"
ENV GO_DEBUG_PORT="2345"
ENV GO_DEBUG_HOST=""

# Run the application
ENTRYPOINT ["/docker-entrypoint.sh"] 