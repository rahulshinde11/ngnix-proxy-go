#!/bin/bash
set -e

IMAGE_NAME="nginx-proxy-go"
CONTAINER_NAME="nginx-proxy-go"

# Create named volumes if they don't exist
docker volume create nginx-proxy-go-nginx >/dev/null
docker volume create nginx-proxy-go-ssl >/dev/null
docker volume create nginx-proxy-go-acme >/dev/null

# Stop and remove any existing container
docker rm -f $CONTAINER_NAME 2>/dev/null || true

docker run -d \
  --name $CONTAINER_NAME \
  -e NGINX_CONF_DIR=/etc/nginx \
  -e CHALLENGE_DIR=/tmp/acme-challenges \
  -e SSL_DIR=/etc/ssl \
  -e HOSTNAME=$(hostname) \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v nginx-proxy-go-nginx:/etc/nginx \
  -v nginx-proxy-go-ssl:/etc/ssl \
  -v nginx-proxy-go-acme:/tmp/acme-challenges \
  --network bridge \
  $IMAGE_NAME 