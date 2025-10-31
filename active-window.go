package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// Configuration constants for tracking behavior
const (
	// Session tracking thresholds
	defaultMergeThreshold = 30 * time.Second // Merge sessions if gap is less than this
	defaultMinDuration    = 10 * time.Second // Ignore sessions shorter than this
	defaultPollInterval   = 1000 * time.Millisecond
	defaultSubmitInterval = 15 * time.Minute

	// API retry configuration
	maxAPIRetries     = 3
	baseRetryDelay    = 1 * time.Second
	apiTimeout        = 10 * time.Second
	maxOfflineDuration = 4 * time.Hour // RescueTime API limit for offline time
)

// Global variables for configuration
var (
	debugMode   bool
	verboseMode bool
)

// debugLog prints debug messages if debug mode is enabled
func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf("[DEBUG] "+format, args...)
	}
}

// verboseLog prints verbose messages if verbose mode is enabled
func verboseLog(format string, args ...interface{}) {
	if verboseMode || debugMode {
		log.Printf("[VERBOSE] "+format, args...)
	}
}

// infoLog prints info messages (always shown)
func infoLog(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

// errorLog prints error messages (always shown)
func errorLog(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
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

// ActivitySummary represents aggregated time spent in an application
type ActivitySummary struct {
	AppClass        string        `json:"app_class"`
	ActivityDetails string        `json:"activity_details"`
	TotalDuration   time.Duration `json:"total_duration"`
	SessionCount    int           `json:"session_count"`
	FirstSeen       time.Time     `json:"first_seen"`
	LastSeen        time.Time     `json:"last_seen"`
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

// RescueTimePayload represents the data structure for RescueTime API (legacy offline time API)
type RescueTimePayload struct {
	StartTime       string `json:"start_time"`       // YYYY-MM-DD HH:MM:SS format
	Duration        int    `json:"duration"`         // duration in minutes
	ActivityName    string `json:"activity_name"`    // application class
	ActivityDetails string `json:"activity_details"` // window title/details
}

// UserClientEventPayload represents the native RescueTime user_client_events API format
type UserClientEventPayload struct {
	UserClientEvent UserClientEvent `json:"user_client_event"`
}

// UserClientEvent represents a single activity tracking event
type UserClientEvent struct {
	EventDescription string `json:"event_description"` // application class
	StartTime        string `json:"start_time"`        // RFC 3339 format: 2025-09-30T12:00:00Z
	EndTime          string `json:"end_time"`          // RFC 3339 format: 2025-09-30T12:01:00Z
	WindowTitle      string `json:"window_title"`      // window title
	Application      string `json:"application"`       // application class (redundant with event_description)
}

// ActivationRequest represents the payload for the /activate endpoint
type ActivationRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ActivationResponse represents the response from the /activate endpoint
type ActivationResponse struct {
	AccountKey string `json:"account_key"`
	DataKey    string `json:"data_key"`
	ApiURL     string `json:"api_url"`
	URL        string `json:"url"`
}

// activateWithRescueTime authenticates with RescueTime and retrieves account keys
func activateWithRescueTime(email, password string) (*ActivationResponse, error) {
	// Discovered through testing: endpoint uses form-encoded data with username/password fields
	url := "https://api.rescuetime.com/activate"

	// Create form-encoded payload
	formData := fmt.Sprintf("username=%s&password=%s",
		strings.ReplaceAll(email, "@", "%40"), // URL encode @ sign
		password)

	// Create request
	req, err := http.NewRequest("POST", url, strings.NewReader(formData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "RescueTime/2.16.5.1 (Linux)")

	// Send request
	client := &http.Client{Timeout: apiTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// Check for error in response
	// Response format is YAML-like: "c:\n- 0\n- RT:ok\naccount_key: xxx\nkey: xxx"
	bodyStr := string(body)
	if strings.Contains(bodyStr, "RT:error") {
		return nil, fmt.Errorf("activation failed: %s", bodyStr)
	}

	// Parse response to extract account_key
	// TODO: The response only contains account_key, not data_key
	// We need to discover how to obtain the data_key (separate endpoint? different auth flow?)
	var accountKey string
	for _, line := range strings.Split(bodyStr, "\n") {
		if strings.HasPrefix(line, "account_key:") {
			accountKey = strings.TrimSpace(strings.TrimPrefix(line, "account_key:"))
			break
		}
	}

	if accountKey == "" {
		return nil, fmt.Errorf("no account_key in response: %s", bodyStr)
	}

	// Return response with account_key
	// Note: data_key is empty - needs further investigation
	return &ActivationResponse{
		AccountKey: accountKey,
		DataKey:    "", // TODO: Discover how to obtain data_key
		ApiURL:     "api.rescuetime.com",
		URL:        "www.rescuetime.com",
	}, nil
}

// saveCredentialsToEnv saves the activation credentials to .env file
func saveCredentialsToEnv(filepath string, response *ActivationResponse) error {
	// Read existing .env file to preserve RESCUE_TIME_API_KEY if it exists
	existingVars := make(map[string]string)

	file, err := os.Open(filepath)
	if err == nil {
		// File exists, read it
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				existingVars[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		file.Close()
	}

	// Update with new credentials
	existingVars["RESCUE_TIME_ACCOUNT_KEY"] = response.AccountKey
	existingVars["RESCUE_TIME_DATA_KEY"] = response.DataKey

	// Write back to file
	f, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create .env file: %v", err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)

	// Write header
	fmt.Fprintln(writer, "# RescueTime API Credentials")
	fmt.Fprintln(writer, "# Generated by active-window")
	fmt.Fprintln(writer, "")

	// Write all variables
	for key, value := range existingVars {
		fmt.Fprintf(writer, "%s=%s\n", key, value)
	}

	return writer.Flush()
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

// summaryToPayload converts an ActivitySummary to RescueTimePayload format (legacy)
func summaryToPayload(summary ActivitySummary) RescueTimePayload {
	// Convert duration to minutes (rounded up)
	durationMinutes := int(math.Ceil(summary.TotalDuration.Minutes()))

	// Format start time as "YYYY-MM-DD HH:MM:SS"
	startTimeFormatted := summary.FirstSeen.Format("2006-01-02 15:04:05")

	// For offline time API, activity_name is the application name
	activityName := summary.AppClass

	return RescueTimePayload{
		StartTime:       startTimeFormatted,
		Duration:        durationMinutes,
		ActivityName:    activityName,
		ActivityDetails: summary.ActivityDetails,
	}
}

// summaryToUserClientEvent converts an ActivitySummary to UserClientEventPayload format
func summaryToUserClientEvent(summary ActivitySummary) UserClientEventPayload {
	// Calculate end time: start time + total duration
	endTime := summary.FirstSeen.Add(summary.TotalDuration)

	// Format timestamps in RFC 3339 (ISO 8601) format with UTC timezone
	startTimeFormatted := summary.FirstSeen.UTC().Format(time.RFC3339)
	endTimeFormatted := endTime.UTC().Format(time.RFC3339)

	return UserClientEventPayload{
		UserClientEvent: UserClientEvent{
			EventDescription: summary.AppClass,
			StartTime:        startTimeFormatted,
			EndTime:          endTimeFormatted,
			WindowTitle:      summary.ActivityDetails,
			Application:      summary.AppClass, // Same as EventDescription
		},
	}
}

// submitToRescueTime submits activity data to RescueTime API with retry logic (legacy offline time API)
func submitToRescueTime(apiKey string, payload RescueTimePayload) error {
	var lastErr error

	// Check if API key is present
	if apiKey == "" {
		return fmt.Errorf("API key is empty - cannot submit to RescueTime")
	}
	
	debugLog("API key length: %d characters", len(apiKey))
	debugLog("API key first 5 chars: %s..., last 5 chars: ...%s", apiKey[:5], apiKey[len(apiKey)-5:])

	for attempt := 0; attempt < maxAPIRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			fmt.Printf("Retrying in %v... (attempt %d/%d)\n", delay, attempt+1, maxAPIRetries)
			time.Sleep(delay)
		}

		// Convert payload to JSON (disable HTML escaping)
		buffer := &bytes.Buffer{}
		encoder := json.NewEncoder(buffer)
		encoder.SetEscapeHTML(false)
		err := encoder.Encode(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %v", err)
		}
		
		// Remove trailing newline that Encode adds
		jsonData := bytes.TrimSpace(buffer.Bytes())

		debugLog("Submitting payload: %s", string(jsonData))

		// Create request
		url := fmt.Sprintf("https://www.rescuetime.com/anapi/offline_time_post?key=%s", apiKey)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %v", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "rescuetime-linux-mutter/1.0")
		req.Header.Set("Accept", "*/*")

		debugLog("Sending POST to: %s", "https://www.rescuetime.com/anapi/offline_time_post?key=***")
		debugLog("Request headers: Content-Type=%s, User-Agent=%s", req.Header.Get("Content-Type"), req.Header.Get("User-Agent"))
		debugLog("Request body: %s", string(jsonData))

		// Send request
		client := &http.Client{Timeout: apiTimeout}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %v", err)
			continue
		}

		// Read response body
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		debugLog("Response status: %d", resp.StatusCode)
		debugLog("Response headers: %v", resp.Header)
		debugLog("Response body: %s", string(body))

		// Check response status
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Printf("✓ Submitted to RescueTime: %s (%d min)\n", payload.ActivityName, payload.Duration)
			return nil
		}

		lastErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))

		// Don't retry on client errors (4xx)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
	}

	return fmt.Errorf("failed after %d attempts: %v", maxAPIRetries, lastErr)
}

// submitUserClientEvent submits activity data to native RescueTime user_client_events API
func submitUserClientEvent(apiKey string, payload UserClientEventPayload) error {
	var lastErr error
	var tryBearerAuth bool

	for attempt := 0; attempt < maxAPIRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			fmt.Printf("Retrying in %v... (attempt %d/%d)\n", delay, attempt+1, maxAPIRetries)
			time.Sleep(delay)
		}

		// Convert payload to JSON
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal payload: %v", err)
		}

		var req *http.Request

		// Try Bearer token auth if query param auth failed with 401
		if tryBearerAuth {
			// Create request WITHOUT query parameter
			url := "https://api.rescuetime.com/api/resource/user_client_events"
			req, err = http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				lastErr = fmt.Errorf("failed to create request: %v", err)
				continue
			}
			// Use Bearer token authentication with data_key
			// The desktop app uses the data_key as the Bearer token
			dataKey := os.Getenv("RESCUE_TIME_DATA_KEY")
			if dataKey == "" {
				dataKey = apiKey // Fallback to provided API key
			}
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", dataKey))

			// Also try adding account_key as a query parameter along with Bearer token
			accountKey := os.Getenv("RESCUE_TIME_ACCOUNT_KEY")
			if accountKey != "" {
				req.URL.RawQuery = fmt.Sprintf("key=%s", accountKey)
			}
		} else {
			// Try query parameter authentication first with account_key
			authKey := os.Getenv("RESCUE_TIME_ACCOUNT_KEY")
			if authKey == "" {
				authKey = apiKey
			}
			url := fmt.Sprintf("https://api.rescuetime.com/api/resource/user_client_events?key=%s", authKey)
			req, err = http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
			if err != nil {
				lastErr = fmt.Errorf("failed to create request: %v", err)
				continue
			}
		}

		// Set headers matching the official app
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("User-Agent", "RescueTime/2.16.5.1 (Linux)")

		// Send request
		client := &http.Client{Timeout: apiTimeout}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %v", err)
			continue
		}

		// Read response body
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Check response status
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			authMethod := "query parameter"
			if tryBearerAuth {
				authMethod = "Bearer token"
			}
			fmt.Printf("✓ Submitted to RescueTime via %s: %s (%s to %s)\n",
				authMethod,
				payload.UserClientEvent.Application,
				payload.UserClientEvent.StartTime,
				payload.UserClientEvent.EndTime)
			return nil
		}

		lastErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))

		// If we got 401 with query param auth, try Bearer token auth next
		if resp.StatusCode == 401 && !tryBearerAuth {
			fmt.Println("Query parameter auth failed (401), trying Bearer token authentication...")
			tryBearerAuth = true
			continue
		}

		// Don't retry on other client errors (4xx)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
	}

	return fmt.Errorf("failed after %d attempts: %v", maxAPIRetries, lastErr)
}

