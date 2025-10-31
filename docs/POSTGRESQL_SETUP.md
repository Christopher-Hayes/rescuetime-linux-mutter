# PostgreSQL Database Setup Guide

This guide will walk you through setting up a PostgreSQL database for RescueTime activity tracking data.

## Quick Setup (Copy-Paste Ready)

### 1. Install PostgreSQL

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install postgresql postgresql-contrib
```

**Fedora/RHEL/CentOS:**
```bash
sudo dnf install postgresql-server postgresql-contrib
sudo postgresql-setup --initdb
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

**Arch Linux:**
```bash
sudo pacman -S postgresql
sudo -u postgres initdb -D /var/lib/postgres/data
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

### 2. Create Database and User

```bash
# Switch to postgres user
sudo -u postgres psql

# In the PostgreSQL prompt, run these commands:
```

```sql
-- Create a dedicated user for RescueTime
CREATE USER rescuetime_user WITH PASSWORD 'your_secure_password_here';

-- Create the database
CREATE DATABASE rescuetime OWNER rescuetime_user;

-- Grant all privileges on the database
GRANT ALL PRIVILEGES ON DATABASE rescuetime TO rescuetime_user;

-- Connect to the new database
\c rescuetime

-- Grant schema privileges (PostgreSQL 15+)
GRANT ALL ON SCHEMA public TO rescuetime_user;
GRANT ALL ON ALL TABLES IN SCHEMA public TO rescuetime_user;
GRANT ALL ON ALL SEQUENCES IN SCHEMA public TO rescuetime_user;

-- Make future objects accessible
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON TABLES TO rescuetime_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL ON SEQUENCES TO rescuetime_user;

-- Exit PostgreSQL
\q
```

### 3. Configure Connection String

The application will **automatically create tables** on first connection. You just need to provide the connection string.

**Add to your `.env` file:**
```bash
POSTGRES_CONNECTION_STRING=postgres://rescuetime_user:your_secure_password_here@localhost/rescuetime?sslmode=disable
```

**Or use the command-line flag:**
```bash
./active-window -track -submit -postgres "postgres://rescuetime_user:your_secure_password_here@localhost/rescuetime?sslmode=disable"
```

### 4. Test Connection

```bash
# Test PostgreSQL connection
psql "postgres://rescuetime_user:your_secure_password_here@localhost/rescuetime"
```

If successful, you'll see:
```
psql (15.x)
Type "help" for help.

rescuetime=>
```

### 5. Run the Application

When you run the application with the PostgreSQL connection string, it will **automatically**:
1. Connect to the database
2. Create the `activity_sessions` table (if it doesn't exist)
3. Create the `activity_summaries` table (if it doesn't exist)
4. Create all necessary indexes
5. Start tracking and storing data

```bash
# Track and store in both RescueTime and PostgreSQL
./active-window -track -submit -postgres "postgres://rescuetime_user:your_secure_password_here@localhost/rescuetime?sslmode=disable"
```

## Database Schema (Auto-Created)

The application creates these tables automatically. **You don't need to run these manually**, but this shows you what will be created:

### Table: `activity_sessions`

Stores individual continuous sessions with applications.

```sql
CREATE TABLE activity_sessions (
    id SERIAL PRIMARY KEY,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    app_class VARCHAR(255) NOT NULL,
    window_title TEXT,
    duration_seconds INTEGER NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT valid_duration CHECK (duration_seconds >= 0),
    CONSTRAINT valid_time_range CHECK (end_time >= start_time)
);

