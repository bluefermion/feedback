"""
Report generator for multi-app UI testing.
Produces per-app markdown reports and a master cross-app recommendation file.
"""

import json
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List


class ReportGenerator:
    def __init__(self, output_dir: str = "reports"):
        self.output_dir = Path(output_dir)
        self.output_dir.mkdir(parents=True, exist_ok=True)
        self.timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

    def generate_app_report(self, app_key: str, app_name: str, results: List[Dict]) -> str:
        """Generate a per-app markdown report. Returns file path."""
        filepath = self.output_dir / f"{app_key}_report_{self.timestamp}.md"

        lines = [
            f"# UI Test Report: {app_name}",
            f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
            "",
            "## Summary",
            "",
        ]

        # Summary table
        lines.append("| Page | Viewport | Score | Issues | Console Errors |")
        lines.append("|------|----------|-------|--------|----------------|")

        for r in results:
            for vp, analysis in r.get("analyses", {}).items():
                score = self._get_score(analysis)
                emoji = self._score_emoji(score)
                issue_count = len(analysis.get("analysis", {}).get("issues", []))
                console_errs = r.get("console", {}).get("error_count", 0)
                lines.append(
                    f"| {r['title']} | {vp} | {emoji} {score}/10 | {issue_count} | {console_errs} |"
                )

        lines.append("")

        # Page details
        lines.append("## Page Analysis")
        lines.append("")

        for r in results:
            lines.append(f"### {r['title']}")
            lines.append(f"**URL:** `{r['url']}`")
            lines.append(f"**Purpose:** {r.get('purpose', 'N/A')}")
            lines.append("")

            # Console errors
            console = r.get("console", {})
            if console.get("error_count", 0) > 0:
                lines.append("**Console Errors:**")
                for err in console.get("errors", []):
                    lines.append(f"- `{err['text'][:150]}`")
                lines.append("")

            # Browser action result
            action = r.get("browser_action")
            if action:
                status = "PASS" if action.get("success") else "FAIL"
                lines.append(f"**Browser Action:** {status}")
                lines.append(f"- Task: {action.get('action', 'N/A')}")
                lines.append(f"- Result: {action.get('result', 'N/A')}")
                if action.get("errors"):
                    for err in action["errors"]:
                        lines.append(f"- Error: {err}")
                lines.append("")

            # Per-viewport analysis
            for vp, analysis_result in r.get("analyses", {}).items():
                if not analysis_result.get("success"):
                    lines.append(f"**{vp.title()}:** Analysis failed - {analysis_result.get('error', 'unknown')}")
                    lines.append("")
                    continue

                analysis = analysis_result.get("analysis", {})
                score = self._get_score(analysis_result)
                lines.append(f"#### {vp.title()} {self._score_emoji(score)} {score}/10")
                lines.append("")

                # First impression
                fi = analysis.get("first_impression", {})
                if fi:
                    lines.append(f"**First Impression:** {fi.get('3_second_understanding', 'N/A')}")
                    lines.append(f"- Purpose clear: {'Yes' if fi.get('purpose_clear') else 'No'}")
                    lines.append("")

                # Scores breakdown
                scores = analysis.get("scores", {})
                if scores:
                    lines.append("**Scores:**")
                    for key, val in scores.items():
                        if val is not None and key != "overall":
                            lines.append(f"- {key.replace('_', ' ').title()}: {val}/10")
                    lines.append("")

                # Issues
                issues = analysis.get("issues", [])
                if issues:
                    lines.append("**Issues:**")
                    for issue in issues:
                        sev = issue.get("severity", "medium").upper()
                        lines.append(f"- [{sev}] **{issue.get('element', 'Unknown')}**: {issue.get('problem', '')}")
                        if issue.get("fix"):
                            lines.append(f"  - Fix: {issue['fix']}")
                    lines.append("")

                # Positive findings
                positives = analysis.get("positive_findings", [])
                if positives:
                    lines.append("**What Works Well:**")
                    for p in positives:
                        lines.append(f"- {p}")
                    lines.append("")

                # Priority recommendations
                recs = analysis.get("priority_recommendations", [])
                if recs:
                    lines.append("**Recommendations:**")
                    for rec in recs:
                        lines.append(f"{rec.get('priority', '-')}. {rec.get('action', '')}")
                        if rec.get("expected_impact"):
                            lines.append(f"   Impact: {rec['expected_impact']}")
                    lines.append("")

            lines.append("---")
            lines.append("")

        report_text = "\n".join(lines)
        filepath.write_text(report_text, encoding="utf-8")

        # Also save JSON
        json_path = self.output_dir / f"{app_key}_report_{self.timestamp}.json"
        json_path.write_text(json.dumps(results, indent=2, default=str), encoding="utf-8")

        return str(filepath)

    def generate_master_report(self, all_app_results: Dict[str, Dict]) -> str:
        """
        Generate a cross-app master recommendation report.
        all_app_results: {app_key: {name: str, results: [...]}}
        """
        filepath = self.output_dir / f"master_recommendation_{self.timestamp}.md"

        lines = [
            "# Master UI Test Report",
            f"Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}",
            "",
            "## Executive Summary",
            "",
            "| App | Pages Tested | Avg Score | Critical Issues | Console Errors |",
            "|-----|-------------|-----------|-----------------|----------------|",
        ]

        all_issues: List[Dict] = []
        all_recommendations: List[Dict] = []

        for app_key, app_data in all_app_results.items():
            app_name = app_data["name"]
            results = app_data["results"]

            scores = []
            critical_count = 0
            console_errors_total = 0

            for r in results:
                console_errors_total += r.get("console", {}).get("error_count", 0)
                for vp, analysis_result in r.get("analyses", {}).items():
                    score = self._get_score(analysis_result)
                    if score > 0:
                        scores.append(score)
                    analysis = analysis_result.get("analysis", {})
                    for issue in analysis.get("issues", []):
                        if issue.get("severity") == "critical":
                            critical_count += 1
                        issue_with_context = {**issue, "app": app_name, "page": r["title"], "viewport": vp}
                        all_issues.append(issue_with_context)
                    for rec in analysis.get("priority_recommendations", []):
                        rec_with_context = {**rec, "app": app_name, "page": r["title"]}
                        all_recommendations.append(rec_with_context)

            avg_score = sum(scores) / len(scores) if scores else 0
            emoji = self._score_emoji(avg_score)
            lines.append(
                f"| {app_name} | {len(results)} | {emoji} {avg_score:.1f}/10 | {critical_count} | {console_errors_total} |"
            )

        lines.append("")

        # Critical issues across all apps
        critical_issues = [i for i in all_issues if i.get("severity") == "critical"]
        high_issues = [i for i in all_issues if i.get("severity") == "high"]

        if critical_issues:
            lines.append("## Critical Issues (Immediate Action Required)")
            lines.append("")
            for i, issue in enumerate(critical_issues, 1):
                lines.append(f"### {i}. [{issue['app']}] {issue.get('page', '')} - {issue.get('element', '')}")
                lines.append(f"- **Problem:** {issue.get('problem', '')}")
                lines.append(f"- **Impact:** {issue.get('impact', '')}")
                lines.append(f"- **Fix:** {issue.get('fix', '')}")
                lines.append(f"- **Viewport:** {issue.get('viewport', '')}")
                lines.append("")

        if high_issues:
            lines.append("## High Priority Issues")
            lines.append("")
            for i, issue in enumerate(high_issues, 1):
                lines.append(f"{i}. **[{issue['app']}] {issue.get('page', '')}** - {issue.get('element', '')}: {issue.get('problem', '')}")
                if issue.get("fix"):
                    lines.append(f"   - Fix: {issue['fix']}")
            lines.append("")

        # Top 10 recommendations
        if all_recommendations:
            lines.append("## Top Recommendations")
            lines.append("")
            # Sort by priority (lower = higher priority)
            sorted_recs = sorted(all_recommendations, key=lambda r: r.get("priority", 99))
            for i, rec in enumerate(sorted_recs[:10], 1):
                lines.append(f"{i}. **[{rec['app']}] {rec.get('page', '')}**: {rec.get('action', '')}")
                if rec.get("expected_impact"):
                    lines.append(f"   - Expected impact: {rec['expected_impact']}")
            lines.append("")

        # Worst scoring pages
        all_page_scores = []
        for app_key, app_data in all_app_results.items():
            for r in app_data["results"]:
                for vp, analysis_result in r.get("analyses", {}).items():
                    score = self._get_score(analysis_result)
                    if score > 0:
                        all_page_scores.append({
                            "app": app_data["name"],
                            "page": r["title"],
                            "viewport": vp,
                            "score": score,
                        })

        if all_page_scores:
            worst = sorted(all_page_scores, key=lambda x: x["score"])[:10]
            lines.append("## Lowest Scoring Pages")
            lines.append("")
            lines.append("| App | Page | Viewport | Score |")
            lines.append("|-----|------|----------|-------|")
            for w in worst:
                emoji = self._score_emoji(w["score"])
                lines.append(f"| {w['app']} | {w['page']} | {w['viewport']} | {emoji} {w['score']}/10 |")
            lines.append("")

        report_text = "\n".join(lines)
        filepath.write_text(report_text, encoding="utf-8")
        return str(filepath)

    def _get_score(self, analysis_result: Dict) -> float:
        """Extract overall score from an analysis result."""
        if not analysis_result.get("success"):
            return 0
        analysis = analysis_result.get("analysis", {})
        scores = analysis.get("scores", {})
        return float(scores.get("overall", 0))

    def _score_emoji(self, score: float) -> str:
        if score >= 8:
            return "[PASS]"
        elif score >= 6:
            return "[WARN]"
        elif score > 0:
            return "[FAIL]"
        return "[N/A]"
