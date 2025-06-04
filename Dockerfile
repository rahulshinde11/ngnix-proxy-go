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

# Install Delve
RUN go install github.com/go-delve/delve/cmd/dlv@latest

# Final stage
FROM alpine:latest

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache nginx openssl

# Copy the binary from builder
COPY --from=builder /app/nginx-proxy-go .
COPY --from=builder /go/bin/dlv /usr/local/bin/

# Copy nginx configuration
COPY nginx/nginx.conf /etc/nginx/nginx.conf

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