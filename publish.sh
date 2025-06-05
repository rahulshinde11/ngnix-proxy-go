#!/bin/bash
set -e

# Configuration
IMAGE_NAME="shinde11/nginx-proxy"
BUILD_IMAGE_NAME="nginx-proxy-go"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if docker is running
check_docker() {
    if ! docker info >/dev/null 2>&1; then
        log_error "Docker is not running or not accessible"
        exit 1
    fi
}

# Function to check if buildx is available and setup multi-platform builder
setup_buildx() {
    log_info "Setting up Docker buildx for multi-platform builds..."
    
    # Check if buildx is available
    if ! docker buildx version >/dev/null 2>&1; then
        log_error "Docker buildx is not available. Please update Docker to a newer version."
        exit 1
    fi
    
    # Use default buildx builder
    log_info "Using default buildx builder (no container driver needed)"
    docker buildx use default
    log_success "Using default buildx builder"
}

# Function to check if user is logged into Docker Hub
check_docker_login() {
    if ! docker info 2>/dev/null | grep -q "Username"; then
        log_warning "You may not be logged into Docker Hub"
        log_info "Run 'docker login' if the push fails"
    fi
}

# Function to get version tag
get_version_tag() {
    if [ -n "$1" ]; then
        echo "$1"
    else
        # Try to get version from git tag, fallback to timestamp
        if git describe --tags --exact-match HEAD 2>/dev/null; then
            git describe --tags --exact-match HEAD
        else
            echo "v$(date +%Y%m%d-%H%M%S)"
        fi
    fi
}

# Function to build the image (single platform - for build-only mode)
build_image_single() {
    local version_tag=$1
    
    log_info "Building Docker image (single platform)..."
    log_info "Version tag: $version_tag"
    
    # Build the image with version tag
    docker build -t $BUILD_IMAGE_NAME .
    docker tag $BUILD_IMAGE_NAME $IMAGE_NAME:$version_tag
    docker tag $BUILD_IMAGE_NAME $IMAGE_NAME:latest
    
    log_success "Docker image built successfully"
    log_info "Tagged as: $IMAGE_NAME:$version_tag"
    log_info "Tagged as: $IMAGE_NAME:latest"
}

# Function to build multi-platform image
build_image_multiplatform() {
    local version_tag=$1
    local push_flag=$2
    
    log_info "Building multi-platform Docker image..."
    log_info "Version tag: $version_tag"
    log_info "Platforms: linux/amd64,linux/arm64"
    
    # Prepare buildx command
    local buildx_cmd="docker buildx build --platform linux/amd64,linux/arm64"
    
    if [ "$NO_CACHE" = "true" ]; then
        buildx_cmd="$buildx_cmd --no-cache"
        log_info "Building without cache..."
    fi
    
    # Add tags
    buildx_cmd="$buildx_cmd -t $IMAGE_NAME:$version_tag -t $IMAGE_NAME:latclearest"
    
    # Add push flag if needed
    if [ "$push_flag" = "true" ]; then
        buildx_cmd="$buildx_cmd --push"
        log_info "Building and pushing to registry..."
    else
        buildx_cmd="$buildx_cmd --load"
        log_info "Building for local use..."
    fi
    
    # Add build context
    buildx_cmd="$buildx_cmd ."
    
    # Execute build command
    eval $buildx_cmd
    
    if [ "$push_flag" = "true" ]; then
        log_success "Multi-platform Docker image built and pushed successfully"
        log_success "Available at: https://hub.docker.com/r/$IMAGE_NAME"
    else
        log_success "Multi-platform Docker image built successfully"
    fi
    
    log_info "Tagged as: $IMAGE_NAME:$version_tag"
    log_info "Tagged as: $IMAGE_NAME:latest"
}

# Function to push single-platform image (fallback)
push_image() {
    local version_tag=$1
    
    log_info "Pushing Docker image to Docker Hub..."
    
    # Push version tag
    log_info "Pushing $IMAGE_NAME:$version_tag..."
    docker push $IMAGE_NAME:$version_tag
    
    # Push latest tag
    log_info "Pushing $IMAGE_NAME:latest..."
    docker push $IMAGE_NAME:latest
    
    log_success "Docker image pushed successfully"
    log_success "Available at: https://hub.docker.com/r/$IMAGE_NAME"
}

