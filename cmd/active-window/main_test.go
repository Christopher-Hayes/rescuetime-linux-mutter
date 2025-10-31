package main

import (
"testing"
"time"
)

// TestConstants verifies configuration constants are set correctly
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
if defaultIdleThreshold != 5*time.Minute {
t.Errorf("defaultIdleThreshold should be 5m, got %v", defaultIdleThreshold)
}
}
