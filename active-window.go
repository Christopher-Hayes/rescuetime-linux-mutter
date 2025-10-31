package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
	"github.com/fatih/color"
	"github.com/godbus/dbus/v5"
)

// Type aliases for rescuetime package types to maintain compatibility
type ActivitySummary = rescuetime.ActivitySummary

// Configuration constants for tracking behavior
const (
	// Session tracking thresholds
	defaultMergeThreshold = 30 * time.Second // Merge sessions if gap is less than this
	defaultMinDuration    = 10 * time.Second // Ignore sessions shorter than this
	defaultPollInterval   = 1000 * time.Millisecond
	defaultSubmitInterval = 15 * time.Minute
	
	// Idle detection
	defaultIdleThreshold = 5 * time.Minute // Consider user idle after 5 minutes of inactivity
)

// Global variables for configuration
var (
	debugMode   bool
	verboseMode bool
	
	// Color functions for different log levels
	colorDebug   = color.New(color.FgCyan).SprintfFunc()
	colorVerbose = color.New(color.FgBlue).SprintfFunc()
	colorInfo    = color.New(color.FgGreen).SprintfFunc()
	colorError   = color.New(color.FgRed, color.Bold).SprintfFunc()
	colorWarning = color.New(color.FgYellow).SprintfFunc()
	colorSuccess = color.New(color.FgGreen, color.Bold).SprintfFunc()
	colorKey     = color.New(color.FgMagenta).SprintfFunc()
	colorValue   = color.New(color.FgWhite, color.Bold).SprintfFunc()
)

// debugLog prints debug messages if debug mode is enabled
func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf(colorDebug("[DEBUG] "+format, args...))
	}
}

// verboseLog prints verbose messages if verbose mode is enabled
func verboseLog(format string, args ...interface{}) {
	if verboseMode || debugMode {
		log.Printf(colorVerbose("[VERBOSE] "+format, args...))
	}
}

// infoLog prints info messages (always shown)
func infoLog(format string, args ...interface{}) {
	log.Printf(colorInfo("[INFO] "+format, args...))
}

// errorLog prints error messages (always shown)
func errorLog(format string, args ...interface{}) {
	log.Printf(colorError("[ERROR] "+format, args...))
}

// warningLog prints warning messages (always shown)
func warningLog(format string, args ...interface{}) {
	log.Printf(colorWarning("[WARNING] "+format, args...))
}

// successLog prints success messages (always shown)
func successLog(format string, args ...interface{}) {
	log.Printf(colorSuccess("[SUCCESS] "+format, args...))
}

// ActivitySession represents a single continuous session with an application
type ActivitySession struct {
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	AppClass    string        `json:"app_class"`
	WindowTitle string        `json:"window_title"`
	Duration    time.Duration `json:"duration"`
	Active      bool          `json:"active"` // true if session is currently ongoing
}

// ActivityTracker manages tracking of application usage sessions
type ActivityTracker struct {
	mu               sync.RWMutex
	currentSession   *ActivitySession
	sessions         []ActivitySession
	mergeThreshold   time.Duration // merge sessions shorter than this threshold
	minDuration      time.Duration // ignore sessions shorter than this
	ignoredApps      map[string]bool // WmClass values to ignore
	ignoreConfigPath string          // path to ignore list file
}

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(filepath string) error {
	file, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("failed to open .env file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on first '=' sign
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Set environment variable
		os.Setenv(key, value)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading .env file: %v", err)
	}

	return nil
}

// submitActivitiesToRescueTime submits all activity summaries to RescueTime
// Attempts native user_client_events API first if credentials are available,
// falls back to offline_time_post API if native fails or credentials are missing.
func submitActivitiesToRescueTime(apiKey string, summaries map[string]ActivitySummary) {
	// Create RescueTime client
	client := rescuetime.NewClient(apiKey, "", "")
	client.DebugMode = debugMode
	
	// Delegate to the rescuetime package
	client.SubmitActivities(summaries)
}

// NewActivityTracker creates a new activity tracker with default settings
func NewActivityTracker() *ActivityTracker {
	tracker := &ActivityTracker{
		sessions:         make([]ActivitySession, 0),
		mergeThreshold:   defaultMergeThreshold,
		minDuration:      defaultMinDuration,
		ignoredApps:      make(map[string]bool),
		ignoreConfigPath: ".rescuetime-ignore",
	}
	
	// Load ignored applications from config file
	if err := tracker.loadIgnoredApps(); err != nil {
		debugLog("No ignore list found or error loading: %v", err)
	}
	
	return tracker
}

