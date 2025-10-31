// Test program to demonstrate standalone usage of the rescuetime package
package main

import (
	"fmt"
	"time"

	"github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
)

func main() {
	fmt.Println("=== RescueTime Client Package Test ===\n")

	// Create a test activity summary
	summary := rescuetime.ActivitySummary{
		AppClass:        "Firefox",
		ActivityDetails: "GitHub - Projects",
		TotalDuration:   15 * time.Minute,
		SessionCount:    3,
		FirstSeen:       time.Now().Add(-15 * time.Minute),
		LastSeen:        time.Now(),
	}

	// Test payload conversion
	fmt.Println("1. Testing SummaryToPayload (Legacy API format)...")
	legacyPayload := rescuetime.SummaryToPayload(summary)
	fmt.Printf("   App: %s\n", legacyPayload.ActivityName)
	fmt.Printf("   Duration: %d minutes\n", legacyPayload.Duration)
	fmt.Printf("   Start: %s\n", legacyPayload.StartTime)
	fmt.Printf("   Details: %s\n\n", legacyPayload.ActivityDetails)

	// Test validation
	fmt.Println("2. Testing ValidatePayload...")
	if err := rescuetime.ValidatePayload(legacyPayload); err != nil {
		fmt.Printf("   ✗ Validation failed: %v\n", err)
	} else {
		fmt.Println("   ✓ Payload is valid\n")
	}

	// Test native API payload conversion
	fmt.Println("3. Testing SummaryToUserClientEvent (Native API format)...")
	nativePayload := rescuetime.SummaryToUserClientEvent(summary)
	fmt.Printf("   App: %s\n", nativePayload.UserClientEvent.Application)
	fmt.Printf("   Start: %s\n", nativePayload.UserClientEvent.StartTime)
	fmt.Printf("   End: %s\n", nativePayload.UserClientEvent.EndTime)
	fmt.Printf("   Window: %s\n\n", nativePayload.UserClientEvent.WindowTitle)

	// Test client creation
	fmt.Println("4. Testing Client creation...")
	client := rescuetime.NewClient("test-api-key-123", "", "")
	client.DebugMode = true
	fmt.Printf("   ✓ Client created with API key: %s...\n\n", client.APIKey[:4])

	// Test batch submission (won't actually submit without valid API key)
	fmt.Println("5. Testing SubmitActivities (dry-run - no real API call)...")
	summaries := map[string]rescuetime.ActivitySummary{
		"Firefox": summary,
	}
	
	// Note: This would attempt to submit if we had a valid API key
	// For this test, it will fail gracefully with an API error
	fmt.Println("   (Would submit to RescueTime API here)")
	fmt.Printf("   client.SubmitActivities(summaries) - %d activities ready\n", len(summaries))
	
	fmt.Println("\n=== Test Complete ===")
	fmt.Println("\nThe rescuetime package can now be imported and used in other Go projects:")
	fmt.Println("  import \"github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime\"")
}
