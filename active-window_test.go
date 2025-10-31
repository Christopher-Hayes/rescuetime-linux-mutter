package main

import (
	"testing"
	"time"
)

// Test constants
func TestConstants(t *testing.T) {
	if defaultMergeThreshold != 30*time.Second {
		t.Errorf("defaultMergeThreshold should be 30s, got %v", defaultMergeThreshold)
	}
	if defaultMinDuration != 10*time.Second {
		t.Errorf("defaultMinDuration should be 10s, got %v", defaultMinDuration)
	}
	if defaultPollInterval != 1000*time.Millisecond {
		t.Errorf("defaultPollInterval should be 1000ms, got %v", defaultPollInterval)
	}
	if maxAPIRetries != 3 {
		t.Errorf("maxAPIRetries should be 3, got %d", maxAPIRetries)
	}
}

// Test validatePayload
func TestValidatePayload(t *testing.T) {
	tests := []struct {
		name    string
		payload RescueTimePayload
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid payload",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				Duration:        30,
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: false,
		},
		{
			name: "missing activity name",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				Duration:        30,
				ActivityName:    "",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "activity_name is required",
		},
		{
			name: "zero duration",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				Duration:        0,
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "duration must be positive",
		},
		{
			name: "negative duration",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				Duration:        -5,
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "duration must be positive",
		},
		{
			name: "duration exceeds limit",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				Duration:        300, // 5 hours > 4 hour limit
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "duration exceeds RescueTime limit",
		},
		{
			name: "missing start time",
			payload: RescueTimePayload{
				StartTime:       "",
				Duration:        30,
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "start_time is required",
		},
		{
			name: "invalid start time format",
			payload: RescueTimePayload{
				StartTime:       "2025/10/29 10:00:00", // wrong format
				Duration:        30,
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "invalid start_time format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePayload(tt.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePayload() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || err.Error() == "" {
					t.Errorf("Expected error containing %q but got nil", tt.errMsg)
				}
			}
		})
	}
}

// Test validateConfiguration
func TestValidateConfiguration(t *testing.T) {
	tests := []struct {
		name               string
		submitToAPI        bool
		dryRun             bool
		apiKey             string
		submissionInterval time.Duration
		pollInterval       time.Duration
		wantErr            bool
		errMsg             string
	}{
		{
			name:               "valid config with API submission",
			submitToAPI:        true,
			dryRun:             false,
			apiKey:             "valid_api_key_12345678901234567890",
			submissionInterval: 15 * time.Minute,
			pollInterval:       200 * time.Millisecond,
			wantErr:            false,
		},
		{
			name:               "valid config without API submission",
			submitToAPI:        false,
			dryRun:             false,
			apiKey:             "",
			submissionInterval: 15 * time.Minute,
			pollInterval:       200 * time.Millisecond,
			wantErr:            false,
		},
		{
			name:               "dry-run mode without API key",
			submitToAPI:        false,
			dryRun:             true,
			apiKey:             "",
			submissionInterval: 15 * time.Minute,
			pollInterval:       200 * time.Millisecond,
			wantErr:            false,
		},
		{
			name:               "missing API key",
			submitToAPI:        true,
			dryRun:             false,
			apiKey:             "",
			submissionInterval: 15 * time.Minute,
			pollInterval:       200 * time.Millisecond,
			wantErr:            true,
			errMsg:             "RESCUE_TIME_API_KEY not found",
		},
		{
			name:               "API key too short",
			submitToAPI:        true,
			dryRun:             false,
			apiKey:             "short",
			submissionInterval: 15 * time.Minute,
			pollInterval:       200 * time.Millisecond,
			wantErr:            true,
			errMsg:             "appears invalid",
		},
		{
			name:               "submission interval too short",
			submitToAPI:        true,
			dryRun:             false,
			apiKey:             "valid_api_key_12345678901234567890",
			submissionInterval: 30 * time.Second,
			pollInterval:       200 * time.Millisecond,
			wantErr:            true,
			errMsg:             "submission interval must be at least 1 minute",
		},
		{
			name:               "poll interval too short",
			submitToAPI:        false,
			dryRun:             false,
			apiKey:             "",
			submissionInterval: 15 * time.Minute,
			pollInterval:       10 * time.Millisecond,
			wantErr:            true,
			errMsg:             "poll interval too short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfiguration(tt.submitToAPI, tt.dryRun, tt.apiKey, tt.submissionInterval, tt.pollInterval)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateConfiguration() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if err == nil || err.Error() == "" {
					t.Errorf("Expected error containing %q but got nil", tt.errMsg)
				}
			}
		})
	}
}

