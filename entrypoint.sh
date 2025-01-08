#!/bin/sh

# Run the check for the latest version
/app/checklatestversion.sh

# Start Cloudflare Tunnel
cloudflared tunnel --url http://localhost:8080