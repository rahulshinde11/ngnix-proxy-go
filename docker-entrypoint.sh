#!/usr/bin/env sh
mkdir -p /etc/nginx/dhparam

if ! openssl dhparam -in /etc/nginx/dhparam/dhparam.pem >/dev/null 2>&1; then
  openssl dhparam -out /etc/nginx/dhparam/dhparam.pem ${DHPARAM_SIZE:-2048}
fi

nginx

exec ./nginx-proxy-go 