// Package postgres provides a Go client for storing RescueTime activity tracking data
// in a PostgreSQL database. This allows users to maintain their own local copy of
// activity data alongside RescueTime's cloud service.
//
// Example usage:
//
//	client, err := postgres.NewClient("postgres://user:password@localhost/rescuetime?sslmode=disable")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer client.Close()
//
//	summary := postgres.ActivitySummary{
//		AppClass:        "Firefox",
//		ActivityDetails: "GitHub - Projects",
//		TotalDuration:   15 * time.Minute,
//		SessionCount:    3,
//		FirstSeen:       time.Now().Add(-15 * time.Minute),
//		LastSeen:        time.Now(),
//	}
//
//	err = client.SubmitSummary(summary)
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/Christopher-Hayes/rescuetime-linux-mutter/rescuetime"
	"github.com/fatih/color"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// Configuration constants
const (
	defaultConnectTimeout = 10 * time.Second
	defaultQueryTimeout   = 5 * time.Second
	maxRetries            = 3
	baseRetryDelay        = 1 * time.Second
)

// Type aliases to use RescueTime's types for consistency
type ActivitySummary = rescuetime.ActivitySummary

// ActivitySession represents a single continuous session with an application.
// This maps to the activity_sessions table.
type ActivitySession struct {
	ID          int64         `json:"id,omitempty"`
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	AppClass    string        `json:"app_class"`
	WindowTitle string        `json:"window_title"`
	Duration    time.Duration `json:"duration"`
	Ignored     bool          `json:"ignored"` // true if app is in ignore list (excluded from RescueTime)
	CreatedAt   time.Time     `json:"created_at,omitempty"`
}

// StoredSummary represents a summary retrieved from the database with metadata
type StoredSummary struct {
	ID              int64         `json:"id"`
	AppClass        string        `json:"app_class"`
	ActivityDetails string        `json:"activity_details"`
	TotalDuration   time.Duration `json:"total_duration"`
	SessionCount    int           `json:"session_count"`
	FirstSeen       time.Time     `json:"first_seen"`
	LastSeen        time.Time     `json:"last_seen"`
	SubmittedAt     time.Time     `json:"submitted_at"`
}

// Client provides methods for storing activity data in PostgreSQL.
type Client struct {
	db            *sql.DB
	connectionStr string
	DebugMode     bool
}

// NewClient creates a new PostgreSQL client and initializes the database schema.
// The connection string should be in the format:
// postgres://username:password@hostname:port/database?sslmode=disable
//
// If connectionStr is empty, it will attempt to read from POSTGRES_CONNECTION_STRING
// environment variable.
func NewClient(connectionStr string) (*Client, error) {
	// Use provided connection string, or fall back to environment variable
	if connectionStr == "" {
		connectionStr = os.Getenv("POSTGRES_CONNECTION_STRING")
	}

	if connectionStr == "" {
		return nil, fmt.Errorf("PostgreSQL connection string not provided\n\nSet via:\n  1. POSTGRES_CONNECTION_STRING environment variable\n  2. -postgres flag\n\nExample: postgres://user:password@localhost/rescuetime?sslmode=disable")
	}

	// Create connection with timeout context
	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectTimeout)
	defer cancel()

	db, err := sql.Open("postgres", connectionStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %v\n\nTroubleshooting:\n  1. Verify connection string format\n  2. Check PostgreSQL is running: sudo systemctl status postgresql\n  3. Test connection: psql '%s'", err, connectionStr)
	}

	// Verify connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %v\n\nTroubleshooting:\n  1. Verify PostgreSQL is running\n  2. Check credentials in connection string\n  3. Verify database exists\n  4. Check firewall/network settings", err)
	}

	client := &Client{
		db:            db,
		connectionStr: connectionStr,
		DebugMode:     false,
	}

	// Initialize database schema
	if err := client.initializeSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database schema: %v", err)
	}

	return client, nil
}

