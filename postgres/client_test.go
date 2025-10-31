package postgres

import (
	"testing"
	"time"

	"github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
)

// TestValidateSession tests session validation logic
func TestValidateSession(t *testing.T) {
	client := &Client{DebugMode: false}

	tests := []struct {
		name    string
		session ActivitySession
		wantErr bool
	}{
		{
			name: "valid session",
			session: ActivitySession{
				StartTime:   time.Now().Add(-10 * time.Minute),
				EndTime:     time.Now(),
				AppClass:    "Firefox",
				WindowTitle: "GitHub",
				Duration:    10 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "missing app class",
			session: ActivitySession{
				StartTime: time.Now().Add(-10 * time.Minute),
				EndTime:   time.Now(),
				Duration:  10 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "zero start time",
			session: ActivitySession{
				EndTime:  time.Now(),
				AppClass: "Firefox",
				Duration: 10 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "zero end time",
			session: ActivitySession{
				StartTime: time.Now(),
				AppClass:  "Firefox",
				Duration:  10 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "end before start",
			session: ActivitySession{
				StartTime: time.Now(),
				EndTime:   time.Now().Add(-10 * time.Minute),
				AppClass:  "Firefox",
				Duration:  10 * time.Minute,
			},
			wantErr: true,
		},
		{
			name: "negative duration",
			session: ActivitySession{
				StartTime: time.Now().Add(-10 * time.Minute),
				EndTime:   time.Now(),
				AppClass:  "Firefox",
				Duration:  -10 * time.Minute,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateSession(tt.session)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSession() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateSummary tests summary validation logic
func TestValidateSummary(t *testing.T) {
	client := &Client{DebugMode: false}

	tests := []struct {
		name    string
		summary ActivitySummary
		wantErr bool
	}{
		{
			name: "valid summary",
			summary: rescuetime.ActivitySummary{
				AppClass:        "Firefox",
				ActivityDetails: "GitHub",
				TotalDuration:   15 * time.Minute,
				SessionCount:    3,
				FirstSeen:       time.Now().Add(-15 * time.Minute),
				LastSeen:        time.Now(),
			},
			wantErr: false,
		},
		{
			name: "missing app class",
			summary: rescuetime.ActivitySummary{
				TotalDuration: 15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     time.Now().Add(-15 * time.Minute),
				LastSeen:      time.Now(),
			},
			wantErr: true,
		},
		{
			name: "zero duration",
			summary: rescuetime.ActivitySummary{
				AppClass:     "Firefox",
				SessionCount: 3,
				FirstSeen:    time.Now().Add(-15 * time.Minute),
				LastSeen:     time.Now(),
			},
			wantErr: true,
		},
		{
			name: "negative duration",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: -15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     time.Now().Add(-15 * time.Minute),
				LastSeen:      time.Now(),
			},
			wantErr: true,
		},
		{
			name: "zero session count",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				FirstSeen:     time.Now().Add(-15 * time.Minute),
				LastSeen:      time.Now(),
			},
			wantErr: true,
		},
		{
			name: "negative session count",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				SessionCount:  -3,
				FirstSeen:     time.Now().Add(-15 * time.Minute),
				LastSeen:      time.Now(),
			},
			wantErr: true,
		},
		{
			name: "zero first seen",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				SessionCount:  3,
				LastSeen:      time.Now(),
			},
			wantErr: true,
		},
		{
			name: "zero last seen",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     time.Now().Add(-15 * time.Minute),
			},
			wantErr: true,
		},
		{
			name: "last seen before first seen",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     time.Now(),
				LastSeen:      time.Now().Add(-15 * time.Minute),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateSummary(tt.summary)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSummary() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestNewClient_MissingConnectionString tests that NewClient fails appropriately
func TestNewClient_MissingConnectionString(t *testing.T) {
	// Clear environment variable
	t.Setenv("POSTGRES_CONNECTION_STRING", "")

	_, err := NewClient("")
	if err == nil {
		t.Error("NewClient() should fail with empty connection string")
	}

	if err != nil && err.Error() == "" {
		t.Error("Error message should not be empty")
	}
}

// TestSubmitActivities_EmptyMap tests handling of empty summaries map
func TestSubmitActivities_EmptyMap(t *testing.T) {
	// This test doesn't require a database connection since it should exit early
	client := &Client{DebugMode: false}
	
	summaries := make(map[string]ActivitySummary)
	
	// Should not panic with empty map
	client.SubmitActivities(summaries)
}
