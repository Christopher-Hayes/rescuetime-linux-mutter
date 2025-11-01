# Webhook Module

This module provides optional webhook integration for RescueTime activity tracking data. It allows you to send activity data to your own HTTP endpoint for custom integrations, automation, or data processing.

## Features

- **Simple HTTP POST** - Sends JSON payloads to any HTTP/HTTPS endpoint
- **Retry logic** - Automatic retries with exponential backoff for transient failures
- **Custom headers** - Add authentication tokens or API keys via custom headers
- **Type-safe submissions** - Uses the same `ActivitySummary` type as the RescueTime module
- **Validation** - Validates all data before submission
- **Error handling** - Comprehensive error messages with troubleshooting steps

## Webhook Payload Format

The webhook receives a JSON POST request with the following structure:

```json
{
  "timestamp": "2025-10-31T14:30:00Z",
  "source": "rescuetime-linux-mutter",
  "version": "1.0.0",
  "summaries": [
    {
      "app_class": "Firefox",
      "activity_details": "GitHub - Projects",
      "total_duration": 900000000000,
      "session_count": 3,
      "first_seen": "2025-10-31T14:15:00Z",
      "last_seen": "2025-10-31T14:30:00Z"
    }
  ],
  "metadata": {
    "count": 1,
    "submitted": "2025-10-31T14:30:00Z"
  }
}
```

### Field Descriptions

- **timestamp**: When the payload was created (RFC3339 format)
- **source**: Always "rescuetime-linux-mutter"
- **version**: Version of the webhook format
- **summaries**: Array of activity summaries
  - **app_class**: Application name (e.g., "Firefox", "VSCode")
  - **activity_details**: Window title or additional details
  - **total_duration**: Duration in nanoseconds (Go's `time.Duration` format)
  - **session_count**: Number of separate sessions aggregated
  - **first_seen**: Timestamp when activity first started
  - **last_seen**: Timestamp when activity last occurred
- **metadata**: Optional metadata about the submission

## Usage

### Setup

1. **Create a webhook endpoint** - You need an HTTP/HTTPS endpoint that accepts POST requests with JSON payloads.

2. **Set webhook URL** in `.env`:
```env
WEBHOOK_URL=https://your-domain.com/rescuetime/webhook
```

3. **Optional: Set authentication header**:
```env
WEBHOOK_URL=https://your-domain.com/rescuetime/webhook
WEBHOOK_AUTH_HEADER=Authorization: Bearer your-secret-token
```

### Command Line Usage

**Enable webhook**:
```bash
./active-window -track -webhook "https://your-domain.com/webhook"
```

**Use environment variable**:
```bash
export WEBHOOK_URL="https://your-domain.com/webhook"
./active-window -track
```

**With RescueTime API and webhook**:
```bash
./active-window -track -submit -webhook "https://your-domain.com/webhook"
```

**With PostgreSQL and webhook**:
```bash
./active-window -track -postgres "postgres://..." -webhook "https://your-domain.com/webhook"
```

### Programmatic Usage

```go
import "github.com/Christopher-Hayes/rescuetime-linux-mutter/webhook"

// Initialize client
client, err := webhook.NewClient("https://your-domain.com/webhook")
if err != nil {
    log.Fatal(err)
}

// Enable debug logging
client.DebugMode = true

// Set custom authentication header
client.SetHeader("Authorization", "Bearer your-secret-token")

// Set custom timeout (default is 30 seconds)
client.SetTimeout(60 * time.Second)

// Submit activity summary
summary := rescuetime.ActivitySummary{
    AppClass:        "Firefox",
    ActivityDetails: "GitHub - Projects",
    TotalDuration:   15 * time.Minute,
    SessionCount:    3,
    FirstSeen:       time.Now().Add(-15 * time.Minute),
    LastSeen:        time.Now(),
}

err = client.SubmitSummary(summary)
if err != nil {
    log.Printf("Failed to submit: %v", err)
}

// Submit multiple summaries
summaries := map[string]rescuetime.ActivitySummary{
    "Firefox": summary,
    // ... more summaries
}
client.SubmitActivities(summaries)
```

## Example Webhook Implementations

### Simple Node.js/Express Webhook

```javascript
const express = require('express');
const app = express();

app.use(express.json());

app.post('/rescuetime/webhook', (req, res) => {
  const { timestamp, summaries, metadata } = req.body;
  
  console.log(`Received ${summaries.length} activities at ${timestamp}`);
  
  summaries.forEach(activity => {
    const minutes = Math.round(activity.total_duration / 1000000000 / 60);
    console.log(`  ${activity.app_class}: ${minutes} minutes (${activity.session_count} sessions)`);
  });
  
  // Process the data however you want
  // - Save to database
  // - Send notifications
  // - Trigger automation
  // - Update dashboards
  
  res.status(200).json({ success: true, received: summaries.length });
});

app.listen(3000, () => console.log('Webhook listening on port 3000'));
```

### Python/Flask Webhook

```python
from flask import Flask, request, jsonify
from datetime import datetime

app = Flask(__name__)

@app.route('/rescuetime/webhook', methods=['POST'])
def webhook():
    data = request.json
    timestamp = data.get('timestamp')
    summaries = data.get('summaries', [])
    
    print(f"Received {len(summaries)} activities at {timestamp}")
    
    for activity in summaries:
        duration_minutes = activity['total_duration'] / 1000000000 / 60
        print(f"  {activity['app_class']}: {duration_minutes:.1f} minutes ({activity['session_count']} sessions)")
    
    # Process the data
    # - Save to database
    # - Send to analytics platform
    # - Trigger automation workflows
    
    return jsonify({'success': True, 'received': len(summaries)}), 200

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=3000)
```

### Go Webhook Handler

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "time"
)

