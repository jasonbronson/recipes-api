#!/bin/sh

# Define variables
APP_NAME="myapp"
REMOTE_VERSION_URL="https://raw.githubusercontent.com/jasonbronson/recipes/master/latest-version.txt"
LOCAL_BINARY="/app/${APP_NAME}"
TEMP_BINARY="/tmp/${APP_NAME}"

# Fetch the latest version
echo "Checking for the latest version..."
REMOTE_VERSION=$(curl -s -f "${REMOTE_VERSION_URL}" || echo "0")
if [ -f "${LOCAL_BINARY}" ]; then
    LOCAL_VERSION=$(${LOCAL_BINARY} --version || echo "0")
else
    LOCAL_VERSION="0"
fi

echo "Local version: ${LOCAL_VERSION}"
echo "Remote version: ${REMOTE_VERSION}"

# Compare and download if needed
if [ "$REMOTE_VERSION" != "$LOCAL_VERSION" ]; then
    echo "New version found. Downloading..."
    BINARY_URL="https://github.com/jasonbronson/recipes/releases/download/$REMOTE_VERSION/${APP_NAME}"
    curl -L -o "${TEMP_BINARY}" "${BINARY_URL}"
    if [ $? -eq 0 ]; then
        mv "${TEMP_BINARY}" "${LOCAL_BINARY}"
        chmod +x "${LOCAL_BINARY}"
    else
        echo "Failed to download the binary. Exiting."
        exit 1
    fi
else
    echo "Already up-to-date. Running the existing binary."
fi

# Run the application
exec "${LOCAL_BINARY}"