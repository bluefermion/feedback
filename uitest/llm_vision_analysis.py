"""
LLM Vision Analysis module.
Sends screenshots + page context to Groq via Demeterics API for UI/UX scoring.
"""

import base64
import json
import os
import re
from pathlib import Path
from typing import Any, Dict, Optional

import aiohttp


class LLMVisionAnalyzer:
    def __init__(self):
        self.api_base = os.environ.get("LLM_API_BASE", "https://api.demeterics.com")
        self.api_key = os.environ.get("DEMETERICS_API_KEY", "")
        self.model = os.environ.get("LLM_MODEL", "meta-llama/llama-4-scout-17b-16e-instruct")
        self.max_tokens = int(os.environ.get("LLM_MAX_TOKENS", "8000"))
        self.temperature = float(os.environ.get("LLM_TEMPERATURE", "0.1"))

    def configure(self, settings: Dict[str, Any]):
        """Override defaults from config.yaml settings."""
        llm = settings.get("llm", {})
        if not self.api_key:
            self.api_key = os.environ.get("DEMETERICS_API_KEY", "")
        if "api_base" in llm and not os.environ.get("LLM_API_BASE"):
            self.api_base = llm["api_base"]
        if "model" in llm and not os.environ.get("LLM_MODEL"):
            self.model = llm["model"]
        if "max_tokens" in llm and not os.environ.get("LLM_MAX_TOKENS"):
            self.max_tokens = llm["max_tokens"]
        if "temperature" in llm and not os.environ.get("LLM_TEMPERATURE"):
            self.temperature = llm["temperature"]

    async def analyze_page(
        self,
        screenshot_path: str,
        page_info: Dict[str, Any],
        viewport: str,
        markdown_content: Optional[str] = None,
        console_errors: Optional[Dict] = None,
    ) -> Dict[str, Any]:
        """
        Analyze a page screenshot with LLM vision.
        Returns structured analysis with scores and recommendations.
        """
        if not self.api_key:
            return {"success": False, "error": "DEMETERICS_API_KEY not set"}

        prompt = self._build_prompt(page_info, viewport, markdown_content, console_errors)
        # Prepend Demeterics tracking directives for cost attribution
        tracking = f"/// APP Feedback-UI-Test\n/// FLOW vision-{viewport}\n"
        prompt = tracking + prompt

        try:
            result = await self._call_vision_api(screenshot_path, prompt)
            return {"success": True, "analysis": result, "model": self.model}
        except Exception as e:
            return {"success": False, "error": str(e)}

    def _build_prompt(
        self,
        page_info: Dict[str, Any],
        viewport: str,
        markdown_content: Optional[str] = None,
        console_errors: Optional[Dict] = None,
    ) -> str:
        title = page_info.get("title", "Unknown")
        purpose = page_info.get("purpose", "Not specified")
        objectives = page_info.get("objectives", [])

        viewport_labels = {
            "desktop": "Desktop (1920x1080)",
            "laptop": "Laptop (1366x768)",
            "mobile": "Mobile (375x667)",
        }
        vp_label = viewport_labels.get(viewport, viewport)

        objectives_text = "\n".join(f"  - {obj}" for obj in objectives)

        console_section = ""
        if console_errors and (console_errors.get("error_count", 0) > 0 or console_errors.get("warning_count", 0) > 0):
            console_section = "\n\nCONSOLE ISSUES DETECTED:\n"
            for err in console_errors.get("errors", []):
                console_section += f"  ERROR: {err['text'][:200]}\n"
            for warn in console_errors.get("warnings", []):
                console_section += f"  WARNING: {warn['text'][:200]}\n"

        content_section = ""
        if markdown_content:
            # Truncate to avoid token limits
            truncated = markdown_content[:3000]
            content_section = f"\n\nPAGE CONTENT (extracted markdown):\n{truncated}"

        return f"""You are a senior UI/UX analyst. Analyze this screenshot of a web page.

PAGE CONTEXT:
  Title: {title}
  Purpose: {purpose}
  Viewport: {vp_label}

REQUIRED OUTCOMES (check each):
{objectives_text}
{console_section}{content_section}

ANALYSIS INSTRUCTIONS:
Evaluate this page on these dimensions:

1. FIRST IMPRESSION (3-second test)
   - What does a visitor understand in 3 seconds?
   - Is the purpose immediately clear?

2. ABOVE-THE-FOLD AUDIT
   - What elements are visible without scrolling?
   - Is there a clear call-to-action?
   - Can the user take meaningful action without scrolling?

3. VISUAL HIERARCHY
   - What draws the eye first, second, third?
   - Does prominence match importance?

4. SPACE EFFICIENCY
   - Any large empty areas (>100px)?
   - Is content sized appropriately?

5. INTERACTIVE ELEMENTS
   - Are buttons/links visually distinct?
   - Touch targets adequate (44x44px min for mobile)?

6. ACCESSIBILITY
   - Text contrast adequate?
   - Font sizes readable (16px+ preferred)?
   - Color-only indicators?

SCORING (1-10 scale, calibrated to 2026 modern standards):
  9-10: Exceptional (Stripe/Linear quality)
  7-8: Good (minor polish needed)
  5-6: Average (notable problems)
  3-4: Poor (major issues)
  1-2: Failing (unusable)

Respond with ONLY valid JSON in this exact structure:
{{
  "first_impression": {{
    "3_second_understanding": "what a visitor grasps immediately",
    "purpose_clear": true/false,
    "most_prominent_element": "element description"
  }},
  "scores": {{
    "overall": <1-10>,
    "visual_hierarchy": <1-10>,
    "usability": <1-10>,
    "accessibility": <1-10>,
    "mobile_responsiveness": <1-10 or null if not mobile>,
    "real_estate_efficiency": <1-10>,
    "above_fold_value": <1-10>,
    "brand_consistency": <1-10>
  }},
  "objectives_met": [
    {{"objective": "text", "met": true/false, "note": "explanation"}}
  ],
  "issues": [
    {{
      "severity": "critical|high|medium|low",
      "element": "affected element",
      "location": "above_fold|below_fold|header|footer|sidebar",
      "problem": "what is wrong",
      "impact": "user impact",
      "fix": "concrete recommendation"
    }}
  ],
  "positive_findings": ["what works well"],
  "priority_recommendations": [
    {{"priority": 1, "action": "what to do", "expected_impact": "result"}}
  ]
}}"""

    async def _call_vision_api(self, image_path: str, prompt: str) -> Dict:
        """Send image + prompt to Groq via Demeterics proxy."""
        image_bytes = Path(image_path).read_bytes()
        image_b64 = base64.b64encode(image_bytes).decode("utf-8")

        # Detect format from extension
        ext = Path(image_path).suffix.lower()
        mime = "image/png" if ext == ".png" else "image/jpeg"

        payload = {
            "model": self.model,
            "messages": [
                {
                    "role": "user",
                    "content": [
                        {"type": "text", "text": prompt},
                        {
                            "type": "image_url",
                            "image_url": {
                                "url": f"data:{mime};base64,{image_b64}",
                                "detail": "high",
                            },
                        },
                    ],
                }
            ],
            "max_tokens": self.max_tokens,
            "temperature": self.temperature,
        }

        headers = {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }

        api_url = f"{self.api_base}/api/groq/v1/chat/completions"

        async with aiohttp.ClientSession() as session:
            async with session.post(api_url, headers=headers, json=payload, timeout=aiohttp.ClientTimeout(total=120)) as resp:
                if resp.status != 200:
                    error_text = await resp.text()
                    raise RuntimeError(f"LLM API returned {resp.status}: {error_text[:500]}")
                data = await resp.json()

        content_text = data["choices"][0]["message"]["content"]
        return self._parse_json_response(content_text)

    def _parse_json_response(self, content_text: str) -> Dict:
        """Parse JSON from LLM response with common fixups."""
        text = content_text.strip()

        # Strip markdown code blocks
        if "```json" in text:
            text = text.split("```json")[1].split("```")[0].strip()
        elif "```" in text:
            text = text.split("```")[1].split("```")[0].strip()

        # Fix common JSON issues from LLMs
        text = text.replace(": N/A,", ": null,")
        text = text.replace(": N/A}", ": null}")
        text = re.sub(r",(\s*[}\]])", r"\1", text)  # trailing commas

        try:
            return json.loads(text)
        except json.JSONDecodeError:
            # Return raw text as fallback
            return {
                "raw_response": content_text,
                "parse_error": "Could not parse JSON from LLM response",
                "scores": {"overall": 0},
            }
