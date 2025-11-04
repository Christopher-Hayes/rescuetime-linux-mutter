// Package webhook provides a Go client for sending RescueTime activity tracking data
// to a custom webhook endpoint. This allows users to integrate activity data with
// their own services, automation systems, or data pipelines.
//
// Example usage:
//
//	client, err := webhook.NewClient("https://example.com/webhook")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	summary := webhook.ActivitySummary{
//		AppClass:        "Firefox",
//		ActivityDetails: "GitHub - Projects",
//		TotalDuration:   15 * time.Minute,
//		SessionCount:    3,
//		FirstSeen:       time.Now().Add(-15 * time.Minute),
//		LastSeen:        time.Now(),
//	}
//
//	err = client.SubmitSummary(summary)
package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
	"github.com/fatih/color"
)

// Configuration constants
const (
	defaultRequestTimeout = 30 * time.Second
	maxRetries            = 3
	baseRetryDelay        = 1 * time.Second
)

// Type aliases to use RescueTime's types for consistency
type ActivitySummary = rescuetime.ActivitySummary

// ActivitySession represents a single continuous session with an application.
type ActivitySession struct {
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	AppClass    string        `json:"app_class"`
	WindowTitle string        `json:"window_title"`
	Duration    time.Duration `json:"duration"`
	Ignored     bool          `json:"ignored"` // true if app is in ignore list (excluded from RescueTime)
}

// WebhookPayload represents the JSON structure sent to the webhook endpoint.
// It includes metadata about the submission along with activity summaries and individual sessions.
type WebhookPayload struct {
	Timestamp  time.Time                  `json:"timestamp"`
	Source     string                     `json:"source"`
	Version    string                     `json:"version"`
	Summaries  []ActivitySummary          `json:"summaries"`
	Sessions   []ActivitySession          `json:"sessions,omitempty"`
	Metadata   map[string]interface{}     `json:"metadata,omitempty"`
}

// Client provides methods for sending activity data to a webhook endpoint.
type Client struct {
	webhookURL    string
	httpClient    *http.Client
	DebugMode     bool
	CustomHeaders map[string]string
}

// NewClient creates a new webhook client.
// The webhookURL should be a valid HTTP or HTTPS URL.
//
// If webhookURL is empty, it will attempt to read from WEBHOOK_URL
// environment variable.
func NewClient(webhookURL string) (*Client, error) {
	// Use provided URL, or fall back to environment variable
	if webhookURL == "" {
		webhookURL = os.Getenv("WEBHOOK_URL")
	}

	if webhookURL == "" {
		return nil, fmt.Errorf("webhook URL not provided\n\nSet via:\n  1. WEBHOOK_URL environment variable\n  2. -webhook flag\n\nExample: https://example.com/rescuetime/webhook")
	}

	// Validate URL format (basic validation)
	if len(webhookURL) < 8 || (webhookURL[:7] != "http://" && webhookURL[:8] != "https://") {
		return nil, fmt.Errorf("invalid webhook URL: must start with http:// or https://\n\nProvided: %s", webhookURL)
	}

	client := &Client{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
		DebugMode:     false,
		CustomHeaders: make(map[string]string),
	}

	return client, nil
}

// Close performs any necessary cleanup.
// For webhook client, this is a no-op but included for consistency with other modules.
func (c *Client) Close() error {
	return nil
}

// debugLog prints debug messages if debug mode is enabled
func (c *Client) debugLog(format string, args ...interface{}) {
	if c.DebugMode {
		color.Cyan("[WEBHOOK DEBUG] "+format, args...)
	}
}

// SubmitSummary sends a single activity summary to the webhook endpoint.
func (c *Client) SubmitSummary(summary ActivitySummary) error {
	if err := c.validateSummary(summary); err != nil {
		return fmt.Errorf("invalid summary: %v", err)
	}

	payload := WebhookPayload{
		Timestamp: time.Now(),
		Source:    "rescuetime-linux-mutter",
		Version:   "1.0.0",
		Summaries: []ActivitySummary{summary},
	}

	return c.sendPayload(payload)
}

// SubmitActivities sends multiple activity summaries to the webhook endpoint.
// This sends aggregated summaries matching what RescueTime receives.
func (c *Client) SubmitActivities(summaries map[string]ActivitySummary) {
	if len(summaries) == 0 {
		// No activities to submit - silence is fine, no need to spam logs
		return
	}

	color.New(color.FgCyan, color.Bold).Printf("\n=== Sending %d activities to webhook ===\n", len(summaries))

	// Convert map to slice for JSON payload
	summaryList := make([]ActivitySummary, 0, len(summaries))
	for _, summary := range summaries {
		if err := c.validateSummary(summary); err != nil {
			color.Red("[WEBHOOK] ✗ Skipping invalid summary for %s: %v\n", summary.AppClass, err)
			continue
		}
		summaryList = append(summaryList, summary)
	}

	if len(summaryList) == 0 {
		color.Red("[WEBHOOK] No valid activities to submit after validation.")
		return
	}

	payload := WebhookPayload{
		Timestamp: time.Now(),
		Source:    "rescuetime-linux-mutter",
		Version:   "1.0.0",
		Summaries: summaryList,
		Metadata: map[string]interface{}{
			"count":     len(summaryList),
			"submitted": time.Now().Format(time.RFC3339),
		},
	}

	if err := c.sendPayload(payload); err != nil {
		color.Red("[WEBHOOK] ✗ Failed to send activities: %v\n", err)
		return
	}

	color.New(color.FgGreen, color.Bold).Printf("[SUCCESS] Sent %d activities to webhook\n", len(summaryList))
}

