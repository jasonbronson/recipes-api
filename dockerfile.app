FROM golang:1.22.5-bullseye

# Set environment variables
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64

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