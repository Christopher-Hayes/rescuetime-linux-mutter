# PostgreSQL Storage Module

This module provides optional PostgreSQL database storage for RescueTime activity tracking data. It allows you to maintain your own local copy of activity data alongside RescueTime's cloud service.

## Features

- **Automatic schema initialization** - Creates tables and indexes on first connection
- **Type-safe submissions** - Uses the same `ActivitySummary` type as the RescueTime module
- **Validation** - Validates all data before insertion
- **Error handling** - Comprehensive error messages with troubleshooting steps
- **Data retrieval** - Query stored sessions and summaries

## Database Schema

### `activity_sessions` Table
Stores individual continuous sessions with applications.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| start_time | TIMESTAMPTZ | Session start time |
| end_time | TIMESTAMPTZ | Session end time |
| app_class | VARCHAR(255) | Application name |
| window_title | TEXT | Window title |
| duration_seconds | INTEGER | Duration in seconds |
| created_at | TIMESTAMPTZ | Record creation timestamp |

### `activity_summaries` Table
Stores aggregated activity summaries submitted to RescueTime.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| app_class | VARCHAR(255) | Application name |
| activity_details | TEXT | Additional details |
| total_duration_seconds | INTEGER | Total duration in seconds |
| session_count | INTEGER | Number of sessions |
| first_seen | TIMESTAMPTZ | First occurrence |
| last_seen | TIMESTAMPTZ | Last occurrence |
| submitted_at | TIMESTAMPTZ | Submission timestamp |

## Usage

### Setup

1. **Install PostgreSQL** (if not already installed):
```bash
# Ubuntu/Debian
sudo apt install postgresql postgresql-contrib

# Fedora/RHEL
sudo dnf install postgresql-server postgresql-contrib
```

2. **Create a database**:
```bash
sudo -u postgres createdb rescuetime
sudo -u postgres createuser -P myuser  # Sets password
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE rescuetime TO myuser;"
```

3. **Set connection string** in `.env`:
```env
POSTGRES_CONNECTION_STRING=postgres://myuser:mypassword@localhost/rescuetime?sslmode=disable
```

### Command Line Usage

**Enable PostgreSQL storage**:
```bash
./active-window -track -postgres "postgres://user:pass@localhost/rescuetime"
```

**Use environment variable**:
```bash
export POSTGRES_CONNECTION_STRING="postgres://user:pass@localhost/rescuetime"
./active-window -track
```

**With RescueTime API**:
```bash
./active-window -track -submit -postgres "postgres://user:pass@localhost/rescuetime"
```

### Programmatic Usage

```go
import "github.com/Christopher-Hayes/rescuetime-linux-mutter/postgres"

// Initialize client
client, err := postgres.NewClient("postgres://user:pass@localhost/rescuetime")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Enable debug logging
client.DebugMode = true

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

// Query recent summaries
recent, err := client.GetRecentSummaries(10)
if err != nil {
    log.Printf("Failed to query: %v", err)
}
for _, s := range recent {
    fmt.Printf("%s: %v (%d sessions)\n", s.AppClass, s.TotalDuration, s.SessionCount)
}
```

## Connection String Format

The connection string uses the standard PostgreSQL URL format:

```
postgres://username:password@hostname:port/database?options
```

**Examples**:
- Local: `postgres://user:pass@localhost/rescuetime?sslmode=disable`
- Remote: `postgres://user:pass@db.example.com:5432/rescuetime?sslmode=require`
- Unix socket: `postgres://user:pass@/rescuetime?host=/var/run/postgresql`

**Common options**:
- `sslmode=disable` - No SSL (local development only)
- `sslmode=require` - Require SSL connection
- `connect_timeout=10` - Connection timeout in seconds

## Querying Your Data

Once data is stored, you can query it directly with `psql`:

```bash
psql postgres://user:pass@localhost/rescuetime
```

**Example queries**:

```sql
-- Most used applications today
SELECT app_class, SUM(duration_seconds)/3600.0 as hours
FROM activity_sessions
WHERE start_time >= CURRENT_DATE
GROUP BY app_class
ORDER BY hours DESC
LIMIT 10;

-- Activity by hour
SELECT EXTRACT(HOUR FROM start_time) as hour, 
       COUNT(*) as sessions,
       SUM(duration_seconds)/3600.0 as hours
FROM activity_sessions
WHERE start_time >= CURRENT_DATE
GROUP BY hour
ORDER BY hour;

-- Recent submissions
SELECT app_class, total_duration_seconds/60 as minutes, session_count,
       submitted_at
FROM activity_summaries
ORDER BY submitted_at DESC
LIMIT 20;
```

## Troubleshooting

### Connection Refused
```
Error: failed to connect to database: connection refused
```
**Solution**: Ensure PostgreSQL is running:
```bash
sudo systemctl status postgresql
sudo systemctl start postgresql
```

### Authentication Failed
```
Error: failed to connect to database: authentication failed
```
**Solution**: Verify credentials and database permissions:
```bash
psql -U myuser -d rescuetime -h localhost
```

### Database Does Not Exist
```
Error: database "rescuetime" does not exist
```
**Solution**: Create the database:
```bash
sudo -u postgres createdb rescuetime
```

### Permission Denied
```
Error: permission denied for table activity_sessions
```
**Solution**: Grant permissions:
```sql
GRANT ALL PRIVILEGES ON DATABASE rescuetime TO myuser;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO myuser;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO myuser;
```

## Performance Considerations

- **Indexes**: All tables have indexes on frequently queried columns
- **Batch inserts**: `SubmitActivities` batches multiple submissions
- **Connection pooling**: Reuses the same database connection
- **Timeouts**: All queries have 5-second timeout to prevent hanging

## Security Best Practices

1. **Never commit credentials** - Use `.env` file (in `.gitignore`)
2. **Use SSL in production** - Set `sslmode=require` for remote connections
3. **Limit permissions** - Grant only necessary privileges to the database user
4. **Regular backups** - PostgreSQL data is valuable, back it up regularly

```bash
# Backup
pg_dump -U myuser rescuetime > rescuetime_backup.sql

# Restore
psql -U myuser rescuetime < rescuetime_backup.sql
```