// loadIgnoredApps loads the list of ignored applications from config file
func (at *ActivityTracker) loadIgnoredApps() error {
	file, err := os.Open(at.ignoreConfigPath)
	if err != nil {
		return err
	}
	defer file.Close()

	at.mu.Lock()
	defer at.mu.Unlock()

	at.ignoredApps = make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		at.ignoredApps[line] = true
		debugLog("Loaded ignored application: %s", line)
	}

	if len(at.ignoredApps) > 0 {
		verboseLog("Loaded %d ignored applications from %s", len(at.ignoredApps), at.ignoreConfigPath)
	}

	return scanner.Err()
}

// isAppIgnored checks if an application should be ignored
func (at *ActivityTracker) isAppIgnored(appClass string) bool {
	at.mu.RLock()
	defer at.mu.RUnlock()
	return at.ignoredApps[appClass]
}

// addIgnoredApp adds an application to the ignore list and saves to file
func (at *ActivityTracker) addIgnoredApp(appClass string) error {
	at.mu.Lock()
	at.ignoredApps[appClass] = true
	at.mu.Unlock()

	return at.saveIgnoredApps()
}

// saveIgnoredApps saves the current ignore list to file
func (at *ActivityTracker) saveIgnoredApps() error {
	at.mu.RLock()
	defer at.mu.RUnlock()

	file, err := os.Create(at.ignoreConfigPath)
	if err != nil {
		return fmt.Errorf("failed to create ignore file: %v", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	
	// Write header
	fmt.Fprintln(writer, "# RescueTime Ignored Applications")
	fmt.Fprintln(writer, "# One WmClass per line")
	fmt.Fprintln(writer, "# Lines starting with # are comments")
	fmt.Fprintln(writer, "")

	// Write ignored apps
	for appClass := range at.ignoredApps {
		fmt.Fprintln(writer, appClass)
	}

	return writer.Flush()
}

// StartSession begins tracking a new activity session
func (at *ActivityTracker) StartSession(appClass, windowTitle string) {
	// Check if app should be ignored
	if at.isAppIgnored(appClass) {
		debugLog("Ignoring application: %s", appClass)
		
		// End current session if exists, but don't start a new one
		at.mu.Lock()
		if at.currentSession != nil && at.currentSession.Active {
			at.endCurrentSessionUnsafe(time.Now())
		}
		at.currentSession = nil
		at.mu.Unlock()
		return
	}

	at.mu.Lock()
	defer at.mu.Unlock()

	now := time.Now()

	// End the current session if one exists
	if at.currentSession != nil && at.currentSession.Active {
		at.endCurrentSessionUnsafe(now)
	}

	// Start new session
	at.currentSession = &ActivitySession{
		StartTime:   now,
		AppClass:    appClass,
		WindowTitle: windowTitle,
		Active:      true,
	}
}

// endCurrentSessionUnsafe ends the current session (must be called with lock held)
func (at *ActivityTracker) endCurrentSessionUnsafe(endTime time.Time) {
	if at.currentSession == nil || !at.currentSession.Active {
		return
	}

	at.currentSession.EndTime = endTime
	at.currentSession.Duration = endTime.Sub(at.currentSession.StartTime)
	at.currentSession.Active = false

	// Only store sessions that meet minimum duration requirement
	if at.currentSession.Duration >= at.minDuration {
		// Check if we should merge with the last session
		if at.shouldMergeWithLastSession() {
			at.mergeWithLastSession()
		} else {
			// Store the session
			at.sessions = append(at.sessions, *at.currentSession)
		}
	}
}

// EndCurrentSession ends the currently active session
func (at *ActivityTracker) EndCurrentSession() {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.endCurrentSessionUnsafe(time.Now())
}

// shouldMergeWithLastSession checks if current session should be merged with the previous one
func (at *ActivityTracker) shouldMergeWithLastSession() bool {
	if len(at.sessions) == 0 || at.currentSession == nil {
		return false
	}

	lastSession := &at.sessions[len(at.sessions)-1]

	// Can only merge sessions of the same application
	if lastSession.AppClass != at.currentSession.AppClass {
		return false
	}

	// Check if the gap between sessions is within merge threshold
	gap := at.currentSession.StartTime.Sub(lastSession.EndTime)
	return gap <= at.mergeThreshold
}

// mergeWithLastSession merges current session with the last stored session
func (at *ActivityTracker) mergeWithLastSession() {
	if len(at.sessions) == 0 || at.currentSession == nil {
		return
	}

	lastSession := &at.sessions[len(at.sessions)-1]

	// Extend the last session to include the current session
	lastSession.EndTime = at.currentSession.EndTime
	lastSession.Duration = lastSession.EndTime.Sub(lastSession.StartTime)

	// Use the most recent window title
	lastSession.WindowTitle = at.currentSession.WindowTitle
}

// GetActivitySummaries aggregates sessions by application class
func (at *ActivityTracker) GetActivitySummaries() map[string]ActivitySummary {
	at.mu.RLock()
	defer at.mu.RUnlock()

	summaries := make(map[string]ActivitySummary)

	// Process all completed sessions
	for _, session := range at.sessions {
		key := session.AppClass
		summary, exists := summaries[key]

		if !exists {
			summary = ActivitySummary{
				AppClass:        session.AppClass,
				ActivityDetails: session.WindowTitle,
				FirstSeen:       session.StartTime,
				LastSeen:        session.EndTime,
			}
		}

		// Update summary
		summary.TotalDuration += session.Duration
		summary.SessionCount++

		// Update time boundaries
		if session.StartTime.Before(summary.FirstSeen) {
			summary.FirstSeen = session.StartTime
		}
		if session.EndTime.After(summary.LastSeen) {
			summary.LastSeen = session.EndTime
			// Use the most recent window title as activity details
			summary.ActivityDetails = session.WindowTitle
		}

		summaries[key] = summary
	}

	// Include current active session if exists
	if at.currentSession != nil && at.currentSession.Active {
		key := at.currentSession.AppClass
		summary, exists := summaries[key]

		currentDuration := time.Since(at.currentSession.StartTime)

		if !exists {
			summary = ActivitySummary{
				AppClass:        at.currentSession.AppClass,
				ActivityDetails: at.currentSession.WindowTitle,
				FirstSeen:       at.currentSession.StartTime,
				LastSeen:        time.Now(),
			}
		}

		summary.TotalDuration += currentDuration
		summary.SessionCount++

		// Update activity details to current window title
		summary.ActivityDetails = at.currentSession.WindowTitle
		summary.LastSeen = time.Now()

		summaries[key] = summary
	}

	return summaries
}

// ClearCompletedSessions removes all completed sessions, keeping only the current active session
func (at *ActivityTracker) ClearCompletedSessions() {
	at.mu.Lock()
	defer at.mu.Unlock()

	// Clear all stored sessions but keep the current active one
	at.sessions = make([]ActivitySession, 0)
}

func getActiveWindow() (*MutterWindow, error) {
	// Connect to session bus
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to session bus: %v", err)
	}
	defer conn.Close()

	debugLog("Connected to D-Bus session bus")

	// Call the FocusedWindow extension
	obj := conn.Object(dbusDestination, dbusObjectPath)
	call := obj.Call(dbusMethod, 0)
	
	if call.Err != nil {
		return nil, fmt.Errorf("failed to call FocusedWindow.Get: %v\n\nTroubleshooting:\n  1. Verify extension is installed: gnome-extensions list | grep focused\n  2. Enable if needed: gnome-extensions enable focused-window-dbus@nichijou.github.io\n  3. Test D-Bus manually: gdbus call --session --dest org.gnome.Shell --object-path /org/gnome/shell/extensions/FocusedWindow --method org.gnome.shell.extensions.FocusedWindow.Get\n  4. Run: ./verify-setup.sh", call.Err)
	}

	// The response is a tuple with a JSON string
	var jsonStr string
	err = call.Store(&jsonStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse D-Bus response: %v", err)
	}

	debugLog("Received D-Bus response: %s", jsonStr)

	// Parse the JSON response
	var window MutterWindow
	err = json.Unmarshal([]byte(jsonStr), &window)
	if err != nil {
		return nil, fmt.Errorf("failed to parse window JSON: %v", err)
	}

	return &window, nil
}

// getIdleTime queries Mutter's IdleMonitor to get user idle time in milliseconds
func getIdleTime() (time.Duration, error) {
	// Connect to session bus
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return 0, fmt.Errorf("failed to connect to session bus: %v", err)
	}
	defer conn.Close()

	debugLog("Querying idle time from Mutter IdleMonitor")

	// Call the IdleMonitor GetIdletime method
	obj := conn.Object(idleMonitorDestination, dbus.ObjectPath(idleMonitorObjectPath))
	call := obj.Call(idleMonitorMethod, 0)
	
	if call.Err != nil {
		return 0, fmt.Errorf("failed to call IdleMonitor.GetIdletime: %v\n\nTroubleshooting:\n  1. Verify you're running GNOME/Mutter\n  2. Test D-Bus manually: gdbus call --session --dest org.gnome.Mutter.IdleMonitor --object-path /org/gnome/Mutter/IdleMonitor/Core --method org.gnome.Mutter.IdleMonitor.GetIdletime", call.Err)
	}

	// The response is an unsigned 64-bit integer representing idle time in milliseconds
	var idleMs uint64
	err = call.Store(&idleMs)
	if err != nil {
		return 0, fmt.Errorf("failed to parse IdleMonitor response: %v", err)
	}

	idleDuration := time.Duration(idleMs) * time.Millisecond
	debugLog("Current idle time: %v (%d ms)", idleDuration, idleMs)

	return idleDuration, nil
}