// Close closes the database connection.
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// debugLog prints debug messages if debug mode is enabled
func (c *Client) debugLog(format string, args ...interface{}) {
	if c.DebugMode {
		color.Cyan("[POSTGRES DEBUG] "+format, args...)
	}
}

// initializeSchema creates the necessary tables and indexes if they don't exist.
func (c *Client) initializeSchema() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultQueryTimeout)
	defer cancel()

	// Create activity_sessions table
	sessionsTableSQL := `
	CREATE TABLE IF NOT EXISTS activity_sessions (
		id SERIAL PRIMARY KEY,
		start_time TIMESTAMP WITH TIME ZONE NOT NULL,
		end_time TIMESTAMP WITH TIME ZONE NOT NULL,
		app_class VARCHAR(255) NOT NULL,
		window_title TEXT,
		duration_seconds INTEGER NOT NULL,
		ignored BOOLEAN DEFAULT FALSE,
		created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		CONSTRAINT valid_duration CHECK (duration_seconds >= 0),
		CONSTRAINT valid_time_range CHECK (end_time >= start_time)
	);
	`

	if _, err := c.db.ExecContext(ctx, sessionsTableSQL); err != nil {
		return fmt.Errorf("failed to create activity_sessions table: %v", err)
	}

	// Create indexes on activity_sessions for common queries
	sessionIndexesSQL := []string{
		`CREATE INDEX IF NOT EXISTS idx_sessions_app_class ON activity_sessions(app_class);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_start_time ON activity_sessions(start_time);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_end_time ON activity_sessions(end_time);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_app_time ON activity_sessions(app_class, start_time);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_ignored ON activity_sessions(ignored);`,
	}

	for _, indexSQL := range sessionIndexesSQL {
		if _, err := c.db.ExecContext(ctx, indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	// Create activity_summaries table
	summariesTableSQL := `
	CREATE TABLE IF NOT EXISTS activity_summaries (
		id SERIAL PRIMARY KEY,
		app_class VARCHAR(255) NOT NULL,
		activity_details TEXT,
		total_duration_seconds INTEGER NOT NULL,
		session_count INTEGER NOT NULL,
		first_seen TIMESTAMP WITH TIME ZONE NOT NULL,
		last_seen TIMESTAMP WITH TIME ZONE NOT NULL,
		submitted_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
		CONSTRAINT valid_summary_duration CHECK (total_duration_seconds >= 0),
		CONSTRAINT valid_session_count CHECK (session_count > 0),
		CONSTRAINT valid_summary_time_range CHECK (last_seen >= first_seen)
	);
	`

	if _, err := c.db.ExecContext(ctx, summariesTableSQL); err != nil {
		return fmt.Errorf("failed to create activity_summaries table: %v", err)
	}

	// Create indexes on activity_summaries for common queries
	summaryIndexesSQL := []string{
		`CREATE INDEX IF NOT EXISTS idx_summaries_app_class ON activity_summaries(app_class);`,
		`CREATE INDEX IF NOT EXISTS idx_summaries_first_seen ON activity_summaries(first_seen);`,
		`CREATE INDEX IF NOT EXISTS idx_summaries_last_seen ON activity_summaries(last_seen);`,
		`CREATE INDEX IF NOT EXISTS idx_summaries_submitted_at ON activity_summaries(submitted_at);`,
	}

	for _, indexSQL := range summaryIndexesSQL {
		if _, err := c.db.ExecContext(ctx, indexSQL); err != nil {
			return fmt.Errorf("failed to create index: %v", err)
		}
	}

	c.debugLog("Database schema initialized successfully")
	return nil
}

// SubmitSession stores a single activity session in the database.
func (c *Client) SubmitSession(session ActivitySession) error {
	if err := c.validateSession(session); err != nil {
		return fmt.Errorf("invalid session: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultQueryTimeout)
	defer cancel()

	insertSQL := `
		INSERT INTO activity_sessions (start_time, end_time, app_class, window_title, duration_seconds, ignored)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	var id int64
	err := c.db.QueryRowContext(ctx, insertSQL,
		session.StartTime,
		session.EndTime,
		session.AppClass,
		session.WindowTitle,
		int(session.Duration.Seconds()),
		session.Ignored,
	).Scan(&id)

	if err != nil {
		return fmt.Errorf("failed to insert session: %v", err)
	}

	ignoredLabel := ""
	if session.Ignored {
		ignoredLabel = " (ignored)"
	}
	c.debugLog("Inserted session ID %d: %s (%v)%s", id, session.AppClass, session.Duration, ignoredLabel)
	return nil
}

// SubmitSummary stores an activity summary in the database.
func (c *Client) SubmitSummary(summary ActivitySummary) error {
	if err := c.validateSummary(summary); err != nil {
		return fmt.Errorf("invalid summary: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultQueryTimeout)
	defer cancel()

	insertSQL := `
		INSERT INTO activity_summaries (
			app_class, activity_details, total_duration_seconds, 
			session_count, first_seen, last_seen
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`

	var id int64
	err := c.db.QueryRowContext(ctx, insertSQL,
		summary.AppClass,
		summary.ActivityDetails,
		int(summary.TotalDuration.Seconds()),
		summary.SessionCount,
		summary.FirstSeen,
		summary.LastSeen,
	).Scan(&id)

	if err != nil {
		return fmt.Errorf("failed to insert summary: %v", err)
	}

	c.debugLog("Inserted summary ID %d: %s (%v, %d sessions)", 
		id, summary.AppClass, summary.TotalDuration, summary.SessionCount)
	
	color.New(color.FgGreen, color.Bold).Printf("[SUCCESS] Stored in PostgreSQL: %s (%v, %d sessions)\n",
		summary.AppClass, summary.TotalDuration.Round(time.Second), summary.SessionCount)
	
	return nil
}

// SubmitActivities stores multiple activity summaries in the database.
// This stores the same aggregated data that gets sent to RescueTime's API,
// allowing users to build their own applications with the same data.
func (c *Client) SubmitActivities(summaries map[string]ActivitySummary) {
	if len(summaries) == 0 {
		color.Yellow("[POSTGRES] No activities to submit.")
		return
	}

	color.New(color.FgCyan, color.Bold).Printf("\n=== Storing %d activities in PostgreSQL ===\n", len(summaries))

	successCount := 0
	failCount := 0

	for _, summary := range summaries {
		err := c.SubmitSummary(summary)
		if err != nil {
			color.Red("[POSTGRES] ✗ Failed to store %s: %v\n", summary.AppClass, err)
			failCount++
		} else {
			successCount++
		}
	}

	color.New(color.FgCyan, color.Bold).Printf("\n=== PostgreSQL Storage Summary ===\n")
	if successCount > 0 {
		color.Green("Stored: %d\n", successCount)
	}
	if failCount > 0 {
		color.Red("Failed: %d\n", failCount)
	}
}

// SubmitSessions stores multiple activity sessions in the database.
// This stores individual session data (start/end times, window titles) which
// provides more granular tracking data than the aggregated summaries.
func (c *Client) SubmitSessions(sessions []ActivitySession) {
	if len(sessions) == 0 {
		color.Yellow("[POSTGRES] No sessions to submit.")
		return
	}

	color.New(color.FgCyan, color.Bold).Printf("\n=== Storing %d sessions in PostgreSQL ===\n", len(sessions))

	successCount := 0
	failCount := 0

	for _, session := range sessions {
		err := c.SubmitSession(session)
		if err != nil {
			color.Red("[POSTGRES] ✗ Failed to store session %s: %v\n", session.AppClass, err)
			failCount++
		} else {
			successCount++
		}
	}

	color.New(color.FgCyan, color.Bold).Printf("\n=== PostgreSQL Session Storage Summary ===\n")
	if successCount > 0 {
		color.Green("Stored: %d sessions\n", successCount)
	}
	if failCount > 0 {
		color.Red("Failed: %d\n", failCount)
	}
}

// validateSession checks if a session is valid before insertion.
func (c *Client) validateSession(session ActivitySession) error {
	if session.AppClass == "" {
		return fmt.Errorf("app_class is required")
	}
	if session.StartTime.IsZero() {
		return fmt.Errorf("start_time is required")
	}
	if session.EndTime.IsZero() {
		return fmt.Errorf("end_time is required")
	}
	if session.EndTime.Before(session.StartTime) {
		return fmt.Errorf("end_time must be after start_time")
	}
	if session.Duration < 0 {
		return fmt.Errorf("duration must be non-negative")
	}
	// Verify duration matches time range (allow small tolerance for rounding)
	expectedDuration := session.EndTime.Sub(session.StartTime)
	if session.Duration > 0 {
		diff := session.Duration - expectedDuration
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Second {
			return fmt.Errorf("duration (%v) doesn't match time range (%v)", session.Duration, expectedDuration)
		}
	}
	return nil
}

// validateSummary checks if a summary is valid before insertion.
func (c *Client) validateSummary(summary ActivitySummary) error {
	if summary.AppClass == "" {
		return fmt.Errorf("app_class is required")
	}
	if summary.TotalDuration <= 0 {
		return fmt.Errorf("total_duration must be positive")
	}
	if summary.SessionCount <= 0 {
		return fmt.Errorf("session_count must be positive")
	}
	if summary.FirstSeen.IsZero() {
		return fmt.Errorf("first_seen is required")
	}
	if summary.LastSeen.IsZero() {
		return fmt.Errorf("last_seen is required")
	}
	if summary.LastSeen.Before(summary.FirstSeen) {
		return fmt.Errorf("last_seen must be after or equal to first_seen")
	}
	return nil
}

// GetRecentSessions retrieves recent activity sessions from the database.
// Limit specifies the maximum number of sessions to return.
func (c *Client) GetRecentSessions(limit int) ([]ActivitySession, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultQueryTimeout)
	defer cancel()

	querySQL := `
		SELECT id, start_time, end_time, app_class, window_title, duration_seconds, created_at
		FROM activity_sessions
		ORDER BY start_time DESC
		LIMIT $1
	`

	rows, err := c.db.QueryContext(ctx, querySQL, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query sessions: %v", err)
	}
	defer rows.Close()

	var sessions []ActivitySession
	for rows.Next() {
		var session ActivitySession
		var durationSeconds int
		err := rows.Scan(
			&session.ID,
			&session.StartTime,
			&session.EndTime,
			&session.AppClass,
			&session.WindowTitle,
			&durationSeconds,
			&session.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %v", err)
		}
		session.Duration = time.Duration(durationSeconds) * time.Second
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %v", err)
	}

	return sessions, nil
}

// GetRecentSummaries retrieves recent activity summaries from the database.
// Limit specifies the maximum number of summaries to return.
func (c *Client) GetRecentSummaries(limit int) ([]StoredSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultQueryTimeout)
	defer cancel()

	querySQL := `
		SELECT id, app_class, activity_details, total_duration_seconds, 
		       session_count, first_seen, last_seen, submitted_at
		FROM activity_summaries
		ORDER BY submitted_at DESC
		LIMIT $1
	`

	rows, err := c.db.QueryContext(ctx, querySQL, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query summaries: %v", err)
	}
	defer rows.Close()

	var summaries []StoredSummary
	for rows.Next() {
		var summary StoredSummary
		var durationSeconds int
		err := rows.Scan(
			&summary.ID,
			&summary.AppClass,
			&summary.ActivityDetails,
			&durationSeconds,
			&summary.SessionCount,
			&summary.FirstSeen,
			&summary.LastSeen,
			&summary.SubmittedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan summary: %v", err)
		}
		summary.TotalDuration = time.Duration(durationSeconds) * time.Second
		summaries = append(summaries, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating summaries: %v", err)
	}

	return summaries, nil
}
