// Package model defines the domain entities and data transfer objects (DTOs) for the system.
//
// EDUCATIONAL CONTEXT:
// In Go clean architecture or layered architecture, the 'model' package (sometimes called 'domain')
// contains the core business objects. These structs should ideally be free of
// database-specific or transport-specific logic (though JSON tags are a common pragmatic exception).
//
// We use 'json' struct tags to define how these objects map to/from JSON when interacting
// with the frontend API.
package model

import "time"

// Feedback represents the core domain entity for a user feedback submission.
// It aggregates user input, system metadata, and subsequent analysis results.
type Feedback struct {
	// ID is the unique identifier for the record.
	// We use int64 for database compatibility (SQLite/Postgres auto-increment IDs).
	ID int64 `json:"id"`

	// User info (optional, as feedback can be anonymous).
	UserEmail string `json:"userEmail,omitempty"`
	UserName  string `json:"userName,omitempty"`

	// Core feedback content provided by the user.
	Title       string `json:"title"`
	Description string `json:"description"`
	// Type categorizes the feedback: 'bug', 'feature', 'improvement', etc.
	// This helps with triage and automatic routing (e.g., bugs go to Jira).
	Type string `json:"type"`

	// Contextual data: Where was the user when they submitted feedback?
	URL       string `json:"url,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`

	// ---------------------------------------------------------------------
	// Display Metrics
	// ---------------------------------------------------------------------
	// These fields help reproduce visual bugs by providing precise viewport info.
	ScreenWidth      int     `json:"screenWidth,omitempty"`
	ScreenHeight     int     `json:"screenHeight,omitempty"`
	ViewportWidth    int     `json:"viewportWidth,omitempty"`
	ViewportHeight   int     `json:"viewportHeight,omitempty"`
	ScreenResolution string  `json:"screenResolution,omitempty"`
	PixelRatio       float64 `json:"pixelRatio,omitempty"` // Essential for Retina/High-DPI displays

	// ---------------------------------------------------------------------
	// Platform Info
	// ---------------------------------------------------------------------
	// Parsed from User-Agent or provided by the browser API.
	BrowserName    string `json:"browserName,omitempty"`
	BrowserVersion string `json:"browserVersion,omitempty"`
	OS             string `json:"os,omitempty"`
	DeviceType     string `json:"deviceType,omitempty"` // desktop, mobile, tablet
	IsMobile       bool   `json:"isMobile,omitempty"`
	Language       string `json:"language,omitempty"`
	Timezone       string `json:"timezone,omitempty"`

	// ---------------------------------------------------------------------
	// Artifacts (Blobs)
	// ---------------------------------------------------------------------
	// Screenshot is a Base64 encoded string of the image.
	// In a production system, you might store this in S3/BlobStorage and just keep a URL here.
	Screenshot string `json:"screenshot,omitempty"`

	// Annotations represents the JSON string of drawing data on the screenshot (lines, boxes).
	Annotations string `json:"annotations,omitempty"`

	// ConsoleLogs is a JSON string containing the browser console history (log/warn/error)
	// captured just before submission. Vital for debugging frontend crashes.
	ConsoleLogs string `json:"consoleLogs,omitempty"`

	// Journey could track the user's last N interactions/clicks (not fully implemented in this demo).
	Journey string `json:"journey,omitempty"`

	// ---------------------------------------------------------------------
	// Triage & Workflow
	// ---------------------------------------------------------------------
	// These fields track the lifecycle of the feedback item.
	Status         string `json:"status,omitempty"`   // open, in_progress, resolved, closed
	Priority       string `json:"priority,omitempty"` // high, medium, low
	Category       string `json:"category,omitempty"` // e.g., 'ui', 'backend', 'performance'
	EffortEstimate string `json:"effortEstimate,omitempty"`

	// ---------------------------------------------------------------------
	// AI Analysis Outputs (Self-Healing)
	// ---------------------------------------------------------------------
	// These fields are populated by the LLM analysis process.
	Analysis          string `json:"analysis,omitempty"`          // Markdown report of the bug analysis
	PredictedPriority string `json:"predictedPriority,omitempty"` // AI-suggested priority
	PredictedCategory string `json:"predictedCategory,omitempty"` // AI-suggested category
	PredictedEffort   string `json:"predictedEffort,omitempty"`   // AI-suggested fix effort

	// ---------------------------------------------------------------------
	// Timestamps
	// ---------------------------------------------------------------------
	// Standard audit timestamps.
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// FeedbackRequest represents the structure of the JSON payload sent by the frontend widget.
// This is a "Data Transfer Object" (DTO). It separates the API contract from the internal database model.
// While similar to 'Feedback', decoupling them allows the API and Database to evolve independently.
type FeedbackRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Type        string `json:"type"`

	// Screenshot handling:
	// 'ScreenshotUrl' is the modern Data URI field.
	// 'Screenshot' is kept for legacy compatibility if needed.
	Screenshot    string `json:"screenshot,omitempty"`
	ScreenshotUrl string `json:"screenshotUrl,omitempty"`

	Annotations string `json:"annotations,omitempty"`
	ConsoleLogs string `json:"consoleLogs,omitempty"`

	// Metadata captures all the unstructured browser/device info map.
	// We use map[string]interface{} to be flexible to whatever the JS widget sends.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// FeedbackResponse defines the standard success response for the API.
type FeedbackResponse struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

// ErrorResponse defines the standard error structure.
// Structured errors help the frontend display meaningful messages to the user.
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}