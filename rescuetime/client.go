// Package rescuetime provides a Go client for interacting with RescueTime's API.
// It supports both the legacy offline_time_post API and the native user_client_events API.
//
// Example usage:
//
//	client := rescuetime.NewClient("your-api-key", "", "")
//	client.DebugMode = true
//
//	summary := rescuetime.ActivitySummary{
//		AppClass:        "Firefox",
//		ActivityDetails: "GitHub - Projects",
//		TotalDuration:   15 * time.Minute,
//		FirstSeen:       time.Now().Add(-15 * time.Minute),
//		LastSeen:        time.Now(),
//	}
//
//	payload := rescuetime.SummaryToPayload(summary)
//	err := client.SubmitLegacy(payload)
package rescuetime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
)

// API configuration constants
const (
	maxAPIRetries      = 3
	baseRetryDelay     = 1 * time.Second
	apiTimeout         = 10 * time.Second
	maxOfflineDuration = 4 * time.Hour // RescueTime API limit for offline time
)

// ActivitySummary represents aggregated time spent in an application.
// This is the primary data structure used to submit activity data to RescueTime.
type ActivitySummary struct {
	AppClass        string        `json:"app_class"`
	ActivityDetails string        `json:"activity_details"`
	TotalDuration   time.Duration `json:"total_duration"`
	SessionCount    int           `json:"session_count"`
	FirstSeen       time.Time     `json:"first_seen"`
	LastSeen        time.Time     `json:"last_seen"`
}

// RescueTimePayload represents the data structure for RescueTime's legacy offline time API.
type RescueTimePayload struct {
	StartTime       string `json:"start_time"`       // YYYY-MM-DD HH:MM:SS format
	Duration        int    `json:"duration"`         // duration in minutes
	ActivityName    string `json:"activity_name"`    // application class
	ActivityDetails string `json:"activity_details"` // window title/details
}

// UserClientEventPayload represents the native RescueTime user_client_events API format.
type UserClientEventPayload struct {
	UserClientEvent UserClientEvent `json:"user_client_event"`
}

// UserClientEvent represents a single activity tracking event.
type UserClientEvent struct {
	EventDescription string `json:"event_description"` // application class
	StartTime        string `json:"start_time"`        // RFC 3339 format: 2025-09-30T12:00:00Z
	EndTime          string `json:"end_time"`          // RFC 3339 format: 2025-09-30T12:01:00Z
	WindowTitle      string `json:"window_title"`      // window title
	Application      string `json:"application"`       // application class (redundant with event_description)
}

// ActivationRequest represents the payload for the /activate endpoint.
type ActivationRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// ActivationResponse represents the response from the /activate endpoint.
type ActivationResponse struct {
	AccountKey string `json:"account_key"`
	DataKey    string `json:"data_key"`
	ApiURL     string `json:"api_url"`
	URL        string `json:"url"`
}

// Client provides methods for interacting with RescueTime's API.
type Client struct {
	APIKey     string // Legacy API key for offline_time_post
	AccountKey string // Native API account key
	DataKey    string // Native API data key (Bearer token)
	DebugMode  bool   // Enable debug logging
}

// NewClient creates a new RescueTime API client.
// API keys can be provided directly or will be read from environment variables:
// - RESCUE_TIME_API_KEY (legacy API)
// - RESCUE_TIME_ACCOUNT_KEY (native API)
// - RESCUE_TIME_DATA_KEY (native API)
func NewClient(apiKey, accountKey, dataKey string) *Client {
	// Use provided keys, or fall back to environment variables
	if apiKey == "" {
		apiKey = os.Getenv("RESCUE_TIME_API_KEY")
	}
	if accountKey == "" {
		accountKey = os.Getenv("RESCUE_TIME_ACCOUNT_KEY")
	}
	if dataKey == "" {
		dataKey = os.Getenv("RESCUE_TIME_DATA_KEY")
	}

	return &Client{
		APIKey:     apiKey,
		AccountKey: accountKey,
		DataKey:    dataKey,
		DebugMode:  false,
	}
}

