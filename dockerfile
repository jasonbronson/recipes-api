FROM golang:1.22.5-alpine

# Set environment variables
ENV CGO_ENABLED=0 GOOS=linux

# Build Go app
RUN mkdir /app
WORKDIR /app
COPY . .
RUN go build -o /app/myapp .

# Install dependencies for cloudflared
RUN apk add --no-cache curl

# Download and install Cloudflare Tunnel
RUN curl -L https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 \
    -o /app/cloudflared && chmod +x /app/cloudflared

# Copy Go app binary and schema.json
COPY ./schema.json /app/
COPY ./latest-version.txt /app/
COPY ./checklatestversion.sh /app/

# Add a script to manage both processes
COPY entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh
RUN chmod +x /app/checklatestversion.sh

# Expose necessary ports
EXPOSE 8080

# Default command
ENTRYPOINT ["./entrypoint.sh"]