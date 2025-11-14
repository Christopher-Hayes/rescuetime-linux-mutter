package rescuetime

import (
	"testing"
	"time"
)

func TestSplitLongDurationSummaries(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name          string
		input         map[string]ActivitySummary
		expectChunks  int
		expectTotal   time.Duration
	}{
		{
			name: "Short duration - no split",
			input: map[string]ActivitySummary{
				"firefox": {
					AppClass:        "firefox",
					ActivityDetails: "Browsing",
					TotalDuration:   2 * time.Hour,
					SessionCount:    1,
					FirstSeen:       now,
					LastSeen:        now.Add(2 * time.Hour),
				},
			},
			expectChunks: 1,
			expectTotal:  2 * time.Hour,
		},
		{
			name: "Exactly 4 hours - no split",
			input: map[string]ActivitySummary{
				"steam": {
					AppClass:        "steam",
					ActivityDetails: "Gaming",
					TotalDuration:   4 * time.Hour,
					SessionCount:    1,
					FirstSeen:       now,
					LastSeen:        now.Add(4 * time.Hour),
				},
			},
			expectChunks: 1,
			expectTotal:  4 * time.Hour,
		},
		{
			name: "6 hours - split into 2 chunks",
			input: map[string]ActivitySummary{
				"steam": {
					AppClass:        "steam",
					ActivityDetails: "Gaming",
					TotalDuration:   6 * time.Hour,
					SessionCount:    1,
					FirstSeen:       now,
					LastSeen:        now.Add(6 * time.Hour),
				},
			},
			expectChunks: 2,
			expectTotal:  6 * time.Hour,
		},
		{
			name: "10 hours - split into 3 chunks",
			input: map[string]ActivitySummary{
				"vscode": {
					AppClass:        "vscode",
					ActivityDetails: "Coding",
					TotalDuration:   10 * time.Hour,
					SessionCount:    1,
					FirstSeen:       now,
					LastSeen:        now.Add(10 * time.Hour),
				},
			},
			expectChunks: 3,
			expectTotal:  10 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLongDurationSummaries(tt.input)

			// Check number of chunks
			if len(result) != tt.expectChunks {
				t.Errorf("Expected %d chunks, got %d", tt.expectChunks, len(result))
			}

			// Check total duration is preserved
			totalDuration := time.Duration(0)
			for _, summary := range result {
				totalDuration += summary.TotalDuration

				// Verify each chunk is within API limit
				if summary.TotalDuration > maxOfflineDuration {
					t.Errorf("Chunk duration %v exceeds API limit of %v", summary.TotalDuration, maxOfflineDuration)
				}
			}

			if totalDuration != tt.expectTotal {
				t.Errorf("Expected total duration %v, got %v", tt.expectTotal, totalDuration)
			}

			// If split into chunks, verify timestamps are sequential
			if len(result) > 1 {
				// Note: map iteration order is random, so we can't easily verify sequence
				// but we can verify all chunks are within the original time window
				originalStart := tt.input[getFirstKey(tt.input)].FirstSeen
				originalEnd := tt.input[getFirstKey(tt.input)].LastSeen

				for _, summary := range result {
					if summary.FirstSeen.Before(originalStart) {
						t.Errorf("Chunk start time %v is before original start %v", summary.FirstSeen, originalStart)
					}
					if summary.LastSeen.After(originalEnd) {
						t.Errorf("Chunk end time %v is after original end %v", summary.LastSeen, originalEnd)
					}
				}
			}
		})
	}
}

// Helper to get first key from map (for testing)
func getFirstKey(m map[string]ActivitySummary) string {
	for k := range m {
		return k
	}
	return ""
}

func TestChunkSizeIsUnderLimit(t *testing.T) {
	// Verify our chunk size constant is safe
	if chunkSize >= maxOfflineDuration {
		t.Errorf("chunkSize (%v) must be less than maxOfflineDuration (%v)", chunkSize, maxOfflineDuration)
	}

	// Verify it's reasonably close to the limit (not too small)
	if chunkSize < 3*time.Hour {
		t.Errorf("chunkSize (%v) seems too conservative, should be closer to %v", chunkSize, maxOfflineDuration)
	}
}
