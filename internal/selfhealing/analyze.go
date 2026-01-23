// Package selfhealing implements the "Agentic" logic of the application.
//
// EDUCATIONAL CONTEXT:
// This package demonstrates how to build a basic autonomous agent that can:
// 1. Receive a task (Analyze this bug report).
// 2. Reason about what to do next (LLM Chain of Thought).
// 3. Execute tools (Read file, List directory) to gather information.
// 4. Synthesize a final answer.
//
// We implementation "Tool Calling" (or Function Calling), which is the standard way
// modern LLMs (like GPT-4, Claude, Llama 3) interact with external systems.
package selfhealing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluefermion/feedback/internal/model"
)

// LLMAnalyzer manages the conversation state and tool execution loop.
type LLMAnalyzer struct {
	apiKey     string
	baseURL    string
	model      string
	sourceDir  string // Security boundary: The agent can only read files inside this root.
	httpClient *http.Client
}

// NewLLMAnalyzer initializes the analyzer with credentials and configuration.
func NewLLMAnalyzer(apiKey, baseURL, model, sourceDir string) *LLMAnalyzer {
	return &LLMAnalyzer{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		sourceDir:  sourceDir,
		httpClient: &http.Client{}, // Default client. In prod, set timeouts here!
	}
}

// -----------------------------------------------------------------------------
// LLM API TYPES (OpenAI-compatible Schema)
// -----------------------------------------------------------------------------

// ChatMessage represents a single turn in the conversation history.
type ChatMessage struct {
	Role       string     `json:"role"`              // "system", "user", "assistant", or "tool"
	Content    string     `json:"content,omitempty"` // Text content (can be empty if tool_calls present)
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // Required when responding to a tool call
}

// ToolCall represents the LLM's request to execute a specific function.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // Usually "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON string of arguments
	} `json:"function"`
}

// Tool defines the schema of a function available to the LLM.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction describes the signature (name, description, params) of a tool.
// The description is CRITICAL: it's the "prompt" that tells the LLM when/how to use it.
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema of inputs
}

// ChatRequest is the payload sent to the inference API.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Tools       []Tool        `json:"tools,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"` // Lower (0.2) = more deterministic/factual
}

// ChatResponse parses the API response.
type ChatResponse struct {
	Choices []struct {
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"` // "stop" (done) or "tool_calls" (needs action)
	} `json:"choices"`
	// Error handling for various API provider formats
	Error   interface{} `json:"error,omitempty"`
	Code    int         `json:"code,omitempty"`
	Message string      `json:"message,omitempty"`
}

// -----------------------------------------------------------------------------
// CORE LOGIC
// -----------------------------------------------------------------------------

// Analyze orchestrates the "Thinking Loop" (ReAct Pattern).
// 1. Prepare Prompt.
// 2. Loop:
//    a. Send history to LLM.
//    b. Did LLM ask for a tool?
//       Yes -> Execute tool -> Add result to history -> Continue.
//       No  -> Return final answer.
func (a *LLMAnalyzer) Analyze(ctx context.Context, feedback *model.Feedback) (string, error) {
	// SYSTEM PROMPT: Sets the persona and operational constraints.
	systemPrompt := `You are a senior software engineer analyzing user feedback and bug reports for a web application.

Your job is to:
1. FIRST use list_files to explore the codebase structure
2. Read relevant source files to understand the code
3. Identify the root cause if it's a bug
4. Suggest specific fixes with code snippets

ALWAYS provide your analysis in this format:

## Summary
One sentence describing what the user is reporting.

## Relevant Files
List the files you examined and why.

## Analysis
Your technical analysis of the issue. If it's a bug, explain the root cause.
If it's a feature request or question, explain the current behavior.

## Suggested Fix
If applicable, provide specific code changes. Use markdown code blocks.
If no code change needed, explain what action to take.

IMPORTANT:
- Start by listing files to understand the project structure
- Read the most relevant files based on the feedback
- Be specific - reference line numbers and function names
- If the feedback is vague, still explore the code and provide useful context`

	// USER PROMPT: The specific task input.
	userPrompt := fmt.Sprintf(`Analyze this user feedback:

**Title:** %s
**Type:** %s
**Page URL:** %s

**User's Description:**
%s`, feedback.Title, feedback.Type, feedback.URL, feedback.Description)

	// Context Injection: Provide console logs if available.
	// This is "RAG" (Retrieval Augmented Generation) in its simplest form.
	if feedback.ConsoleLogs != "" {
		userPrompt += fmt.Sprintf("\n\n**Console Logs:**\n```\n%s\n```", feedback.ConsoleLogs)
	}

	// Heuristic: If description is short, guide the LLM to be proactive.
	if feedback.Description == "" || len(feedback.Description) < 20 {
		userPrompt += "\n\nNote: The user provided minimal description. Focus on exploring the codebase structure and providing an overview of the relevant components based on the title."
	}

	// TOOL DEFINITIONS: The "Capabilities" we give the agent.

tools := []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_file_content",
				Description: "Read the content of a source file from the codebase. Only files in the source directory can be read.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Relative path to the file (e.g., 'handler/feedback.go' or 'components/Button.tsx')",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_files",
				Description: "List files in a directory of the source code. Use this to discover what files exist.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Relative directory path (e.g., 'handler' or 'components'). Use '.' for root.",
						},
					},
					"required": []string{"path"},
				},
			},
		},
	}

	// Initialize conversation history
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	// THE LOOP:
	// We limit iterations to prevent infinite loops (and huge bills).
	maxIterations := 10
	for i := 0; i < maxIterations; i++ {
		// 1. Ask the LLM
		response, err := a.callLLM(ctx, messages, tools)
		if err != nil {
			return "", fmt.Errorf("LLM call failed: %w", err)
		}

		if len(response.Choices) == 0 {
			return "", fmt.Errorf("no response from LLM")
		}

		choice := response.Choices[0]
		// Add the assistant's reply (which might contain tool calls) to history
		messages = append(messages, choice.Message)

		// 2. Check termination condition
		// If "stop", the LLM is done and has produced the final text answer.
		// If no tool calls, it's also done.
		if choice.FinishReason == "stop" || len(choice.Message.ToolCalls) == 0 {
			return choice.Message.Content, nil
		}

		// 3. Execute requested tools
		for _, toolCall := range choice.Message.ToolCalls {
			result := a.executeTool(toolCall)

			// 4. Feed result back to LLM
			// The Role is "tool", and ToolCallID links it back to the specific request.
			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}
	}

	// Fallback: If we run out of turns, return the last thing the assistant said.
	if len(messages) > 0 {
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "assistant" && messages[i].Content != "" {
				return messages[i].Content, nil
			}
		}
	}

	return "", fmt.Errorf("analysis incomplete after %d tool iterations", maxIterations)
}

