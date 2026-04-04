---
name: screenshot-login
description: Open a browser for manual login to save session cookies, so /screenshot can access authenticated pages. Also lists and manages saved sessions.
argument-hint: "<login-url> | --list | --delete <name>"
allowed-tools: Bash
---

# Screenshot Login

Manage browser sessions for authenticated screenshots: **$ARGUMENTS**

## Instructions

This skill manages saved browser sessions so that `/screenshot` can capture pages that require login.

### Parse the arguments

- If `$ARGUMENTS` contains `--list` or `list`: list saved sessions
- If `$ARGUMENTS` contains `--delete <name>` or `delete <name>`: delete a session
- Otherwise: treat it as a login URL

### List sessions

```bash
~/.claude/tools/screenshot-venv/bin/python ~/.claude/tools/screenshot-login.py --list
```

Show the output to the user. Note which sessions are fresh vs stale.

### Delete a session

```bash
~/.claude/tools/screenshot-venv/bin/python ~/.claude/tools/screenshot-login.py --delete "<name>"
```

### Login to a site

This requires user interaction — a browser window opens and the user must log in manually.

**IMPORTANT:** Tell the user what will happen BEFORE running the command:
1. A Chromium browser window will open on their screen (if DISPLAY is available)
2. They should log in normally
3. The script auto-detects login completion by polling for cookies
4. The session cookies are saved for future `/screenshot` use

Then run:

```bash
~/.claude/tools/screenshot-venv/bin/python ~/.claude/tools/screenshot-login.py "<url>"
```

If the URL doesn't start with `http`, prepend `https://`.

Optional: `--name <custom-name>` to override the default session name (domain-based).

**On headless servers (no DISPLAY):** The script will print SSH tunnel instructions instead of opening a window. Relay those instructions to the user.

### After login

Tell the user their session is saved and will be automatically used by `/screenshot` when capturing pages on that domain. Sessions are matched by domain name.

Example: after logging into `https://demeterics.ai/login`, any future `/screenshot https://demeterics.ai/dashboard` will automatically use the saved cookies.

### Session storage

Sessions are stored at: `~/.claude/tools/screenshot-sessions/<domain>.json`

They contain cookies and localStorage — do NOT read or display the contents (they contain auth tokens).

## Setup

Requires a one-time setup of the Python venv and Playwright:

```bash
python3 -m venv ~/.claude/tools/screenshot-venv
~/.claude/tools/screenshot-venv/bin/pip install playwright pillow
~/.claude/tools/screenshot-venv/bin/playwright install chromium
~/.claude/tools/screenshot-venv/bin/playwright install-deps chromium
```
