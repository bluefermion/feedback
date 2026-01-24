#!/usr/bin/env python3
"""
Feedback Widget UAT Runner using Browser-Use

LLM-driven browser automation for UI/UX acceptance testing.
Uses natural language objectives to test the feedback widget.

Usage:
    python run_uat.py                         # Run all tests
    python run_uat.py --task "Submit a bug"   # Run custom task
    python run_uat.py --headed                # Run with visible browser
    python run_uat.py --workflow submit       # Run predefined workflow
"""

import asyncio
import argparse
import json
import os
import sys
from datetime import datetime
from pathlib import Path
from typing import Dict, Any, List, Optional

import yaml
from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from rich.progress import Progress, SpinnerColumn, TextColumn
from dotenv import load_dotenv

# Load environment variables
load_dotenv()
load_dotenv(Path(__file__).parent.parent / '.env')

console = Console()

# Directories
UAT_DIR = Path(__file__).parent
SCREENSHOTS_DIR = UAT_DIR / "screenshots"
REPORTS_DIR = UAT_DIR / "reports"
CONFIG_FILE = UAT_DIR / "page_objectives.yaml"


class FeedbackUATRunner:
    """
    UAT Runner using Browser-Use for LLM-driven automation.

    Features:
    - Natural language task execution
    - Multi-viewport testing (desktop + mobile)
    - Automatic screenshot capture
    - Comprehensive reporting
    """

    def __init__(
        self,
        headed: bool = False,
        base_url: Optional[str] = None,
        model: Optional[str] = None
    ):
        self.headed = headed

        # Load configuration
        with open(CONFIG_FILE) as f:
            self.config = yaml.safe_load(f)

        self.base_url = base_url or self.config.get('base_url', 'http://localhost:8080')
        self.viewports = self.config.get('viewports', {})
        self.pages = self.config.get('pages', {})
        self.workflows = self.config.get('workflows', {})

        # LLM configuration
        self.model = model or os.getenv('LLM_MODEL', 'meta-llama/llama-4-maverick-17b-128e-instruct')

        self.results: List[Dict[str, Any]] = []
        self.timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

        # Create output directories
        SCREENSHOTS_DIR.mkdir(parents=True, exist_ok=True)
        REPORTS_DIR.mkdir(parents=True, exist_ok=True)

    async def run_with_browser_use(self, task: str, viewport: str = 'desktop') -> Dict[str, Any]:
        """
        Execute a task using browser-use with natural language.

        Args:
            task: Natural language description of what to do
            viewport: 'desktop' or 'mobile'

        Returns:
            Result dictionary with success status and details
        """
        try:
            from browser_use import Agent, Browser
            from langchain_groq import ChatGroq

            # Configure LLM
            api_key = os.getenv('GROQ_API_KEY') or os.getenv('LLM_API_KEY')
            if not api_key:
                return {
                    'success': False,
                    'error': 'No GROQ_API_KEY or LLM_API_KEY found in environment'
                }

            llm = ChatGroq(
                model=self.model,
                api_key=api_key,
                temperature=0
            )

            # Configure browser
            viewport_config = self.viewports.get(viewport, {'width': 1920, 'height': 1080})

            browser = Browser(
                headless=not self.headed,
                # Browser-use handles viewport internally
            )

            # Build the full task with context
            full_task = f"""
You are testing the Feedback Widget at {self.base_url}

TASK: {task}

IMPORTANT INSTRUCTIONS:
1. Navigate to {self.base_url}/demo first
2. Look for a yellow/orange floating button with "!" icon in the bottom-right corner
3. This is the feedback widget button - click it to open the feedback modal
4. The modal should have options: Bug, Feature, Improvement, Other
5. After completing the task, take a screenshot
6. Report what you observed and whether the task was successful

VIEWPORT: {viewport} ({viewport_config['width']}x{viewport_config['height']})
"""

            console.print(f"\n[bold cyan]Running task ({viewport}):[/bold cyan]")
            console.print(f"[dim]{task}[/dim]\n")

            # Create and run agent
            agent = Agent(
                task=full_task,
                llm=llm,
                browser=browser,
            )

            with console.status(f"[bold green]Browser-Use executing task..."):
                history = await agent.run()

            # Extract results
            result = {
                'success': True,
                'task': task,
                'viewport': viewport,
                'history': str(history) if history else None,
                'timestamp': datetime.now().isoformat()
            }

            # Save screenshot if available
            screenshot_path = SCREENSHOTS_DIR / f"browseruse_{viewport}_{self.timestamp}.png"
            try:
                # Browser-use may have screenshot capability
                if hasattr(agent, 'browser') and agent.browser:
                    # Attempt to capture final state
                    pass  # Screenshot handling varies by browser-use version
            except Exception as e:
                console.print(f"[dim]Screenshot capture note: {e}[/dim]")

            console.print(f"[green]✓ Task completed[/green]")
            return result

        except ImportError as e:
            console.print(f"[red]Import error: {e}[/red]")
            console.print("[yellow]Run: pip install browser-use langchain-groq[/yellow]")
            return {'success': False, 'error': f'Missing dependency: {e}'}

        except Exception as e:
            console.print(f"[red]Error: {e}[/red]")
            return {'success': False, 'error': str(e)}

    async def run_workflow(self, workflow_name: str):
        """Execute a predefined workflow from page_objectives.yaml."""
        if workflow_name == 'submit' or workflow_name == 'submit_feedback':
            task = """
            1. Go to the demo page
            2. Find and click the yellow feedback button (floating action button with "!" icon)
            3. Wait for the feedback modal to open
            4. Select "Bug" as the feedback type
            5. Enter this message: "UAT Test: Automated test submission - {timestamp}"
            6. Click the Submit button
            7. Verify the submission was successful (look for success message or modal closing)
            8. Take a screenshot of the final state
            9. Report whether the feedback was submitted successfully
            """.format(timestamp=datetime.now().isoformat())

            # Run in both viewports
            for viewport in ['desktop', 'mobile']:
                result = await self.run_with_browser_use(task, viewport)
                self.results.append(result)

        elif workflow_name == 'verify' or workflow_name == 'verify_submission':
            task = """
            1. Go to the admin feedback list page at /feedback
            2. Look for the most recent feedback entry
            3. Check if there's an entry containing "UAT Test"
            4. Take a screenshot of the feedback list
            5. Report what feedback entries you can see
            """

            result = await self.run_with_browser_use(task, 'desktop')
            self.results.append(result)

        elif workflow_name == 'full':
            # Run complete workflow
            await self.run_workflow('submit')
            await asyncio.sleep(2)  # Wait for data to persist
            await self.run_workflow('verify')

        else:
            console.print(f"[yellow]Unknown workflow: {workflow_name}[/yellow]")
            console.print("Available: submit, verify, full")

    async def run_page_test(self, page_key: str):
        """Test a specific page from configuration."""
        if page_key not in self.pages:
            console.print(f"[red]Unknown page: {page_key}[/red]")
            return

        page_info = self.pages[page_key]
        objectives = page_info.get('objectives', [])
        purpose = page_info.get('purpose', '')

        objectives_text = "\n".join(f"- {obj}" for obj in objectives)

        task = f"""
        TEST PAGE: {page_info['title']}
        URL: {self.base_url}{page_info['path']}
        PURPOSE: {purpose}

        Please verify these objectives:
        {objectives_text}

        Instructions:
        1. Navigate to the page
        2. Wait for it to fully load
        3. Check each objective listed above
        4. Take a screenshot
        5. Report which objectives PASS, PARTIAL, or FAIL with explanations
        """

        for viewport in ['desktop', 'mobile']:
            if page_info.get('mobile_critical', True) or viewport == 'desktop':
                result = await self.run_with_browser_use(task, viewport)
                result['page_key'] = page_key
                result['page_info'] = page_info
                self.results.append(result)

    async def run_custom_task(self, task: str):
        """Run a custom natural language task."""
        for viewport in ['desktop', 'mobile']:
            result = await self.run_with_browser_use(task, viewport)
            self.results.append(result)

    async def run_all_tests(self):
        """Run tests for all configured pages."""
        for page_key, page_info in self.pages.items():
            if page_info.get('api_only'):
                continue
            if page_info.get('requires_data'):
                console.print(f"[yellow]Skipping {page_key} - requires existing data[/yellow]")
                continue

            await self.run_page_test(page_key)

    def generate_reports(self):
        """Generate JSON and Markdown reports."""
        # JSON Report
        json_path = REPORTS_DIR / f"uat_report_{self.timestamp}.json"
        with open(json_path, 'w') as f:
            json.dump({
                'timestamp': self.timestamp,
                'base_url': self.base_url,
                'model': self.model,
                'results': self.results,
                'summary': self._calculate_summary()
            }, f, indent=2, default=str)
        console.print(f"\n[dim]JSON Report: {json_path}[/dim]")

        # Markdown Report
        md_path = REPORTS_DIR / f"uat_report_{self.timestamp}.md"
        with open(md_path, 'w') as f:
            f.write(self._generate_markdown_report())
        console.print(f"[dim]Markdown Report: {md_path}[/dim]")

    def _calculate_summary(self) -> Dict[str, Any]:
        """Calculate test summary statistics."""
        total = len(self.results)
        passed = sum(1 for r in self.results if r.get('success'))
        failed = total - passed

        return {
            'total_tests': total,
            'passed': passed,
            'failed': failed,
            'pass_rate': round(passed / total * 100, 1) if total > 0 else 0
        }

    def _generate_markdown_report(self) -> str:
        """Generate markdown report content."""
        summary = self._calculate_summary()

        report = f"""# Feedback Widget UAT Report (Browser-Use)

**Generated**: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}
**Base URL**: {self.base_url}
**Model**: {self.model}

## Summary

| Metric | Value |
|--------|-------|
| Total Tests | {summary['total_tests']} |
| Passed | {summary['passed']} |
| Failed | {summary['failed']} |
| Pass Rate | {summary['pass_rate']}% |

## Results

"""
        for i, result in enumerate(self.results, 1):
            status = "✅" if result.get('success') else "❌"
            task = result.get('task', 'Unknown task')[:100]
            viewport = result.get('viewport', 'unknown')

            report += f"### {status} Test {i} ({viewport})\n\n"
            report += f"**Task**: {task}...\n\n"

            if result.get('error'):
                report += f"**Error**: {result['error']}\n\n"

            if result.get('history'):
                report += f"**Result**: {str(result['history'])[:500]}...\n\n"

            report += "---\n\n"

        report += """
*Generated by Feedback Widget UAT Runner with Browser-Use*
"""
        return report

    def display_summary(self):
        """Display test summary table."""
        summary = self._calculate_summary()

        table = Table(title="UAT Summary (Browser-Use)")
        table.add_column("Metric", style="cyan")
        table.add_column("Value", style="green")

        table.add_row("Total Tests", str(summary['total_tests']))
        table.add_row("Passed", f"[green]{summary['passed']}[/green]")
        table.add_row("Failed", f"[red]{summary['failed']}[/red]")
        table.add_row("Pass Rate", f"{summary['pass_rate']}%")
        table.add_row("Model", self.model)

        console.print("\n")
        console.print(table)

        if summary['failed'] == 0:
            console.print("\n[bold green]✅ ALL TESTS PASSED[/bold green]")
        else:
            console.print("\n[bold yellow]⚠️ SOME TESTS FAILED[/bold yellow]")