func getActiveWindowName() (string, error) {
	window, err := getActiveWindow()
	if err != nil {
		return "", err
	}
	return window.Title, nil
}

func getActiveWindowClass() (string, error) {
	window, err := getActiveWindow()
	if err != nil {
		return "", err
	}
	return window.WmClass, nil
}

func formatWindowOutput(windowName, windowClass string) string {
	if windowClass != "" {
		return fmt.Sprintf("%s: %s %s",
			colorKey("Active Window"),
			colorValue(windowName),
			color.HiBlackString("(%s)", windowClass))
	}
	return fmt.Sprintf("%s: %s",
		colorKey("Active Window"),
		colorValue(windowName))
}

// validateConfiguration checks critical configuration before starting
func validateConfiguration(submitToAPI bool, dryRun bool, apiKey string, submissionInterval time.Duration, pollInterval time.Duration) error {
	// Validate submission interval
	if submissionInterval < 1*time.Minute {
		return fmt.Errorf("submission interval must be at least 1 minute, got %v", submissionInterval)
	}

	// Validate poll interval
	if pollInterval < 50*time.Millisecond {
		return fmt.Errorf("poll interval too short (minimum 50ms), got %v", pollInterval)
	}
	if pollInterval > 5*time.Second {
		errorLog("Warning: poll interval %v is unusually long, may miss window changes", pollInterval)
	}

	// Validate API key if submission is enabled
	if submitToAPI && !dryRun {
		if apiKey == "" {
			return fmt.Errorf("RESCUE_TIME_API_KEY not found in .env file\nRun: cp .env.example .env\nThen edit .env and add your API key from https://www.rescuetime.com/anapi/manage")
		}
		if len(apiKey) < 20 {
			return fmt.Errorf("RESCUE_TIME_API_KEY appears invalid (too short: %d chars)\nGet your API key from https://www.rescuetime.com/anapi/manage", len(apiKey))
		}
	}

	// Check environment
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		return fmt.Errorf("no graphical display found (neither WAYLAND_DISPLAY nor DISPLAY set)\nMake sure you're running this in a graphical session")
	}

	return nil
}

