#!/bin/bash
# Quick verification script for RescueTime Mutter setup

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  RescueTime Linux Mutter - Quick Verification"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Track overall status
ERRORS=0

# Test 1: Check Go installation
echo -n "1. Checking Go installation... "
if command -v go &> /dev/null; then
    echo -e "${GREEN}✓${NC} ($(go version))"
else
    echo -e "${RED}✗${NC}"
    echo "   Install Go: sudo apt install golang-go"
    ERRORS=$((ERRORS + 1))
fi

# Test 2: Check if we're in the right directory
echo -n "2. Checking project files... "
if [ -f "cmd/active-window/main.go" ] && [ -f "go.mod" ]; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗${NC}"
    echo "   Run this script from the project root directory"
    ERRORS=$((ERRORS + 1))
fi

# Test 3: Check desktop environment
echo -n "3. Checking desktop environment... "
if [ -n "$XDG_CURRENT_DESKTOP" ]; then
    # Check if GNOME Shell is actually running (Ubuntu sets XDG_CURRENT_DESKTOP=Unity even when running GNOME)
    if pgrep -x gnome-shell > /dev/null; then
        GNOME_VERSION=$(gnome-shell --version 2>/dev/null | cut -d' ' -f3)
        if [ -n "$GNOME_VERSION" ]; then
            echo -e "${GREEN}✓${NC} (GNOME Shell $GNOME_VERSION)"
        else
            echo -e "${GREEN}✓${NC} (GNOME Shell)"
        fi
    elif pgrep -x mutter > /dev/null || pgrep -f "mutter-x11-frames" > /dev/null; then
        echo -e "${GREEN}✓${NC} (Mutter-based: $XDG_CURRENT_DESKTOP)"
    elif [[ "$XDG_CURRENT_DESKTOP" == *"GNOME"* ]]; then
        echo -e "${GREEN}✓${NC} ($XDG_CURRENT_DESKTOP)"
    else
        echo -e "${YELLOW}⚠${NC} ($XDG_CURRENT_DESKTOP - may not work)"
        echo "   This application requires GNOME Shell with Mutter"
    fi
else
    echo -e "${RED}✗${NC}"
    echo "   Not running in a graphical environment"
    ERRORS=$((ERRORS + 1))
fi

# Test 4: Check session type
echo -n "4. Checking session type... "
if [ -n "$XDG_SESSION_TYPE" ]; then
    echo -e "${GREEN}✓${NC} ($XDG_SESSION_TYPE)"
else
    echo -e "${YELLOW}⚠${NC} (unknown)"
fi

# Test 5: Check GNOME Shell extension
echo -n "5. Checking FocusedWindow extension... "
if gdbus call --session --dest org.gnome.Shell \
   --object-path /org/gnome/shell/extensions/FocusedWindow \
   --method org.gnome.shell.extensions.FocusedWindow.Get &> /dev/null; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗${NC}"
    echo "   Install: https://extensions.gnome.org/extension/5839/focused-window-dbus/"
    ERRORS=$((ERRORS + 1))
fi

# Test 6: Check if binary exists
echo -n "6. Checking if binary is built... "
if [ -f "active-window" ]; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${YELLOW}⚠${NC}"
    echo "   Run: ./build.sh or go build -o active-window active-window.go"
fi

# Test 7: Check .env file
echo -n "7. Checking .env configuration... "
if [ -f ".env" ]; then
    if grep -q "RESCUE_TIME_API_KEY=your_api_key_here" .env; then
        echo -e "${YELLOW}⚠${NC}"
        echo "   Update .env with your actual RescueTime API key"
    elif grep -q "RESCUE_TIME_API_KEY=" .env && [ -n "$(grep RESCUE_TIME_API_KEY .env | cut -d'=' -f2)" ]; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${YELLOW}⚠${NC}"
        echo "   .env file exists but API key may not be set"
    fi
else
    echo -e "${YELLOW}⚠${NC}"
    echo "   Copy .env.example to .env and add your API key"
fi

# Summary
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ $ERRORS -eq 0 ]; then
    echo -e "${GREEN}✓ All critical checks passed!${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. If binary not built: ./scripts/build.sh"
    echo "  2. Test window detection: ./active-window"
    echo "  3. Test monitoring: timeout 30s ./active-window -monitor"
    echo "  4. See README.md for full setup"
else
    echo -e "${RED}✗ $ERRORS critical check(s) failed${NC}"
    echo ""
    echo "Fix the errors above, then run this script again."
fi
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Optional: Run quick functionality test if all checks pass
if [ $ERRORS -eq 0 ] && [ -f "active-window" ]; then
    echo "Want to run a quick functionality test? (y/N)"
    read -r response
    if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo ""
        echo "Running quick test (10 seconds)..."
        timeout 10s ./active-window -monitor -debug || true
        echo ""
        echo "If you saw window information above, everything is working!"
    fi
fi

exit $ERRORS
