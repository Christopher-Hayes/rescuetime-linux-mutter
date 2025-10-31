# RescueTime Linux Mutter - AI Coding Agent Instructions

> **Project Type**: Time tracking daemon · **Language**: Go 1.21+ · **Platform**: Linux/GNOME/Mutter  
> **Status**: Production-ready, monolithic implementation in `active-window.go` (~1000 LOC)

## Project Overview

A native Linux activity tracker for RescueTime that monitors active windows on GNOME/Mutter via D-Bus and submits time tracking data to RescueTime's API. Forked from a Hyprland implementation.

## Quick Start for New Contributors

```bash
# 1. Prerequisites check
./verify-setup.sh  # ALWAYS run this first

# 2. Build and test
./build.sh
./active-window -monitor -debug  # Watch window detection for 30s

# 3. Safe testing workflow (never touches production API)
./active-window -track -dry-run -submission-interval 1m -save

# 4. Inspect what would be submitted
cat rescuetime-sessions.json | jq .
```

**Common first tasks**: See [Development Workflows](#development-workflows) section below.

## Architecture Essentials

### Core Communication Pattern: D-Bus Extension Bridge
- **Critical dependency**: GNOME Shell FocusedWindow extension (`org.gnome.Shell.extensions.FocusedWindow`)
- **Idle detection**: Mutter IdleMonitor (`org.gnome.Mutter.IdleMonitor`)
- **Why D-Bus**: Wayland security model prevents direct window inspection; GNOME Shell extension has privileged access
- **Connection flow**: 
  ```
  Go app → D-Bus session bus → GNOME Shell → Extension → Window metadata (JSON)
     ↓         1000ms poll          ↓             ↓              ↓
  Parse JSON ← D-Bus response ← Extension ← Wayland compositor
  
  Go app → D-Bus session bus → Mutter IdleMonitor → User idle time (ms)
  ```
- **Detection method**: Polling at 1000ms intervals (not event-driven due to D-Bus limitations)
- **Idle detection**: Queries idle time every poll cycle, pauses tracking when user is idle (default: 5 minutes)
- **Performance impact**: <1% CPU, ~10MB RAM (polling is lightweight, D-Bus handles multiplexing)

### Activity Tracking State Machine

```
Window Focus Change Detected (WmClass changes)
         ↓
    End current session (if exists)
         ↓
    Should merge with last session?
    ├─ Yes (same app, gap <30s) → Extend last session
    └─ No → Store session if duration ≥10s
         ↓
    Start new session
         ↓
    Continue polling...
    
Idle State Change Detected
    ├─ User became idle → End current session, pause tracking
    └─ User returned from idle → Resume tracking with new session
```

**State transitions**:
1. **Window changes** → End current session → Start new session
2. **Session merging**: Gaps <30s to same app are merged (handles Alt+Tab, brief app switches)
3. **Filtering**: Sessions <10s are discarded as noise (prevents spam from window-hopping)
4. **Idle detection**: User inactivity >5m (configurable) → End session, pause tracking
5. **Return from idle**: User activity detected → Resume tracking
6. **Thread safety**: `sync.RWMutex` protects `ActivityTracker` state (read-heavy workload, rare writes)
7. **Graceful shutdown**: SIGINT/SIGTERM triggers final session end + API submission (no data loss)

**Why these thresholds?**
- 30s merge: Users often switch windows briefly then return (checking docs, alt-tab)
- 10s minimum: Accidental window activations, passing through apps
- 1000ms poll: Balance between responsiveness and CPU usage
- 5m idle: Standard threshold for "away from keyboard", matches RescueTime's own client behavior

### Dual API Strategy (Legacy + Native)
```go
// Current: Legacy Offline Time API
POST https://www.rescuetime.com/anapi/offline_time_post?key=API_KEY
// Duration in minutes, max 4 hours, simple auth

// Future: Native user_client_events API (reverse-engineered)
POST https://api.rescuetime.com/api/resource/user_client_events
Authorization: Bearer {data_key}
// RFC3339 timestamps, separate account_key/data_key auth
```
**Implementation status**: Legacy working; native API code exists but auth flow incomplete (see TODOs in code)

## Development Workflows

### Build & Test Cycle
```bash
# Quick compile + verify extension
./build.sh && ./verify-setup.sh

# Debug window detection (watch D-Bus responses)
./active-window -monitor -debug

# Test tracking without API calls (safe for dev)
./active-window -track -dry-run -submission-interval 2m

# Inspect session data structure
./active-window -track -save -submission-interval 1m
# Creates rescuetime-sessions.json
```

### Critical Testing Approach
- **Never use production API during development** - always start with `-dry-run`
- **Short intervals for testing** - use `-submission-interval 1m` instead of default 15m
- **Save to JSON** - use `-save` flag to inspect exact data structure before API submission
- **Extension verification first** - run `verify-setup.sh` before debugging application logic

### Environment Configuration Pattern
```bash
# .env file (never commit)
RESCUE_TIME_API_KEY=xxx        # Legacy API
RESCUE_TIME_ACCOUNT_KEY=xxx    # Native API (32-char hex)
RESCUE_TIME_DATA_KEY=xxx       # Native API (44-char base64)
```
**Loading**: `loadEnvFile(".env")` reads key=value pairs, sets `os.Setenv()`

## Code Conventions & Best Practices

### Constants Over Magic Numbers
All configurable values are extracted as named constants at the top of `active-window.go`:
```go
const (
    defaultMergeThreshold = 30 * time.Second
    defaultMinDuration    = 10 * time.Second
    defaultPollInterval   = 200 * time.Millisecond
    defaultIdleThreshold  = 5 * time.Minute  // Idle detection threshold
    maxAPIRetries         = 3
    baseRetryDelay        = 1 * time.Second
    maxOfflineDuration    = 4 * time.Hour  // RescueTime API limit
)
```
**Why**: Makes configuration changes easy, improves code readability, enables testing of threshold logic.

### Input Validation Pattern
All external inputs are validated before use:
- **Configuration**: `validateConfiguration()` checks intervals, API keys, environment on startup
- **API Payloads**: `validatePayload()` validates before submission (duration limits, required fields, format)
- **Fail fast**: Return actionable errors immediately rather than failing deep in execution

### Error Messages Must Be Actionable
Bad: `"failed to connect"`  
Good: `"failed to connect to D-Bus: %v\n\nTroubleshooting:\n  1. Run: ./verify-setup.sh\n  2. Check extension: gnome-extensions list | grep focused"`

Every error should tell the user **what to do next**, not just what went wrong.

### Panic Recovery in Critical Paths
The main monitoring loop includes `defer recover()` to prevent crashes:
```go
func monitorWindowChanges(...) {
    defer func() {
        if r := recover(); r != nil {
            errorLog("PANIC recovered: %v", r)
        }
    }()
    // ... critical tracking logic
}
```

### Testing Infrastructure
Tests live in `active-window_test.go` and cover:
- Validation functions (config, payloads)
- Data transformations (summary → payload)
- Core utilities (formatWindowOutput, etc.)

Run tests: `go test -v`

**Testing philosophy**: Focus on testing business logic and validation. Don't mock D-Bus or HTTP - use integration tests for those.

### Data Structure Hierarchy
```go
MutterWindow       // Raw D-Bus response (30+ fields)
  ↓ extract
ActivitySession    // Single continuous app usage (start/end times)
  ↓ aggregate  
ActivitySummary    // Per-app totals (duration, session count)
  ↓ convert & validate
RescueTimePayload  // API format (legacy: minutes, "YYYY-MM-DD HH:MM:SS")
UserClientEvent    // API format (native: RFC3339, start+end times)
```

### Logging Levels (Custom Implementation)
```go
debugLog()    // -debug flag: D-Bus responses, state transitions (cyan)
verboseLog()  // -verbose flag: Window changes, API attempts (blue)
infoLog()     // Always: Tracking started, submission summary (green)
errorLog()    // Always: API failures, setup errors (red, bold)
warningLog()  // Always: Non-fatal issues, fallbacks (yellow)
successLog()  // Always: Successful operations (green, bold)
```
**Pattern**: Use specific log functions, not generic `log.Printf()` - enables filtering by flag
**Color library**: Uses `github.com/fatih/color` for terminal colors (similar to Chalk in Node.js)

### Error Handling Strategy
- **D-Bus failures**: Retry on next poll (1000ms), log once with `debugLog()` to avoid spam
- **API submissions**: Exponential backoff (1s, 2s, 4s), distinguish 4xx (fail fast) from 5xx (retry)
- **Validation failures**: Log and skip invalid data, don't crash the entire submission batch
- **Graceful degradation**: Continue tracking if API fails, submit on next interval
- **Shutdown**: Always attempt final submission even if previous failed
- **Panic recovery**: Main loop wrapped in `defer recover()` to prevent complete crashes

## Key Files & Their Roles

- **`active-window.go`**: Monolithic implementation (1000+ lines) - data structures, D-Bus client (window & idle detection), API client, tracking logic, main loop
- **`common.go`**: Shared D-Bus configuration and data structures (FocusedWindow extension + IdleMonitor)
- **`build.sh`**: Dependency check + `go build` wrapper with user-friendly error messages
- **`verify-setup.sh`**: Pre-flight validation (GNOME version, D-Bus connectivity, extension status)
- **`docs/api-docs.md`**: Official RescueTime API documentation (copy from web for offline reference)
- **`docs/implementation-plan.md`**: Phase-based development roadmap with task status

## Platform-Specific Gotchas

### GNOME Shell Extension Lifecycle
- **Installation alone doesn't enable** - must run `gnome-extensions enable focused-window-dbus@nichijou.github.io`
- **X11 vs Wayland restart** - X11: Alt+F2 → "r" reloads shell; Wayland: must log out/in
- **Extension crashes** - Check `journalctl /usr/bin/gnome-shell` for JavaScript errors
- **D-Bus name changes** - Verify object path with `gdbus introspect` if extension updates

### Window Class Extraction Quirks
- **Use `WmClass`, not `WmClassInstance`** - WmClass is stable ("firefox"), WmClassInstance varies ("Navigator")
- **Title changes don't trigger new sessions** - Only WmClass changes matter for app tracking
- **Empty titles are valid** - Some apps have no window title (e.g., desktop, lock screen)

## Common Implementation Patterns

### Adding New Features - The Right Way
1. **Extract constants** - No magic numbers in code
2. **Add validation** - Create validator function, add tests
3. **Improve errors** - Include troubleshooting steps in error messages
4. **Add tests** - Cover validation, edge cases, and transformations
5. **Update README or AGENTS.md** - NOT a new document

### Adding New RescueTime API Support
1. Create `type XPayload struct` with JSON tags matching API docs
2. Implement `summaryToX(summary ActivitySummary) XPayload` converter
3. Add `validateXPayload(payload XPayload) error` validator
4. Add `submitX(apiKey string, payload XPayload) error` with retry logic (use constants!)
5. Update `submitActivitiesToRescueTime()` to attempt new API, fallback to legacy
6. Add tests to `active-window_test.go` for validation and conversion
7. Test with `-dry-run` to preview payload format before real submission

### Modifying Session Tracking Behavior
- **Merge threshold**: Change `ActivityTracker.mergeThreshold` (default 30s)
- **Minimum duration**: Change `ActivityTracker.minDuration` (default 10s)
- **Submission interval**: Use `-submission-interval` flag (default 15m)
- **Idle threshold**: Use `-idle-threshold` flag (default 5m)
- **Thread safety**: Always use `at.mu.Lock()` when modifying tracker state

### Testing Window Detection Changes
```bash
# Single query (no tracking)
./active-window

# Live monitoring with D-Bus debug output
./active-window -monitor -debug

# Verify JSON parsing from extension
gdbus call --session --dest org.gnome.Shell \
  --object-path /org/gnome/shell/extensions/FocusedWindow \
  --method org.gnome.shell.extensions.FocusedWindow.Get

# Test idle detection
gdbus call --session --dest org.gnome.Mutter.IdleMonitor \
  --object-path /org/gnome/Mutter/IdleMonitor/Core \
  --method org.gnome.Mutter.IdleMonitor.GetIdletime
```

## Future Work Context

### Systemd Service Considerations
When creating `.service` files:
- **Must set `WAYLAND_DISPLAY`** - app checks for graphical environment
- **Use `--user` services** - D-Bus session bus is per-user
- **Restart policy** - `Restart=on-failure` handles extension crashes
- **After dependency** - `After=graphical-session.target` ensures GNOME Shell is ready

## Quick Reference Commands

```bash
# Build and run tests
./build.sh && go test -v

# Development iteration with validation
./build.sh && ./active-window -track -dry-run -submission-interval 1m -verbose

# Verify D-Bus extension is responding
gdbus call --session --dest org.gnome.Shell \
  --object-path /org/gnome/shell/extensions/FocusedWindow \
  --method org.gnome.shell.extensions.FocusedWindow.Get

# Check if GNOME Shell is running (Ubuntu shows "Unity" but runs gnome-shell)
pgrep -a gnome-shell

# Inspect saved session data
./active-window -track -save && cat rescuetime-sessions.json | jq .

# Test configuration validation (will show helpful errors)
./active-window -track -submit  # Without .env file - see error message
./active-window -track -submission-interval 30s  # Invalid interval - see error
```

## Documentation Guidelines

- **Only update existing files**: `README.md` (user-facing) or `AGENTS.md` (developer/AI-facing)
- **Never create new doc files** like `CONTRIBUTING.md`, `ARCHITECTURE.md`, `API.md`, etc.
- **Keep it simple**: All necessary information goes in README or AGENTS.md
- **Exceptions**: Only existing files in `docs/` (api-docs.md, implementation-plan.md) can be updated
