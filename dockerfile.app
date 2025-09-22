FROM golang:1.22.5

# Build env
ENV CGO_ENABLED=1 \
    CHROME_BIN=/usr/bin/chromium \
    CHROMIUM_BIN=/usr/bin/chromium \
    DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
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
    libxinerama1 \
    libx11-6 \
    libx11-xcb1 \
    libxcb1 \
    libxext6 \
    libxfixes3 \
    libxrender1 \
    libxtst6 \
    libdrm2 \
    libxkbcommon0 \
    libgtk-3-0 \
    libgbm1 \
    fonts-liberation \
    fonts-noto-color-emoji \
    gcc \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY . .
RUN go build -trimpath -ldflags="-s -w" -o /app/api .

COPY ./schema.json /app/

EXPOSE 8080

ENTRYPOINT ["/app/api"]