# Function to clean up local images (optional)
cleanup() {
    if [ "$CLEANUP" = "true" ]; then
        log_info "Cleaning up local build images..."
        docker rmi $BUILD_IMAGE_NAME 2>/dev/null || true
        log_success "Cleanup completed"
    fi
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [OPTIONS] [VERSION_TAG]"
    echo ""
    echo "Build and publish multi-platform Docker image to Docker Hub"
    echo ""
    echo "Options:"
    echo "  -h, --help           Show this help message"
    echo "  -c, --cleanup        Remove local build images after push"
    echo "  --no-cache           Build without using cache"
    echo "  --build-only         Only build, don't push to Docker Hub"
    echo "  --single-platform    Build only for current platform (faster for testing)"
    echo ""
    echo "Arguments:"
    echo "  VERSION_TAG          Optional version tag (default: auto-generated from git tag or timestamp)"
    echo ""
    echo "Examples:"
    echo "  $0                      # Build and push multi-platform with auto-generated version"
    echo "  $0 v1.2.3               # Build and push multi-platform with specific version"
    echo "  $0 --build-only         # Only build multi-platform, don't push"
    echo "  $0 --single-platform    # Build and push single platform (current architecture)"
    echo "  $0 -c v1.2.3            # Build, push, and cleanup"
}

# Parse command line arguments
CLEANUP=false
NO_CACHE=false
BUILD_ONLY=false
SINGLE_PLATFORM=false
VERSION_TAG=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            show_usage
            exit 0
            ;;
        -c|--cleanup)
            CLEANUP=true
            shift
            ;;
        --no-cache)
            NO_CACHE=true
            shift
            ;;
        --build-only)
            BUILD_ONLY=true
            shift
            ;;
        --single-platform)
            SINGLE_PLATFORM=true
            shift
            ;;
        -*)
            log_error "Unknown option: $1"
            show_usage
            exit 1
            ;;
        *)
            if [ -z "$VERSION_TAG" ]; then
                VERSION_TAG="$1"
            else
                log_error "Multiple version tags specified"
                show_usage
                exit 1
            fi
            shift
            ;;
    esac
done

# Main script execution
main() {
    log_info "Starting Docker image build and publish process..."
    
    # Pre-flight checks
    check_docker
    if [ "$BUILD_ONLY" != "true" ]; then
        check_docker_login
    fi
    
    # Setup buildx for multi-platform builds (unless single-platform mode)
    if [ "$SINGLE_PLATFORM" != "true" ]; then
        setup_buildx
    fi
    
    # Get version tag
    VERSION_TAG=$(get_version_tag "$VERSION_TAG")
    
    # Build logic based on platform choice
    if [ "$SINGLE_PLATFORM" = "true" ]; then
        log_info "Single-platform mode: Building for current architecture only"
        
        # Single platform build
        if [ "$NO_CACHE" = "true" ]; then
            log_info "Building without cache..."
            docker build --no-cache -t $BUILD_IMAGE_NAME .
            docker tag $BUILD_IMAGE_NAME $IMAGE_NAME:$VERSION_TAG
            docker tag $BUILD_IMAGE_NAME $IMAGE_NAME:latest
            log_success "Docker image built successfully (no cache)"
        else
            build_image_single "$VERSION_TAG"
        fi
        
        # Push to Docker Hub (unless build-only mode)
        if [ "$BUILD_ONLY" != "true" ]; then
            push_image "$VERSION_TAG"
        else
            log_info "Build-only mode: Skipping push to Docker Hub"
        fi
        
    else
        log_info "Multi-platform mode: Building for linux/amd64 and linux/arm64"
        
        # Multi-platform build
        if [ "$BUILD_ONLY" = "true" ]; then
            # Build only (load to local registry, but this has limitations with multi-platform)
            log_warning "Note: Multi-platform --build-only mode has limitations. Image will be built but not loaded locally."
            build_image_multiplatform "$VERSION_TAG" "false"
        else
            # Build and push in one step (more efficient for multi-platform)
            build_image_multiplatform "$VERSION_TAG" "true"
        fi
    fi
    
    # Cleanup if requested
    cleanup
    
    log_success "Process completed successfully!"
    
    if [ "$BUILD_ONLY" != "true" ]; then
        log_info "You can now use the image with:"
        log_info "  docker run $IMAGE_NAME:$VERSION_TAG"
        log_info "  docker run $IMAGE_NAME:latest"
        if [ "$SINGLE_PLATFORM" != "true" ]; then
            log_info "Multi-platform image supports: linux/amd64, linux/arm64"
        fi
    fi
}

# Run main function
main "$@" 