// Package repository implements the data persistence layer using SQLite.
//
// EDUCATIONAL CONTEXT:
// The Repository pattern acts as a mediator between the domain layer (business logic)
// and data mapping layers (database). It isolates the application from details of
// data access logic.
//
// Advantages:
// 1. Decouples business logic from database schema.
// 2. Makes unit testing easier (can mock the repository interface).
// 3. Centralizes data access policies (e.g., query building, error handling).
package repository

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/bluefermion/feedback/internal/model"
	// We use the modernc.org/sqlite driver because it's a pure Go implementation (no CGO required).
	// This makes cross-compilation and deployment (e.g., in minimal Docker containers) extremely easy.
	_ "modernc.org/sqlite"
)

// SQLiteRepository encapsulates the SQL database connection.
// It implements the interface expected by the handlers (though we use struct directly here for simplicity).
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository initializes the database connection and ensures the schema exists.
func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	// Open connection to the SQLite file.
	// If the file doesn't exist, the driver will create it.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// PERFORMANCE TIP: WAL (Write-Ahead Logging) Mode.
	// By default, SQLite uses a rollback journal which can be slower for concurrent access.
	// WAL mode allows simultaneous readers and writers, significantly improving concurrency.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	repo := &SQLiteRepository{db: db}

	// Auto-migration on startup.
	// For small projects/demos, checking/creating tables on startup is convenient.
	// For large production systems, use dedicated migration tools (like golang-migrate or Goose).
	if err := repo.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return repo, nil
}

// migrate handles schema evolution. It runs SQL DDL statements to ensure tables exist.
func (r *SQLiteRepository) migrate() error {
	// We define the full schema here.
	// SQLite's "IF NOT EXISTS" clause is idempotent, allowing this to run safely on every boot.
	query := `
	CREATE TABLE IF NOT EXISTS feedback (
		-- Primary Key: Auto-incrementing integer ID.
		id INTEGER PRIMARY KEY AUTOINCREMENT,

		-- User Identity
		user_email TEXT,
		user_name TEXT,

		-- Core Content
		title TEXT NOT NULL,
		description TEXT NOT NULL,
		type TEXT DEFAULT 'other',

		-- Context
		url TEXT,
		user_agent TEXT,

		-- Device Metrics (useful for UI debugging)
		screen_width INTEGER,
		screen_height INTEGER,
		viewport_width INTEGER,
		viewport_height INTEGER,
		screen_resolution TEXT,
		pixel_ratio REAL,

		-- Platform Info
		browser_name TEXT,
		browser_version TEXT,
		os TEXT,
		device_type TEXT,
		is_mobile BOOLEAN DEFAULT FALSE,
		language TEXT,
		timezone TEXT,

		-- Large Text Blobs (JSON or Base64 content)
		screenshot TEXT,
		annotations TEXT,
		console_logs TEXT,
		journey TEXT,

		-- Triage Status
		status TEXT DEFAULT 'open',
		priority TEXT,
		category TEXT,
		effort_estimate TEXT,

		-- AI/LLM Analysis Results
		analysis TEXT,
		predicted_priority TEXT,
		predicted_category TEXT,
		predicted_effort TEXT,

		-- Audit Timestamps
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	-- Indexes for common query patterns (Filtering by status, type, or date).
	CREATE INDEX IF NOT EXISTS idx_feedback_status ON feedback(status);
	CREATE INDEX IF NOT EXISTS idx_feedback_type ON feedback(type);
	CREATE INDEX IF NOT EXISTS idx_feedback_created_at ON feedback(created_at);
	`
	_, err := r.db.Exec(query)
	return err
}

// Create inserts a new feedback record into the database.
// It sets the timestamps and default values before insertion.
func (r *SQLiteRepository) Create(feedback *model.Feedback) (int64, error) {
	// Always set server-side timestamps. Never trust client timestamps.
	now := time.Now().UTC()
	feedback.CreatedAt = now
	feedback.UpdatedAt = now

	// Set defaults if missing
	if feedback.Status == "" {
		feedback.Status = "open"
	}
	if feedback.Type == "" {
		feedback.Type = "other"
	}

	// SQL Parameterized Query.
	// SECURITY CRITICAL: Always use ? placeholders. Never concatenate strings into SQL queries.
	// String concatenation leads to SQL Injection vulnerabilities.
	query := `
	INSERT INTO feedback (
		user_email, user_name, title, description, type, url, user_agent,
		screen_width, screen_height, viewport_width, viewport_height,
		screen_resolution, pixel_ratio,
		browser_name, browser_version, os, device_type, is_mobile,
		language, timezone,
		screenshot, annotations, console_logs, journey,
		status, priority, category, effort_estimate,
		analysis, predicted_priority, predicted_category, predicted_effort,
		created_at, updated_at
	) VALUES (
		?, ?, ?, ?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?,
		?, ?, ?, ?, ?,
		?, ?,
		?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?
	)`

	// Execute the statement with arguments in the exact order of the placeholders.
	result, err := r.db.Exec(query,
		feedback.UserEmail, feedback.UserName, feedback.Title, feedback.Description,
		feedback.Type, feedback.URL, feedback.UserAgent,
		feedback.ScreenWidth, feedback.ScreenHeight, feedback.ViewportWidth, feedback.ViewportHeight,
		feedback.ScreenResolution, feedback.PixelRatio,
		feedback.BrowserName, feedback.BrowserVersion, feedback.OS, feedback.DeviceType, feedback.IsMobile,
		feedback.Language, feedback.Timezone,
		feedback.Screenshot, feedback.Annotations, feedback.ConsoleLogs, feedback.Journey,
		feedback.Status, feedback.Priority, feedback.Category, feedback.EffortEstimate,
		feedback.Analysis, feedback.PredictedPriority, feedback.PredictedCategory, feedback.PredictedEffort,
		feedback.CreatedAt, feedback.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert feedback: %w", err)
	}

	// Retrieve the auto-generated ID from SQLite.
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get insert ID: %w", err)
	}

	feedback.ID = id
	return id, nil
}

