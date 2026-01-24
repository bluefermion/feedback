// Package handler provides HTTP handlers for the Feedback Service.
//
// opencode_analyzer.go implements the ErrorAnalyzer interface to trigger
// OpenCode self-healing analysis via the Docker container.
package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"
)

// OpenCodeAnalyzer implements ErrorAnalyzer using the OpenCode Docker container
type OpenCodeAnalyzer struct {
	containerName string
	enabled       bool
}

// OpenCodeConfig holds configuration for the OpenCode analyzer
type OpenCodeConfig struct {
	ContainerName string
	Enabled       bool
}

// NewOpenCodeAnalyzer creates a new OpenCodeAnalyzer
func NewOpenCodeAnalyzer(config OpenCodeConfig) *OpenCodeAnalyzer {
	if config.ContainerName == "" {
		config.ContainerName = "opencode-selfhealing"
	}

	// Check if OpenCode is enabled via environment
	enabled := config.Enabled
	if os.Getenv("OPENCODE_ENABLED") == "true" {
		enabled = true
	}

	return &OpenCodeAnalyzer{
		containerName: config.ContainerName,
		enabled:       enabled,
	}
}

// AnalyzeError triggers async analysis via the OpenCode Docker container
func (a *OpenCodeAnalyzer) AnalyzeError(errCtx ErrorContext) error {
	if !a.enabled {
		log.Printf("[%s] OpenCode analysis skipped (disabled)", errCtx.ErrorID)
		return nil
	}

	// Check if container is running
	if !a.isContainerRunning() {
		log.Printf("[%s] OpenCode container not running, skipping analysis", errCtx.ErrorID)
		return fmt.Errorf("opencode container not running")
	}

	// Build the analysis payload
	payload := map[string]interface{}{
		"error_id":    errCtx.ErrorID,
		"type":        "runtime_error",
		"title":       fmt.Sprintf("Runtime Error in %s", errCtx.HandlerFunc),
		"description": errCtx.ErrorMessage,
		"file":        errCtx.HandlerFile,
		"function":    errCtx.HandlerFunc,
		"request": map[string]string{
			"path":       errCtx.RequestPath,
			"method":     errCtx.RequestMethod,
			"user_agent": errCtx.UserAgent,
		},
		"timestamp": errCtx.Timestamp.Format(time.RFC3339),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Base64 encode for safe transmission
	payloadB64 := base64.StdEncoding.EncodeToString(payloadJSON)

	// Execute analysis in container
	go a.executeAnalysis(errCtx.ErrorID, payloadB64)

	log.Printf("[%s] OpenCode analysis triggered", errCtx.ErrorID)
	return nil
}

// isContainerRunning checks if the OpenCode container is running
func (a *OpenCodeAnalyzer) isContainerRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.State.Running}}", a.containerName)
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return string(bytes.TrimSpace(output)) == "true"
}

// executeAnalysis runs the analysis script in the container
func (a *OpenCodeAnalyzer) executeAnalysis(errorID, payloadB64 string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "exec", a.containerName, "/app/analyze.sh", payloadB64)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[%s] OpenCode analysis failed: %v\nOutput: %s", errorID, err, string(output))
		return
	}

	log.Printf("[%s] OpenCode analysis completed:\n%s", errorID, string(output))
}

// WebhookAnalyzer implements ErrorAnalyzer using a webhook endpoint
type WebhookAnalyzer struct {
	webhookURL string
	apiKey     string
	enabled    bool
}

// WebhookConfig holds configuration for the webhook analyzer
type WebhookConfig struct {
	WebhookURL string
	APIKey     string
	Enabled    bool
}

// NewWebhookAnalyzer creates a new WebhookAnalyzer
func NewWebhookAnalyzer(config WebhookConfig) *WebhookAnalyzer {
	// Check environment variables
	if config.WebhookURL == "" {
		config.WebhookURL = os.Getenv("OPENCODE_WEBHOOK_URL")
	}
	if config.APIKey == "" {
		config.APIKey = os.Getenv("OPENCODE_WEBHOOK_API_KEY")
	}

	enabled := config.Enabled
	if config.WebhookURL != "" {
		enabled = true
	}

	return &WebhookAnalyzer{
		webhookURL: config.WebhookURL,
		apiKey:     config.APIKey,
		enabled:    enabled,
	}
}

// AnalyzeError triggers async analysis via webhook
func (a *WebhookAnalyzer) AnalyzeError(errCtx ErrorContext) error {
	if !a.enabled || a.webhookURL == "" {
		log.Printf("[%s] Webhook analysis skipped (disabled or no URL)", errCtx.ErrorID)
		return nil
	}

	// Build the analysis payload
	payload := map[string]interface{}{
		"error_id":    errCtx.ErrorID,
		"type":        "runtime_error",
		"title":       fmt.Sprintf("Runtime Error in %s", errCtx.HandlerFunc),
		"description": errCtx.ErrorMessage,
		"file":        errCtx.HandlerFile,
		"function":    errCtx.HandlerFunc,
		"request": map[string]string{
			"path":       errCtx.RequestPath,
			"method":     errCtx.RequestMethod,
			"user_agent": errCtx.UserAgent,
		},
		"timestamp": errCtx.Timestamp.Format(time.RFC3339),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Send webhook asynchronously
	go a.sendWebhook(errCtx.ErrorID, payloadJSON)

	log.Printf("[%s] Webhook analysis triggered", errCtx.ErrorID)
	return nil
}

// sendWebhook sends the payload to the webhook URL
func (a *WebhookAnalyzer) sendWebhook(errorID string, payload []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use curl for simplicity (could use net/http but this matches existing patterns)
	args := []string{
		"-s", "-X", "POST",
		"-H", "Content-Type: application/json",
	}

	if a.apiKey != "" {
		args = append(args, "-H", fmt.Sprintf("X-API-Key: %s", a.apiKey))
	}

	args = append(args, "-d", string(payload), a.webhookURL)

	cmd := exec.CommandContext(ctx, "curl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[%s] Webhook failed: %v\nOutput: %s", errorID, err, string(output))
		return
	}

	log.Printf("[%s] Webhook completed: %s", errorID, string(output))
}

// CompositeAnalyzer combines multiple analyzers
type CompositeAnalyzer struct {
	analyzers []ErrorAnalyzer
}

// NewCompositeAnalyzer creates a new CompositeAnalyzer
func NewCompositeAnalyzer(analyzers ...ErrorAnalyzer) *CompositeAnalyzer {
	return &CompositeAnalyzer{analyzers: analyzers}
}

// AnalyzeError triggers analysis on all configured analyzers
func (a *CompositeAnalyzer) AnalyzeError(errCtx ErrorContext) error {
	var lastErr error
	for _, analyzer := range a.analyzers {
		if err := analyzer.AnalyzeError(errCtx); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
