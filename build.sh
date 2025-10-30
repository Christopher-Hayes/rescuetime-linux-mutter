#!/bin/bash
# Build script for RescueTime Linux Mutter

set -e

echo "=== Building RescueTime Linux Mutter ==="
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed"
    echo "   Install Go from: https://go.dev/dl/"
    echo "   Or on Ubuntu/Debian: sudo apt install golang-go"
    exit 1
fi

echo "✓ Go is installed ($(go version))"

# Check if we're in the right directory
if [ ! -f "active-window.go" ]; then
    echo "❌ Error: active-window.go not found"
    echo "   Please run this script from the project root directory"
    exit 1
fi

echo "✓ Found active-window.go"

# Download dependencies
echo ""
echo "Downloading dependencies..."
go mod download

# Build the binary
echo ""
echo "Building binary..."
go build -o active-window active-window.go

if [ -f "active-window" ]; then
    echo ""
    echo "✅ Build successful!"
    echo ""
    echo "Next steps:"
    echo "1. Copy .env.example to .env and add your RescueTime API key"
    echo "2. Test with: ./active-window -monitor"
    echo "3. See QUICKSTART.md for detailed setup instructions"
else
    echo ""
    echo "❌ Build failed"
    exit 1
fi
