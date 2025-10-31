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
			name: "valid payload with duration",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				Duration:        30,
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: false,
		},
		{
			name: "valid payload with end_time",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				EndTime:         "2025-10-29 10:30:00",
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
			name: "missing both duration and end_time",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				Duration:        0,
				EndTime:         "",
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "either duration or end_time must be provided",
		},
		{
			name: "both duration and end_time provided",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				Duration:        30,
				EndTime:         "2025-10-29 10:30:00",
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "cannot provide both duration and end_time",
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
			errMsg:  "duration exceeds RescueTime API limit",
		},
		{
			name: "end_time before start_time",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				EndTime:         "2025-10-29 09:00:00",
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "end_time must be after start_time",
		},
		{
			name: "end_time equal to start_time",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				EndTime:         "2025-10-29 10:00:00",
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "end_time must be after start_time",
		},
		{
			name: "time span exceeds 4 hour limit",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				EndTime:         "2025-10-29 15:00:00", // 5 hours
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "exceeds RescueTime API limit",
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
			name: "invalid start_time format",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29T10:00:00Z", // ISO format instead of required format
				Duration:        30,
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "invalid start_time format",
		},
		{
			name: "invalid end_time format",
			payload: RescueTimePayload{
				StartTime:       "2025-10-29 10:00:00",
				EndTime:         "2025-10-29T10:30:00Z", // ISO format instead of required format
				ActivityName:    "firefox",
				ActivityDetails: "GitHub",
			},
			wantErr: true,
			errMsg:  "invalid end_time format",
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
	if payload.EndTime != "" {
		t.Errorf("EndTime = %s, want empty (duration should be used)", payload.EndTime)
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

// TestSummaryToPayloadWithEndTime tests the end_time conversion
func TestSummaryToPayloadWithEndTime(t *testing.T) {
	testTime := time.Date(2025, 10, 31, 10, 0, 0, 0, time.UTC)
	
	summary := ActivitySummary{
		AppClass:        "firefox",
		ActivityDetails: "GitHub - Projects",
		TotalDuration:   15 * time.Minute,
		SessionCount:    3,
		FirstSeen:       testTime,
		LastSeen:        testTime.Add(15 * time.Minute),
	}

	payload := SummaryToPayloadWithEndTime(summary)

	if payload.ActivityName != "firefox" {
		t.Errorf("ActivityName = %s, want firefox", payload.ActivityName)
	}
	if payload.Duration != 0 {
		t.Errorf("Duration = %d, want 0 (end_time should be used)", payload.Duration)
	}
	if payload.ActivityDetails != "GitHub - Projects" {
		t.Errorf("ActivityDetails = %s, want 'GitHub - Projects'", payload.ActivityDetails)
	}
	// StartTime and EndTime should be in format "YYYY-MM-DD HH:MM:SS"
	expectedStart := "2025-10-31 10:00:00"
	expectedEnd := "2025-10-31 10:15:00"
	if payload.StartTime != expectedStart {
		t.Errorf("StartTime = %s, want %s", payload.StartTime, expectedStart)
	}
	if payload.EndTime != expectedEnd {
		t.Errorf("EndTime = %s, want %s", payload.EndTime, expectedEnd)
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