// previewSubmission shows what would be submitted in dry-run mode
func previewSubmission(summaries map[string]ActivitySummary) {
	if len(summaries) == 0 {
		color.Yellow("No activities to preview.")
		return
	}

	color.New(color.FgMagenta, color.Bold).Printf("\n=== DRY-RUN: Would submit %d activities ===\n", len(summaries))
	
	for _, summary := range summaries {
		// Skip activities with very short duration (< 1 minute)
		if summary.TotalDuration < time.Minute {
			debugLog("Skipping %s (duration < 1 minute)", summary.AppClass)
			continue
		}

		payload := rescuetime.SummaryToPayload(summary)
		
		// Validate payload before submission
		if err := rescuetime.ValidatePayload(payload); err != nil {
			errorLog("Invalid payload for %s: %v", summary.AppClass, err)
			continue
		}
		
		jsonData, _ := json.MarshalIndent(payload, "", "  ")
		
		color.Cyan("\n[PREVIEW] Would submit:")
		fmt.Printf("\n%s\n", string(jsonData))
	}
	
	color.New(color.FgMagenta, color.Bold).Println("\n=== End of preview ===")
}

// saveSummariesToFile saves activity summaries to a JSON file
func saveSummariesToFile(filepath string, summaries map[string]ActivitySummary) error {
	// Convert map to slice for better JSON formatting
	type SavedSummary struct {
		AppClass        string    `json:"app_class"`
		ActivityDetails string    `json:"activity_details"`
		TotalDuration   string    `json:"total_duration"`
		SessionCount    int       `json:"session_count"`
		FirstSeen       time.Time `json:"first_seen"`
		LastSeen        time.Time `json:"last_seen"`
	}

	type SavedData struct {
		Timestamp time.Time       `json:"timestamp"`
		Summaries []SavedSummary  `json:"summaries"`
	}

	savedSummaries := make([]SavedSummary, 0, len(summaries))
	for _, summary := range summaries {
		savedSummaries = append(savedSummaries, SavedSummary{
			AppClass:        summary.AppClass,
			ActivityDetails: summary.ActivityDetails,
			TotalDuration:   summary.TotalDuration.String(),
			SessionCount:    summary.SessionCount,
			FirstSeen:       summary.FirstSeen,
			LastSeen:        summary.LastSeen,
		})
	}

	data := SavedData{
		Timestamp: time.Now(),
		Summaries: savedSummaries,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal summaries: %v", err)
	}

	err = os.WriteFile(filepath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	return nil
}

// printActivitySummary prints a summary of tracked activities
func printActivitySummary(tracker *ActivityTracker) {
	color.New(color.FgCyan, color.Bold).Println("\n=== Activity Summary ===")

	summaries := tracker.GetActivitySummaries()
	if len(summaries) == 0 {
		color.Yellow("No activities tracked.")
		return
	}

	totalTime := time.Duration(0)
	for _, summary := range summaries {
		totalTime += summary.TotalDuration
	}

	color.New(color.FgWhite, color.Bold).Printf("Total tracking time: %v\n\n", totalTime.Round(time.Second))

	for appClass, summary := range summaries {
		percentage := float64(summary.TotalDuration) / float64(totalTime) * 100
		color.New(color.FgGreen, color.Bold).Printf("%s: ", appClass)
		fmt.Printf("%v ", summary.TotalDuration.Round(time.Second))
		color.Cyan("(%.1f%%) ", percentage)
		color.New(color.FgWhite).Printf("- %d sessions\n", summary.SessionCount)
		color.New(color.FgHiBlack).Printf("  └─ %s\n\n", summary.ActivityDetails)
	}
}

func getCurrentWindowInfo() (string, error) {
	windowName, err := getActiveWindowName()
	if err != nil {
		return "", err
	}

	windowClass, _ := getActiveWindowClass()
	return formatWindowOutput(windowName, windowClass), nil
}

func monitorWindowChanges(interval time.Duration, submitToAPI bool, apiKey string, submissionInterval time.Duration, dryRun bool, saveToFile bool, idleThreshold time.Duration) {
	// Add panic recovery to prevent crashes
	defer func() {
		if r := recover(); r != nil {
			errorLog("PANIC recovered in monitorWindowChanges: %v", r)
			errorLog("Stack trace will be printed by the runtime")
		}
	}()

	var lastAppClass, lastWindowTitle string
	var wasIdle bool

	// Create activity tracker
	tracker := NewActivityTracker()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Get initial window info and start the first session
	window, err := getActiveWindow()
	if err != nil {
		errorLog("Error getting initial window info: %v", err)
		return
	}

	// Check initial idle state
	idleTime, err := getIdleTime()
	if err != nil {
		errorLog("Error getting initial idle time: %v", err)
		// Continue anyway, will retry on next poll
	} else if idleTime >= idleThreshold {
		wasIdle = true
		verboseLog("User is currently idle (%v), not starting tracking yet", idleTime)
	} else {
		// Start the initial session only if not idle
		tracker.StartSession(window.WmClass, window.Title)
		lastAppClass = window.WmClass
		lastWindowTitle = window.Title

		// Print initial window
		currentInfo := formatWindowOutput(window.Title, window.WmClass)
		fmt.Printf("%s [%s]\n", currentInfo, time.Now().Format("15:04:05"))
		verboseLog("Started tracking: %s", currentInfo)
	}

	pollTicker := time.NewTicker(interval)
	defer pollTicker.Stop()

	var submitTicker *time.Ticker
	var submitChan <-chan time.Time

	if submitToAPI && !dryRun {
		submitTicker = time.NewTicker(submissionInterval)
		defer submitTicker.Stop()
		submitChan = submitTicker.C
		infoLog("API submission enabled: will submit every %v", submissionInterval)
	} else if dryRun {
		submitTicker = time.NewTicker(submissionInterval)
		defer submitTicker.Stop()
		submitChan = submitTicker.C
		infoLog("DRY-RUN mode: will show what would be submitted every %v (no actual API calls)", submissionInterval)
	}

	for {
		select {
		case <-sigChan:
			color.Yellow("\nShutting down window monitor...")
			infoLog("Received shutdown signal")

			// End the current session
			tracker.EndCurrentSession()

			// Submit final data if API submission is enabled
			if submitToAPI && !dryRun {
				infoLog("Submitting final data before shutdown...")
				summaries := tracker.GetActivitySummaries()
				submitActivitiesToRescueTime(apiKey, summaries)
			} else if dryRun {
				infoLog("DRY-RUN: Final submission preview")
				summaries := tracker.GetActivitySummaries()
				previewSubmission(summaries)
			}

			// Save to file if requested
			if saveToFile {
				summaries := tracker.GetActivitySummaries()
				err := saveSummariesToFile("rescuetime-sessions.json", summaries)
				if err != nil {
					errorLog("Failed to save sessions to file: %v", err)
				} else {
					infoLog("Saved sessions to rescuetime-sessions.json")
				}
			}

			// Print summary before exit
			printActivitySummary(tracker)
			return

		case <-submitChan:
			// Time to submit data to RescueTime (or preview in dry-run mode)
			summaries := tracker.GetActivitySummaries()
			
			if dryRun {
				infoLog("DRY-RUN: Submission preview")
				previewSubmission(summaries)
			} else {
				submitActivitiesToRescueTime(apiKey, summaries)
			}

			// Save to file if requested
			if saveToFile {
				err := saveSummariesToFile("rescuetime-sessions.json", summaries)
				if err != nil {
					errorLog("Failed to save sessions to file: %v", err)
				} else {
					verboseLog("Saved sessions to rescuetime-sessions.json")
				}
			}

			// Clear completed sessions after submission
			tracker.ClearCompletedSessions()

		case <-pollTicker.C:
			// Check idle status first
			idleTime, err := getIdleTime()
			if err != nil {
				debugLog("Error getting idle time: %v", err)
				// Continue with window tracking even if idle detection fails
			} else {
				isIdle := idleTime >= idleThreshold
				
				// Handle idle state transitions
				if isIdle && !wasIdle {
					// User just became idle - end current session
					verboseLog("User went idle (idle for %v), pausing tracking", idleTime)
					tracker.EndCurrentSession()
					wasIdle = true
					continue // Skip window tracking while idle
				} else if !isIdle && wasIdle {
					// User returned from idle - resume tracking
					verboseLog("User returned from idle (idle time: %v), resuming tracking", idleTime)
					wasIdle = false
					// Will start new session below if window info is available
				} else if isIdle {
					// Still idle - skip this iteration
					debugLog("User still idle (%v), skipping window poll", idleTime)
					continue
				}
				// If not idle and wasn't idle, continue normal tracking below
			}

			window, err := getActiveWindow()
			if err != nil {
				// Don't spam errors, just skip this iteration
				debugLog("Error getting window: %v", err)
				continue
			}

			// Check if the application or window title changed
			if window.WmClass != lastAppClass || window.Title != lastWindowTitle {
				// Start a new session for the new window/app
				tracker.StartSession(window.WmClass, window.Title)

				// Print the change
				currentInfo := formatWindowOutput(window.Title, window.WmClass)
				fmt.Printf("%s [%s]\n", currentInfo, time.Now().Format("15:04:05"))
				verboseLog("Window changed to: %s (%s)", window.Title, window.WmClass)

				// Update tracking variables
				lastAppClass = window.WmClass
				lastWindowTitle = window.Title
			}
		}
	}
}

func main() {
	// Command line flags
	monitor := flag.Bool("monitor", false, "Continuously monitor for window changes")
	track := flag.Bool("track", false, "Monitor and track time spent in applications")
	submit := flag.Bool("submit", false, "Submit activity data to RescueTime API")
	dryRun := flag.Bool("dry-run", false, "Show what would be submitted without making API calls")
	saveToFile := flag.Bool("save", false, "Save activity summaries to rescuetime-sessions.json")
	debug := flag.Bool("debug", false, "Enable debug logging")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	interval := flag.Duration("interval", defaultPollInterval, "Polling interval for monitoring mode (e.g., 100ms, 1s)")
	submissionInterval := flag.Duration("submission-interval", defaultSubmitInterval, "Interval for submitting data to RescueTime (e.g., 15m, 1h)")
	idleThreshold := flag.Duration("idle-threshold", defaultIdleThreshold, "Consider user idle after this duration of inactivity (e.g., 5m, 10m)")
	flag.Parse()

	// Set global debug/verbose flags
	debugMode = *debug
	verboseMode = *verbose

	// Configure logging
	log.SetFlags(log.Ldate | log.Ltime)
	if debugMode {
		log.SetPrefix("[rescuetime] ")
		debugLog("Debug mode enabled")
	}

	// Check if we're running in a graphical environment (Wayland or X11)
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		errorLog("No graphical display found. Make sure you're running this in a Wayland or X11 environment.")
		os.Exit(1)
	}

	// Check if running on GNOME/Mutter
	sessionType := os.Getenv("XDG_SESSION_TYPE")
	desktopSession := os.Getenv("XDG_CURRENT_DESKTOP")
	debugLog("Session type: %s, Desktop: %s", sessionType, desktopSession)

	// Verify D-Bus connection to GNOME Shell extension
	if *monitor || *track {
		_, err := getActiveWindow()
		if err != nil {
			errorLog("Failed to connect to GNOME Shell FocusedWindow extension: %v", err)
			fmt.Fprintf(os.Stderr, "\nMake sure the FocusedWindow GNOME Shell extension is installed and enabled.\n")
			fmt.Fprintf(os.Stderr, "Installation: https://extensions.gnome.org/extension/5839/focused-window-dbus/\n")
			os.Exit(1)
		}
		verboseLog("Successfully connected to FocusedWindow D-Bus extension")
		
		// Verify idle monitor is available
		_, err = getIdleTime()
		if err != nil {
			errorLog("Warning: Failed to connect to Mutter IdleMonitor: %v", err)
			errorLog("Idle detection will be disabled. Make sure you're running GNOME/Mutter.")
		} else {
			verboseLog("Successfully connected to Mutter IdleMonitor (idle threshold: %v)", *idleThreshold)
		}
	}

	if *monitor || *track {
		if *track {
			infoLog("Tracking application usage (polling every %v). Press Ctrl+C to stop and see summary.", *interval)
		} else {
			infoLog("Monitoring window changes (polling every %v). Press Ctrl+C to stop.", *interval)
		}

		// Handle API submission setup
		var apiKey string
		if *submit || *dryRun {
			// Get API key from environment (can be set via .env file or op run)
			apiKey = os.Getenv("RESCUE_TIME_API_KEY")
			
			// If not in environment, try loading from .env file
			if apiKey == "" {
				err := loadEnvFile(".env")
				if err != nil {
					errorLog("Error loading .env file: %v", err)
					fmt.Fprintf(os.Stderr, "\nCreate .env file: cp .env.example .env\n")
					fmt.Fprintf(os.Stderr, "Then add your RescueTime API key: https://www.rescuetime.com/anapi/manage\n")
					os.Exit(1)
				}
				apiKey = os.Getenv("RESCUE_TIME_API_KEY")
			}
			
			// Validate configuration before starting
			if err := validateConfiguration(*submit, *dryRun, apiKey, *submissionInterval, *interval); err != nil {
				errorLog("Configuration validation failed: %v", err)
				os.Exit(1)
			}

			// Call with API submission enabled
			monitorWindowChanges(*interval, *submit, apiKey, *submissionInterval, *dryRun, *saveToFile, *idleThreshold)
		} else {
			// Validate basic configuration even without API submission
			if err := validateConfiguration(false, false, "", *submissionInterval, *interval); err != nil {
				errorLog("Configuration validation failed: %v", err)
				os.Exit(1)
			}
			// Call without API submission
			monitorWindowChanges(*interval, false, "", 0, false, *saveToFile, *idleThreshold)
		}
	} else {
		// Single execution mode
		currentInfo, err := getCurrentWindowInfo()
		if err != nil {
			errorLog("Error getting window info: %v", err)
			os.Exit(1)
		}
		fmt.Println(currentInfo)
	}
}
