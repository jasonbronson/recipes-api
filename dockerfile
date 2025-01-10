# Base image for Go app
FROM golang:1.22.5-alpine as builder

# Set environment variables
ENV CGO_ENABLED=0 GOOS=linux

# Build Go app
WORKDIR /app
COPY . .
RUN go build -o /tmp/myapp .

# Base image for the final container
FROM alpine:latest

# Install dependencies for cloudflared
RUN apk add --no-cache curl

# Download and install Cloudflare Tunnel
RUN curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 \
    -o /usr/local/bin/cloudflared && chmod +x /usr/local/bin/cloudflared

WORKDIR /app

# Copy Go app binary and schema.json
COPY --from=builder /tmp/myapp /app/
COPY --from=builder /app/schema.json /app/
COPY --from=builder /app/latest-version.txt /app/
COPY --from=builder /app/checklatestversion.sh /app/

# Add a script to manage both processes
COPY entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh
RUN chmod +x /app/checklatestversion.sh

# Expose necessary ports
EXPOSE 8080

# Default command
ENTRYPOINT ["./entrypoint.sh"]