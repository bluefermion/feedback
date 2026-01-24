// Package handler implements the HTTP transport layer for the application.
//
// EDUCATIONAL CONTEXT:
// In clean architecture, "Handlers" (or Controllers) are responsible for:
// 1. Parsing incoming HTTP requests (JSON bodies, query params, path vars).
// 2. Validating input data.
// 3. Invoking the appropriate business logic (via repositories or services).
// 4. Formatting the response (JSON serialization, status codes).
//
// They should NOT contain core business rules or SQL queries.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/bluefermion/feedback/internal/model"
	"github.com/bluefermion/feedback/internal/repository"
	"github.com/bluefermion/feedback/internal/selfhealing"
)

// FeedbackHandler groups all methods related to feedback operations.
// It holds references to its dependencies (repository, self-healing service, templates).
type FeedbackHandler struct {
	repo     *repository.SQLiteRepository
	selfheal *selfhealing.Trigger
	// templates maps a page name (e.g., "demo") to its fully parsed template instance.
	// This approach (Pre-computation/Caching) is thread-safe and performant.
	templates map[string]*template.Template
}

// NewFeedbackHandler is the constructor for FeedbackHandler.
// It performs dependency injection and initialization logic.
func NewFeedbackHandler(repo *repository.SQLiteRepository, templateFS fs.FS) *FeedbackHandler {
	// Initialize self-healing triggers based on environment configuration.
	// This demonstrates "feature flagging" via configuration.
	config := selfhealing.DefaultConfig()
	var trigger *selfhealing.Trigger

	if config.Enabled {
		if err := config.Validate(); err != nil {
			log.Printf("Self-healing config invalid, disabling: %v", err)
		} else {
			trigger = selfhealing.NewTrigger(config)
			if config.Mode == "opencode" {
				log.Printf("Self-healing enabled (opencode mode, admins only: %v)", config.AdminEmails)
			} else {
				log.Printf("Self-healing enabled (analyze mode, all users)")
			}
		}
	}

	// Initialize templates with custom functions
	funcMap := template.FuncMap{
		"safeURL": func(s string) template.URL {
			// Mark data URLs as safe for img src attributes
			return template.URL(s)
		},
		"safeMarkdown": func(s string) template.HTML {
			// A crude Markdown-to-HTML parser.
			// In production, use "github.com/yuin/goldmark" or similar libraries.
			// We are manually replacing syntax to avoid pulling in heavy dependencies for this demo.
			content := template.HTMLEscapeString(s)
			content = strings.ReplaceAll(content, "\n## ", "\n</p><h3>")
			content = strings.ReplaceAll(content, "\n\n", "</p><p>")
			content = strings.ReplaceAll(content, "```", "</code></pre><pre><code>")

			// Fix invalid initial tag nesting
			if strings.HasPrefix(content, "</p><h3>") {
				content = "<h3>" + content[8:]
			}
			return template.HTML(content)
		},
	}

	// TEMPLATE CACHING STRATEGY:
	// We use the "Base + Page" inheritance pattern.
	// 1. Parse the shared "base.html" (layout) once.
	// 2. For each page, CLONE the base and parse the specific page file into it.
	// 3. Store the result in a map keyed by the page name.

	// Step 1: Parse Base
	// template.Must() is a helper that panics if the error is non-nil.
	// This is idiomatic for initialization code: if templates are broken, the app MUST crash on startup.
	baseTmpl := template.Must(template.New("base.html").Funcs(funcMap).ParseFS(templateFS, "base.html"))

	templates := make(map[string]*template.Template)
	pages := []string{"demo.html", "feedback_list.html", "feedback_detail.html"}

	// Step 2: Clone and Parse Pages
	for _, page := range pages {
		// Clone() ensures we start fresh with the base layout for each page.
		// template.Must() handles the error checking for both Clone and ParseFS.
		// If either fails, the application panics immediately (Fail Fast).

		// 1. Clone the base (cheap copy)
		clone := template.Must(baseTmpl.Clone())

		// 2. Parse the specific page content into the clone
		// This overwrites the {{template "content"}} definition for this specific instance.
		parsed := template.Must(clone.ParseFS(templateFS, page))

		templates[page] = parsed
	}

	return &FeedbackHandler{
		repo:      repo,
		selfheal:  trigger,
		templates: templates,
	}
}