// debugLog prints debug messages if debug mode is enabled
func (c *Client) debugLog(format string, args ...interface{}) {
	if c.DebugMode {
		color.Cyan("[DEBUG] "+format, args...)
	}
}

// SummaryToPayload converts an ActivitySummary to RescueTimePayload format (legacy API).
func SummaryToPayload(summary ActivitySummary) RescueTimePayload {
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

// SummaryToUserClientEvent converts an ActivitySummary to UserClientEventPayload format (native API).
func SummaryToUserClientEvent(summary ActivitySummary) UserClientEventPayload {
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

// ValidatePayload checks if a RescueTimePayload is valid before submission.
func ValidatePayload(payload RescueTimePayload) error {
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

// SubmitLegacy submits activity data to RescueTime's legacy offline_time_post API with retry logic.
func (c *Client) SubmitLegacy(payload RescueTimePayload) error {
	var lastErr error

	// Check if API key is present
	if c.APIKey == "" {
		return fmt.Errorf("API key is empty - cannot submit to RescueTime")
	}

	c.debugLog("API key length: %d characters", len(c.APIKey))
	if len(c.APIKey) >= 10 {
		c.debugLog("API key first 5 chars: %s..., last 5 chars: ...%s", c.APIKey[:5], c.APIKey[len(c.APIKey)-5:])
	}

	for attempt := 0; attempt < maxAPIRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			color.Yellow("Retrying in %v... (attempt %d/%d)", delay, attempt+1, maxAPIRetries)
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

		c.debugLog("Submitting payload: %s", string(jsonData))

		// Create request
		url := fmt.Sprintf("https://www.rescuetime.com/anapi/offline_time_post?key=%s", c.APIKey)
		req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %v", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "rescuetime-linux-mutter/1.0")
		req.Header.Set("Accept", "*/*")

		c.debugLog("Sending POST to: %s", "https://www.rescuetime.com/anapi/offline_time_post?key=***")
		c.debugLog("Request headers: Content-Type=%s, User-Agent=%s", req.Header.Get("Content-Type"), req.Header.Get("User-Agent"))
		c.debugLog("Request body: %s", string(jsonData))

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

		c.debugLog("Response status: %d", resp.StatusCode)
		c.debugLog("Response headers: %v", resp.Header)
		c.debugLog("Response body: %s", string(body))

		// Check response status
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			color.New(color.FgGreen, color.Bold).Printf("[SUCCESS] Submitted to RescueTime: %s (%d min)\n", payload.ActivityName, payload.Duration)
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

// SubmitNative submits activity data to RescueTime's native user_client_events API.
func (c *Client) SubmitNative(payload UserClientEventPayload) error {
	var lastErr error
	var tryBearerAuth bool

	for attempt := 0; attempt < maxAPIRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := baseRetryDelay * time.Duration(math.Pow(2, float64(attempt-1)))
			color.Yellow("Retrying in %v... (attempt %d/%d)", delay, attempt+1, maxAPIRetries)
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
			dataKey := c.DataKey
			if dataKey == "" {
				dataKey = c.APIKey // Fallback to provided API key
			}
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", dataKey))

			// Also try adding account_key as a query parameter along with Bearer token
			if c.AccountKey != "" {
				req.URL.RawQuery = fmt.Sprintf("key=%s", c.AccountKey)
			}
		} else {
			// Try query parameter authentication first with account_key
			authKey := c.AccountKey
			if authKey == "" {
				authKey = c.APIKey
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
			color.New(color.FgGreen, color.Bold).Printf("[SUCCESS] Submitted to RescueTime via %s: %s (%s to %s)\n",
				authMethod,
				payload.UserClientEvent.Application,
				payload.UserClientEvent.StartTime,
				payload.UserClientEvent.EndTime)
			return nil
		}

		lastErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))

		// If we got 401 with query param auth, try Bearer token auth next
		if resp.StatusCode == 401 && !tryBearerAuth {
			color.Yellow("[WARNING] Query parameter auth failed (401), trying Bearer token authentication...")
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

// SubmitActivities submits all activity summaries to RescueTime.
// Attempts native user_client_events API first if credentials are available,
// falls back to offline_time_post API if native fails or credentials are missing.
func (c *Client) SubmitActivities(summaries map[string]ActivitySummary) {
	if len(summaries) == 0 {
		color.Yellow("No activities to submit.")
		return
	}

	// Check if we have native API credentials
	hasNativeCredentials := c.DataKey != "" || c.AccountKey != ""

	color.New(color.FgCyan, color.Bold).Printf("\n=== Submitting %d activities to RescueTime ===\n", len(summaries))
	if hasNativeCredentials {
		color.Cyan("[INFO] Native API credentials detected, will try native API first with legacy fallback\n")
	} else {
		color.Cyan("[INFO] Using legacy offline time API (no native credentials found)\n")
	}

	successCount := 0
	failCount := 0
	nativeSuccessCount := 0
	legacyFallbackCount := 0

	for _, summary := range summaries {
		// RescueTime API appears to require minimum 5 minutes duration
		if summary.TotalDuration < 5*time.Minute {
			c.debugLog("Skipping %s: duration %v is less than 5 minutes", summary.AppClass, summary.TotalDuration)
			continue
		}

		var err error
		usedFallback := false

		if hasNativeCredentials {
			// Try native API first
			color.Cyan("[ATTEMPT] Trying native API for %s...\n", summary.AppClass)
			payload := SummaryToUserClientEvent(summary)
			err = c.SubmitNative(payload)

			if err != nil {
				// Native API failed, log and try legacy fallback
				color.Yellow("[WARNING] Native API failed for %s: %v\n", summary.AppClass, err)
				color.Yellow("[FALLBACK] Attempting legacy API for %s...\n", summary.AppClass)

				legacyPayload := SummaryToPayload(summary)

				// Print the payload we're about to send
				if c.DebugMode {
					payloadJSON, _ := json.MarshalIndent(legacyPayload, "", "  ")
					c.debugLog("Legacy payload for %s:\n%s", summary.AppClass, string(payloadJSON))
				}

				// Validate before submitting
				if validateErr := ValidatePayload(legacyPayload); validateErr != nil {
					err = fmt.Errorf("invalid payload: %v", validateErr)
				} else {
					err = c.SubmitLegacy(legacyPayload)
					usedFallback = true
				}
			} else {
				nativeSuccessCount++
			}
		} else {
			// No native credentials, use legacy API directly
			payload := SummaryToPayload(summary)

			// Print the payload we're about to send
			if c.DebugMode {
				payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
				c.debugLog("Submitting payload for %s:\n%s", summary.AppClass, string(payloadJSON))
			}

			// Validate before submitting
			if validateErr := ValidatePayload(payload); validateErr != nil {
				err = fmt.Errorf("invalid payload: %v", validateErr)
			} else {
				err = c.SubmitLegacy(payload)
			}
		}

		if err != nil {
			color.Red("✗ Failed to submit %s: %v\n", summary.AppClass, err)
			failCount++
		} else {
			successCount++
			if usedFallback {
				legacyFallbackCount++
			}
		}
	}

	color.New(color.FgCyan, color.Bold).Printf("\n=== Submission Summary ===\n")
	if successCount > 0 {
		color.Green("Total succeeded: %d\n", successCount)
	}
	if failCount > 0 {
		color.Red("Total failed: %d\n", failCount)
	}
	if successCount == 0 && failCount == 0 {
		fmt.Println("No activities submitted.")
	}
	if hasNativeCredentials {
		if nativeSuccessCount > 0 {
			color.Cyan("Native API successes: %d\n", nativeSuccessCount)
		}
		if legacyFallbackCount > 0 {
			color.Yellow("Legacy fallback successes: %d\n", legacyFallbackCount)
		}
	}
}

// Activate authenticates with RescueTime and retrieves account keys.
// Note: This currently only retrieves the account_key. The data_key retrieval
// mechanism is not yet fully reverse-engineered.
func Activate(email, password string) (*ActivationResponse, error) {
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
