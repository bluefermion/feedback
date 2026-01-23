// Package selfhealing coordinates the automated response to bug reports.
//
// EDUCATIONAL CONTEXT:
// This package implements an "Event Trigger" pattern. When a user reports a bug:
// 1. We check if the event matches criteria (Is it a bug? Is the user authorized?).
// 2. We check system constraints (Rate limits/Cooldowns).
// 3. We dispatch an asynchronous worker to handle the heavy lifting.
//
// This decouples the fast-path HTTP response (user gets an instant "Thanks!")
// from the slow-path AI analysis (taking 30s+).
package selfhealing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/bluefermion/feedback/internal/model"
)

// Config aggregates all settings for the self-healing subsystem.
// We load this once at startup to fail-fast if configuration is missing.
type Config struct {
	Enabled       bool
	Mode          string        // "analyze" (Go-native LLM) or "opencode" (External Docker container)
	SourceDir     string        // Where is the code? (for "analyze" mode)
	RepoDir       string        // Where is the git repo? (for "opencode" mode)
	TriggerScript string        // Bridge script to call Docker
	ContainerName string        // Name of the persistent agent container
	SkipGuards    bool          // Danger: Disable safety checks (dev only)
	DryRun        bool          // Simulation mode: log what would happen but don't do it
	Cooldown      time.Duration // Rate limiting: Prevent burning API credits
	Timeout       time.Duration // Watchdog timer: Kill stuck processes
	AdminEmails   []string      // Authorization: Who can trigger costly agents?
	AllowedTypes  []string      // Filtering: usually only "bug" triggers analysis
	// LLM API settings
	LLMAPIKey  string
	LLMBaseURL string
	LLMModel   string
}

// DefaultConfig builds the configuration from Environment Variables.
// This follows the 12-Factor App config principle.
func DefaultConfig() Config {
	return Config{
		Enabled:       os.Getenv("OPENCODE_ENABLED") == "true",
		Mode:          getEnvOrDefault("SELFHEALING_MODE", "analyze"),
		SourceDir:     getEnvOrDefault("SOURCE_DIR", "."),
		RepoDir:       getEnvOrDefault("OPENCODE_REPO_DIR", ""),
		TriggerScript: getEnvOrDefault("TRIGGER_SCRIPT", "./scripts/trigger-analysis.sh"),
		ContainerName: getEnvOrDefault("OPENCODE_CONTAINER", "opencode-selfhealing"),
		SkipGuards:    os.Getenv("SKIP_GUARDS") == "true",
		DryRun:        os.Getenv("DRY_RUN") == "true",
		Cooldown:      time.Hour,
		Timeout:       30 * time.Minute,
		AdminEmails:   parseCSV(os.Getenv("ADMIN_EMAILS")),
		AllowedTypes:  parseAllowedTypes(os.Getenv("SELFHEALING_TYPES")),
		LLMAPIKey:     os.Getenv("LLM_API_KEY"),
		LLMBaseURL:    getEnvOrDefault("LLM_BASE_URL", "https://api.demeterics.com/chat/v1"),
		LLMModel:      getEnvOrDefault("LLM_MODEL", "groq/llama-3.3-70b-versatile"),
	}
}

// Trigger is the singleton service that manages the lifecycle of analysis jobs.
type Trigger struct {
	config    Config
	lastRun   time.Time
	// Mutex protects shared state (lastRun, isRunning) from concurrent access
	// if multiple HTTP requests hit CanTrigger/Execute simultaneously.
	mu        sync.Mutex
	isRunning bool
}

// NewTrigger constructor.
func NewTrigger(config Config) *Trigger {
	return &Trigger{
		config: config,
	}
}

