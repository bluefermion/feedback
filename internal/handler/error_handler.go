// Package handler provides HTTP handlers for the Feedback Service.
//
// error_handler.go provides centralized error handling with a beautiful error page
// and integration with the OpenCode self-healing system.
package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"
)

var (
	errorTemplate     *template.Template
	errorTemplateOnce sync.Once
	errorTemplateErr  error
)

// ErrorPageData holds data for the error page template
type ErrorPageData struct {
	ErrorID      string
	ErrorMessage string
	ImageURL     string
}

// ErrorHandler handles errors with the beautiful error page and OpenCode integration
type ErrorHandler struct {
	templateFS fs.FS
	analyzer   ErrorAnalyzer
}

// ErrorAnalyzer is the interface for triggering error analysis (OpenCode integration)
type ErrorAnalyzer interface {
	// AnalyzeError triggers async analysis of an error
	// Returns immediately, analysis happens in background
	AnalyzeError(ctx ErrorContext) error
}

// ErrorContext contains all context needed for error analysis
type ErrorContext struct {
	ErrorID       string
	ErrorMessage  string
	StackTrace    string
	RequestPath   string
	RequestMethod string
	HandlerFile   string
	HandlerFunc   string
	Timestamp     time.Time
	UserAgent     string
}

// NewErrorHandler creates a new ErrorHandler
func NewErrorHandler(templateFS fs.FS, analyzer ErrorAnalyzer) *ErrorHandler {
	return &ErrorHandler{
		templateFS: templateFS,
		analyzer:   analyzer,
	}
}

// loadErrorTemplate loads the error template once
func (h *ErrorHandler) loadErrorTemplate() error {
	errorTemplateOnce.Do(func() {
		errorTemplate, errorTemplateErr = template.ParseFS(h.templateFS, "error.html")
		if errorTemplateErr != nil {
			log.Printf("Failed to load error template: %v", errorTemplateErr)
		}
	})
	return errorTemplateErr
}

// generateErrorID creates a unique error ID for tracking
func generateErrorID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("ERR-%d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("OC-%s", hex.EncodeToString(b)[:12])
}

// HandleError handles an error by logging, triggering analysis, and rendering the error page
func (h *ErrorHandler) HandleError(w http.ResponseWriter, r *http.Request, statusCode int, err error, handlerFile, handlerFunc string) {
	errorID := generateErrorID()

	var errorMsg string
	if err != nil {
		errorMsg = err.Error()
	}

	// Log the error
	log.Printf("[%s] ERROR in %s.%s: %v (path: %s)", errorID, handlerFile, handlerFunc, err, r.URL.Path)

	// Build error context for analysis
	ctx := ErrorContext{
		ErrorID:       errorID,
		ErrorMessage:  errorMsg,
		RequestPath:   r.URL.Path,
		RequestMethod: r.Method,
		HandlerFile:   handlerFile,
		HandlerFunc:   handlerFunc,
		Timestamp:     time.Now(),
		UserAgent:     r.UserAgent(),
	}

	// Trigger async analysis if analyzer is configured
	if h.analyzer != nil {
		go func() {
			if analyzeErr := h.analyzer.AnalyzeError(ctx); analyzeErr != nil {
				log.Printf("[%s] Failed to trigger analysis: %v", errorID, analyzeErr)
			}
		}()
	}

	// Render the error page
	h.RenderErrorPage(w, errorID, errorMsg, statusCode)
}

// RenderErrorPage renders the beautiful error page
func (h *ErrorHandler) RenderErrorPage(w http.ResponseWriter, errorID, errorMessage string, statusCode int) {
	if err := h.loadErrorTemplate(); err != nil {
		// Fallback to plain text if template fails
		http.Error(w, fmt.Sprintf("Error ID: %s - OpenCode is working on a fix.", errorID), statusCode)
		return
	}

	data := ErrorPageData{
		ErrorID:      errorID,
		ErrorMessage: errorMessage,
		ImageURL:     "", // Will use robot emoji by default
	}

	var buf bytes.Buffer
	if err := errorTemplate.Execute(&buf, data); err != nil {
		log.Printf("Failed to execute error template: %v", err)
		http.Error(w, fmt.Sprintf("Error ID: %s - OpenCode is working on a fix.", errorID), statusCode)
		return
	}

	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write(buf.Bytes())
}

// RenderErrorPagePreview renders the error page for preview without triggering analysis
func (h *ErrorHandler) RenderErrorPagePreview(w http.ResponseWriter, errorID string) {
	if err := h.loadErrorTemplate(); err != nil {
		http.Error(w, "Error template not available", http.StatusInternalServerError)
		return
	}

	data := ErrorPageData{
		ErrorID:      errorID,
		ErrorMessage: "", // No error message for preview
		ImageURL:     "",
	}

	var buf bytes.Buffer
	if err := errorTemplate.Execute(&buf, data); err != nil {
		http.Error(w, "Failed to render error page", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Use 200 OK for preview (not 500)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// HandleErrorPreview is the HTTP handler for /error route (preview mode)
func (h *ErrorHandler) HandleErrorPreview(w http.ResponseWriter, r *http.Request) {
	// Generate a sample error ID for preview
	errorID := generateErrorID()
	h.RenderErrorPagePreview(w, errorID)
}

// HandleFakeError is the HTTP handler for /error/trigger route (simulates an error)
func (h *ErrorHandler) HandleFakeError(w http.ResponseWriter, r *http.Request) {
	// Simulate an error to demonstrate the full flow
	fakeErr := fmt.Errorf("simulated error for demonstration: database connection timeout after 30s")
	h.HandleError(w, r, http.StatusInternalServerError, fakeErr, "handler/error_handler.go", "HandleFakeError")
}