// submitActivitiesToRescueTime submits all activity summaries to RescueTime
// Attempts native user_client_events API first if credentials are available,
// falls back to offline_time_post API if native fails or credentials are missing.
func submitActivitiesToRescueTime(apiKey string, summaries map[string]ActivitySummary) {
	if len(summaries) == 0 {
		fmt.Println("No activities to submit.")
		return
	}

	// Check if we have native API credentials
	dataKey := os.Getenv("RESCUE_TIME_DATA_KEY")
	accountKey := os.Getenv("RESCUE_TIME_ACCOUNT_KEY")
	hasNativeCredentials := dataKey != "" || accountKey != ""

	fmt.Printf("\n=== Submitting %d activities to RescueTime ===\n", len(summaries))
	if hasNativeCredentials {
		fmt.Println("[INFO] Native API credentials detected, will try native API first with legacy fallback")
	} else {
		fmt.Println("[INFO] Using legacy offline time API (no native credentials found)")
	}

	successCount := 0
	failCount := 0
	nativeSuccessCount := 0
	legacyFallbackCount := 0

	for _, summary := range summaries {
		// RescueTime API appears to require minimum 5 minutes duration
		if summary.TotalDuration < 5*time.Minute {
			debugLog("Skipping %s: duration %v is less than 5 minutes", summary.AppClass, summary.TotalDuration)
			continue
		}

		var err error
		usedFallback := false

		if hasNativeCredentials {
			// Try native API first
			fmt.Printf("[ATTEMPT] Trying native API for %s...\n", summary.AppClass)
			payload := summaryToUserClientEvent(summary)
			err = submitUserClientEvent(apiKey, payload)

			if err != nil {
				// Native API failed, log and try legacy fallback
				fmt.Fprintf(os.Stderr, "[WARN] Native API failed for %s: %v\n", summary.AppClass, err)
				fmt.Printf("[FALLBACK] Attempting legacy API for %s...\n", summary.AppClass)

				legacyPayload := summaryToPayload(summary)
				
				// Print the payload we're about to send
				payloadJSON, _ := json.MarshalIndent(legacyPayload, "", "  ")
				fmt.Printf("[DEBUG] Legacy payload for %s:\n%s\n", summary.AppClass, string(payloadJSON))
				
				// Validate before submitting
				if validateErr := validatePayload(legacyPayload); validateErr != nil {
					err = fmt.Errorf("invalid payload: %v", validateErr)
				} else {
					err = submitToRescueTime(apiKey, legacyPayload)
					usedFallback = true
				}
			} else {
				nativeSuccessCount++
			}
		} else {
			// No native credentials, use legacy API directly
			payload := summaryToPayload(summary)
			
			// Print the payload we're about to send
			payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Printf("[DEBUG] Submitting payload for %s:\n%s\n", summary.AppClass, string(payloadJSON))
			
			// Validate before submitting
			if validateErr := validatePayload(payload); validateErr != nil {
				err = fmt.Errorf("invalid payload: %v", validateErr)
			} else {
				err = submitToRescueTime(apiKey, payload)
			}
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ Failed to submit %s: %v\n", summary.AppClass, err)
			failCount++
		} else {
			successCount++
			if usedFallback {
				legacyFallbackCount++
			}
		}
	}

	fmt.Printf("\n=== Submission Summary ===\n")
	fmt.Printf("Total succeeded: %d, failed: %d\n", successCount, failCount)
	if hasNativeCredentials {
		fmt.Printf("Native API successes: %d\n", nativeSuccessCount)
		fmt.Printf("Legacy fallback successes: %d\n", legacyFallbackCount)
	}
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
		return fmt.Sprintf("Active Window: %s (%s)", windowName, windowClass)
	}
	return fmt.Sprintf("Active Window: %s", windowName)
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

