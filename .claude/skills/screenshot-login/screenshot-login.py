#!/usr/bin/env python3
"""Open a headed browser for manual login, then save the session for screenshot-capture.py.

Usage:
    screenshot-login.py <url> [--name SESSION_NAME] [--port 9222]

Sessions are saved to ~/.claude/tools/screenshot-sessions/<domain>.json
The session file contains cookies + localStorage that screenshot-capture.py loads automatically.

On headless servers (no DISPLAY), launches Chromium with --remote-debugging-port
so you can connect from a local browser via SSH tunnel.
"""

import argparse
import asyncio
import json
import os
import sys
import time
from pathlib import Path
from urllib.parse import urlparse

from playwright.async_api import async_playwright

SESSIONS_DIR = Path.home() / ".claude" / "tools" / "screenshot-sessions"
SESSION_MAX_AGE_HOURS = 72


def get_domain(url: str) -> str:
    """Extract domain from URL for session naming."""
    parsed = urlparse(url)
    return parsed.netloc or parsed.path.split("/")[0]


def session_path(name: str) -> Path:
    """Get session file path."""
    return SESSIONS_DIR / f"{name}.json"


def list_sessions():
    """List saved sessions with age."""
    if not SESSIONS_DIR.exists():
        print("No sessions saved yet.")
        return
    sessions = sorted(SESSIONS_DIR.glob("*.json"))
    if not sessions:
        print("No sessions saved yet.")
        return
    print(f"{'Session':<30} {'Age':>8}  {'Status':<10}  Path")
    print("-" * 80)
    for sp in sessions:
        name = sp.stem
        age_hours = (time.time() - sp.stat().st_mtime) / 3600
        if age_hours < 1:
            age_str = f"{age_hours * 60:.0f}m"
        elif age_hours < 24:
            age_str = f"{age_hours:.1f}h"
        else:
            age_str = f"{age_hours / 24:.1f}d"
        status = "fresh" if age_hours < SESSION_MAX_AGE_HOURS else "stale"
        print(f"{name:<30} {age_str:>8}  {status:<10}  {sp}")


async def wait_for_cookies(context, domain: str, timeout_seconds: int = 300):
    """Poll for any meaningful cookies on the target domain. Returns True if found."""
    poll_interval = 3
    elapsed = 0
    # Ignore these generic cookies that appear before login
    ignore_names = {"id", "_ga", "_gid", "_gat", "nid", "consent", "1p_jar"}

    while elapsed < timeout_seconds:
        await asyncio.sleep(poll_interval)
        elapsed += poll_interval
        cookies = await context.cookies()
        # Match cookies for the target domain (handles both ".domain.com" and "domain.com")
        domain_cookies = [
            c for c in cookies
            if domain in c.get("domain", "") and c["name"].lower() not in ignore_names
        ]
        if domain_cookies:
            # Wait a couple more seconds for any remaining cookie writes
            await asyncio.sleep(3)
            return True
        if elapsed % 30 == 0:
            print(f"  Still waiting... ({elapsed}s elapsed, {timeout_seconds - elapsed}s remaining)")
    return False


async def login(url: str, session_name: str, debug_port: int = 9222, wait_seconds: int = 300):
    """Open browser for login, save session."""
    SESSIONS_DIR.mkdir(parents=True, exist_ok=True)

    has_display = bool(os.environ.get("DISPLAY") or os.environ.get("WAYLAND_DISPLAY"))

    async with async_playwright() as p:
        launch_args = [
            "--disable-blink-features=AutomationControlled",
            "--disable-dev-shm-usage",
        ]

        if has_display:
            print(f"Opening browser to: {url}")
            print(f"Log in normally. The browser stays open for up to {wait_seconds // 60} minutes.")
            print("Session auto-saves when login cookies are detected.")
            print()
            browser = await p.chromium.launch(headless=False, args=launch_args)
        else:
            launch_args.append(f"--remote-debugging-port={debug_port}")
            print(f"No display detected — launching headless with remote debugging.")
            print(f"Connect from your local machine:")
            print(f"  1. SSH tunnel: ssh -L {debug_port}:localhost:{debug_port} $(hostname)")
            print(f"  2. Open in browser: http://localhost:{debug_port}")
            print(f"Then log in via the remote browser. Session auto-saves on login detection.")
            browser = await p.chromium.launch(headless=True, args=launch_args)

        context = await browser.new_context(
            user_agent="Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36"
        )
        page = await context.new_page()

        try:
            await page.goto(url, wait_until="domcontentloaded", timeout=30000)
        except Exception as e:
            print(f"Navigation warning: {e}")
            # Page may still be usable even if timeout fires

        # Auto-detect login by polling for cookies only
        domain = get_domain(url)
        detected = await wait_for_cookies(context, domain, wait_seconds)

        # Collect final cookie state
        cookies = await context.cookies()
        all_domain_cookies = [c for c in cookies if domain in c.get("domain", "")]

        if detected and all_domain_cookies:
            print(f"Login detected! Found {len(all_domain_cookies)} cookie(s): "
                  f"{', '.join(c['name'] for c in all_domain_cookies)}")
        elif all_domain_cookies:
            print(f"Found {len(all_domain_cookies)} cookie(s): "
                  f"{', '.join(c['name'] for c in all_domain_cookies)}")
            print("Saving — they may still work.")
        else:
            print(f"Timed out after {wait_seconds // 60} minutes. No cookies found for {domain}.")
            print("Login may not have completed. Try again if needed.")

        # Save session state (cookies + localStorage)
        sp = session_path(session_name)
        await context.storage_state(path=str(sp))
        print(f"Session saved: {sp}")

        await context.close()
        await browser.close()


def main():
    parser = argparse.ArgumentParser(description="Login and save browser session for screenshots")
    parser.add_argument("url", nargs="?", help="Login URL")
    parser.add_argument("--name", default=None,
                        help="Session name (default: domain from URL)")
    parser.add_argument("--port", type=int, default=9222,
                        help="Remote debugging port for headless servers (default: 9222)")
    parser.add_argument("--list", action="store_true",
                        help="List saved sessions")
    parser.add_argument("--delete", metavar="NAME",
                        help="Delete a saved session")
    args = parser.parse_args()

    if args.list:
        list_sessions()
        return

    if args.delete:
        sp = session_path(args.delete)
        if sp.exists():
            sp.unlink()
            print(f"Deleted: {sp}")
        else:
            print(f"Session not found: {args.delete}")
        return

    if not args.url:
        parser.print_help()
        sys.exit(1)

    url = args.url
    if not url.startswith("http"):
        url = f"https://{url}"

    session_name = args.name or get_domain(url)

    asyncio.run(login(url, session_name, args.port))


if __name__ == "__main__":
    main()
