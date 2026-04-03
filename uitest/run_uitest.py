#!/usr/bin/env python3
"""
Multi-App UI Testing Framework
Automated visual testing with LLM-powered analysis across multiple web applications.

Usage:
    python run_uitest.py                          # Test all apps in config.yaml
    python run_uitest.py --apps feedback           # Test specific app
    python run_uitest.py --reuse-sessions          # Reuse saved login sessions
    python run_uitest.py --skip-llm                # Screenshots only, no LLM analysis
    python run_uitest.py --skip-browser-use        # Skip browser-use actions
    python run_uitest.py --config config.local.yaml  # Custom config file

Environment variables:
    DEMETERICS_API_KEY  - Required for LLM analysis (get from your provider)
    GROQ_API_KEY        - Alternative API key for browser-use
    LLM_API_BASE        - Override LLM API base URL
    LLM_MODEL           - Override LLM model name
"""

import argparse
import asyncio
import os
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional

from dotenv import load_dotenv
load_dotenv(override=True)

import yaml
from rich.console import Console
from rich.progress import BarColumn, Progress, SpinnerColumn, TextColumn
from rich.table import Table

from browser_actions import run_browser_action
from console_capture import ConsoleCapture
from llm_vision_analysis import LLMVisionAnalyzer
from report_generator import ReportGenerator
from screenshot_capture import ScreenshotCapture

console = Console()