// GetByID retrieves a single feedback record.
func (r *SQLiteRepository) GetByID(id int64) (*model.Feedback, error) {
	// Explicitly listing columns is better than 'SELECT *' because:
	// 1. It's resilient to schema changes (adding columns won't break scanning).
	// 2. It's explicit about what data is being retrieved.
	query := `
	SELECT
		id, user_email, user_name, title, description, type, url, user_agent,
		screen_width, screen_height, viewport_width, viewport_height,
		screen_resolution, pixel_ratio,
		browser_name, browser_version, os, device_type, is_mobile,
		language, timezone,
		screenshot, annotations, console_logs, journey,
		status, priority, category, effort_estimate,
		analysis, predicted_priority, predicted_category, predicted_effort,
		created_at, updated_at
	FROM feedback WHERE id = ?`

	feedback := &model.Feedback{}
	// QueryRow returns a single row. We use Scan to copy column values into struct fields.
	err := r.db.QueryRow(query, id).Scan(
		&feedback.ID, &feedback.UserEmail, &feedback.UserName,
		&feedback.Title, &feedback.Description, &feedback.Type,
		&feedback.URL, &feedback.UserAgent,
		&feedback.ScreenWidth, &feedback.ScreenHeight,
		&feedback.ViewportWidth, &feedback.ViewportHeight,
		&feedback.ScreenResolution, &feedback.PixelRatio,
		&feedback.BrowserName, &feedback.BrowserVersion,
		&feedback.OS, &feedback.DeviceType, &feedback.IsMobile,
		&feedback.Language, &feedback.Timezone,
		&feedback.Screenshot, &feedback.Annotations,
		&feedback.ConsoleLogs, &feedback.Journey,
		&feedback.Status, &feedback.Priority, &feedback.Category, &feedback.EffortEstimate,
		&feedback.Analysis, &feedback.PredictedPriority,
		&feedback.PredictedCategory, &feedback.PredictedEffort,
		&feedback.CreatedAt, &feedback.UpdatedAt,
	)

	// Handle the "record not found" case gracefully.
	if err == sql.ErrNoRows {
		return nil, nil // Not an error, just no data.
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get feedback: %w", err)
	}

	return feedback, nil
}

// List returns a paginated list of feedback, ordered by newest first.
// Pagination is crucial for performance to avoid loading thousands of records into memory.
func (r *SQLiteRepository) List(limit, offset int) ([]*model.Feedback, error) {
	// Safety check: protect against huge allocations or negative limits.
	if limit <= 0 {
		limit = 50
	}

	query := `
	SELECT
		id, user_email, user_name, title, description, type, url, user_agent,
		screen_width, screen_height, viewport_width, viewport_height,
		screen_resolution, pixel_ratio,
		browser_name, browser_version, os, device_type, is_mobile,
		language, timezone,
		screenshot, annotations, console_logs, journey,
		status, priority, category, effort_estimate,
		analysis, predicted_priority, predicted_category, predicted_effort,
		created_at, updated_at
	FROM feedback
	ORDER BY created_at DESC
	LIMIT ? OFFSET ?`

	rows, err := r.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list feedback: %w", err)
	}
	// Always close rows to release the database connection back to the pool.
	defer rows.Close()

	var feedbacks []*model.Feedback
	for rows.Next() {
		feedback := &model.Feedback{}
		// Scan MUST match the column order in the SELECT statement.
		err := rows.Scan(
			&feedback.ID, &feedback.UserEmail, &feedback.UserName,
			&feedback.Title, &feedback.Description, &feedback.Type,
			&feedback.URL, &feedback.UserAgent,
			&feedback.ScreenWidth, &feedback.ScreenHeight,
			&feedback.ViewportWidth, &feedback.ViewportHeight,
			&feedback.ScreenResolution, &feedback.PixelRatio,
			&feedback.BrowserName, &feedback.BrowserVersion,
			&feedback.OS, &feedback.DeviceType, &feedback.IsMobile,
			&feedback.Language, &feedback.Timezone,
			&feedback.Screenshot, &feedback.Annotations,
			&feedback.ConsoleLogs, &feedback.Journey,
			&feedback.Status, &feedback.Priority, &feedback.Category, &feedback.EffortEstimate,
			&feedback.Analysis, &feedback.PredictedPriority,
			&feedback.PredictedCategory, &feedback.PredictedEffort,
			&feedback.CreatedAt, &feedback.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan feedback: %w", err)
		}
		feedbacks = append(feedbacks, feedback)
	}

	// Check for errors that might have occurred during iteration.
	return feedbacks, rows.Err()
}

// UpdateAnalysis saves the LLM analysis result for a specific feedback item.
// This is used by the asynchronous self-healing worker.
func (r *SQLiteRepository) UpdateAnalysis(id int64, analysis string) error {
	query := `UPDATE feedback SET analysis = ?, updated_at = ? WHERE id = ?`
	_, err := r.db.Exec(query, analysis, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to update analysis: %w", err)
	}
	return nil
}

// Close terminates the database connection.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}
