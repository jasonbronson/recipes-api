#!/bin/sh

# Define variables
APP_NAME="myapp"
LATEST_RELEASE_API="https://api.github.com/repos/jasonbronson/recipes/releases/latest"
LOCAL_BINARY="/app/${APP_NAME}"

# Fetch the latest version from GitHub API
echo "Fetching the latest version..."
REMOTE_VERSION=$(curl -s "$LATEST_RELEASE_API" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$REMOTE_VERSION" ]; then
    echo "Failed to fetch the latest version. Exiting."
    exit 1
fi

echo "Latest version: $REMOTE_VERSION"

# Construct the download URL
BINARY_URL="https://github.com/jasonbronson/recipes/releases/download/${REMOTE_VERSION}/${APP_NAME}"

# Check the local version
if [ -f "$LOCAL_BINARY" ]; then
    LOCAL_VERSION=$($LOCAL_BINARY --version || echo "0")
else
    LOCAL_VERSION="0"
fi

echo "Local version: $LOCAL_VERSION"

# Compare and download if needed
if [ "$REMOTE_VERSION" != "$LOCAL_VERSION" ]; then
    echo "Downloading version $REMOTE_VERSION..."
    curl -L -o "$LOCAL_BINARY" "$BINARY_URL"
    chmod +x "$LOCAL_BINARY"
else
    echo "Already up-to-date. Running the existing binary."
fi

# Run the application
exec "$LOCAL_BINARY"