// callLLM handles the low-level HTTP networking to the inference provider.
func (a *LLMAnalyzer) callLLM(ctx context.Context, messages []ChatMessage, tools []Tool) (*ChatResponse, error) {
	reqBody := ChatRequest{
		Model:       a.model,
		Messages:    messages,
		Tools:       tools,
		MaxTokens:   4000,
		Temperature: 0.2, // Low temperature for stability during tool use
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := strings.TrimSuffix(a.baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM API error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(body))
	}

	// Provider-agnostic error handling
	if chatResp.Error != nil {
		switch e := chatResp.Error.(type) {
		case string:
			return nil, fmt.Errorf("LLM API error: %s", e)
		case map[string]interface{}:
			if msg, ok := e["message"].(string); ok {
				return nil, fmt.Errorf("LLM API error: %s", msg)
			}
			return nil, fmt.Errorf("LLM API error: %v", e)
		default:
			return nil, fmt.Errorf("LLM API error: %v", e)
		}
	}

	return &chatResp, nil
}

// executeTool acts as the router/dispatcher for tool calls.
func (a *LLMAnalyzer) executeTool(toolCall ToolCall) string {
	var args map[string]string
	// Tool arguments are always JSON strings
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		log.Printf("[selfhealing] Tool %s: error parsing args: %v", toolCall.Function.Name, err)
		return fmt.Sprintf("Error parsing arguments: %v", err)
	}

	var result string
	switch toolCall.Function.Name {
	case "get_file_content":
		path := args["path"]
		log.Printf("[selfhealing] Tool get_file_content: %s", path)
		result = a.getFileContent(path)
	case "list_files":
		path := args["path"]
		log.Printf("[selfhealing] Tool list_files: %s", path)
		result = a.listFiles(path)
	default:
		log.Printf("[selfhealing] Unknown tool: %s", toolCall.Function.Name)
		result = fmt.Sprintf("Unknown tool: %s", toolCall.Function.Name)
	}

	return result
}

// getFileContent implements the file reading tool.
func (a *LLMAnalyzer) getFileContent(path string) string {
	if a.sourceDir == "" {
		return "Error: Source directory not configured"
	}

	// SECURITY: Path Traversal Prevention
	// Users (or confused LLMs) might try to read "../../../etc/passwd".
	// We must ensure the resolved path stays within sourceDir.
	cleanPath := filepath.Clean(path)

	if strings.HasPrefix(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
		return "Error: Invalid path - must be relative within source directory"
	}

	fullPath := filepath.Join(a.sourceDir, cleanPath)

	// Double-check using absolute paths
	absSource, _ := filepath.Abs(a.sourceDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absSource) {
		return "Error: Path escapes source directory"
	}

	// Safety: File size check to prevent loading massive binaries into memory/LLM context
	info, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: File not found: %s", path)
	}
	if err != nil {
		return fmt.Sprintf("Error: Cannot access file: %v", err)
	}
	if info.IsDir() {
		return fmt.Sprintf("Error: Path is a directory, not a file: %s", path)
	}
	if info.Size() > 100*1024 { // 100KB limit
		return fmt.Sprintf("Error: File too large (%d bytes, max 100KB)", info.Size())
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v", err)
	}

	return string(content)
}

// listFiles implements the directory listing tool.
func (a *LLMAnalyzer) listFiles(path string) string {
	if a.sourceDir == "" {
		return "Error: Source directory not configured"
	}

	cleanPath := filepath.Clean(path)
	if cleanPath == "." {
		cleanPath = ""
	}

	// Security checks (same as above)
	if strings.HasPrefix(cleanPath, "..") || strings.HasPrefix(cleanPath, "/") {
		return "Error: Invalid path - must be relative within source directory"
	}

	fullPath := filepath.Join(a.sourceDir, cleanPath)
	absSource, _ := filepath.Abs(a.sourceDir)
	absPath, _ := filepath.Abs(fullPath)
	if !strings.HasPrefix(absPath, absSource) {
		return "Error: Path escapes source directory"
	}

	entries, err := os.ReadDir(fullPath)
	if os.IsNotExist(err) {
		return fmt.Sprintf("Error: Directory not found: %s", path)
	}
	if err != nil {
		return fmt.Sprintf("Error reading directory: %v", err)
	}

	var files []string
	for _, entry := range entries {
		name := entry.Name()
		// Noise Reduction: Skip hidden files and dependency folders to keep context small
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
			continue
		}
		if entry.IsDir() {
			name += "/"
		}
		files = append(files, name)
	}

	if len(files) == 0 {
		return "Directory is empty or contains only hidden files"
	}

	return strings.Join(files, "\n")
}