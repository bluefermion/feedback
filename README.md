# Feedback

A standalone feedback collection and analysis system. Features a floating widget for instant user feedback capture with optional screenshot annotation, Go backend with SQLite storage, and LLM-powered analysis.

Built by [Blue Fermion Labs](https://bluefermionlabs.com)

## Features

- **Floating Feedback Button** - Material Design yellow "!" button for instant feedback capture
- **Screenshot Annotation** - Capture and annotate screenshots with highlight/redact tools
- **Console Log Capture** - Automatically collect browser console logs for debugging
- **Device Metadata** - Collect browser, OS, screen, and timezone information
- **SQLite Storage** - Simple file-based storage, no cloud dependencies for local development
- **LLM Analysis** - Automatic bug analysis with file exploration (available to all users)
- **Self-Healing** - Two modes: lightweight `analyze` (direct LLM) or full `opencode` (Docker)
- **Tool Calling** - LLM can explore codebase via `list_files` and `get_file_content` tools

## Quick Start

### Option 1: Run Locally (Basic)

```bash
# Clone the repository
git clone https://github.com/bluefermion/feedback.git
cd feedback

# Run the server
make run
```

Open http://localhost:8080/demo to see the widget in action.

### Option 2: Run with LLM Analysis

```bash
# Create .env from template
make setup

# Edit .env and set your API key
# LLM_API_KEY=your-demeterics-api-key
# OPENCODE_ENABLED=true
# SELFHEALING_MODE=analyze
# SELFHEALING_TYPES=all

# Run the server
make run
```

Feedback submissions will trigger automatic LLM analysis. Results are stored in SQLite and logged to console.

### Add the Widget to Your Site

Include the feedback widget in your HTML:

```html
<script src="/static/js/feedback-widget.js"></script>
<script>
  FeedbackWidget.init({
    endpoint: '/api/feedback',
    debug: true
  });
</script>
```

## Architecture

### Basic Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                         Frontend                                 │
│  ┌─────────────┐  ┌──────────────┐  ┌─────────────────────────┐ │
│  │  Floating   │  │  Feedback    │  │  Console Log Capture    │ │
│  │  Button (!) │─▶│  Modal       │─▶│  + Device Metadata      │ │
│  └─────────────┘  └──────────────┘  └─────────────────────────┘ │
└────────────────────────────┬────────────────────────────────────┘
                             │ POST /api/feedback
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Go Backend Server                            │
│  ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐ │
│  │ Prompt Guard │─▶│    SQLite    │─▶│   Self-Healing         │ │
│  │ (injection)  │  │   Storage    │  │   (async)              │ │
│  └──────────────┘  └──────────────┘  └────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### Self-Healing: Analyze Mode (Recommended)

Direct LLM calls with tool calling. No Docker required. Available to all users.

```
┌─────────────────────────────────────────────────────────────────┐
│                     Go Backend Server                            │
│                                                                  │
│  Feedback ──▶ LLM Analyzer ──▶ Tool Calls ──▶ Analysis          │
│                    │              │                              │
│                    ▼              ▼                              │
│              ┌──────────┐  ┌─────────────┐                      │
│              │ Demeterics│  │ list_files  │                      │
│              │ API       │  │ get_file    │                      │
│              └──────────┘  │ (SOURCE_DIR)│                      │
│                            └─────────────┘                      │
└─────────────────────────────────────────────────────────────────┘
```

### Self-Healing: OpenCode Mode (Advanced)

Full OpenCode CLI in Docker. Can modify code and create PRs. Admin only.

```
┌─────────────────────────────────────────────────────────────────┐
│                     Go Backend Server                            │
│                                                                  │
│  Feedback ──▶ trigger-analysis.sh ──▶ docker exec               │
│                                            │                     │
└────────────────────────────────────────────┼─────────────────────┘
                                             ▼
┌─────────────────────────────────────────────────────────────────┐
│              Docker: opencode-selfhealing                        │
│                                                                  │
│  analyze.sh ──▶ OpenCode CLI ──▶ Full Codebase Access           │
│                      │                   │                       │
│                      ▼                   ▼                       │
│              ┌──────────────┐    ┌──────────────┐               │
│              │ Read/Write   │    │ Git Commits  │               │
│              │ Any File     │    │ Create PRs   │               │
│              └──────────────┘    └──────────────┘               │
│                                                                  │
│  Volume: /workspace ◀── Your Repository                          │
└─────────────────────────────────────────────────────────────────┘
```

## Frontend Widget

The feedback widget provides a non-intrusive floating button that expands into a full feedback form:

### Button Appearance
- Material Orange (#FF9800) circular button
- White "!" exclamation icon
- Fixed position: bottom-left corner
- Hover animation with scale transform

### Submission Flow
1. Click button → Select feedback type (Bug, Feature, Improvement, Other)
2. Fill title and description
3. Submit → Console logs and device info captured automatically

### Captured Data
- **User Input:** Title, description, type (bug/feature/improvement/other)
- **Console Logs:** Last 50 log/warn/error entries with timestamps
- **Device Info:** Browser, OS, screen dimensions, timezone, language
- **Page Context:** Current URL, user agent

## Backend API

### POST /api/feedback
Submit feedback.

**Request:**
```json
{
  "title": "Button not responding",
  "description": "The submit button on checkout page...",
  "type": "bug",
  "consoleLogs": "[...]",
  "metadata": {
    "browserName": "Chrome",
    "os": "macOS",
    "screenWidth": 1920
  }
}
```

**Response:**
```json
{
  "id": 1,
  "message": "Feedback submitted successfully"
}
```

### GET /api/feedback
List all feedback (paginated).

**Query Parameters:**
- `limit` - Max items (default: 50, max: 100)
- `offset` - Skip items (default: 0)

### GET /api/feedback/{id}
Get specific feedback entry with all details.

## Data Model

```go
type Feedback struct {
    ID          int64     // Auto-generated
    UserEmail   string    // Submitter email
    UserName    string    // Submitter name
    Title       string    // Brief description
    Description string    // Detailed explanation
    Type        string    // bug, feature, improvement, other
    URL         string    // Page URL

    // Device info
    BrowserName    string
    OS             string
    ScreenWidth    int
    DeviceType     string

    // Artifacts
    Screenshot  string    // Base64 encoded
    ConsoleLogs string    // JSON array

    // LLM Analysis (auto-populated when self-healing enabled)
    Analysis          string  // Full analysis with suggested fixes
    PredictedPriority string
    PredictedCategory string

    // Triage
    Status   string    // open, in_progress, resolved, closed
    Priority string    // high, medium, low

    CreatedAt time.Time
    UpdatedAt time.Time
}
```

The `Analysis` field contains structured markdown output from the LLM:
- **Summary** - One sentence describing the issue
- **Relevant Files** - Files examined during analysis
- **Analysis** - Technical root cause analysis
- **Suggested Fix** - Code snippets with specific fixes

## Configuration

All configuration is done via environment variables. Copy `.env.example` to `.env` and customize:

```bash
make setup  # Creates .env from .env.example
```

### Core Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | Server port | `8080` |
| `FEEDBACK_DB_PATH` | SQLite database path | `feedback.db` |
| `COMMON_DEBUG` | Enable debug logging | `false` |

### LLM Settings

The default LLM provider is [Demeterics](https://demeterics.ai) which routes to optimal models.

| Variable | Description | Default |
|----------|-------------|---------|
| `LLM_API_KEY` | LLM provider API key | - |
| `LLM_BASE_URL` | LLM API endpoint | `https://api.demeterics.com/chat/v1` |
| `LLM_MODEL` | Model for analysis | `groq/qwen3-32b` |

### Self-Healing Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `OPENCODE_ENABLED` | Enable self-healing | `false` |
| `SELFHEALING_MODE` | `analyze` (direct LLM) or `opencode` (Docker) | `analyze` |
| `SELFHEALING_TYPES` | Feedback types to analyze (`bug`, `all`, or comma-separated) | `bug` |
| `SOURCE_DIR` | Directory for file access (analyze mode) | `.` |
| `OPENCODE_REPO_DIR` | Repository to mount (opencode mode) | `.` |
| `ADMIN_EMAILS` | Comma-separated admin emails (required for opencode) | - |
| `SKIP_GUARDS` | Skip safety guards | `false` |
| `DRY_RUN` | Log but don't execute | `false` |

### GitHub Integration (Optional)

| Variable | Description | Default |
|----------|-------------|---------|
| `GITHUB_TOKEN` | GitHub personal access token | - |
| `GIT_USER_NAME` | Git commit author name | `OpenCode Bot` |
| `GIT_USER_EMAIL` | Git commit author email | `opencode@bluefermionlabs.com` |

## Self-Healing Analysis

When enabled, feedback submissions trigger automatic LLM analysis. Two modes are available:

| Mode | Who Can Trigger | Requirements | Capabilities |
|------|-----------------|--------------|--------------|
| `analyze` | All users | `LLM_API_KEY` | Read files, suggest fixes |
| `opencode` | Admins only | `LLM_API_KEY`, Docker | Full code modification, PRs |

### How It Works

1. User submits feedback via the widget
2. System validates the request (API key, cooldown, feedback type)
3. LLM explores the codebase using tools:
   - `list_files` - Discover project structure
   - `get_file_content` - Read source files (restricted to `SOURCE_DIR`)
4. LLM provides structured analysis with suggested fixes
5. Analysis is stored in SQLite and logged to console

### Tool Call Logging

Each tool invocation is logged for debugging:
```
[selfhealing] Tool list_files: .
[selfhealing] Tool list_files: handler
[selfhealing] Tool get_file_content: handler/feedback.go
[selfhealing] Completed: analysis completed
[selfhealing] Analysis stored for feedback #10
```

### Analysis Storage

Analysis results are stored in the `analysis` column of the feedback table:

```bash
# View stored analyses
sqlite3 feedback.db "SELECT id, title, substr(analysis, 1, 200) FROM feedback WHERE analysis IS NOT NULL"
```

### Analyze Mode (Recommended)

Lightweight mode that runs direct LLM API calls with tool calling. No Docker required.

```bash
# 1. Create .env from template
make setup

# 2. Edit .env
#    LLM_API_KEY=your-demeterics-api-key
#    OPENCODE_ENABLED=true
#    SELFHEALING_MODE=analyze
#    SELFHEALING_TYPES=all
#    SOURCE_DIR=.

# 3. Run
make run
```

**Security:** The LLM can only read files within `SOURCE_DIR`. Path traversal is prevented.

### OpenCode Mode (Advanced)

Full [OpenCode.ai](https://opencode.ai) integration via Docker. Can create branches and PRs.

```bash
# 1. Create .env from template
make setup

# 2. Edit .env
#    LLM_API_KEY=your-demeterics-api-key
#    OPENCODE_ENABLED=true
#    SELFHEALING_MODE=opencode
#    ADMIN_EMAILS=admin@example.com
#    OPENCODE_REPO_DIR=.

# 3. Start OpenCode container
make opencode-start

# 4. Run server (in another terminal)
make run
```

**Security:** Only admins (listed in `ADMIN_EMAILS`) can trigger OpenCode mode.

### Test LLM Connection

```bash
# Test your API key
./scripts/test-llm.sh
```

## Development

```bash
# See all available commands
make help

# Run locally (builds and runs, loads .env automatically)
make run

# Run tests
make test

# Run tests with coverage
make test-coverage

# Build binary only
make build

# Format code
make fmt

# Run linters
make lint

# Clean build artifacts
make clean
```

### OpenCode Docker Commands

```bash
# Build OpenCode container
make opencode-build

# Start OpenCode container
make opencode-start

# View container logs
make opencode-logs

# Stop container
make opencode-stop

# Shell into container
make opencode-shell

# Test analysis with sample data
make opencode-test
```

## Production Deployment

For production, you may want to:
- Switch from SQLite to PostgreSQL for high concurrency
- Add authentication middleware
- Enable content moderation via LLM guards
- Configure self-healing with OpenCode

See the production examples at [demeterics.ai](https://demeterics.ai) for a full implementation.

## Related Projects

- **AI Chat Widget** - Sister project providing conversational AI interface at [demeterics.com](https://demeterics.com)
- **Common Utilities** - Shared Go utilities at [github.com/patdeg/common](https://github.com/patdeg/common)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test ./...`
5. Submit a pull request

## Dependencies

### Go Dependencies

| Package | Version | License | Description |
|---------|---------|---------|-------------|
| [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) | v1.44.3 | BSD-3-Clause | Pure Go SQLite driver (no CGO required) |
| [github.com/joho/godotenv](https://github.com/joho/godotenv) | v1.5.1 | MIT | Load environment variables from .env files |

#### Transitive Dependencies

| Package | Version | License | Description |
|---------|---------|---------|-------------|
| [github.com/dustin/go-humanize](https://github.com/dustin/go-humanize) | v1.0.1 | MIT | Human-friendly byte sizes and time formatting |
| [github.com/google/uuid](https://github.com/google/uuid) | v1.6.0 | BSD-3-Clause | UUID generation |
| [github.com/mattn/go-isatty](https://github.com/mattn/go-isatty) | v0.0.20 | MIT | Terminal detection |
| [github.com/ncruces/go-strftime](https://github.com/ncruces/go-strftime) | v1.0.0 | MIT | strftime implementation for Go |
| [github.com/remyoudompheng/bigfft](https://github.com/remyoudompheng/bigfft) | v0.0.0-20230129092748 | BSD-3-Clause | Big integer FFT multiplication |
| [golang.org/x/exp](https://pkg.go.dev/golang.org/x/exp) | v0.0.0-20251023183803 | BSD-3-Clause | Experimental Go packages |
| [golang.org/x/sys](https://pkg.go.dev/golang.org/x/sys) | v0.37.0 | BSD-3-Clause | Go system call interface |
| [modernc.org/libc](https://pkg.go.dev/modernc.org/libc) | v1.67.6 | BSD-3-Clause | C runtime library in pure Go |
| [modernc.org/mathutil](https://pkg.go.dev/modernc.org/mathutil) | v1.7.1 | BSD-3-Clause | Math utilities |
| [modernc.org/memory](https://pkg.go.dev/modernc.org/memory) | v1.11.0 | BSD-3-Clause | Memory allocator |

### JavaScript Dependencies

The frontend widget (`widget/js/feedback-widget.js`) is written in **vanilla JavaScript** with no external dependencies. It uses only browser-native APIs:
- `fetch` API for HTTP requests
- `html2canvas` pattern for screenshot capture (inline implementation)
- DOM APIs for UI rendering

### Runtime Requirements

| Requirement | Version | Notes |
|-------------|---------|-------|
| Go | 1.24+ | Required for building the server |
| SQLite | 3.x | Embedded via modernc.org/sqlite (no system SQLite needed) |
| Docker | 20.10+ | Optional, only for OpenCode self-healing mode |

### Commercial Use

All dependencies use permissive licenses (MIT or BSD-3-Clause) that allow commercial use without restrictions. See [THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES) for complete license texts.

## License

MIT License - see [LICENSE](LICENSE) for details.

**Attribution Required:** When using substantial portions of this codebase, include a reference to the original source: https://github.com/bluefermion/feedback

## About

Built by [Blue Fermion Labs](https://bluefermionlabs.com)

This project provides the feedback collection component used in production at [demeterics.ai](https://demeterics.ai). It leverages utilities from [github.com/patdeg/common](https://github.com/patdeg/common) for logging, PII protection, and LLM integration.
