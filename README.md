# Linux application tracking for RescueTime (Wayland + Mutter)

A Go CLI client for sending application usage to [RescueTime](https://www.rescuetime.com). This client is built specifically for GNOME + Wayland (via Mutter). It's designed to be an alternative to the official Linux client, which struggles on Wayland.

> **Fork Notice:** This is a fork adapted for Mutter/GNOME Shell. The original project was designed for Hyprland. This version uses D-Bus to communicate with a GNOME Shell extension for window tracking.

## Features

- **Mutter/GNOME Shell Support** - Monitors active window via D-Bus FocusedWindow extension
- **Idle Detection** - Automatically pauses tracking when you're away from your computer
- **Application Filtering** - Ignore specific applications to avoid double-tracking
- **Intelligent Merging** - Merges brief window switches to the same app (< 30 seconds)
- **Session Filtering** - Ignores very short sessions (< 10 seconds) to reduce noise
- **Automatic Submission** - Sends activity data to RescueTime every 15 minutes (configurable)

## Requirements

- **OS:** Linux with Wayland
- **Compositor:** Mutter (GNOME Shell, Ubuntu Unity, etc.)
- **GNOME Shell Extension:** [Focused Window D-Bus](https://extensions.gnome.org/extension/5839/focused-window-dbus/)
- **Runtime:** Go 1.21+ (for building)
- **RescueTime Account:** Free or paid account with API access

## Installation

### 1. Install GNOME Shell Extension

The application requires the **Focused Window D-Bus** extension to access window information:

**Option A: Via GNOME Extensions Website**
1. Visit https://extensions.gnome.org/extension/5839/focused-window-dbus/
2. Click "Install" (requires browser extension)
3. Enable the extension

**Option B: Manual Installation**
```bash
git clone https://github.com/nichijou/gnome-shell-extension-focused-window-dbus.git
cd gnome-shell-extension-focused-window-dbus
make install
# Log out and log back in to reload GNOME Shell
gnome-extensions enable focused-window-dbus@nichijou.github.io
```

**Verify Installation:**
```bash
gdbus call --session --dest org.gnome.Shell \
  --object-path /org/gnome/shell/extensions/FocusedWindow \
  --method org.gnome.shell.extensions.FocusedWindow.Get
```

You should see JSON output with window information.

### 2. Install Go (if not already installed)

```bash
# Ubuntu/Debian
sudo snap install go --classic

# Or download from https://go.dev/dl/
```

### 3. Build from Source

```bash
# Clone the repository
git clone https://github.com/Christopher-Hayes/rescuetime-linux-mutter.git
cd rescuetime-linux-mutter

# Build using the build script (recommended)
./scripts/build.sh

# This creates two binaries in the root:
# - active-window: Main tracking application
# - ignoreApplication: Tool to manage ignored apps

# Create environment file
cp .env.example .env
# Edit .env and add your RescueTime API key
```

### Environment Setup

Create a `.env` file in the project directory:

```bash
RESCUE_TIME_API_KEY=your_api_key_here
```

**Getting your API key:**
1. Log in to [RescueTime](https://www.rescuetime.com)
2. Open your Account Settings.
3. Navigate to "Key Management" under "API".
3. Generate or copy your API key.

## Usage

### Testing & Debugging

```bash
# 1. Test window detection (single query)
./active-window

# 2. Monitor window changes with debug output
./active-window -monitor -debug

# 3. Track time with verbose logging (no API submission)
./active-window -track -verbose

# 4. Dry-run mode: see what would be submitted without making API calls
./active-window -track -dry-run -submission-interval 2m

# 5. Save sessions to JSON file for inspection
./active-window -track -save -submission-interval 2m
# Creates: rescuetime-sessions.json

# 6. Test actual API submission with short interval
./active-window -track -submit -submission-interval 2m -verbose
```

### Production Commands

```bash
# Track and submit to RescueTime API (production mode)
./active-window -track -submit

# Custom polling interval (default: 1000ms)
./active-window -track -submit -interval 500ms

# Custom submission interval (default: 15m)
./active-window -track -submit -submission-interval 5m

# Custom idle threshold (default: 5m)
./active-window -track -submit -idle-threshold 10m

# With debug logging for troubleshooting
./active-window -track -submit -debug
```

### PostgreSQL Storage (Optional)

You can optionally store activity data in your own PostgreSQL database alongside submitting to RescueTime:

```bash
# Track and submit to both RescueTime and PostgreSQL
./active-window -track -submit -postgres "postgres://user:pass@localhost/rescuetime"

# Use PostgreSQL without RescueTime API
./active-window -track -postgres "postgres://user:pass@localhost/rescuetime"

# Set via environment variable
export POSTGRES_CONNECTION_STRING="postgres://user:pass@localhost/rescuetime"
./active-window -track -submit
```

**Benefits:**
- Own your data - maintain a local copy of all activity tracking
- Custom analytics - query your data directly with SQL
- Privacy - keep sensitive work data on your own infrastructure
- Backup - redundant storage alongside RescueTime's cloud

**Setup:**
See [postgres/README.md](postgres/README.md) for detailed setup instructions, database schema, and example queries.

### Ignoring Applications

To avoid double-tracking (e.g., when using RescueTime plugins for VS Code or browsers), you can ignore specific applications:

**Option 1: Interactive CLI Tool (Recommended)**

```bash
# Build the ignoreApplication tool if not already built
./scripts/build.sh

# Run the interactive tool
./ignoreApplication
```

The tool will:
1. Monitor your active windows for 10 seconds
2. Display a list of detected applications
3. Let you select which one to ignore
4. Save it to `.rescuetime-ignore`

**Option 2: Manual Configuration**

Create or edit `.rescuetime-ignore` in the project directory:

```bash
# .rescuetime-ignore
# One WmClass per line
# Lines starting with # are comments

Code              # VS Code
firefox           # Firefox browser
Google-chrome     # Chrome browser
```

**Finding WmClass Names:**

```bash
# Option 1: Use the ignoreApplication tool (easiest)
./ignoreApplication

# Option 2: Monitor and check the output
./active-window -monitor
# The WmClass is shown in parentheses: "Window Title (WmClass)"

# Option 3: Query current window
./active-window
# Output: Active Window: Title (WmClass)
```

**Note:** Changes to `.rescuetime-ignore` require restarting the tracker if it's already running.

### Idle Detection

The application automatically detects when you're away from your computer and pauses tracking:

**How it works:**
- Uses GNOME/Mutter's idle monitor via D-Bus to track keyboard/mouse inactivity
- Default idle threshold: 5 minutes (configurable via `-idle-threshold` flag)
- When you become idle, the current session is ended
- Tracking automatically resumes when you return

**Customizing idle detection:**

```bash
# Set idle threshold to 10 minutes
./active-window -track -submit -idle-threshold 10m

# Set idle threshold to 2 minutes
./active-window -track -submit -idle-threshold 2m

# View idle state changes with verbose logging
./active-window -track -submit -idle-threshold 5m -verbose
```

**Troubleshooting idle detection:**

If idle detection isn't working:

1. Verify Mutter's IdleMonitor is available:
   ```bash
   gdbus call --session --dest org.gnome.Mutter.IdleMonitor \
     --object-path /org/gnome/Mutter/IdleMonitor/Core \
     --method org.gnome.Mutter.IdleMonitor.GetIdletime
   ```
   This should return your current idle time in milliseconds.

2. The application will show a warning at startup if idle detection is unavailable but will continue tracking without it.

3. Use verbose logging to see idle state changes:
   ```bash
   ./active-window -track -verbose
   ```

### Command-Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `-monitor` | Continuously monitor window changes (display only) | `false` |
| `-track` | Monitor and track time spent in applications | `false` |
| `-submit` | Submit activity data to RescueTime API | `false` |
| `-dry-run` | Preview submissions without making API calls | `false` |
| `-save` | Save activity summaries to `rescuetime-sessions.json` | `false` |
| `-debug` | Enable debug logging | `false` |
| `-verbose` | Enable verbose logging | `false` |
| `-interval` | Polling interval for window detection | `1000ms` |
| `-submission-interval` | How often to submit data to RescueTime | `15m` |
| `-idle-threshold` | Time of inactivity before considering user idle | `5m` |

### Running as a Service

**Systemd service (recommended for autostart):**

Change `/path/to/active-window` to the full path of your built binary.

```ini
# ~/.config/systemd/user/rescuetime.service
[Unit]
Description=RescueTime Activity Tracker for GNOME/Mutter
After=graphical-session.target

[Service]
Type=simple
ExecStart=/path/to/active-window -track -submit
Restart=on-failure
RestartSec=10
Environment="WAYLAND_DISPLAY=wayland-1"
Environment="XDG_RUNTIME_DIR=/run/user/1000"

[Install]
WantedBy=default.target
```

Enable and start:
```bash
systemctl --user enable rescuetime.service
systemctl --user start rescuetime.service

# Check status
systemctl --user status rescuetime.service

# View logs
journalctl --user -u rescuetime.service -f
```

<details>
<summary><h2>Testing</h2></summary>

### Manual Testing

```bash
# Short tracking session to verify window detection
./active-window -track
# Switch between windows for ~30 seconds, then Ctrl+C to see summary

# Test API submission with short interval (2 minutes)
./active-window -track -submit -submission-interval 2m
# Use windows for 2+ minutes, verify API submission succeeds
```

### API Testing

HTTP requests for testing authentication and endpoints are in `rescuetime-auth.http` (use with REST client or curl).

## Platform Notes

- **GNOME/Mutter-specific:** Uses D-Bus to communicate with GNOME Shell extension
- **Requires extension:** FocusedWindow D-Bus extension must be installed
- **Wayland-focused:** Optimized for Wayland, checks for `WAYLAND_DISPLAY` environment variable
- **Also works on X11:** Should work on GNOME with X11 as well
- **Unity compatible:** Works with Ubuntu Unity (which uses Mutter)

## Development & Testing Workflow

Recommended workflow for testing and debugging:

1. **Test window detection:**
   ```bash
   ./active-window  # Single query
   ./active-window -monitor -debug  # Live monitoring
   ```

2. **Test activity tracking:**
   ```bash
   ./active-window -track -save -submission-interval 2m
   # Switch between windows for 2-3 minutes
   # Press Ctrl+C
   # Inspect rescuetime-sessions.json
   ```

3. **Test API submission (dry-run):**
   ```bash
   ./active-window -track -dry-run -submission-interval 1m -verbose
   # Verify the preview output looks correct
   ```

4. **Test actual API submission:**
   ```bash
   ./active-window -track -submit -submission-interval 2m -verbose
   # Monitor logs to ensure successful submission
   ```

5. **Deploy to production:**
   ```bash
   # Set up systemd service with 15-minute intervals
   ./active-window -track -submit
   ```

</details>

<details>
<summary>
<h2>Troubleshooting</h2>
</summary>

### Extension Not Found Error

If you get: `Failed to connect to GNOME Shell FocusedWindow extension`

1. Verify the extension is installed:
   ```bash
   gnome-extensions list | grep focused-window
   ```

2. Check if it's enabled:
   ```bash
   gnome-extensions info focused-window-dbus@nichijou.github.io
   ```

3. Test D-Bus connection manually:
   ```bash
   gdbus call --session --dest org.gnome.Shell \
     --object-path /org/gnome/shell/extensions/FocusedWindow \
     --method org.gnome.shell.extensions.FocusedWindow.Get
   ```

### No Window Detection

If the application runs but doesn't detect window changes:

1. Enable debug mode:
   ```bash
   ./active-window -monitor -debug
   ```

2. Check if you're on Wayland:
   ```bash
   echo $XDG_SESSION_TYPE  # Should output: wayland
   ```

3. Verify GNOME Shell version compatibility:
   ```bash
   gnome-shell --version
   ```

### API Submission Failures

If data isn't reaching RescueTime:

1. Test in dry-run mode first:
   ```bash
   ./active-window -track -dry-run -submission-interval 1m
   ```

2. Verify your API key:
   ```bash
   cat .env  # Check RESCUE_TIME_API_KEY is set
   ```

3. Test with verbose logging:
   ```bash
   ./active-window -track -submit -submission-interval 2m -verbose
   ```

### Debugging Session Data

Save sessions to a file for inspection:

```bash
./active-window -track -save -submission-interval 2m
# Switch between windows for a few minutes
# Press Ctrl+C to stop
# Check rescuetime-sessions.json
cat rescuetime-sessions.json | jq .
```

</details>


<details>
<summary><h2>Architecture</h2></summary>

### Core Components

**1. Window Monitoring** (`getActiveWindow()`)
- Connects to D-Bus session bus
- Calls `org.gnome.Shell` → `/org/gnome/shell/extensions/FocusedWindow`
- Returns structured `MutterWindow` information
- Configurable polling interval (default: 1000ms)

**2. Activity Tracking** (`ActivityTracker`)
- Thread-safe session management with `sync.RWMutex`
- Automatic session start/end on window focus changes
- Session merging for brief interruptions (< 30s)
- Filters out sessions shorter than 10 seconds

**3. Data Aggregation** (`GetActivitySummaries()`)
- Aggregates multiple sessions per application
- Calculates total duration and session counts
- Includes currently active session in real-time

**4. API Submission** (`submitToRescueTime()`)
- Posts to RescueTime Offline Time API
- Exponential backoff retry (3 attempts: 1s, 2s, 4s)
- 10-second HTTP timeout per request
- Distinguishes retryable (5xx) vs non-retryable (4xx) errors

**5. Debug & Testing Features**
- Dry-run mode: preview submissions without API calls
- Session persistence: save to JSON file for inspection
- Multi-level logging: debug, verbose, info, error
- Graceful shutdown with final data submission

### Key Data Structures

```go
// Window information from GNOME Shell extension
type MutterWindow struct {
    Title           string `json:"title"`
    WmClass         string `json:"wm_class"`
    WmClassInstance string `json:"wm_class_instance"`
    Pid             int32  `json:"pid"`
    Focus           bool   `json:"focus"`
    // ... additional fields
}

// Single continuous session with an application
type ActivitySession struct {
    AppClass    string
    WindowTitle string
    StartTime   time.Time
    EndTime     time.Time
    Duration    time.Duration
}

// Aggregated time across multiple sessions
type ActivitySummary struct {
    AppClass       string
    TotalDuration  time.Duration
    SessionCount   int
    LastWindowTitle string
    MostRecentTime time.Time
}

// RescueTime API payload
type RescueTimePayload struct {
    StartTime       string `json:"start_time"`        // "YYYY-MM-DD HH:MM:SS"
    Duration        int    `json:"duration"`          // minutes
    ActivityName    string `json:"activity_name"`     // app class
    ActivityDetails string `json:"activity_details"`  // window title
}
```

</details>

<details>
<summary><h2>API Integration</h2></summary>

### Current Implementation (Legacy API)

Uses the public **Offline Time POST API**:
- **Endpoint:** `https://www.rescuetime.com/anapi/offline_time_post`
- **Auth:** Query parameter `?key=API_KEY`
- **Method:** POST JSON
- **Max duration:** 4 hours per entry

### Native Client API (Reverse Engineered)

Documentation available in `RescueTime-Complete-Authentication-Reverse-Engineering-Report.md`

**Authentication Flow:**
```bash
POST https://www.rescuetime.com/activate
Content-Type: application/json
Accept: application/json

{
  "username": "your@email.com",
  "password": "your_password",
  "computer_name": "my-linux-machine"
}

# Response:
{
  "account_key": "7f9e2a8b1c3d4e5f6a7b8c9d0e1f2a3b",  // 32-char hex
  "data_key": "xYz9AbC123dEfGhI456jKlMnOpQr789sTuVwXyZ0"   // 44-char base64
}
```

**Event Submission:**
```bash
POST https://api.rescuetime.com/api/resource/user_client_events
Authorization: Bearer {data_key}
Content-Type: application/json; charset=utf-8

{
  "user_client_event": {
    "event_description": "firefox",
    "start_time": "2025-10-02T14:00:00Z",
    "end_time": "2025-10-02T14:05:00Z",
    "window_title": "Example Window Title",
    "application": "firefox"
  }
}
```

</details>

## Related Documentation

- [RescueTime API Documentation](docs/api-docs.md)
- [FocusedWindow Extension](https://github.com/nichijou/gnome-shell-extension-focused-window-dbus)

## Acknowledgments

- [robwilde](https://github.com/robwilde/rescuetime-linux) for the original implementation that uses Hyprland
- [nichijou](https://github.com/nichijou) for the FocusedWindow D-Bus extension

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
