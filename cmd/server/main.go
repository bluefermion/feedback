// Package main is the entry point for the Feedback Server application.
//
// EDUCATIONAL CONTEXT:
// In Go, the 'main' package is special. It defines a standalone executable program,
// not a library. The 'main' function within this package is where execution begins.
//
// This server is designed to:
// 1. Load configuration from environment variables (12-Factor App methodology).
// 2. Initialize a persistent storage layer (SQLite).
// 3. Set up HTTP routing (using Go's standard net/http mux).
// 4. Start listening for incoming network requests.
//
// ARCHITECTURE NOTE:
// We use a clean architecture approach where:
// - 'cmd/server' contains the application assembly and startup logic.
// - 'internal/handler' contains the HTTP transport layer logic.
// - 'internal/repository' contains the data persistence logic.
// - 'internal/model' contains the domain data structures.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/bluefermion/feedback/internal/handler"
	"github.com/bluefermion/feedback/internal/repository"
	"github.com/joho/godotenv"
)

// main is the entry point of the application.
// It orchestrates the startup sequence: config -> db -> handlers -> router -> server.
func main() {
	// -------------------------------------------------------------------------
	// 1. CONFIGURATION LOADING
	// -------------------------------------------------------------------------

	// We use 'godotenv' to load environment variables from a .env file into the process environment.
	// This is excellent for local development, allowing developers to set secrets and config
	// in a file that is git-ignored (to prevent accidental leaks).
	// In production, these variables would typically be set by the orchestration platform (Docker, K8s).
	if err := godotenv.Load(); err != nil {
		// It's acceptable for .env to be missing (e.g., in production or CI/CD).
		// We only log this if debugging is enabled to reduce noise in production logs.
		if os.Getenv("COMMON_DEBUG") == "true" {
			log.Printf("No .env file found, using environment variables")
		}
	}

	// Retrieve the server port. We default to "8080" if not specified.
	// Hardcoding defaults is a good practice for developer experience (DX),
	// allowing the app to run "out of the box" without extensive configuration.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Determine where the SQLite database file should be stored.
	// SQLite is a serverless, file-based database. It's perfect for this demo/standalone
	// application because it requires no separate database process to manage.
	dbPath := os.Getenv("FEEDBACK_DB_PATH")
	if dbPath == "" {
		dbPath = "feedback.db"
	}

	// -------------------------------------------------------------------------
	// 2. DEPENDENCY INJECTION & INITIALIZATION
	// -------------------------------------------------------------------------

	// We initialize the SQLite repository. This object encapsulates all database interactions.
	// By initializing it here and passing it to the handler, we are performing "Dependency Injection".
	// This makes testing easier (we could pass a mock repository) and decouples the handler from
	// the specific database implementation details.
	repo, err := repository.NewSQLiteRepository(dbPath)
	if err != nil {
		// If the database fails to start, the application cannot function.
		// log.Fatalf logs the message and immediately exits with status code 1.
		log.Fatalf("Failed to initialize database: %v", err)
	}
	// 'defer' schedules the Close method to run when main() exits.
	// This ensures database connections are cleaned up gracefully, even if we exit due to a panic.
	defer repo.Close()

	// Initialize the main HTTP handler.
	// We pass the repository and the templates filesystem (os.DirFS) to it.
	// Using os.DirFS allows us to load templates from the disk in development.
	// In production, we could swap this with embed.FS without changing the handler logic.
	templateFS := os.DirFS("templates")
	feedbackHandler := handler.NewFeedbackHandler(repo, templateFS)

	// Initialize the error handler with OpenCode self-healing integration.
	// This provides beautiful error pages and triggers automated analysis.
	openCodeAnalyzer := handler.NewOpenCodeAnalyzer(handler.OpenCodeConfig{
		ContainerName: "opencode-selfhealing",
		Enabled:       os.Getenv("OPENCODE_ENABLED") == "true",
	})
	errorHandler := handler.NewErrorHandler(templateFS, openCodeAnalyzer)

	// -------------------------------------------------------------------------
	// 3. ROUTER SETUP
	// -------------------------------------------------------------------------

	// We use Go's standard library ServeMux.
	// As of Go 1.22, it supports method matching (e.g., "GET /path") and wildcards!
	mux := http.NewServeMux()

	// -- System Routes --

	// Root endpoint: identifies the service. Good for sanity checks.
	mux.HandleFunc("GET /", handleRoot)
	// Health check: used by load balancers or orchestrators (like K8s) to know if the app is alive.
	mux.HandleFunc("GET /health", handleHealth)

	// -- API Routes (JSON) --

	// Submit new feedback. RESTful convention: POST to the resource collection creates a new item.
	mux.HandleFunc("POST /api/feedback", feedbackHandler.HandleSubmit)
	// List feedback. RESTful convention: GET on collection retrieves list.
	mux.HandleFunc("GET /api/feedback", feedbackHandler.HandleList)
	// Get single feedback. RESTful convention: GET on specific ID.
	mux.HandleFunc("GET /api/feedback/{id}", feedbackHandler.HandleGet)
	// Status endpoint for the self-healing system (showing background worker state).
	mux.HandleFunc("GET /api/selfhealing/status", feedbackHandler.HandleSelfHealingStatus)

	// -- Static Assets --

	// Serve static files (JS, CSS, Images) from the "widget" directory.
	// http.StripPrefix is needed because the request comes in as "/static/js/..."
	// but the file on disk is just "js/...".
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("widget"))))

	// -- UI Routes (HTML) --

	// Demo page: A simple HTML page to show the widget in action.
	mux.HandleFunc("GET /demo", feedbackHandler.HandleDemo)

	// Admin Interface: Pages to view the collected feedback.
	mux.HandleFunc("GET /feedback", feedbackHandler.HandleFeedbackList)
	mux.HandleFunc("GET /feedback/{id}", feedbackHandler.HandleFeedbackDetail)

	// Error Page: Preview and trigger routes for the beautiful error page.
	mux.HandleFunc("GET /error", errorHandler.HandleErrorPreview)
	mux.HandleFunc("GET /error/trigger", errorHandler.HandleFakeError)

	// -------------------------------------------------------------------------
	// 4. MIDDLEWARE & SERVER START
	// -------------------------------------------------------------------------

	// Wrap the mux in our logging middleware.
	// Middleware pattern: Intercepts every request to perform cross-cutting concerns
	// (logging, auth, metrics) before passing control to the actual handler.
	handler := loggingMiddleware(mux)

	// Log helpful information for the developer.
	log.Printf("Starting server on :%s", port)
	log.Printf("Database: %s", dbPath)
	log.Printf("Demo: http://localhost:%s/demo", port)
	log.Printf("Feedback list: http://localhost:%s/feedback", port)
	log.Printf("Error page preview: http://localhost:%s/error", port)

	// Start the blocking server loop.
	// ListenAndServe will only return if there's an error (like port already in use).
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// handleRoot returns basic metadata about the service.
// This is useful for service discovery or manual verification.
func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"service": "feedback",
		"version": "0.1.0",
		"status":  "ok",
	})
}

// handleHealth is a standard health check endpoint.
// In a real deployment, this might check DB connectivity before returning "healthy".
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// loggingMiddleware logs the HTTP method, path, and duration of every request.
//
// EDUCATIONAL CONTEXT:
// Middleware in Go is typically a function that takes an http.Handler and returns an http.Handler.
// It wraps the 'next' handler, allowing code to run before and after the inner handler.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Call the next handler in the chain
		next.ServeHTTP(w, r)
		// Log after the request has been served
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