type WebhookPayload struct {
    Timestamp string                   `json:"timestamp"`
    Source    string                   `json:"source"`
    Version   string                   `json:"version"`
    Summaries []ActivitySummary        `json:"summaries"`
    Metadata  map[string]interface{}   `json:"metadata"`
}

type ActivitySummary struct {
    AppClass        string        `json:"app_class"`
    ActivityDetails string        `json:"activity_details"`
    TotalDuration   time.Duration `json:"total_duration"`
    SessionCount    int           `json:"session_count"`
    FirstSeen       time.Time     `json:"first_seen"`
    LastSeen        time.Time     `json:"last_seen"`
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != "POST" {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    var payload WebhookPayload
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    log.Printf("Received %d activities from %s", len(payload.Summaries), payload.Source)
    
    for _, activity := range payload.Summaries {
        minutes := activity.TotalDuration.Minutes()
        log.Printf("  %s: %.1f minutes (%d sessions)",
            activity.AppClass, minutes, activity.SessionCount)
    }

    // Process the data as needed
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]interface{}{
        "success":  true,
        "received": len(payload.Summaries),
    })
}

func main() {
    http.HandleFunc("/rescuetime/webhook", webhookHandler)
    log.Println("Webhook server listening on :3000")
    log.Fatal(http.ListenAndServe(":3000", nil))
}
```

## Authentication

For production use, you should secure your webhook endpoint. Here are common approaches:

### 1. Bearer Token

```go
client.SetHeader("Authorization", "Bearer your-secret-token")
```

Your webhook endpoint should verify this token:

```javascript
app.post('/webhook', (req, res) => {
  const authHeader = req.headers.authorization;
  if (authHeader !== 'Bearer your-secret-token') {
    return res.status(401).json({ error: 'Unauthorized' });
  }
  // ... process webhook
});
```

### 2. API Key Header

```go
client.SetHeader("X-API-Key", "your-api-key")
```

### 3. HMAC Signature (Advanced)

For the highest security, implement HMAC signature verification:

```go
// In your application, compute signature
secret := "your-shared-secret"
payload := /* JSON payload */
signature := hmac.SHA256(secret, payload)
client.SetHeader("X-Webhook-Signature", signature)
```

## Use Cases

- **Custom Analytics** - Build your own activity dashboards and reports
- **Automation** - Trigger actions based on activity patterns (e.g., Slack notifications)
- **Data Integration** - Send to analytics platforms (Datadog, Grafana, etc.)
- **Productivity Tools** - Integrate with personal productivity systems
- **Time Billing** - Automatically track billable hours for freelancers
- **Team Monitoring** - Aggregate team activity data (with proper consent)
- **Research** - Collect data for productivity research studies

## Troubleshooting

### Connection Refused
```
Error: request failed: connection refused
```
**Solution**: Ensure your webhook endpoint is running and accessible:
```bash
# Test with curl
curl -X POST https://your-domain.com/webhook \
  -H "Content-Type: application/json" \
  -d '{"test": true}'