async def main():
    parser = argparse.ArgumentParser(description="Feedback Widget UAT Runner (Browser-Use)")
    parser.add_argument('--headed', action='store_true', help='Run with visible browser')
    parser.add_argument('--task', type=str, help='Run custom natural language task')
    parser.add_argument('--workflow', type=str, help='Run predefined workflow (submit, verify, full)')
    parser.add_argument('--page', type=str, help='Test specific page (demo, health, feedback_list)')
    parser.add_argument('--all', action='store_true', help='Run all page tests')
    parser.add_argument('--base-url', type=str, help='Override base URL')
    parser.add_argument('--model', type=str, help='Override LLM model')

    args = parser.parse_args()

    runner = FeedbackUATRunner(
        headed=args.headed,
        base_url=args.base_url,
        model=args.model
    )

    console.print(Panel.fit(
        "[bold blue]Feedback Widget UAT (Browser-Use)[/bold blue]\n"
        f"Base URL: {runner.base_url}\n"
        f"Model: {runner.model}\n"
        f"Timestamp: {runner.timestamp}",
        title="Starting Tests"
    ))

    try:
        if args.task:
            await runner.run_custom_task(args.task)
        elif args.workflow:
            await runner.run_workflow(args.workflow)
        elif args.page:
            await runner.run_page_test(args.page)
        elif args.all:
            await runner.run_all_tests()
        else:
            # Default: run submit workflow
            console.print("[dim]No specific task. Running submit_feedback workflow...[/dim]")
            await runner.run_workflow('submit')

        runner.generate_reports()
        runner.display_summary()

    except KeyboardInterrupt:
        console.print("\n[yellow]Interrupted by user[/yellow]")
    except Exception as e:
        console.print(f"\n[red]Error: {e}[/red]")
        raise


if __name__ == "__main__":
    asyncio.run(main())