// Result is the structured output of a self-healing job.
// We return this via a channel to the caller (who may be logging it or updating the DB).
type Result struct {
	Triggered   bool      `json:"triggered"`
	Success     bool      `json:"success"`
	Message     string    `json:"message"`
	PRNumber    int       `json:"pr_number,omitempty"`
	PRURL       string    `json:"pr_url,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Output      string    `json:"output,omitempty"` // The raw LLM analysis text
	Error       string    `json:"error,omitempty"`
}

// CanTrigger acts as a Policy Enforcement Point (PEP).
// It decides if a specific feedback item qualifies for automated analysis.
func (t *Trigger) CanTrigger(feedback *model.Feedback) (bool, string) {
	if !t.config.Enabled {
		return false, "self-healing disabled"
	}

	// Authorization Check: "opencode" mode can modify code/create PRs.
	// We MUST restrict this to trusted admins only to prevent attacks.
	if t.config.Mode == "opencode" {
		if !t.isAdmin(feedback.UserEmail) {
			return false, "opencode mode requires admin privileges"
		}
		// Infrastructure Health Checks
		if _, err := os.Stat(t.config.TriggerScript); os.IsNotExist(err) {
			return false, "trigger script not found"
		}
		if !t.isContainerRunning() {
			return false, "opencode container not running"
		}
	} else {
		// "analyze" mode is read-only (safe for public demos), just needs API key.
		if t.config.LLMAPIKey == "" {
			return false, "LLM_API_KEY not configured"
		}
	}

	// Filter by content type (e.g., don't analyze "Feature Requests" as bugs)
	if !t.isAllowedType(feedback.Type) {
		return false, fmt.Sprintf("feedback type '%s' not allowed for self-healing", feedback.Type)
	}

	// Rate Limiting (Thread-Safe)
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.isRunning {
		return false, "self-healing already in progress"
	}

	if time.Since(t.lastRun) < t.config.Cooldown {
		remaining := t.config.Cooldown - time.Since(t.lastRun)
		return false, fmt.Sprintf("cooldown active, %v remaining", remaining.Round(time.Minute))
	}

	return true, ""
}

// isContainerRunning checks Docker status via shell command.
func (t *Trigger) isContainerRunning() bool {
	// We use 'docker ps' with a filter to strictly match the container name.
	cmd := exec.Command("docker", "ps", "--filter", "name="+t.config.ContainerName, "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == t.config.ContainerName
}

// TriggerAsync launches the analysis in a goroutine and returns a channel for the result.
// This is the "Fire and Forget" (with optional notification) pattern.
func (t *Trigger) TriggerAsync(ctx context.Context, feedback *model.Feedback) <-chan Result {
	resultCh := make(chan Result, 1)

	go func() {
		defer close(resultCh)
		result := t.execute(ctx, feedback)
		resultCh <- result
	}()

	return resultCh
}

// execute runs the actual business logic of the analysis.
// It manages the lock to ensure only one analysis runs at a time (Singleton Worker).
func (t *Trigger) execute(ctx context.Context, feedback *model.Feedback) Result {
	result := Result{
		Triggered: true,
		StartedAt: time.Now(),
	}

	// Critical Section: Acquire Lock
	t.mu.Lock()
	if t.isRunning {
		t.mu.Unlock()
		result.Success = false
		result.Message = "already running"
		return result
	}
	t.isRunning = true
	t.lastRun = time.Now()
	t.mu.Unlock()

	// Ensure we release the lock when done, no matter what happens (panic/return).
	defer func() {
		t.mu.Lock()
		t.isRunning = false
		t.mu.Unlock()
	}()

	log.Printf("[selfhealing] Starting %s analysis for feedback #%d: %s", t.config.Mode, feedback.ID, feedback.Title)

	if t.config.DryRun {
		result.Success = true
		result.Message = fmt.Sprintf("dry run - would execute %s", t.config.Mode)
		result.Output = fmt.Sprintf("Feedback: %s - %s", feedback.Title, feedback.Description)
		result.CompletedAt = time.Now()
		return result
	}

	var output string
	var err error

	// Strategy Pattern: Choose the execution engine.
	if t.config.Mode == "opencode" {
		// External Agent: Call Docker
		output, err = t.runOpenCode(ctx, feedback)
	} else {
		// Internal Agent: Call LLM API
		output, err = t.runAnalyze(ctx, feedback)
	}

	result.CompletedAt = time.Now()
	result.Output = output

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.Message = fmt.Sprintf("%s execution failed", t.config.Mode)
		log.Printf("[selfhealing] Failed: %v", err)
		return result
	}

	// Success Path
	result.Success = true
	result.Message = "analysis completed"
	if t.config.Mode == "opencode" {
		// Parse the unstructured text log to find structured data (PR URLs).
		t.parseOutput(output, &result)
	}

	log.Printf("[selfhealing] Completed: %s", result.Message)
	
	// Logging (truncated to avoid spamming stdout)
	if len(output) > 500 {
		log.Printf("[selfhealing] Analysis (truncated):\n%s...", output[:500])
	} else if output != "" {
		log.Printf("[selfhealing] Analysis:\n%s", output)
	}
	if result.PRURL != "" {
		log.Printf("[selfhealing] PR created: %s", result.PRURL)
	}

	return result
}

// runOpenCode shells out to a script that pipes data into a Docker container.
// This is used for "Full Autonomy" where the agent needs a sandbox to run git/tests.
func (t *Trigger) runOpenCode(ctx context.Context, feedback *model.Feedback) (string, error) {
	// Create context with timeout to prevent hanging forever.
	ctx, cancel := context.WithTimeout(ctx, t.config.Timeout)
	defer cancel()

	// IPC (Inter-Process Communication): Serialize data to JSON to pass to the script.
	feedbackJSON, err := json.Marshal(map[string]interface{}{
		"id":          feedback.ID,
		"title":       feedback.Title,
		"description": feedback.Description,
		"type":        feedback.Type,
		"url":         feedback.URL,
		"consoleLogs": feedback.ConsoleLogs,
	})
	if err != nil {
		return "", fmt.Errorf("failed to serialize feedback: %v", err)
	}

	// `exec.CommandContext` kills the process if ctx is cancelled/times out.
	cmd := exec.CommandContext(ctx, t.config.TriggerScript, string(feedbackJSON))
	// Pass configuration via Environment Variables to the script
	cmd.Env = append(os.Environ(), "OPENCODE_CONTAINER="+t.config.ContainerName)

	// Capture stdout/stderr for debugging
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("timeout after %v", t.config.Timeout)
		}
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}

	return stdout.String(), nil
}

// runAnalyze runs the native Go-based LLM agent.
// This is used for "Read-Only" analysis where we just want a bug report.
func (t *Trigger) runAnalyze(ctx context.Context, feedback *model.Feedback) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, t.config.Timeout)
	defer cancel()

	// Note: Prompt injection guard is skipped here because it was already run
	// synchronously in the HTTP handler (Fail-Fast).

	sourceDir := t.config.SourceDir
	if sourceDir == "" {
		sourceDir = "."
	}

	log.Printf("[selfhealing] Source directory for file access: %s", sourceDir)

	analyzer := NewLLMAnalyzer(
		t.config.LLMAPIKey,
		t.config.LLMBaseURL,
		t.config.LLMModel,
		sourceDir,
	)

	return analyzer.Analyze(ctx, feedback)
}

// parseOutput scrapes stdout for artifacts like PR URLs or branch names.
func (t *Trigger) parseOutput(output string, result *Result) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Regex-like parsing to find GitHub PR URLs
		if strings.Contains(line, "github.com") && strings.Contains(line, "/pull/") {
			words := strings.Fields(line)
			for _, word := range words {
				if strings.Contains(word, "github.com") && strings.Contains(word, "/pull/") {
					result.PRURL = strings.Trim(word, "()[]<>")
					
					// Extract the numeric ID from the URL
					parts := strings.Split(result.PRURL, "/pull/")
					if len(parts) > 1 {
						numStr := strings.Split(parts[1], "/")[0]
						numStr = strings.TrimSpace(numStr)
						fmt.Sscanf(numStr, "%d", &result.PRNumber)
					}
					break
				}
			}
		}

		// Look for branch information
		if strings.HasPrefix(line, "Branch:") {
			result.Branch = strings.TrimSpace(strings.TrimPrefix(line, "Branch:"))
		} else if strings.HasPrefix(line, "fix/") || strings.HasPrefix(line, "feature/") {
			if result.Branch == "" {
				result.Branch = strings.Fields(line)[0]
			}
		}
	}
}

// isAdmin checks if the user is in the allow-list.
func (t *Trigger) isAdmin(email string) bool {
	if len(t.config.AdminEmails) == 0 {
		return false
	}
	email = strings.ToLower(strings.TrimSpace(email))
	for _, admin := range t.config.AdminEmails {
		if strings.ToLower(strings.TrimSpace(admin)) == email {
			return true
		}
	}
	return false
}

// isAllowedType checks feedback categorization.
func (t *Trigger) isAllowedType(feedbackType string) bool {
	feedbackType = strings.ToLower(strings.TrimSpace(feedbackType))
	for _, allowed := range t.config.AllowedTypes {
		allowed = strings.ToLower(allowed)
		if allowed == "all" || allowed == feedbackType {
			return true
		}
	}
	return false
}

// Helper: robust environment variable retrieval.
func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// Helper: parse CSV string to slice.
func parseCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Helper: parse feedback types config.
func parseAllowedTypes(s string) []string {
	if s == "" {
		return []string{"bug"}
	}
	return parseCSV(s)
}

// Status returns a snapshot of the internal state (Health Check pattern).
func (t *Trigger) Status() map[string]interface{} {
	t.mu.Lock()
	defer t.mu.Unlock()

	status := map[string]interface{}{
		"enabled":        t.config.Enabled,
		"is_running":     t.isRunning,
		"container_name": t.config.ContainerName,
	}

	if !t.lastRun.IsZero() {
		status["last_run"] = t.lastRun
		status["cooldown_remaining"] = (t.config.Cooldown - time.Since(t.lastRun)).Round(time.Second).String()
	}

	status["container_running"] = t.isContainerRunning()

	if _, err := os.Stat(t.config.TriggerScript); err == nil {
		status["trigger_script"] = t.config.TriggerScript
	}

	return status
}

// Validate ensures configuration is sane before we start.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.Mode == "opencode" {
		if _, err := os.Stat(c.TriggerScript); os.IsNotExist(err) {
			return fmt.Errorf("trigger script not found: %s", c.TriggerScript)
		}
		if len(c.AdminEmails) == 0 {
			return fmt.Errorf("ADMIN_EMAILS is required for opencode mode")
		}
	} else {
		if c.LLMAPIKey == "" {
			return fmt.Errorf("LLM_API_KEY is required for analyze mode")
		}
	}

	return nil
}