// Test summaryToPayload conversion
func TestSummaryToPayload(t *testing.T) {
	summary := ActivitySummary{
		AppClass:        "firefox",
		ActivityDetails: "GitHub - Repository",
		TotalDuration:   90 * time.Second, // 1.5 minutes
		SessionCount:    2,
		FirstSeen:       time.Date(2025, 10, 29, 10, 0, 0, 0, time.UTC),
		LastSeen:        time.Date(2025, 10, 29, 10, 1, 30, 0, time.UTC),
	}

	payload := summaryToPayload(summary)

	if payload.ActivityName != "firefox" {
		t.Errorf("Expected activity_name 'firefox', got '%s'", payload.ActivityName)
	}
	if payload.ActivityDetails != "GitHub - Repository" {
		t.Errorf("Expected activity_details 'GitHub - Repository', got '%s'", payload.ActivityDetails)
	}
	if payload.Duration != 2 {
		t.Errorf("Expected duration 2 minutes (rounded up from 1.5), got %d", payload.Duration)
	}
	if payload.StartTime != "2025-10-29 10:00:00" {
		t.Errorf("Expected start_time '2025-10-29 10:00:00', got '%s'", payload.StartTime)
	}
}

// Test ActivityTracker session management
func TestActivityTrackerBasics(t *testing.T) {
	tracker := NewActivityTracker()

	if tracker == nil {
		t.Fatal("NewActivityTracker() returned nil")
	}
	if tracker.mergeThreshold != defaultMergeThreshold {
		t.Errorf("Expected mergeThreshold %v, got %v", defaultMergeThreshold, tracker.mergeThreshold)
	}
	if tracker.minDuration != defaultMinDuration {
		t.Errorf("Expected minDuration %v, got %v", defaultMinDuration, tracker.minDuration)
	}
	if len(tracker.sessions) != 0 {
		t.Errorf("Expected 0 initial sessions, got %d", len(tracker.sessions))
	}
}

// Test formatWindowOutput
func TestFormatWindowOutput(t *testing.T) {
	tests := []struct {
		name        string
		windowName  string
		windowClass string
		expected    string
	}{
		{
			name:        "with window class",
			windowName:  "README.md - VSCode",
			windowClass: "code",
			expected:    "Active Window: README.md - VSCode (code)",
		},
		{
			name:        "without window class",
			windowName:  "Firefox",
			windowClass: "",
			expected:    "Active Window: Firefox",
		},
		{
			name:        "empty window name",
			windowName:  "",
			windowClass: "gnome-shell",
			expected:    "Active Window:  (gnome-shell)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatWindowOutput(tt.windowName, tt.windowClass)
			if result != tt.expected {
				t.Errorf("formatWindowOutput() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Test ignore functionality
func TestIsAppIgnored(t *testing.T) {
	tracker := NewActivityTracker()

	// Initially should not ignore anything
	if tracker.isAppIgnored("firefox") {
		t.Error("Expected firefox to not be ignored initially")
	}

	// Add firefox to ignore list
	tracker.ignoredApps["firefox"] = true

	// Now should be ignored
	if !tracker.isAppIgnored("firefox") {
		t.Error("Expected firefox to be ignored after adding to list")
	}

	// Other apps should not be ignored
	if tracker.isAppIgnored("chrome") {
		t.Error("Expected chrome to not be ignored")
	}
}

func TestStartSessionWithIgnoredApp(t *testing.T) {
	tracker := NewActivityTracker()

	// Add Code to ignore list
	tracker.ignoredApps["Code"] = true

	// Start session with ignored app
	tracker.StartSession("Code", "Visual Studio Code")

	// Should not create a session
	if tracker.currentSession != nil {
		t.Error("Expected no session to be created for ignored app")
	}

	// Try with non-ignored app
	tracker.StartSession("firefox", "Mozilla Firefox")

	// Should create a session
	if tracker.currentSession == nil {
		t.Error("Expected session to be created for non-ignored app")
	}
	if tracker.currentSession.AppClass != "firefox" {
		t.Errorf("Expected session AppClass 'firefox', got '%s'", tracker.currentSession.AppClass)
	}
}

func TestIgnoredAppsNotInSummary(t *testing.T) {
	tracker := NewActivityTracker()
	
	// Set a lower minimum duration for testing
	tracker.minDuration = 50 * time.Millisecond

	// Ignore Code
	tracker.ignoredApps["Code"] = true

	// Start and end session with ignored app
	tracker.StartSession("Code", "Visual Studio Code")
	time.Sleep(100 * time.Millisecond)
	tracker.EndCurrentSession()

	// Start and end session with non-ignored app
	tracker.StartSession("firefox", "Mozilla Firefox")
	time.Sleep(100 * time.Millisecond)
	tracker.EndCurrentSession()

	// Get summaries
	summaries := tracker.GetActivitySummaries()

	// Should only have firefox, not Code
	if len(summaries) != 1 {
		t.Errorf("Expected 1 summary, got %d", len(summaries))
	}
	if _, exists := summaries["firefox"]; !exists {
		t.Error("Expected firefox in summaries")
	}
	if _, exists := summaries["Code"]; exists {
		t.Error("Did not expect Code in summaries (it should be ignored)")
	}
}