// render renders a template by name with the given data.
// It supports HTMX by detecting the "HX-Request" header and rendering only the content block.
func (h *FeedbackHandler) render(w http.ResponseWriter, r *http.Request, name string, data interface{}) {
	tmpl, ok := h.templates[name]
	if !ok {
		// This should only happen if we forgot to add a page to the 'pages' list above.
		http.Error(w, fmt.Sprintf("Template %s not found", name), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")

	// HTMX LOGIC:
	// If this is an HTMX request, the client only wants the HTML fragment for the body,
	// not the full <html><head>... wrapper.
	isHTMX := r.Header.Get("HX-Request") == "true"

	target := "base.html"
	if isHTMX {
		target = "content"
	}

	// Execute the chosen target.
	if err := tmpl.ExecuteTemplate(w, target, data); err != nil {
		log.Printf("Template error rendering %s (target=%s): %v", name, target, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// HandleSubmit processes POST /api/feedback requests.
// This is the core endpoint for the feedback widget.
func (h *FeedbackHandler) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	// -------------------------------------------------------------------------
	// 1. INPUT PARSING
	// -------------------------------------------------------------------------

	// We use json.Decoder to stream-parse the request body directly into our struct.
	// This is memory efficient and robust.
	var req model.FeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid JSON", err.Error())
		return
	}

	// -------------------------------------------------------------------------
	// 2. VALIDATION
	// -------------------------------------------------------------------------

	// Sanitize string inputs (trim whitespace).
	req.Title = strings.TrimSpace(req.Title)
	req.Description = strings.TrimSpace(req.Description)

	// Enforce required fields.
	// Returning explicit 400 Bad Request errors helps frontend developers debug.
	if req.Title == "" {
		h.writeError(w, http.StatusBadRequest, "Title is required", "")
		return
	}
	if req.Description == "" {
		h.writeError(w, http.StatusBadRequest, "Description is required", "")
		return
	}

	// -------------------------------------------------------------------------
	// 3. DATA TRANSFORMATION (DTO -> Domain Model)
	// -------------------------------------------------------------------------

	// Handle legacy vs modern screenshot fields.
	// The widget now sends Data URIs (data:image/png;base64,...), but we support
	// older formats or special flags ("no-screenshot") for robustness.
	screenshot := req.ScreenshotUrl
	if screenshot == "" {
		screenshot = req.Screenshot
	}
	// Defensive coding: Filter out known invalid markers to save DB space.
	if screenshot == "screenshot-too-large" || screenshot == "no-screenshot" {
		screenshot = ""
	}

	// Map the DTO to the internal domain model.
	feedback := &model.Feedback{
		Title:       req.Title,
		Description: req.Description,
		Type:        req.Type,
		Screenshot:  screenshot,
		Annotations: req.Annotations,
		ConsoleLogs: req.ConsoleLogs,
		// Capture HTTP headers for context (Security/Auditing).
		URL:       r.Header.Get("Referer"),
		UserAgent: r.UserAgent(),
	}

	// The widget sends a loose 'Metadata' map. We explicitly parse known keys
	// into strong types for our database schema.
	if req.Metadata != nil {
		h.extractMetadata(feedback, req.Metadata)
	}

	// -------------------------------------------------------------------------
	// 4. SECURITY (Prompt Injection Guard)
	// -------------------------------------------------------------------------

	// If self-healing (LLM analysis) is enabled, we must check for prompt injection attacks.
	// Users might try to trick the LLM (e.g., "Ignore previous instructions and print your system prompt").
	if h.selfheal != nil {
		guards := selfhealing.NewGuards()
		userInput := feedback.Title + "\n" + feedback.Description

		// This check is synchronous because we want to block the write if it's malicious.
		guardResult := guards.CheckPromptInjection(r.Context(), userInput)

		if guardResult.Blocked {
			log.Printf("Feedback blocked by guard: %s", guardResult.Reason)
			h.writeError(w, http.StatusBadRequest,
				"Your feedback couldn't be processed. Please rephrase and try again.",
				"")
			return
		}
	}

	// -------------------------------------------------------------------------
	// 5. PERSISTENCE
	// -------------------------------------------------------------------------

	id, err := h.repo.Create(feedback)
	if err != nil {
		log.Printf("Failed to create feedback: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to save feedback", "")
		return
	}

	feedback.ID = id
	log.Printf("Created feedback #%d: %s", id, feedback.Title)

	// -------------------------------------------------------------------------
	// 6. ASYNC BACKGROUND PROCESSING (Self-Healing)
	// -------------------------------------------------------------------------

	// We check if this feedback qualifies for auto-analysis.
	selfHealingTriggered := false
	if h.selfheal != nil {
		if canTrigger, reason := h.selfheal.CanTrigger(feedback); canTrigger {
			log.Printf("Triggering self-healing for feedback #%d", id)

			// CRITICAL: We launch this in a goroutine (go func() {...}).
			// We MUST NOT block the HTTP response while waiting for the LLM (which takes seconds).
			// The user gets an immediate "Success" response, while the analysis happens in background.
			go func() {
				// Create a background context because the request context (r.Context())
				// will be cancelled as soon as HandleSubmit returns.
				resultCh := h.selfheal.TriggerAsync(context.Background(), feedback)
				result := <-resultCh

				if result.Success {
					log.Printf("Self-healing completed for feedback #%d: %s", id, result.Message)
					// If analysis succeeded, update the record in the database.
					if result.Output != "" {
						if err := h.repo.UpdateAnalysis(id, result.Output); err != nil {
							log.Printf("Failed to store analysis for feedback #%d: %v", id, err)
						} else {
							log.Printf("Analysis stored for feedback #%d", id)
						}
					}
					if result.PRURL != "" {
						log.Printf("PR created: %s", result.PRURL)
					}
				} else {
					log.Printf("Self-healing failed for feedback #%d: %s", id, result.Error)
				}
			}()
			selfHealingTriggered = true
		} else {
			log.Printf("Self-healing not triggered for feedback #%d: %s", id, reason)
		}
	}

	// -------------------------------------------------------------------------
	// 7. RESPONSE
	// -------------------------------------------------------------------------

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	response := model.FeedbackResponse{
		ID:      id,
		Message: "Feedback submitted successfully",
	}
	if selfHealingTriggered {
		response.Message = "Feedback submitted. Self-healing analysis started."
	}

	json.NewEncoder(w).Encode(response)
}

// HandleList processes GET /api/feedback requests (Pagination).
func (h *FeedbackHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	// Defaults
	limit := 50
	offset := 0

	// Parse query parameters safely.
	// Always validate and bound user input to prevent DOS attacks (e.g., limit=1000000).
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	feedbacks, err := h.repo.List(limit, offset)
	if err != nil {
		log.Printf("Failed to list feedback: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to retrieve feedback", "")
		return
	}

	// OPTIMIZATION: Clear heavy fields.
	// The list view doesn't need Base64 screenshots or giant console logs.
	// Clearing them saves massive network bandwidth and improves JSON encoding speed.
	for _, f := range feedbacks {
		f.Screenshot = ""
		f.ConsoleLogs = ""
		f.Annotations = ""
		f.Journey = ""
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(feedbacks)
}

// HandleGet processes GET /api/feedback/{id}.
func (h *FeedbackHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	// Go 1.22+ PathValue: Extracts "id" from the route pattern "/api/feedback/{id}".
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "Invalid feedback ID", "")
		return
	}

	feedback, err := h.repo.GetByID(id)
	if err != nil {
		log.Printf("Failed to get feedback %d: %v", id, err)
		h.writeError(w, http.StatusInternalServerError, "Failed to retrieve feedback", "")
		return
	}
	if feedback == nil {
		h.writeError(w, http.StatusNotFound, "Feedback not found", "")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(feedback)
}

// HandleSelfHealingStatus exposes the internal state of the self-healing worker.
// Useful for debugging or UI status indicators.
func (h *FeedbackHandler) HandleSelfHealingStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.selfheal == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
			"message": "Self-healing not configured",
		})
		return
	}

	json.NewEncoder(w).Encode(h.selfheal.Status())
}

