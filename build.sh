#!/bin/bash
set -e

IMAGE_NAME="nginx-proxy-go"

echo "Building Docker image: $IMAGE_NAME"
docker build -t $IMAGE_NAME . 