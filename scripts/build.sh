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
if [ ! -f "cmd/active-window/main.go" ]; then
    echo "❌ Error: cmd/active-window/main.go not found"
    echo "   Please run this script from the project root directory"
    exit 1
fi

echo "✓ Found cmd/active-window/main.go"

# Download dependencies
echo ""
echo "Downloading dependencies..."
go mod download

# Build the binary
echo ""
echo "Building binaries..."
go build -o active-window ./cmd/active-window
go build -tags ignore_app -o ignoreApplication ./cmd/ignoreApplication

if [ -f "active-window" ] && [ -f "ignoreApplication" ]; then
    echo ""
    echo "✅ Build successful!"
    echo ""
    echo "Binaries created:"
    echo "  - active-window (main tracking application)"
    echo "  - ignoreApplication (tool to manage ignored apps)"
    echo ""
    echo "Next steps:"
    echo "1. Copy .env.example to .env and add your RescueTime API key"
    echo "2. Test with: ./active-window -monitor"
    echo "3. Ignore apps with: ./ignoreApplication"
    echo "4. See README.md for detailed setup instructions"
else
    echo ""
    echo "❌ Build failed"
    exit 1
fi