// SubmitActivitiesWithSessions sends both activity summaries and individual sessions to the webhook endpoint.
// This provides the same granular data that gets sent to RescueTime's API, allowing users to build
// their own applications with complete tracking information.
func (c *Client) SubmitActivitiesWithSessions(summaries map[string]ActivitySummary, sessions []ActivitySession) {
	if len(summaries) == 0 && len(sessions) == 0 {
		// No activities to submit - silence is fine, no need to spam logs
		return
	}

	color.New(color.FgCyan, color.Bold).Printf("\n=== Sending %d activities and %d sessions to webhook ===\n", len(summaries), len(sessions))

	// Convert summaries map to slice for JSON payload
	summaryList := make([]ActivitySummary, 0, len(summaries))
	for _, summary := range summaries {
		if err := c.validateSummary(summary); err != nil {
			color.Red("[WEBHOOK] ✗ Skipping invalid summary for %s: %v\n", summary.AppClass, err)
			continue
		}
		summaryList = append(summaryList, summary)
	}

	// Validate sessions
	validSessions := make([]ActivitySession, 0, len(sessions))
	for _, session := range sessions {
		if err := c.validateSession(session); err != nil {
			color.Red("[WEBHOOK] ✗ Skipping invalid session for %s: %v\n", session.AppClass, err)
			continue
		}
		validSessions = append(validSessions, session)
	}

	if len(summaryList) == 0 && len(validSessions) == 0 {
		color.Red("[WEBHOOK] No valid activities to submit after validation.")
		return
	}

	payload := WebhookPayload{
		Timestamp: time.Now(),
		Source:    "rescuetime-linux-mutter",
		Version:   "1.0.0",
		Summaries: summaryList,
		Sessions:  validSessions,
		Metadata: map[string]interface{}{
			"summary_count": len(summaryList),
			"session_count": len(validSessions),
			"submitted":     time.Now().Format(time.RFC3339),
		},
	}

	if err := c.sendPayload(payload); err != nil {
		color.Red("[WEBHOOK] ✗ Failed to send activities: %v\n", err)
		return
	}

	color.New(color.FgGreen, color.Bold).Printf("[SUCCESS] Sent %d summaries and %d sessions to webhook\n", len(summaryList), len(validSessions))
}

// sendPayload sends the webhook payload with retry logic.
func (c *Client) sendPayload(payload WebhookPayload) error {
	// Marshal payload to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}

	c.debugLog("Payload: %s", string(jsonData))

	var lastErr error
	retryDelay := baseRetryDelay

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			c.debugLog("Retry attempt %d/%d after %v", attempt, maxRetries, retryDelay)
			time.Sleep(retryDelay)
			retryDelay *= 2 // Exponential backoff
		}

		// Create request
		req, err := http.NewRequest("POST", c.webhookURL, bytes.NewBuffer(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %v", err)
			continue
		}

		// Set headers
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "rescuetime-linux-mutter/1.0.0")

		// Add custom headers if configured
		for key, value := range c.CustomHeaders {
			req.Header.Set(key, value)
		}

		c.debugLog("Sending POST request to %s", c.webhookURL)

		// Send request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %v", err)
			continue
		}

		// Read response body
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		c.debugLog("Response status: %d, body: %s", resp.StatusCode, string(body))

		// Check response status
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			c.debugLog("Successfully sent payload")
			return nil
		}

		// Handle different error codes
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			// Client errors - don't retry
			return fmt.Errorf("webhook endpoint returned error %d: %s\n\nTroubleshooting:\n  1. Verify webhook URL is correct\n  2. Check authentication headers if required\n  3. Verify endpoint accepts JSON payloads", resp.StatusCode, string(body))
		}

		// Server errors - retry
		lastErr = fmt.Errorf("webhook endpoint returned error %d: %s", resp.StatusCode, string(body))
	}

	return fmt.Errorf("failed after %d attempts: %v\n\nTroubleshooting:\n  1. Check network connectivity\n  2. Verify webhook endpoint is accessible\n  3. Check endpoint logs for errors", maxRetries, lastErr)
}

// validateSummary checks if a summary is valid before submission.
func (c *Client) validateSummary(summary ActivitySummary) error {
	if summary.AppClass == "" {
		return fmt.Errorf("app_class is required")
	}
	if summary.TotalDuration <= 0 {
		return fmt.Errorf("total_duration must be positive")
	}
	if summary.SessionCount <= 0 {
		return fmt.Errorf("session_count must be positive")
	}
	if summary.FirstSeen.IsZero() {
		return fmt.Errorf("first_seen is required")
	}
	if summary.LastSeen.IsZero() {
		return fmt.Errorf("last_seen is required")
	}
	if summary.LastSeen.Before(summary.FirstSeen) {
		return fmt.Errorf("last_seen must be after or equal to first_seen")
	}
	return nil
}

// validateSession checks if a session is valid before submission.
func (c *Client) validateSession(session ActivitySession) error {
	if session.AppClass == "" {
		return fmt.Errorf("app_class is required")
	}
	if session.StartTime.IsZero() {
		return fmt.Errorf("start_time is required")
	}
	if session.EndTime.IsZero() {
		return fmt.Errorf("end_time is required")
	}
	if session.EndTime.Before(session.StartTime) {
		return fmt.Errorf("end_time must be after start_time")
	}
	if session.Duration < 0 {
		return fmt.Errorf("duration must be non-negative")
	}
	return nil
}

// SetHeader sets a custom HTTP header to be included in all webhook requests.
// This is useful for authentication tokens or API keys.
func (c *Client) SetHeader(key, value string) {
	c.CustomHeaders[key] = value
}

// SetTimeout sets the HTTP request timeout.
func (c *Client) SetTimeout(timeout time.Duration) {
	c.httpClient.Timeout = timeout
}
