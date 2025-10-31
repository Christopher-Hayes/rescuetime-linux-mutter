# RescueTime Go Client Package

A Go client library for interacting with [RescueTime's](https://www.rescuetime.com/) API. This package supports both the legacy offline_time_post API and the native user_client_events API.

## Features

- ✅ **Legacy offline_time_post API** - Simple time tracking submissions
- ✅ **Native user_client_events API** - Advanced event tracking with fallback to legacy API
- ✅ **Automatic retry logic** with exponential backoff
- ✅ **Payload validation** to catch errors before submission
- ✅ **Multiple authentication methods** (API key, account key, data key)
- ✅ **Debug mode** for troubleshooting API requests

## Installation

```bash
go get github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime
```

## Quick Start

```go
package main

import (
    "fmt"
    "time"
    "github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
)

func main() {
    // Create a client (API key from environment or explicit)
    client := rescuetime.NewClient("your-api-key", "", "")
    client.DebugMode = true // Optional: enable debug output

    // Create an activity summary
    summary := rescuetime.ActivitySummary{
        AppClass:        "Firefox",
        ActivityDetails: "GitHub - Projects",
        TotalDuration:   15 * time.Minute,
        SessionCount:    3,
        FirstSeen:       time.Now().Add(-15 * time.Minute),
        LastSeen:        time.Now(),
    }

    // Option 1: Submit individual payload (Legacy API)
    payload := rescuetime.SummaryToPayload(summary)
    if err := rescuetime.ValidatePayload(payload); err != nil {
        fmt.Printf("Invalid payload: %v\n", err)
        return
    }
    err := client.SubmitLegacy(payload)
    if err != nil {
        fmt.Printf("Submission failed: %v\n", err)
    }

    // Option 2: Submit multiple activities with automatic fallback
    summaries := map[string]rescuetime.ActivitySummary{
        "Firefox": summary,
    }
    client.SubmitActivities(summaries)
}
```

## API Reference

### Client

#### `NewClient(apiKey, accountKey, dataKey string) *Client`

Creates a new RescueTime API client. If keys are not provided, they will be read from environment variables:
- `RESCUE_TIME_API_KEY` - Legacy API key
- `RESCUE_TIME_ACCOUNT_KEY` - Native API account key  
- `RESCUE_TIME_DATA_KEY` - Native API data key

```go
client := rescuetime.NewClient("", "", "") // Uses environment variables
client.DebugMode = true
```

### Types

#### `ActivitySummary`

Represents aggregated time spent in an application.

```go
type ActivitySummary struct {
    AppClass        string        // Application class (e.g., "Firefox")
    ActivityDetails string        // Window title or details
    TotalDuration   time.Duration // Total time spent
    SessionCount    int           // Number of sessions
    FirstSeen       time.Time     // First activity timestamp
    LastSeen        time.Time     // Last activity timestamp
}
```

#### `RescueTimePayload`

Legacy API payload format.

```go
type RescueTimePayload struct {
    StartTime       string // "YYYY-MM-DD HH:MM:SS"
    Duration        int    // Minutes
    ActivityName    string // Application name
    ActivityDetails string // Window title
}
```

#### `UserClientEventPayload`

Native API payload format.

```go
type UserClientEventPayload struct {
    UserClientEvent UserClientEvent
}

type UserClientEvent struct{
    EventDescription string // Application class
    StartTime        string // RFC 3339: "2025-10-30T12:00:00Z"
    EndTime          string // RFC 3339: "2025-10-30T12:15:00Z"
    WindowTitle      string // Window title
    Application      string // Application name
}
```

### Functions

#### `SummaryToPayload(summary ActivitySummary) RescueTimePayload`

Converts an ActivitySummary to legacy API format.

```go
payload := rescuetime.SummaryToPayload(summary)
```

#### `SummaryToUserClientEvent(summary ActivitySummary) UserClientEventPayload`

Converts an ActivitySummary to native API format.

```go
payload := rescuetime.SummaryToUserClientEvent(summary)
```

#### `ValidatePayload(payload RescueTimePayload) error`

Validates a payload before submission.

```go
if err := rescuetime.ValidatePayload(payload); err != nil {
    log.Fatal("Invalid payload:", err)
}
```

#### `(c *Client) SubmitLegacy(payload RescueTimePayload) error`

Submits a single activity to the legacy offline_time_post API with automatic retry logic.

```go
err := client.SubmitLegacy(payload)
```

#### `(c *Client) SubmitNative(payload UserClientEventPayload) error`

Submits a single event to the native user_client_events API.

```go
err := client.SubmitNative(payload)
```

#### `(c *Client) SubmitActivities(summaries map[string]ActivitySummary)`

Submits multiple activities with automatic API selection:
1. Tries native API if credentials available
2. Falls back to legacy API if native fails
3. Handles validation, retry logic, and error reporting

```go
summaries := map[string]rescuetime.ActivitySummary{
    "Firefox": firefoxSummary,
    "VS Code": vscodeSummary,
}
client.SubmitActivities(summaries)
```

#### `Activate(email, password string) (*ActivationResponse, error)`

Authenticates with RescueTime to retrieve account keys (experimental).

```go
response, err := rescuetime.Activate("user@example.com", "password")
if err != nil {
    log.Fatal(err)
}
fmt.Println("Account Key:", response.AccountKey)
```

## Environment Variables

The package respects these environment variables:

- `RESCUE_TIME_API_KEY` - Legacy API key (get from [RescueTime API settings](https://www.rescuetime.com/anapi/manage))
- `RESCUE_TIME_ACCOUNT_KEY` - Native API account key  
- `RESCUE_TIME_DATA_KEY` - Native API data key (Bearer token)

## Error Handling

The client includes automatic retry logic with exponential backoff:
- **Retries**: Up to 3 attempts
- **Backoff**: 1s, 2s, 4s
- **4xx errors**: No retry (client error)
- **5xx errors**: Retry with backoff (server error)

```go
err := client.SubmitLegacy(payload)
if err != nil {
    // Handle error - retries already exhausted
    log.Printf("Failed after retries: %v", err)
}
```

## Debug Mode

Enable debug mode to see detailed request/response information:

```go
client := rescuetime.NewClient(apiKey, "", "")
client.DebugMode = true

// Will print:
// - API key length
// - Request payloads
// - Request headers
// - Response status codes
// - Response bodies
```

## Validation

Payloads are validated before submission:

- ✅ Activity name is required
- ✅ Duration must be positive
- ✅ Duration cannot exceed 4 hours (RescueTime limit)
- ✅ Start time is required and properly formatted
- ✅ Minimum duration of 5 minutes enforced by `SubmitActivities`

## Example: Building a Time Tracker

```go
package main

import (
    "time"
    "github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
)

type Tracker struct {
    client    *rescuetime.Client
    summaries map[string]rescuetime.ActivitySummary
}

func NewTracker(apiKey string) *Tracker {
    return &Tracker{
        client:    rescuetime.NewClient(apiKey, "", ""),
        summaries: make(map[string]rescuetime.ActivitySummary),
    }
}

func (t *Tracker) Track(app, title string, duration time.Duration) {
    summary, exists := t.summaries[app]
    if !exists {
        summary = rescuetime.ActivitySummary{
            AppClass:        app,
            ActivityDetails: title,
            FirstSeen:       time.Now(),
        }
    }
    
    summary.TotalDuration += duration
    summary.SessionCount++
    summary.LastSeen = time.Now()
    summary.ActivityDetails = title // Use latest title
    
    t.summaries[app] = summary
}

func (t *Tracker) Submit() {
    t.client.SubmitActivities(t.summaries)
    t.summaries = make(map[string]rescuetime.ActivitySummary)
}

func main() {
    tracker := NewTracker("your-api-key")
    
    // Track some activities
    tracker.Track("Firefox", "GitHub", 15*time.Minute)
    tracker.Track("VS Code", "project.go", 30*time.Minute)
    
    // Submit to RescueTime
    tracker.Submit()
}
```

## Contributing

This package was extracted from [rescuetime-linux-mutter](https://github.com/Christopher-Hayes/rescuetime-linux-mutter), a native Linux activity tracker for GNOME/Mutter.

## License

See the main project's [LICENSE](../LICENSE) file.

## Related Projects

- [rescuetime-linux-mutter](https://github.com/Christopher-Hayes/rescuetime-linux-mutter) - The full activity tracking daemon this package was extracted from
- [RescueTime](https://www.rescuetime.com/) - Official RescueTime service and clients
