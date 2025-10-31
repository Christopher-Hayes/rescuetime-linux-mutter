package rescuetime

import (
	"testing"
	"time"
)

// TestValidatePayload tests the ValidatePayload function
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePayload(tt.payload)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidatePayload() expected error containing %q, got nil", tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidatePayload() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestSummaryToPayload tests the SummaryToPayload conversion
func TestSummaryToPayload(t *testing.T) {
	testTime := time.Date(2025, 10, 31, 10, 0, 0, 0, time.UTC)
	
	summary := ActivitySummary{
		AppClass:        "firefox",
		ActivityDetails: "GitHub - Projects",
		TotalDuration:   15 * time.Minute,
		SessionCount:    3,
		FirstSeen:       testTime,
		LastSeen:        testTime.Add(15 * time.Minute),
	}

	payload := SummaryToPayload(summary)

	if payload.ActivityName != "firefox" {
		t.Errorf("ActivityName = %s, want firefox", payload.ActivityName)
	}
	if payload.Duration != 15 {
		t.Errorf("Duration = %d, want 15", payload.Duration)
	}
	if payload.ActivityDetails != "GitHub - Projects" {
		t.Errorf("ActivityDetails = %s, want 'GitHub - Projects'", payload.ActivityDetails)
	}
	// StartTime should be in format "YYYY-MM-DD HH:MM:SS"
	expectedStart := "2025-10-31 10:00:00"
	if payload.StartTime != expectedStart {
		t.Errorf("StartTime = %s, want %s", payload.StartTime, expectedStart)
	}
}

// TestSummaryToUserClientEvent tests the native API conversion
func TestSummaryToUserClientEvent(t *testing.T) {
	testTime := time.Date(2025, 10, 31, 10, 0, 0, 0, time.UTC)
	
	summary := ActivitySummary{
		AppClass:        "code",
		ActivityDetails: "main.go",
		TotalDuration:   30 * time.Minute,
		SessionCount:    2,
		FirstSeen:       testTime,
		LastSeen:        testTime.Add(30 * time.Minute),
	}

	event := SummaryToUserClientEvent(summary)

	if event.UserClientEvent.Application != "code" {
		t.Errorf("Application = %s, want code", event.UserClientEvent.Application)
	}
	if event.UserClientEvent.WindowTitle != "main.go" {
		t.Errorf("WindowTitle = %s, want main.go", event.UserClientEvent.WindowTitle)
	}
	if event.UserClientEvent.EventDescription != "code" {
		t.Errorf("EventDescription = %s, want code", event.UserClientEvent.EventDescription)
	}
}

// TestNewClient tests client creation
func TestNewClient(t *testing.T) {
	apiKey := "test-api-key"
	accountKey := "test-account-key"
	dataKey := "test-data-key"

	client := NewClient(apiKey, accountKey, dataKey)

	if client.APIKey != apiKey {
		t.Errorf("APIKey = %s, want %s", client.APIKey, apiKey)
	}
	if client.AccountKey != accountKey {
		t.Errorf("AccountKey = %s, want %s", client.AccountKey, accountKey)
	}
	if client.DataKey != dataKey {
		t.Errorf("DataKey = %s, want %s", client.DataKey, dataKey)
	}
	if client.DebugMode != false {
		t.Errorf("DebugMode = %v, want false", client.DebugMode)
	}
}