// validatePayload checks if a payload is valid before submission
func validatePayload(payload RescueTimePayload) error {
	if payload.ActivityName == "" {
		return fmt.Errorf("activity_name is required")
	}
	if payload.Duration <= 0 {
		return fmt.Errorf("duration must be positive, got %d", payload.Duration)
	}
	if payload.Duration > int(maxOfflineDuration.Minutes()) {
		return fmt.Errorf("duration exceeds RescueTime limit of %v hours: %d minutes", maxOfflineDuration.Hours(), payload.Duration)
	}
	if payload.StartTime == "" {
		return fmt.Errorf("start_time is required")
	}
	// Validate start_time format "YYYY-MM-DD HH:MM:SS"
	_, err := time.Parse("2006-01-02 15:04:05", payload.StartTime)
	if err != nil {
		return fmt.Errorf("invalid start_time format (expected YYYY-MM-DD HH:MM:SS): %s", payload.StartTime)
	}
	return nil
}

// previewSubmission shows what would be submitted in dry-run mode
func previewSubmission(summaries map[string]ActivitySummary) {
	if len(summaries) == 0 {
		fmt.Println("No activities to preview.")
		return
	}

	fmt.Printf("\n=== DRY-RUN: Would submit %d activities ===\n", len(summaries))
	
	for _, summary := range summaries {
		// Skip activities with very short duration (< 1 minute)
		if summary.TotalDuration < time.Minute {
			debugLog("Skipping %s (duration < 1 minute)", summary.AppClass)
			continue
		}

		payload := summaryToPayload(summary)
		
		// Validate payload before submission
		if err := validatePayload(payload); err != nil {
			errorLog("Invalid payload for %s: %v", summary.AppClass, err)
			continue
		}
		
		jsonData, _ := json.MarshalIndent(payload, "", "  ")
		
		fmt.Printf("\n[PREVIEW] Would submit:\n%s\n", string(jsonData))
	}
	
	fmt.Println("\n=== End of preview ===")
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
	fmt.Println("\n=== Activity Summary ===")

	summaries := tracker.GetActivitySummaries()
	if len(summaries) == 0 {
		fmt.Println("No activities tracked.")
		return
	}

	totalTime := time.Duration(0)
	for _, summary := range summaries {
		totalTime += summary.TotalDuration
	}

	fmt.Printf("Total tracking time: %v\n\n", totalTime.Round(time.Second))

	for appClass, summary := range summaries {
		percentage := float64(summary.TotalDuration) / float64(totalTime) * 100
		fmt.Printf("%s: %v (%.1f%%) - %d sessions\n",
			appClass,
			summary.TotalDuration.Round(time.Second),
			percentage,
			summary.SessionCount)
		fmt.Printf("  └─ %s\n\n", summary.ActivityDetails)
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

func monitorWindowChanges(interval time.Duration, submitToAPI bool, apiKey string, submissionInterval time.Duration, dryRun bool, saveToFile bool) {
	// Add panic recovery to prevent crashes
	defer func() {
		if r := recover(); r != nil {
			errorLog("PANIC recovered in monitorWindowChanges: %v", r)
			errorLog("Stack trace will be printed by the runtime")
		}
	}()

	var lastAppClass, lastWindowTitle string

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

	// Start the initial session
	tracker.StartSession(window.WmClass, window.Title)
	lastAppClass = window.WmClass
	lastWindowTitle = window.Title

	// Print initial window
	currentInfo := formatWindowOutput(window.Title, window.WmClass)
	fmt.Printf("%s [%s]\n", currentInfo, time.Now().Format("15:04:05"))
	verboseLog("Started tracking: %s", currentInfo)

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
			fmt.Println("\nShutting down window monitor...")
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
			monitorWindowChanges(*interval, *submit, apiKey, *submissionInterval, *dryRun, *saveToFile)
		} else {
			// Validate basic configuration even without API submission
			if err := validateConfiguration(false, false, "", *submissionInterval, *interval); err != nil {
				errorLog("Configuration validation failed: %v", err)
				os.Exit(1)
			}
			// Call without API submission
			monitorWindowChanges(*interval, false, "", 0, false, *saveToFile)
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
