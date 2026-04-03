"""
Browser console error/warning capture with deduplication.
Attaches to a Playwright page and collects console messages during navigation.
"""

from typing import Dict, List


class ConsoleCapture:
    def __init__(self):
        self._messages: List[Dict] = []

    def attach(self, page):
        """Attach console listener to a Playwright page. Call before navigation."""
        self._messages = []

        def on_console(msg):
            if msg.type in ("error", "warning"):
                self._messages.append({
                    "type": msg.type,
                    "text": msg.text,
                })

        page.on("console", on_console)

    def collect(self) -> Dict:
        """
        Collect and deduplicate captured messages.
        Returns: {errors: [...], warnings: [...], error_count: N, warning_count: N}
        """
        errors = [m for m in self._messages if m["type"] == "error"]
        warnings = [m for m in self._messages if m["type"] == "warning"]

        # Deduplicate by text
        unique_errors = list({e["text"]: e for e in errors}.values())
        unique_warnings = list({w["text"]: w for w in warnings}.values())

        return {
            "errors": unique_errors,
            "warnings": unique_warnings,
            "error_count": len(unique_errors),
            "warning_count": len(unique_warnings),
        }

    def reset(self):
        """Clear captured messages for next page."""
        self._messages = []
