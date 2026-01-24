# Feedback Widget UAT

LLM-driven UI/UX Acceptance Testing using [Browser-Use](https://github.com/browser-use/browser-use).

## Why Browser-Use?

Browser-Use allows AI agents to control browsers through **natural language**. Instead of writing brittle CSS selectors and complex automation scripts, you describe what you want in plain English:

```python
task = "Go to the demo page, click the yellow feedback button, select Bug, type a message, and submit"
```

The LLM interprets this and executes the appropriate browser actions autonomously.

## Features

- **Natural Language Tasks** - Describe tests in plain English
- **LLM-Driven Automation** - Groq Llama 4 Maverick interprets and executes
- **Multi-Viewport Testing** - Desktop (1920x1080) and Mobile (375x667)
- **Objective-Based Verification** - Plain English success criteria
- **Comprehensive Reporting** - JSON and Markdown reports

## Quick Start

### 1. Setup (One-Time)

```bash
# From project root - sets up Python venv, installs deps, playwright
make uat-setup
```

Or manually:
```bash
cd uat
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
playwright install chromium
```

### 2. Configure Environment

Add to `.env` in project root:

```bash
# Groq API (required)
GROQ_API_KEY=your-groq-api-key

# Optional: Override model
LLM_MODEL=meta-llama/llama-4-maverick-17b-128e-instruct
```

Get your Groq API key at: https://console.groq.com/keys

### 3. Start the Feedback Server

```bash
# From project root
make run
# Or: go run ./cmd/server
```

### 4. Run UAT Tests

**Using Makefile (Recommended):**
```bash
# Run submit workflow (default)
make uat

# Run with visible browser
make uat-headed

# Run specific workflows
make uat-submit     # Submit feedback
make uat-verify     # Check admin list
make uat-full       # Submit + verify
make uat-demo       # Full workflow with visible browser

# Run custom task
make uat-task TASK="Click the feedback button and submit a bug"

# Run all page tests
make uat-all

# Clean artifacts
make uat-clean
```

**Using Shell Script:**
```bash
./uat/run_uat.sh                              # Default workflow
./uat/run_uat.sh --headed --workflow full     # Full with visible browser
./uat/run_uat.sh --task "Submit a bug report" # Custom task
```

**Using Python Directly:**
```bash
cd uat && source .venv/bin/activate
python run_uat.py --workflow submit
python run_uat.py --headed --task "Verify the feedback button exists"
```

## How It Works

### 1. Task Definition

Tests are defined as natural language objectives:

```yaml
# page_objectives.yaml
demo:
  objectives:
    - Page loads within 3 seconds
    - Yellow feedback button visible in bottom-right
    - Clicking button opens feedback modal
    - Modal shows type selection (Bug, Feature, Improvement, Other)
```

### 2. LLM Interpretation

Browser-Use passes the task to Groq's Llama 4 Maverick, which:
- Understands the intent
- Plans the necessary steps
- Identifies UI elements to interact with
- Executes browser actions
- Reports results

### 3. Autonomous Execution

The agent autonomously:
- Navigates to pages
- Finds and clicks elements
- Types text
- Handles modals and forms
- Takes screenshots
- Verifies outcomes

## Example Tasks

### Submit Feedback
```bash
python run_uat.py --task "
Go to the demo page, find the yellow feedback button in the corner,
click it to open the modal, select Bug as the type, enter a test
message, and submit the form
"
```

### Verify UI Elements
```bash
python run_uat.py --task "
Navigate to /demo and verify:
1. The page has a title
2. There's a yellow floating button
3. The button has an exclamation mark icon
4. Clicking it opens a modal with feedback options
"
```

### Check Admin Dashboard
```bash
python run_uat.py --task "
Go to /feedback admin page and check if there are any feedback
entries. Report how many entries exist and their types.
"
```

## Configuration

### page_objectives.yaml

```yaml
base_url: "http://localhost:8080"

pages:
  demo:
    path: "/demo"
    title: "Feedback Widget Demo"
    purpose: "Interactive demo of feedback widget"

    objectives:
      - Page loads within 3 seconds
      - Yellow feedback button visible
      - Modal opens on click
      - Type selection works

    key_elements:
      - Feedback FAB button
      - Modal dialog
      - Submit button

    mobile_critical: true

workflows:
  submit_feedback:
    name: "Submit Bug Report"
    description: "Complete feedback submission flow"
```

### Viewports

```yaml
viewports:
  desktop:
    width: 1920
    height: 1080
  mobile:
    width: 375
    height: 667
```

## Reports

### Output Structure

```
uat/
├── screenshots/
│   └── browseruse_desktop_20260123_140530.png
└── reports/
    ├── uat_report_20260123_140530.json
    └── uat_report_20260123_140530.md
```

### JSON Report

```json
{
  "timestamp": "20260123_140530",
  "base_url": "http://localhost:8080",
  "model": "meta-llama/llama-4-maverick-17b-128e-instruct",
  "results": [...],
  "summary": {
    "total_tests": 2,
    "passed": 2,
    "failed": 0,
    "pass_rate": 100.0
  }
}
```

## Comparison: Browser-Use vs Raw Playwright

| Aspect | Raw Playwright | Browser-Use |
|--------|---------------|-------------|
| Test Definition | CSS selectors, XPath | Natural language |
| Maintenance | Brittle, breaks with UI changes | Adapts automatically |
| Element Finding | Manual selector writing | LLM understands context |
| Error Recovery | Custom error handling | Built-in retry logic |
| Learning Curve | Learn Playwright API | Write English |

## Troubleshooting

### Import Errors

```bash
# Ensure all dependencies installed
pip install browser-use langchain-groq playwright
playwright install chromium
```

### No API Key

```bash
# Check environment
echo $GROQ_API_KEY

# Or add to .env file
echo "GROQ_API_KEY=your-key" >> ../.env
```

### Browser Won't Start

```bash
# Reinstall browser
playwright install --force chromium

# Or use browser-use installer
uvx browser-use install
```

### Task Not Completing

Try with `--headed` to see what's happening:
```bash
python run_uat.py --headed --task "your task"
```

## Advanced Usage

### Custom LLM Configuration

```python
from langchain_groq import ChatGroq

llm = ChatGroq(
    model='meta-llama/llama-4-maverick-17b-128e-instruct',
    temperature=0,  # Deterministic
    max_tokens=4096
)
```

### With Vision Analysis

Combine browser-use with the included `llm_vision.py` for screenshot analysis:

```python
from llm_vision import LLMVisionAnalyzer

analyzer = LLMVisionAnalyzer()
result = await analyzer.analyze_screenshot(
    'screenshots/test.png',
    page_info,
    'desktop'
)
```

## Resources

- [Browser-Use GitHub](https://github.com/browser-use/browser-use)
- [Browser-Use Documentation](https://docs.browser-use.com)
- [Groq Console](https://console.groq.com)
- [LangChain Groq](https://python.langchain.com/docs/integrations/providers/groq/)

## License

MIT - See project root LICENSE file.