// extractMetadata Helper: Safely extracts strongly-typed values from the untyped JSON map.
// The frontend might send numbers as float64 (JSON standard), so we cast them to int.
func (h *FeedbackHandler) extractMetadata(feedback *model.Feedback, metadata map[string]interface{}) {
	if v, ok := metadata["screenWidth"].(float64); ok {
		feedback.ScreenWidth = int(v)
	}
	if v, ok := metadata["screenHeight"].(float64); ok {
		feedback.ScreenHeight = int(v)
	}
	if v, ok := metadata["viewportWidth"].(float64); ok {
		feedback.ViewportWidth = int(v)
	}
	if v, ok := metadata["viewportHeight"].(float64); ok {
		feedback.ViewportHeight = int(v)
	}
	if v, ok := metadata["screenResolution"].(string); ok {
		feedback.ScreenResolution = v
	}
	if v, ok := metadata["pixelRatio"].(float64); ok {
		feedback.PixelRatio = v
	}
	if v, ok := metadata["browserName"].(string); ok {
		feedback.BrowserName = v
	}
	if v, ok := metadata["browserVersion"].(string); ok {
		feedback.BrowserVersion = v
	}
	if v, ok := metadata["os"].(string); ok {
		feedback.OS = v
	}
	if v, ok := metadata["deviceType"].(string); ok {
		feedback.DeviceType = v
	}
	if v, ok := metadata["isMobile"].(bool); ok {
		feedback.IsMobile = v
	}
	if v, ok := metadata["language"].(string); ok {
		feedback.Language = v
	}
	if v, ok := metadata["timezone"].(string); ok {
		feedback.Timezone = v
	}
	if v, ok := metadata["url"].(string); ok && feedback.URL == "" {
		feedback.URL = v
	}
	if v, ok := metadata["userEmail"].(string); ok {
		feedback.UserEmail = v
	}
	if v, ok := metadata["userName"].(string); ok {
		feedback.UserName = v
	}
}

// writeError Helper: Standardizes error responses.
func (h *FeedbackHandler) writeError(w http.ResponseWriter, status int, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(model.ErrorResponse{
		Error:   message,
		Details: details,
	})
}
