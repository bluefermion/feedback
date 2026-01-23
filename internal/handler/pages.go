// Package handler includes logic for rendering user-facing HTML pages.
//
// EDUCATIONAL CONTEXT:
// We have refactored from "Raw Strings" to "html/template".
// This is the idiomatic Go way to handle server-side rendering (SSR).
//
// Benefits:
// 1. Separation of Concerns: Go code handles logic, HTML files handle presentation.
// 2. Security: Auto-escaping prevents Cross-Site Scripting (XSS).
// 3. Maintainability: Syntax highlighting and cleaner code structure.
package handler

import (
	"net/http"
	"strconv"
)

// HandleDemo renders the demo page.
// Endpoint: GET /demo
func (h *FeedbackHandler) HandleDemo(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, "demo.html", nil)
}

// HandleFeedbackList renders a table of all feedback submissions.
// Endpoint: GET /feedback
func (h *FeedbackHandler) HandleFeedbackList(w http.ResponseWriter, r *http.Request) {
	// -------------------------------------------------------------------------
	// 1. DATA FETCHING
	// -------------------------------------------------------------------------

	// We fetch the latest 100 items.
	feedbacks, err := h.repo.List(100, 0)
	if err != nil {
		http.Error(w, "Failed to load feedback", http.StatusInternalServerError)
		return
	}

	// -------------------------------------------------------------------------
	// 2. TEMPLATE EXECUTION
	// -------------------------------------------------------------------------

	h.render(w, r, "feedback_list.html", feedbacks)
}

// HandleFeedbackDetail renders the detailed view for a single feedback item.
// Endpoint: GET /feedback/{id}
func (h *FeedbackHandler) HandleFeedbackDetail(w http.ResponseWriter, r *http.Request) {
	// Parse URL Parameter
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid feedback ID", http.StatusBadRequest)
		return
	}

	// Fetch Data
	f, err := h.repo.GetByID(id)
	if err != nil {
		http.Error(w, "Failed to load feedback", http.StatusInternalServerError)
		return
	}
	if f == nil {
		http.Error(w, "Feedback not found", http.StatusNotFound)
		return
	}

	// -------------------------------------------------------------------------
	// 2. TEMPLATE EXECUTION
	// -------------------------------------------------------------------------

	h.render(w, r, "feedback_detail.html", f)
}
