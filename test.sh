#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
print_header() {
    echo -e "${GREEN}===================================${NC}"
    echo -e "${GREEN}$1${NC}"
    echo -e "${GREEN}===================================${NC}"
}

print_error() {
    echo -e "${RED}Error: $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}Warning: $1${NC}"
}

print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

# Parse command line arguments
TEST_TYPE="${1:-all}"

case "$TEST_TYPE" in
    unit)
        print_header "Running Unit Tests"
        go test -v -race -coverprofile=coverage.txt -covermode=atomic ./internal/...
        print_success "Unit tests completed"
        ;;
    
    integration)
        print_header "Running Integration Tests"
        if ! docker info > /dev/null 2>&1; then
            print_error "Docker is not running. Please start Docker first."
            exit 1
        fi
        go test -v -tags=integration ./integration/...
        print_success "Integration tests completed"
        ;;
    
    e2e)
        print_header "Running End-to-End Tests"
        
        # Check if Docker is running
        if ! docker info > /dev/null 2>&1; then
            print_error "Docker is not running. Please start Docker first."
            exit 1
        fi
        
        # Build test image
        print_header "Building test image"
        docker build -t nginx-proxy-go:test .
        print_success "Test image built"
        
        # Create network if it doesn't exist
        print_header "Setting up test network"
        docker network create nginx-proxy 2>/dev/null || print_warning "Network already exists"
        print_success "Network ready"
        
        # Run E2E tests
        print_header "Running E2E tests"
        go test -v -tags=e2e -timeout 15m ./integration/e2e/...
        print_success "E2E tests completed"
        ;;
    
    http)
        print_header "Running HTTP Routing Tests"
        docker build -q -t nginx-proxy-go:test . > /dev/null
        docker network create nginx-proxy 2>/dev/null || true
        go test -v -tags=e2e -timeout 10m ./integration/e2e/ -run "TestBasicVirtualHostRouting|TestPathBasedRouting|TestHostHeaderValidation"
        print_success "HTTP routing tests completed"
        ;;
    
    https)
        print_header "Running HTTPS Tests"
        docker build -q -t nginx-proxy-go:test . > /dev/null
        docker network create nginx-proxy 2>/dev/null || true
        go test -v -tags=e2e -timeout 10m ./integration/e2e/ -run "TestHTTPS"
        print_success "HTTPS tests completed"
        ;;
    
    websocket)
        print_header "Running WebSocket Tests"
        docker build -q -t nginx-proxy-go:test . > /dev/null
        docker network create nginx-proxy 2>/dev/null || true
        go test -v -tags=e2e -timeout 10m ./integration/e2e/ -run "TestWebSocket"
        print_success "WebSocket tests completed"
        ;;
    
    auth)
        print_header "Running Basic Auth Tests"
        docker build -q -t nginx-proxy-go:test . > /dev/null
        docker network create nginx-proxy 2>/dev/null || true
        go test -v -tags=e2e -timeout 10m ./integration/e2e/ -run "TestBasicAuth"
        print_success "Basic auth tests completed"
        ;;
    
    redirect)
        print_header "Running Redirect Tests"
        docker build -q -t nginx-proxy-go:test . > /dev/null
        docker network create nginx-proxy 2>/dev/null || true
        go test -v -tags=e2e -timeout 10m ./integration/e2e/ -run "TestRedirect|TestSimpleRedirect"
        print_success "Redirect tests completed"
        ;;
    
    multi)
        print_header "Running Multi-Container Tests"
        docker build -q -t nginx-proxy-go:test . > /dev/null
        docker network create nginx-proxy 2>/dev/null || true
        go test -v -tags=e2e -timeout 10m ./integration/e2e/ -run "TestMultiple"
        print_success "Multi-container tests completed"
        ;;
    
    all)
        print_header "Running All Tests"
        
        # Unit tests
        print_header "1/3: Unit Tests"
        go test -v -race -coverprofile=coverage.txt -covermode=atomic ./internal/...
        print_success "Unit tests completed"
        
        # Integration tests
        print_header "2/3: Integration Tests"
        if docker info > /dev/null 2>&1; then
            go test -v -tags=integration ./integration/...
            print_success "Integration tests completed"
        else
            print_warning "Skipping integration tests (Docker not running)"
        fi
        
        # E2E tests
        print_header "3/3: End-to-End Tests"
        if docker info > /dev/null 2>&1; then
            docker build -q -t nginx-proxy-go:test . > /dev/null
            docker network create nginx-proxy 2>/dev/null || true
            go test -v -tags=e2e -timeout 15m ./integration/e2e/...
            print_success "E2E tests completed"
        else
            print_warning "Skipping E2E tests (Docker not running)"
        fi
        
        print_success "All tests completed successfully!"
        ;;
    
    clean)
        print_header "Cleaning up test artifacts"
        
        # Remove test containers
        docker ps -a | grep "test-" | awk '{print $1}' | xargs -r docker rm -f 2>/dev/null || true
        
        # Remove test networks
        docker network ls | grep "test-network-" | awk '{print $1}' | xargs -r docker network rm 2>/dev/null || true
        
        # Remove test image
        docker rmi nginx-proxy-go:test 2>/dev/null || true
        
        # Remove coverage files
        rm -f coverage.txt
        
        print_success "Cleanup completed"
        ;;
    
    coverage)
        print_header "Running Tests with Coverage"
        go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
        go tool cover -html=coverage.txt -o coverage.html
        print_success "Coverage report generated: coverage.html"
        
        # Print coverage summary
        go tool cover -func=coverage.txt | tail -1
        ;;
    
    help|*)
        echo "Usage: ./test.sh [TEST_TYPE]"
        echo ""
        echo "Test Types:"
        echo "  unit         - Run unit tests only"
        echo "  integration  - Run integration tests"
        echo "  e2e          - Run end-to-end tests"
        echo "  http         - Run HTTP routing tests"
        echo "  https        - Run HTTPS tests"
        echo "  websocket    - Run WebSocket tests"
        echo "  auth         - Run basic authentication tests"
        echo "  redirect     - Run redirect tests"
        echo "  multi        - Run multi-container tests"
        echo "  all          - Run all tests (default)"
        echo "  coverage     - Run tests with coverage report"
        echo "  clean        - Clean up test artifacts"
        echo "  help         - Show this help message"
        exit 0
        ;;
esac

