FROM golang:1.22.5

# Set environment variables
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

RUN apt-get update && apt-get install -y \
    bash \    
    chromium \
    libnss3 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libxcomposite1 \
    libxrandr2 \
    libxss1 \
    libxcursor1 \
    libxi6 \
    libpangocairo-1.0-0 \
    libgdk-pixbuf2.0-0 \
    libasound2 \
    libxdamage1 \
    libxinerama1

# Build Go app
RUN mkdir /app
WORKDIR /app
COPY . .
RUN go build -o /app/myapp .

COPY ./schema.json /app/

# Expose necessary ports
EXPOSE 8080

# Default command
ENTRYPOINT ["/app/myapp"]