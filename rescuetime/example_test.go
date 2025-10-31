// Example tests for the rescuetime package demonstrating usage patterns
package rescuetime_test

import (
	"fmt"
	"time"

	"github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
)

// Example demonstrates basic usage of the rescuetime package for converting
// activity summaries to API payloads and validating them.
func Example() {
	// Create a test activity summary
	summary := rescuetime.ActivitySummary{
		AppClass:        "Firefox",
		ActivityDetails: "GitHub - Projects",
		TotalDuration:   15 * time.Minute,
		SessionCount:    3,
		FirstSeen:       time.Now().Add(-15 * time.Minute),
		LastSeen:        time.Now(),
	}

	// Convert to legacy API format
	legacyPayload := rescuetime.SummaryToPayload(summary)
	fmt.Printf("App: %s\n", legacyPayload.ActivityName)
	fmt.Printf("Duration: %d minutes\n", legacyPayload.Duration)

	// Validate payload
	if err := rescuetime.ValidatePayload(legacyPayload); err != nil {
		fmt.Printf("Validation failed: %v\n", err)
	} else {
		fmt.Println("Payload is valid")
	}

	// Output:
	// App: Firefox
	// Duration: 15 minutes
	// Payload is valid
}

// ExampleNewClient demonstrates creating a new RescueTime API client.
func ExampleNewClient() {
	client := rescuetime.NewClient("your-api-key", "account-key", "data-key")
	client.DebugMode = true
	
	fmt.Println("Client created successfully")
	// Output: Client created successfully
}

// ExampleSummaryToPayload demonstrates converting an ActivitySummary to the legacy API format.
func ExampleSummaryToPayload() {
	summary := rescuetime.ActivitySummary{
		AppClass:        "code",
		ActivityDetails: "main.go",
		TotalDuration:   30 * time.Minute,
		SessionCount:    2,
		FirstSeen:       time.Date(2025, 10, 31, 10, 0, 0, 0, time.UTC),
		LastSeen:        time.Date(2025, 10, 31, 10, 30, 0, 0, time.UTC),
	}

	payload := rescuetime.SummaryToPayload(summary)
	fmt.Printf("Activity: %s\n", payload.ActivityName)
	fmt.Printf("Duration: %d minutes\n", payload.Duration)
	fmt.Printf("Details: %s\n", payload.ActivityDetails)

	// Output:
	// Activity: code
	// Duration: 30 minutes
	// Details: main.go
}

// ExampleValidatePayload demonstrates payload validation.
func ExampleValidatePayload() {
	validPayload := rescuetime.RescueTimePayload{
		StartTime:       "2025-10-31 10:00:00",
		Duration:        30,
		ActivityName:    "code",
		ActivityDetails: "main.go",
	}

	if err := rescuetime.ValidatePayload(validPayload); err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		fmt.Println("Valid payload")
	}

	invalidPayload := rescuetime.RescueTimePayload{
		StartTime:       "2025-10-31 10:00:00",
		Duration:        0,
		ActivityName:    "",
		ActivityDetails: "main.go",
	}

	if err := rescuetime.ValidatePayload(invalidPayload); err != nil {
		fmt.Println("Invalid payload detected")
	}

	// Output:
	// Valid payload
	// Invalid payload detected
}