```

### SSL/TLS Errors
```
Error: x509: certificate signed by unknown authority
```
**Solution**: For development with self-signed certificates, you can:
1. Use HTTP instead of HTTPS (development only)
2. Add the certificate to your system's trusted certificates
3. Implement custom TLS configuration (advanced)

### Authentication Failed
```
Error: webhook endpoint returned error 401
```
**Solution**: Verify authentication header is correct:
```bash
# Test authentication
curl -X POST https://your-domain.com/webhook \
  -H "Authorization: Bearer your-token" \
  -H "Content-Type: application/json" \
  -d '{"test": true}'
```

### Timeout
```
Error: request failed: context deadline exceeded
```
**Solution**: Your webhook endpoint might be slow. Increase timeout:
```go
client.SetTimeout(60 * time.Second)
```

### 400 Bad Request
```
Error: webhook endpoint returned error 400
```
**Solution**: Your endpoint might not accept the payload format. Check:
1. Content-Type is application/json
2. Payload structure matches your endpoint's expectations
3. Enable debug mode to see the exact payload being sent

## Security Best Practices

1. **Use HTTPS** - Always use HTTPS in production, never HTTP
2. **Validate payloads** - Your webhook endpoint should validate incoming data
3. **Authenticate requests** - Use bearer tokens or API keys
4. **Rate limiting** - Implement rate limiting on your webhook endpoint
5. **Input sanitization** - Sanitize window titles and app names before processing
6. **Monitor failures** - Set up alerts for webhook failures
7. **Keep secrets safe** - Never commit tokens or API keys to version control

## Performance Considerations

- **Timeout**: Default 30 seconds, adjust based on your endpoint's response time
- **Retry logic**: 3 attempts with exponential backoff (1s, 2s, 4s)
- **Batch size**: All summaries sent in a single request per submission interval
- **Network impact**: Minimal - only sends data every 15 minutes by default
- **Error handling**: Webhook failures don't block RescueTime or PostgreSQL submissions

## Testing Your Webhook

Use the dry-run mode to see what would be sent without actually sending:

```bash
./active-window -track -webhook "https://your-domain.com/webhook" -dry-run -submission-interval 1m
```

Or test with a webhook testing service:
- [webhook.site](https://webhook.site) - Free webhook testing
- [requestbin.com](https://requestbin.com) - Inspect webhook payloads
- [pipedream.com](https://pipedream.com) - Advanced webhook workflows

## Environment Variables

```bash
WEBHOOK_URL=https://your-domain.com/webhook  # Required: Webhook endpoint URL
```

## Integration with Other Modules

The webhook module works alongside other storage options:

```bash
# Send to RescueTime, PostgreSQL, AND webhook
./active-window -track -submit \
  -postgres "postgres://user:pass@localhost/rescuetime" \
  -webhook "https://your-domain.com/webhook"
```

All three destinations receive the same activity data independently. Failures in one don't affect the others.
