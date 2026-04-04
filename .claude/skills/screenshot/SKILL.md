---
name: screenshot
description: Capture screenshots of a URL at mobile (375px), laptop (1366px), and desktop (1920px) viewports using Playwright. Use when you need to see the rendered UI of a web page to evaluate design, debug layout issues, or verify changes.
argument-hint: "<url> [--viewports mobile,laptop,desktop] [--viewport-only] [--wait N] [--session NAME]"
allowed-tools: Bash, Read
---

# Screenshot Capture

Capture and view rendered screenshots of: **$ARGUMENTS**

## Instructions

Use the Playwright-based screenshot tool to capture the URL at multiple viewport sizes, then view each screenshot with the Read tool.

### Step 1: Parse arguments

Extract from `$ARGUMENTS`:
- **URL** (required): the first argument that looks like a URL (starts with http/https or is a domain)
- **--viewports**: comma-separated list (default: `mobile,laptop,desktop`). Supports custom sizes like `1440x900`
- **--viewport-only**: capture only the visible viewport, not the full scrollable page
- **--wait N**: seconds to wait after load (default: 2, increase for JS-heavy pages)
- **--no-optimize**: skip image optimization for LLM
- **--session NAME**: explicitly specify a saved session name (default: auto-detect by domain)

If the URL doesn't start with `http`, prepend `https://`.

### Step 2: Capture screenshots

```bash
~/.claude/tools/screenshot-venv/bin/python ~/.claude/tools/screenshot-capture.py "<url>" \
  --viewports mobile,laptop,desktop \
  --wait 2
```

The script outputs one line per viewport:
```
mobile: /tmp/claude-screenshots-XXXX/mobile_375x667.png
laptop: /tmp/claude-screenshots-XXXX/laptop_1366x768.png
desktop: /tmp/claude-screenshots-XXXX/desktop_1920x1080.png
```

### Step 3: View all screenshots

Use the **Read** tool to view each screenshot file path from the output. Read ALL viewport screenshots so you can see the full responsive behavior.

### Step 4: Analyze what you see

After viewing the screenshots, provide a brief UI assessment covering:
- **Layout**: Does it respond well across viewports?
- **Readability**: Text size, contrast, spacing
- **Issues**: Anything broken, overlapping, or cut off
- **Suggestions**: Concrete improvements if asked

## Viewport Reference

| Name | Width | Height | Device Class |
|------|-------|--------|-------------|
| mobile | 375px | 667px | iPhone SE / small phone |
| laptop | 1366px | 768px | Standard laptop |
| desktop | 1920px | 1080px | Full HD monitor |

## Options for specific use cases

- **Quick check** (single viewport): `--viewports desktop`
- **Mobile-first review**: `--viewports mobile,laptop`
- **Above-the-fold only**: `--viewport-only` (no full-page scroll)
- **SPA/JS-heavy sites**: `--wait 5` (longer wait for client rendering)
- **Custom breakpoint**: `--viewports 768x1024` (e.g., tablet)

## Authentication

Sessions saved via `/screenshot-login` are **automatically loaded** when the URL domain matches. No extra flags needed.

- The script looks for `~/.claude/tools/screenshot-sessions/<domain>.json`
- Use `--session NAME` to explicitly pick a session
- If no session exists for the domain, it captures as an anonymous visitor
- If the page shows a login screen instead of content, tell the user to run `/screenshot-login <login-url>` first

**Stderr** will show `session: /path/to/session.json` when a session is loaded.

## Setup

Requires a one-time setup of the Python venv and Playwright:

```bash
python3 -m venv ~/.claude/tools/screenshot-venv
~/.claude/tools/screenshot-venv/bin/pip install playwright pillow
~/.claude/tools/screenshot-venv/bin/playwright install chromium
~/.claude/tools/screenshot-venv/bin/playwright install-deps chromium
```

## Troubleshooting

- If the page requires authentication, screenshots will show the login page. Tell the user to run `/screenshot-login <login-url>` first.
- If the page fails to load, the script will retry with `domcontentloaded` instead of `networkidle`.
- Images are automatically optimized for LLM vision (max 33MP, max 3.5MB). Use `--no-optimize` to get raw screenshots.
