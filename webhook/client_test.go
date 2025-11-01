package webhook

import (
	"testing"
	"time"

	"github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
)

// TestNewClient tests the webhook client initialization
func TestNewClient(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		expectErr bool
	}{
		{
			name:      "Valid HTTPS URL",
			url:       "https://example.com/webhook",
			expectErr: false,
		},
		{
			name:      "Valid HTTP URL",
			url:       "http://localhost:3000/webhook",
			expectErr: false,
		},
		{
			name:      "Empty URL",
			url:       "",
			expectErr: true,
		},
		{
			name:      "Invalid URL - no protocol",
			url:       "example.com/webhook",
			expectErr: true,
		},
		{
			name:      "Invalid URL - wrong protocol",
			url:       "ftp://example.com/webhook",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.url)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error for URL %q, but got none", tt.url)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for URL %q: %v", tt.url, err)
				}
				if client == nil {
					t.Error("Expected non-nil client")
				}
				if client != nil {
					client.Close()
				}
			}
		})
	}
}

// TestValidateSummary tests the summary validation logic
func TestValidateSummary(t *testing.T) {
	now := time.Now()
	validSummary := rescuetime.ActivitySummary{
		AppClass:        "Firefox",
		ActivityDetails: "GitHub",
		TotalDuration:   15 * time.Minute,
		SessionCount:    3,
		FirstSeen:       now.Add(-15 * time.Minute),
		LastSeen:        now,
	}

	tests := []struct {
		name      string
		summary   rescuetime.ActivitySummary
		expectErr bool
	}{
		{
			name:      "Valid summary",
			summary:   validSummary,
			expectErr: false,
		},
		{
			name: "Empty app class",
			summary: rescuetime.ActivitySummary{
				AppClass:      "",
				TotalDuration: 15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     now.Add(-15 * time.Minute),
				LastSeen:      now,
			},
			expectErr: true,
		},
		{
			name: "Zero duration",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 0,
				SessionCount:  3,
				FirstSeen:     now.Add(-15 * time.Minute),
				LastSeen:      now,
			},
			expectErr: true,
		},
		{
			name: "Negative duration",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: -15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     now.Add(-15 * time.Minute),
				LastSeen:      now,
			},
			expectErr: true,
		},
		{
			name: "Zero session count",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				SessionCount:  0,
				FirstSeen:     now.Add(-15 * time.Minute),
				LastSeen:      now,
			},
			expectErr: true,
		},
		{
			name: "Zero first_seen",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     time.Time{},
				LastSeen:      now,
			},
			expectErr: true,
		},
		{
			name: "Zero last_seen",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     now.Add(-15 * time.Minute),
				LastSeen:      time.Time{},
			},
			expectErr: true,
		},
		{
			name: "last_seen before first_seen",
			summary: rescuetime.ActivitySummary{
				AppClass:      "Firefox",
				TotalDuration: 15 * time.Minute,
				SessionCount:  3,
				FirstSeen:     now,
				LastSeen:      now.Add(-15 * time.Minute),
			},
			expectErr: true,
		},
	}

	client, err := NewClient("https://example.com/webhook")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.validateSummary(tt.summary)
			if tt.expectErr {
				if err == nil {
					t.Error("Expected validation error, but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error: %v", err)
				}
			}
		})
	}
}

// TestSetHeader tests custom header setting
func TestSetHeader(t *testing.T) {
	client, err := NewClient("https://example.com/webhook")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Set a custom header
	client.SetHeader("Authorization", "Bearer test-token")
	client.SetHeader("X-API-Key", "test-api-key")

	// Verify headers are set
	if client.CustomHeaders["Authorization"] != "Bearer test-token" {
		t.Error("Authorization header not set correctly")
	}
	if client.CustomHeaders["X-API-Key"] != "test-api-key" {
		t.Error("X-API-Key header not set correctly")
	}
}

// TestSetTimeout tests timeout configuration
func TestSetTimeout(t *testing.T) {
	client, err := NewClient("https://example.com/webhook")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Set custom timeout
	customTimeout := 60 * time.Second
	client.SetTimeout(customTimeout)

	// Verify timeout is set
	if client.httpClient.Timeout != customTimeout {
		t.Errorf("Expected timeout %v, got %v", customTimeout, client.httpClient.Timeout)
	}
}

// TestDebugMode tests debug mode functionality
func TestDebugMode(t *testing.T) {
	client, err := NewClient("https://example.com/webhook")
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Debug mode should be false by default
	if client.DebugMode {
		t.Error("Debug mode should be false by default")
	}

	// Enable debug mode
	client.DebugMode = true
	if !client.DebugMode {
		t.Error("Failed to enable debug mode")
	}
}