class MultiAppUITester:
    def __init__(
        self,
        config_path: str = "config.yaml",
        apps_filter: Optional[str] = None,
        pages_filter: Optional[str] = None,
        reuse_sessions: bool = False,
        headed: bool = False,
        skip_llm: bool = False,
        skip_browser_use: bool = False,
    ):
        self.config_path = config_path
        self.apps_filter = apps_filter
        self.pages_filter = pages_filter
        self.reuse_sessions = reuse_sessions
        self.headed = headed
        self.skip_llm = skip_llm
        self.skip_browser_use = skip_browser_use

        self.config = self._load_config()
        self.settings = self.config.get("settings", {})
        self.viewports = self.settings.get("viewports", {
            "desktop": {"width": 1920, "height": 1080},
            "laptop": {"width": 1366, "height": 768},
            "mobile": {"width": 375, "height": 667},
        })

        self.screenshot_capture = ScreenshotCapture()
        self.console_capture = ConsoleCapture()
        self.llm_analyzer = LLMVisionAnalyzer()
        self.llm_analyzer.configure(self.settings)
        self.report_generator = ReportGenerator()

        self.all_results: Dict[str, Dict] = {}

    def _load_config(self) -> Dict:
        """Load config YAML."""
        config_path = Path(self.config_path)
        if not config_path.exists():
            console.print(f"[red]Config file not found: {config_path}[/red]")
            console.print("[dim]Copy config.yaml.example to config.yaml and customize it.[/dim]")
            sys.exit(1)

        with open(config_path) as f:
            config = yaml.safe_load(f)

        return config

    def _get_apps_to_test(self) -> Dict[str, Dict]:
        """Return filtered apps dict."""
        apps = self.config.get("apps", {})
        if self.apps_filter:
            filter_keys = [k.strip() for k in self.apps_filter.split(",")]
            apps = {k: v for k, v in apps.items() if k in filter_keys}
            if not apps:
                console.print(f"[red]No apps matched filter: {self.apps_filter}[/red]")
                console.print(f"Available: {', '.join(self.config.get('apps', {}).keys())}")
                sys.exit(1)
        return apps

    def _state_path(self, app_key: str) -> Path:
        """Browser state file path for an app."""
        return Path("browser_state") / f"{app_key}_state.json"

    async def collect_logins(self, apps: Dict[str, Dict]):
        """Interactive login collection for apps that require authentication."""
        from playwright.async_api import async_playwright

        login_apps = {k: v for k, v in apps.items() if v.get("login_required")}
        if not login_apps:
            console.print("[dim]No apps require login[/dim]")
            return

        # Check for reusable sessions
        import time
        SESSION_MAX_AGE_HOURS = 12

        if self.reuse_sessions:
            all_have_state = True
            for k in login_apps:
                sp = self._state_path(k)
                if not sp.exists():
                    all_have_state = False
                    break
                age_hours = (time.time() - sp.stat().st_mtime) / 3600
                if age_hours > SESSION_MAX_AGE_HOURS:
                    console.print(f"[yellow]Session for {login_apps[k].get('name', k)} is {age_hours:.0f}h old — will re-login[/yellow]")
                    all_have_state = False
                    break
            if all_have_state:
                console.print("[green]Reusing saved sessions for all apps[/green]")
                return
            console.print("[yellow]Some sessions missing or stale, collecting logins...[/yellow]")

        # Check if we have a display for headed browser
        has_display = bool(os.environ.get("DISPLAY") or os.environ.get("WAYLAND_DISPLAY"))

        # Determine login mode
        remote_debug_port = int(os.environ.get("CHROME_DEBUG_PORT", "9222"))
        use_remote_debug = not has_display

        if use_remote_debug:
            console.print("\n[bold yellow]No display detected (headless server / SSH session)[/bold yellow]")
            console.print(f"[cyan]Launching Chromium with remote debugging on port {remote_debug_port}[/cyan]")
            console.print(f"[cyan]Connect from your local browser:[/cyan]")
            console.print(f"[bold]  http://localhost:{remote_debug_port}[/bold]")
            console.print("[dim]If connecting via SSH, forward the port first:[/dim]")
            console.print(f"[dim]  ssh -L {remote_debug_port}:localhost:{remote_debug_port} <your-server>[/dim]\n")

        async with async_playwright() as p:
            launch_args = [
                "--disable-blink-features=AutomationControlled",
                "--disable-dev-shm-usage",
            ]
            if use_remote_debug:
                launch_args.append(f"--remote-debugging-port={remote_debug_port}")

            browser = await p.chromium.launch(
                headless=use_remote_debug,
                args=launch_args,
            )

            # Use a single shared context so OAuth sessions (Google, etc.)
            # carry over between apps — no need to re-enter password.
            context = await browser.new_context()
            page = await context.new_page()

            total = len(login_apps)
            for i, (app_key, app_config) in enumerate(login_apps.items(), 1):
                app_name = app_config.get("name", app_key)
                login_url = app_config.get("login_url") or app_config.get("base_url")

                # Skip if session exists and reuse requested
                if self.reuse_sessions and self._state_path(app_key).exists():
                    console.print(f"[dim][{i}/{total}] Reusing session for {app_name}[/dim]")
                    continue

                console.print(f"\n[bold yellow][{i}/{total}] Please login to {app_name}[/bold yellow]")
                console.print(f"[cyan]Navigating to: {login_url}[/cyan]")
                if use_remote_debug:
                    console.print(f"[yellow]Open http://localhost:{remote_debug_port} in your local browser to see and interact with the page.[/yellow]")

                await page.goto(login_url, wait_until="networkidle", timeout=30000)

                console.print("[yellow]Login in the browser, then press ENTER here when done...[/yellow]")
                await asyncio.get_event_loop().run_in_executor(None, input)

                # Verify login: check we have cookies for this app's domain
                base_domain = app_config.get("base_url", "").replace("https://", "").replace("http://", "").split("/")[0]
                cookies = await context.cookies()
                app_cookies = [c for c in cookies if base_domain in c.get("domain", "")]
                session_like = [c for c in app_cookies if any(k in c["name"].lower() for k in ["session", "sid", "token", "auth"])]

                if session_like:
                    console.print(f"[green]Session saved for {app_name} ({len(app_cookies)} cookies, {len(session_like)} session cookies)[/green]")
                else:
                    console.print(f"[yellow]Warning: No session cookie found for {base_domain}[/yellow]")
                    console.print(f"[yellow]Found {len(app_cookies)} cookies: {', '.join(c['name'] for c in app_cookies)}[/yellow]")
                    console.print("[yellow]The OAuth redirect may not have completed. Make sure you see the logged-in page before pressing ENTER.[/yellow]")

                # Save browser state (cookies + localStorage) per app
                state_path = self._state_path(app_key)
                state_path.parent.mkdir(parents=True, exist_ok=True)
                await context.storage_state(path=str(state_path))

            await context.close()
            await browser.close()

    async def test_app(self, app_key: str, app_config: Dict, playwright_instance) -> List[Dict]:
        """Test all pages for a single app."""
        app_name = app_config.get("name", app_key)
        base_url = app_config.get("base_url", "")
        pages = app_config.get("pages", {})

        if self.pages_filter:
            filter_keys = [k.strip() for k in self.pages_filter.split(",")]
            pages = {k: v for k, v in pages.items() if k in filter_keys}

        if not pages:
            console.print(f"[yellow]No pages to test for {app_name}[/yellow]")
            return []

        headless = not self.headed
        browser = await playwright_instance.chromium.launch(
            headless=headless,
            args=[
                "--disable-blink-features=AutomationControlled",
                "--disable-dev-shm-usage",
                "--no-sandbox",
            ],
        )

        # Shared context options
        base_context_options = {
            "viewport": {
                "width": self.viewports.get("desktop", {}).get("width", 1920),
                "height": self.viewports.get("desktop", {}).get("height", 1080),
            },
            "user_agent": (
                "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
                "(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
            ),
        }

        # Authenticated context (with saved session)
        auth_context_options = {**base_context_options}
        state_path = self._state_path(app_key)
        if state_path.exists():
            auth_context_options["storage_state"] = str(state_path)
        auth_context = await browser.new_context(**auth_context_options)
        auth_page = await auth_context.new_page()

        # Incognito context (clean, no cookies)
        incognito_context = await browser.new_context(**base_context_options)
        incognito_page = await incognito_context.new_page()

        results = []

        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            TextColumn("[progress.percentage]{task.percentage:>3.0f}%"),
            console=console,
        ) as progress:
            task = progress.add_task(f"Testing {app_name}", total=len(pages))

            for page_key, page_info in pages.items():
                is_incognito = page_info.get("incognito", False)
                active_page = incognito_page if is_incognito else auth_page
                mode_label = "incognito" if is_incognito else "authenticated"
                page_title = page_info.get("title", page_key)

                progress.update(
                    task,
                    description=f"Testing {app_name}: {page_title} ({mode_label})",
                )

                console.print(f"\n[bold]{page_title}[/bold] [dim]({mode_label})[/dim]")

                result = await self._test_page(
                    active_page, page_key, page_info, base_url, app_key, browser
                )
                result["mode"] = mode_label
                results.append(result)
                progress.advance(task)

        await auth_context.close()
        await incognito_context.close()
        await browser.close()

        return results

    async def _test_page(
        self,
        page,
        page_key: str,
        page_info: Dict,
        base_url: str,
        app_key: str,
        browser,
    ) -> Dict:
        """Test a single page: navigate, capture, analyze."""
        url = f"{base_url.rstrip('/')}{page_info['path']}"
        title = page_info.get("title", page_key)
        wait_seconds = self.settings.get("browser", {}).get("wait_after_load", 2)
        timeout = self.settings.get("browser", {}).get("timeout", 30000)

        result = {
            "page_key": page_key,
            "title": title,
            "url": url,
            "purpose": page_info.get("purpose", ""),
            "http_status": None,
            "console": {},
            "screenshots": {},
            "markdown_path": None,
            "analyses": {},
            "browser_action": None,
        }

        # Attach console capture
        self.console_capture.attach(page)

        # Navigate
        try:
            response = await page.goto(url, wait_until="networkidle", timeout=timeout)
            result["http_status"] = response.status if response else None
        except Exception as e:
            console.print(f"[red]  Failed to load {url}: {e}[/red]")
            result["http_status"] = 0
            result["console"] = self.console_capture.collect()
            return result

        # Collect console errors
        await asyncio.sleep(1)  # Let JS finish executing
        result["console"] = self.console_capture.collect()

        # Capture screenshots at all viewports
        try:
            result["screenshots"] = await self.screenshot_capture.capture_viewports(
                page, f"{app_key}_{page_key}", self.viewports, wait_seconds
            )
        except Exception as e:
            console.print(f"[red]  Screenshot failed for {title}: {e}[/red]")

        # Extract markdown content
        try:
            result["markdown_path"] = await self.screenshot_capture.extract_markdown(
                page, url, f"{app_key}_{page_key}"
            )
        except Exception as e:
            console.print(f"[yellow]  Markdown extraction failed: {e}[/yellow]")

        # LLM analysis per viewport
        if not self.skip_llm:
            markdown_text = None
            if result["markdown_path"]:
                try:
                    markdown_text = Path(result["markdown_path"]).read_text(encoding="utf-8")
                except Exception:
                    pass

            for vp_name, screenshot_path in result["screenshots"].items():
                try:
                    analysis = await self.llm_analyzer.analyze_page(
                        screenshot_path=screenshot_path,
                        page_info=page_info,
                        viewport=vp_name,
                        markdown_content=markdown_text,
                        console_errors=result["console"],
                    )
                    result["analyses"][vp_name] = analysis
                    score = analysis.get("analysis", {}).get("scores", {}).get("overall", "?")
                    console.print(f"  [dim]{vp_name}: {score}/10[/dim]")
                except Exception as e:
                    console.print(f"[red]  LLM analysis failed for {vp_name}: {e}[/red]")
                    result["analyses"][vp_name] = {"success": False, "error": str(e)}

        # Browser-use action (optional)
        if not self.skip_browser_use and page_info.get("browser_use_action"):
            try:
                action_result = await run_browser_action(
                    action_prompt=page_info["browser_use_action"],
                    page_url=url,
                    storage_state=str(self._state_path(app_key)),
                )
                result["browser_action"] = action_result
                status = "[green]PASS[/green]" if action_result["success"] else "[red]FAIL[/red]"
                console.print(f"  Browser action: {status}")
            except Exception as e:
                result["browser_action"] = {
                    "success": False,
                    "action": page_info["browser_use_action"],
                    "result": str(e),
                    "errors": [str(e)],
                }

        self.console_capture.reset()
        return result

    async def run(self):
        """Main entry point: collect logins, test all apps, generate reports."""
        from playwright.async_api import async_playwright

        apps = self._get_apps_to_test()

        console.print(f"\n[bold]Multi-App UI Tester[/bold]")
        console.print(f"Apps: {', '.join(a.get('name', k) for k, a in apps.items())}")
        console.print(f"Viewports: {', '.join(self.viewports.keys())}")
        if self.skip_llm:
            console.print("[yellow]LLM analysis: SKIPPED[/yellow]")
        if self.skip_browser_use:
            console.print("[yellow]Browser-use actions: SKIPPED[/yellow]")
        console.print()

        # Phase 1: Collect logins
        await self.collect_logins(apps)

        console.print("\n[bold]Starting tests...[/bold]\n")

        # Phase 2: Test each app
        async with async_playwright() as p:
            for app_key, app_config in apps.items():
                app_name = app_config.get("name", app_key)
                console.print(f"\n[bold blue]{'='*60}[/bold blue]")
                console.print(f"[bold blue]Testing: {app_name}[/bold blue]")
                console.print(f"[bold blue]{'='*60}[/bold blue]\n")

                results = await self.test_app(app_key, app_config, p)

                # Generate per-app report
                report_path = self.report_generator.generate_app_report(
                    app_key, app_name, results
                )
                console.print(f"\n[green]Report: {report_path}[/green]")

                self.all_results[app_key] = {"name": app_name, "results": results}

        # Phase 3: Master report
        if len(self.all_results) > 0:
            master_path = self.report_generator.generate_master_report(self.all_results)
            console.print(f"\n[bold green]Master report: {master_path}[/bold green]")

        # Summary table
        self._print_summary()

    def _print_summary(self):
        """Print a summary table to the console."""
        table = Table(title="\nUI Test Summary")
        table.add_column("App", style="cyan")
        table.add_column("Pages", justify="right")
        table.add_column("Avg Score", justify="right")
        table.add_column("Critical", justify="right", style="red")
        table.add_column("Console Errors", justify="right", style="yellow")

        for app_key, app_data in self.all_results.items():
            results = app_data["results"]
            scores = []
            critical = 0
            console_errs = 0

            for r in results:
                console_errs += r.get("console", {}).get("error_count", 0)
                for vp, analysis in r.get("analyses", {}).items():
                    if analysis.get("success"):
                        s = analysis.get("analysis", {}).get("scores", {}).get("overall", 0)
                        if s > 0:
                            scores.append(s)
                        for issue in analysis.get("analysis", {}).get("issues", []):
                            if issue.get("severity") == "critical":
                                critical += 1

            avg = f"{sum(scores)/len(scores):.1f}" if scores else "N/A"
            table.add_row(
                app_data["name"],
                str(len(results)),
                avg,
                str(critical),
                str(console_errs),
            )

        console.print(table)


def main():
    parser = argparse.ArgumentParser(description="Multi-App UI Testing Framework")
    parser.add_argument("--config", default="config.yaml", help="Config file path")
    parser.add_argument("--apps", default=None, help="Comma-separated app keys to test")
    parser.add_argument("--pages", default=None, help="Comma-separated page keys to test")
    parser.add_argument("--reuse-sessions", action="store_true", help="Reuse saved login sessions")
    parser.add_argument("--headed", action="store_true", help="Run browser in headed mode")
    parser.add_argument("--skip-llm", action="store_true", help="Skip LLM analysis")
    parser.add_argument("--skip-browser-use", action="store_true", help="Skip browser-use actions")
    args = parser.parse_args()

    tester = MultiAppUITester(
        config_path=args.config,
        apps_filter=args.apps,
        pages_filter=args.pages,
        reuse_sessions=args.reuse_sessions,
        headed=args.headed,
        skip_llm=args.skip_llm,
        skip_browser_use=args.skip_browser_use,
    )

    asyncio.run(tester.run())


if __name__ == "__main__":
    main()
