#!/bin/sh

# Start the Go app in the background
/usr/local/bin/myapp &

# Start Cloudflare Tunnel
cloudflared tunnel --url http://localhost:8080