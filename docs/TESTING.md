# Testing Guide

## Prerequisites

Before testing, ensure:

1. ✅ GNOME Shell FocusedWindow extension is installed and enabled
2. ✅ Application is built (`./build.sh` or `go build`)
3. ✅ `.env` file exists with your RescueTime API key

## Quick Verification Tests

### Test 1: Extension Connectivity

Verify the GNOME Shell extension is working:

```bash
gdbus call --session --dest org.gnome.Shell \
  --object-path /org/gnome/shell/extensions/FocusedWindow \
  --method org.gnome.shell.extensions.FocusedWindow.Get
```

**Expected:** JSON output with window information
**If it fails:** The extension is not installed or enabled

### Test 2: Single Window Query

Test basic window detection:

```bash
./active-window
```

**Expected:** 
```
Active Window: filename - appname (wm_class)
```

**If it fails:** Check extension installation

### Test 3: Window Monitoring

Monitor window changes for 30 seconds:

```bash
timeout 30s ./active-window -monitor
```

**Expected:** New line printed each time you switch windows
**Try:** Switch between 2-3 applications during the 30 seconds

### Test 4: Debug Mode

Test with debug logging:

```bash
timeout 30s ./active-window -monitor -debug
```

**Expected:** Detailed logs showing D-Bus connections and window queries
**Look for:** `[DEBUG]` and `[VERBOSE]` prefixed messages

## Session Tracking Tests

### Test 5: Basic Tracking

Track activity for 2 minutes without API submission:

```bash
timeout 120s ./active-window -track
```

**Actions during test:**
1. Switch between 2-3 different applications
2. Spend at least 20-30 seconds in each
3. Wait for timeout or press Ctrl+C

**Expected:** Activity summary showing time per application

Example output:
```
=== Activity Summary ===
Total tracking time: 2m15s

firefox: 45s (33.3%) - 2 sessions
  └─ GitHub - Repository

Code: 1m30s (66.7%) - 1 session
  └─ active-window.go - rescuetime-linux-mutter
```

### Test 6: Session Persistence

Save tracking data to a file:

```bash
timeout 120s ./active-window -track -save
```

**After completion:**
```bash
cat rescuetime-sessions.json | jq .
```

**Expected:** JSON file with activity summaries
**Verify:** 
- Timestamps are correct
- Durations match your activity
- App names are captured correctly

### Test 7: Session Merging

Test the 30-second merge threshold:

```bash
timeout 180s ./active-window -track -verbose
```

**Actions during test:**
1. Open Firefox and use it for 30 seconds
2. Switch to another app for 10 seconds (brief interruption)
3. Switch back to Firefox for 30 seconds
4. Press Ctrl+C

**Expected:** Firefox should show as 1 session (merged), not 2
**Look for:** `[VERBOSE]` logs about session merging

## API Submission Tests

### Test 8: Dry-Run Preview

Preview what would be submitted without making API calls:

```bash
timeout 120s ./active-window -track -dry-run -submission-interval 1m
```

**Actions during test:**
1. Use different applications
2. Wait for the 1-minute mark
3. Continue until timeout

**Expected:** 
- At 1 minute: Preview of submission data
- At 2 minutes: Another preview
- No actual API calls made

**Verify:**
- `start_time` format: "YYYY-MM-DD HH:MM:SS"
- `duration` is in minutes
- `activity_name` matches application
- `activity_details` has window title

### Test 9: Verbose Dry-Run

Get detailed information during dry-run:

```bash
./active-window -track -dry-run -submission-interval 1m -verbose
```

**Expected:** Detailed logs + submission previews
**Look for:** `[VERBOSE]`, and preview outputs

### Test 10: Short Interval API Test (CAREFUL!)

**⚠️ Warning:** This makes real API calls to RescueTime!

Test actual API submission with a short interval:

```bash
timeout 120s ./active-window -track -submit -submission-interval 1m -verbose
```

**Actions during test:**
1. Use 2-3 applications for at least 1 minute total
2. Watch for submission logs at the 1-minute mark

**Expected logs:**
```
API submission enabled: will submit every 1m0s
...
✓ Submitted to RescueTime: firefox (5 min)
✓ Submitted to RescueTime: Code (3 min)
```

