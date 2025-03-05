#!/bin/sh

if ! command -v curl > /dev/null 2>&1; then
    echo "curl not found, installing..."
    apk add --no-cache curl
else
    echo "curl is already installed."
fi
