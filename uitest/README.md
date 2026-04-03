# Multi-App UI Testing Framework

Config-driven visual testing across multiple web applications with LLM-powered analysis.

## Features

- **Multi-app support** — test any number of web apps in a single run
- **Interactive login** — browser opens for manual login, cookies saved for reuse
- **3-viewport screenshots** — desktop (1920x1080), laptop (1366x768), mobile (375x667)
- **LLM vision analysis** — automated UI/UX scoring via Groq (Llama 4 Scout)
- **Console error capture** — browser console errors/warnings per page
- **Browser-use actions** — optional natural language browser automation per page
- **Structured reports** — per-app + master cross-app recommendation reports

## Setup

```bash
cd uitest
./setup.sh

# Configure
cp .env.example .env              # Add your DEMETERICS_API_KEY
cp config.yaml.example config.yaml  # Customize apps and pages
```

## Usage

```bash
# Test all apps in config.yaml
python run_uitest.py

# Test specific app
python run_uitest.py --apps feedback

# Reuse saved login sessions
python run_uitest.py --reuse-sessions

# Screenshots only (no LLM analysis)
python run_uitest.py --skip-llm

# Skip browser-use actions
python run_uitest.py --skip-browser-use

# Use alternate config
python run_uitest.py --config other.yaml

# Watch the browser
python run_uitest.py --headed
```

## Configuration

Copy `config.yaml.example` to `config.yaml` (gitignored) and customize with your apps and pages.

```yaml
apps:
  myapp:
    name: "My App"
    base_url: "https://myapp.example.com"
    login_url: "https://myapp.example.com/login"
    login_required: true
    pages:
      dashboard:
        path: "/dashboard"
        title: "Dashboard"
        purpose: "Main user dashboard"
        objectives:
          - Key metrics visible
          - Navigation is clear
        mobile_critical: true
        browser_use_action: "Click the first item and verify it opens"
```

## Output

- `screenshots/` — captured PNG/JPEG files per page per viewport
- `content/` — extracted markdown content per page
- `reports/` — per-app and master recommendation reports (markdown + JSON)
- `browser_state/` — saved login cookies per app

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DEMETERICS_API_KEY` | Yes (for LLM) | API key for vision analysis and browser-use |
| `LLM_API_BASE` | No | Override API base URL |
| `LLM_MODEL` | No | Override LLM model |