**Verify in RescueTime:**
1. Wait 5-10 minutes for processing
2. Check your RescueTime dashboard
3. Look for the submitted activities

**If submission fails:**
- Check API key in `.env`
- Look for error messages in output
- Try dry-run mode first to verify data format

## Production Readiness Tests

### Test 11: Full Production Simulation

Run a 15-minute test with production settings:

```bash
timeout 900s ./active-window -track -submit -submission-interval 15m -verbose
```

**Actions during test:**
- Use your computer normally for 15 minutes
- Don't worry about the application

**Expected:**
- At 15 minutes: Submission to RescueTime
- Activity summary on exit
- No errors or crashes

### Test 12: Graceful Shutdown

Test handling of Ctrl+C:

```bash
./active-window -track -submit -submission-interval 15m
```

**Actions:**
1. Use computer for 2-3 minutes
2. Press Ctrl+C
3. Wait for shutdown to complete

**Expected:**
```
Shutting down window monitor...
Submitting final data before shutdown...
✓ Submitted to RescueTime: ...

=== Activity Summary ===
...
```

**Verify:** Final data is submitted before exit

### Test 13: Long-Running Stability

Test for stability over several hours (optional):

```bash
./active-window -track -submit -verbose > rescuetime.log 2>&1 &
TRACKER_PID=$!

# Let it run for a few hours, then:
kill -TERM $TRACKER_PID

# Check the log
tail -n 50 rescuetime.log
```

**Verify:**
- No error messages in log
- Regular submissions every 15 minutes
- Graceful shutdown on TERM signal

## Systemd Service Test

### Test 14: Service Installation

Test the systemd service:

```bash
# Create service file
mkdir -p ~/.config/systemd/user
cat > ~/.config/systemd/user/rescuetime.service << 'EOF'
[Unit]
Description=RescueTime Activity Tracker for GNOME/Mutter
After=graphical-session.target

[Service]
Type=simple
ExecStart=/home/chris/src/github/rescuetime-linux-mutter/active-window -track -submit
Restart=on-failure
RestartSec=10
Environment="WAYLAND_DISPLAY=wayland-1"
Environment="XDG_RUNTIME_DIR=/run/user/1000"

[Install]
WantedBy=default.target
EOF

# Update the ExecStart path to match your installation
# Then enable and start
systemctl --user daemon-reload
systemctl --user start rescuetime.service
systemctl --user status rescuetime.service
```

**Expected:** Status shows "active (running)"

**View logs:**
```bash
journalctl --user -u rescuetime.service -f
```

**Stop service:**
```bash
systemctl --user stop rescuetime.service
```

## Troubleshooting Test Failures

### Extension Issues

**Symptom:** "Failed to connect to GNOME Shell FocusedWindow extension"

**Debug:**
```bash
# List extensions
gnome-extensions list | grep focused

# Check if enabled
gnome-extensions info focused-window-dbus@nichijou.github.io

# Try enabling
gnome-extensions enable focused-window-dbus@nichijou.github.io

# Test D-Bus manually
gdbus introspect --session --dest org.gnome.Shell \
  --object-path /org/gnome/shell/extensions/FocusedWindow
```

### API Issues

**Symptom:** "API returned status 401" or similar

**Debug:**
```bash
# Verify API key is set
cat .env | grep RESCUE_TIME_API_KEY

# Test in dry-run
./active-window -track -dry-run -submission-interval 1m

# Check RescueTime API status
curl -I https://www.rescuetime.com/anapi/offline_time_post
```

### Window Detection Issues

**Symptom:** No window changes detected

**Debug:**
```bash
# Enable maximum verbosity
./active-window -monitor -debug

# Check session type
echo $XDG_SESSION_TYPE
echo $XDG_CURRENT_DESKTOP

# Verify you're on Wayland or X11
echo $WAYLAND_DISPLAY
echo $DISPLAY
```

## Next Steps

Once all tests pass:

1. Set up systemd service for autostart (Test 14)
2. Enable service: `systemctl --user enable rescuetime.service`
3. Monitor for the first few days: `journalctl --user -u rescuetime.service -f`
4. Verify data in RescueTime dashboard regularly