-- Indexes for efficient queries
CREATE INDEX idx_sessions_app_class ON activity_sessions(app_class);
CREATE INDEX idx_sessions_start_time ON activity_sessions(start_time);
CREATE INDEX idx_sessions_end_time ON activity_sessions(end_time);
CREATE INDEX idx_sessions_app_time ON activity_sessions(app_class, start_time);
```

### Table: `activity_summaries`

Stores aggregated activity summaries (same data sent to RescueTime).

```sql
CREATE TABLE activity_summaries (
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

-- Indexes for efficient queries
CREATE INDEX idx_summaries_app_class ON activity_summaries(app_class);
CREATE INDEX idx_summaries_first_seen ON activity_summaries(first_seen);
CREATE INDEX idx_summaries_last_seen ON activity_summaries(last_seen);
CREATE INDEX idx_summaries_submitted_at ON activity_summaries(submitted_at);
```

## Connection String Options

### Basic Format
```
postgres://username:password@hostname:port/database?options
```

### Common Connection Strings

**Local development (no SSL):**
```bash
postgres://rescuetime_user:password@localhost/rescuetime?sslmode=disable
```

**Local development (with SSL):**
```bash
postgres://rescuetime_user:password@localhost/rescuetime?sslmode=require
```

**Remote server:**
```bash
postgres://rescuetime_user:password@192.168.1.100:5432/rescuetime?sslmode=require
```

**Unix socket (local only):**
```bash
postgres://rescuetime_user:password@/rescuetime?host=/var/run/postgresql
```

**With connection timeout:**
```bash
postgres://rescuetime_user:password@localhost/rescuetime?sslmode=disable&connect_timeout=10
```

### SSL Mode Options

- `sslmode=disable` - No SSL (local development only - **not secure for remote**)
- `sslmode=require` - Require SSL (recommended for any remote connection)
- `sslmode=verify-ca` - Require SSL and verify certificate authority
- `sslmode=verify-full` - Require SSL and verify hostname

## Useful SQL Queries

Once data is being collected, you can analyze it with SQL:

### Today's Activity Summary
```sql
SELECT 
    app_class,
    COUNT(*) as sessions,
    SUM(duration_seconds)/3600.0 as hours,
    MIN(start_time) as first_use,
    MAX(end_time) as last_use
FROM activity_sessions
WHERE start_time >= CURRENT_DATE
GROUP BY app_class
ORDER BY hours DESC;
```

### Activity by Hour of Day
```sql
SELECT 
    EXTRACT(HOUR FROM start_time) as hour,
    COUNT(*) as sessions,
    SUM(duration_seconds)/3600.0 as hours
FROM activity_sessions
WHERE start_time >= CURRENT_DATE
GROUP BY hour
ORDER BY hour;
```

### Last 7 Days Trend
```sql
SELECT 
    DATE(start_time) as date,
    app_class,
    SUM(duration_seconds)/3600.0 as hours
FROM activity_sessions
WHERE start_time >= CURRENT_DATE - INTERVAL '7 days'
GROUP BY date, app_class
ORDER BY date DESC, hours DESC;
```

### Most Recent Sessions
```sql
SELECT 
    app_class,
    window_title,
    start_time,
    end_time,
    duration_seconds/60 as minutes
FROM activity_sessions
ORDER BY start_time DESC
LIMIT 20;
```

### Top 10 Apps This Week
```sql
SELECT 
    app_class,
    COUNT(*) as sessions,
    SUM(duration_seconds)/3600.0 as hours,
    ROUND(AVG(duration_seconds)/60.0, 1) as avg_session_minutes
FROM activity_sessions
WHERE start_time >= DATE_TRUNC('week', CURRENT_DATE)
GROUP BY app_class
ORDER BY hours DESC
LIMIT 10;
```

### Submission History
```sql
SELECT 
    app_class,
    total_duration_seconds/60 as total_minutes,
    session_count,
    first_seen,
    last_seen,
    submitted_at
FROM activity_summaries
ORDER BY submitted_at DESC
LIMIT 20;
```

## Backup and Restore

### Backup Database
```bash
# Full database backup
pg_dump -U rescuetime_user -d rescuetime > rescuetime_backup_$(date +%Y%m%d).sql

# Compressed backup
pg_dump -U rescuetime_user -d rescuetime | gzip > rescuetime_backup_$(date +%Y%m%d).sql.gz

# Backup specific table
pg_dump -U rescuetime_user -d rescuetime -t activity_sessions > sessions_backup.sql
```

### Restore Database
```bash
# Restore from backup
psql -U rescuetime_user -d rescuetime < rescuetime_backup_20251031.sql

# Restore from compressed backup
gunzip -c rescuetime_backup_20251031.sql.gz | psql -U rescuetime_user -d rescuetime
```

### Automated Backups
Create a cron job for daily backups:

```bash
# Edit crontab
crontab -e

# Add this line for daily backup at 2 AM
0 2 * * * pg_dump -U rescuetime_user -d rescuetime | gzip > /home/yourusername/backups/rescuetime_$(date +\%Y\%m\%d).sql.gz
```

## Troubleshooting

### Connection Refused
```
Error: failed to connect to database: connection refused
```

**Solution:**
```bash
# Check if PostgreSQL is running
sudo systemctl status postgresql

# Start PostgreSQL
sudo systemctl start postgresql

# Enable auto-start on boot
sudo systemctl enable postgresql
```

### Authentication Failed
```
Error: FATAL: password authentication failed for user "rescuetime_user"
```

**Solution:**
```bash
# Reset password
sudo -u postgres psql -c "ALTER USER rescuetime_user WITH PASSWORD 'new_password';"
```

### Database Does Not Exist
```
Error: FATAL: database "rescuetime" does not exist
```

**Solution:**
```bash
sudo -u postgres createdb -O rescuetime_user rescuetime
```

### Permission Denied
```
Error: permission denied for table activity_sessions
```

**Solution:**
```sql
-- Run in psql as postgres user
sudo -u postgres psql rescuetime

-- Grant permissions
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO rescuetime_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO rescuetime_user;
```

### Port Already in Use
```
Error: could not bind IPv4 address "127.0.0.1": Address already in use
```

**Solution:**
```bash
# Check what's using port 5432
sudo lsof -i :5432

# If another PostgreSQL instance is running, stop it
sudo systemctl stop postgresql@<version>
```

## Security Best Practices

1. **Use Strong Passwords**
   ```bash
   # Generate a secure password
   openssl rand -base64 32
   ```

2. **Never Commit Credentials**
   - Add `.env` to `.gitignore` (already done)
   - Use environment variables or secret management tools

3. **Restrict Network Access**
   Edit `/etc/postgresql/*/main/pg_hba.conf`:
   ```
   # Local connections only (default)
   local   all             all                                     peer
   host    rescuetime      rescuetime_user     127.0.0.1/32        scram-sha-256
   ```

4. **Use SSL for Remote Connections**
   - Always use `sslmode=require` or higher for remote databases
   - Never use `sslmode=disable` for internet-facing databases

5. **Regular Backups**
   - Automate daily backups
   - Store backups in a different location
   - Test restore process periodically

6. **Limit User Permissions**
   - Don't use the `postgres` superuser for the application
   - Use dedicated user with minimal required permissions

## Performance Optimization

### Check Index Usage
```sql
SELECT 
    schemaname,
    tablename,
    indexname,
    idx_scan as index_scans,
    idx_tup_read as tuples_read,
    idx_tup_fetch as tuples_fetched
FROM pg_stat_user_indexes
ORDER BY idx_scan DESC;
```

### Check Table Sizes
```sql
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

### Vacuum and Analyze
```sql
-- Analyze tables for query optimization
ANALYZE activity_sessions;
ANALYZE activity_summaries;

-- Vacuum to reclaim space
VACUUM ANALYZE;
```

## Integration with Monitoring Tools

### Grafana Dashboard
You can create visualizations by connecting Grafana to PostgreSQL:

1. Install Grafana: https://grafana.com/docs/grafana/latest/setup-grafana/installation/
2. Add PostgreSQL data source
3. Create dashboards with activity data

### Example Grafana Query (Time Series)
```sql
SELECT 
    DATE_TRUNC('hour', start_time) as time,
    app_class,
    SUM(duration_seconds)/3600.0 as hours
FROM activity_sessions
WHERE $__timeFilter(start_time)
GROUP BY time, app_class
ORDER BY time;
```

## Next Steps

1. ✅ Create database and user
2. ✅ Set `POSTGRES_CONNECTION_STRING` in `.env`
3. ✅ Run `./active-window -track -submit -postgres "..."`
4. ✅ Verify data is being stored: `psql -U rescuetime_user -d rescuetime -c "SELECT COUNT(*) FROM activity_sessions;"`
5. ✅ Set up automated backups
6. ✅ Create custom SQL queries for your analytics needs

For more information, see [`postgres/README.md`](../postgres/README.md).
