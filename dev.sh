#!/bin/bash

set -e
export COMPOSE_BAKE=true
case "$1" in
    "start")
        echo "Starting development environment..."
        docker compose up --build
        ;;
    "quick")
        echo "Quick restart (using cache)..."
        docker compose up
        ;;
    "quick-bg")
        echo "Quick restart in background (using cache)..."
        docker compose up -d
        ;;
    "rebuild-code")
        echo "Rebuilding code only (fast)..."
        docker compose exec nginx-proxy-go sh -c "go build -buildvcs=false -o nginx-proxy-go . && go build -buildvcs=false -o /usr/local/bin/getssl ./cmd/getssl"
        echo "Code rebuilt, restarting container..."
        docker compose restart nginx-proxy-go
        echo "Following logs (Ctrl+C to exit)..."
        docker compose logs -f nginx-proxy-go
        ;;
    "rebuild-code-bg")
        echo "Rebuilding code only (fast) in background..."
        docker compose exec nginx-proxy-go sh -c "go build -buildvcs=false -o nginx-proxy-go . && go build -buildvcs=false -o /usr/local/bin/getssl ./cmd/getssl"
        echo "Code rebuilt, restarting container..."
        docker compose restart nginx-proxy-go
        echo "Container restarted in background. Use './dev.sh logs' to see output."
        ;;
    "rebuild")
        echo "Full rebuild..."
        docker compose down
        docker compose build --no-cache
        docker compose up
        ;;
    "logs")
        docker compose logs -f nginx-proxy-go
        ;;
    "shell")
        docker compose exec nginx-proxy-go sh
        ;;
    "clean")
        echo "Cleaning up..."
        docker compose down -v
        docker system prune -f
        ;;
    *)
        echo "Usage: $0 {start|quick|quick-bg|rebuild-code|rebuild-code-bg|rebuild|logs|shell|clean}"
        echo ""
        echo "  start            - Start with build (stays in terminal)"
        echo "  quick            - Quick restart without rebuild (stays in terminal)"
        echo "  quick-bg         - Quick restart in background (detached)"
        echo "  rebuild-code     - Rebuild only Go code, show logs (fastest for code changes)"
        echo "  rebuild-code-bg  - Rebuild only Go code in background"
        echo "  rebuild          - Full rebuild without cache (slowest, for major changes)"
        echo "  logs             - Follow container logs"
        echo "  shell            - Open shell in container"
        echo "  clean            - Clean up containers and volumes"
        exit 1
        ;;
esac 