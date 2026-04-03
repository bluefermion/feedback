"""
Screenshot capture module for multi-viewport UI testing.
Captures desktop, laptop, and mobile screenshots + extracts markdown content.
"""

import asyncio
from pathlib import Path
from datetime import datetime
from typing import Dict, Optional
from PIL import Image
from bs4 import BeautifulSoup
from markdownify import markdownify as md


class ScreenshotCapture:
    def __init__(self, output_dir: str = "screenshots", content_dir: str = "content"):
        self.screenshots_dir = Path(output_dir)
        self.content_dir = Path(content_dir)
        self.screenshots_dir.mkdir(parents=True, exist_ok=True)
        self.content_dir.mkdir(parents=True, exist_ok=True)

    async def capture_viewports(
        self,
        page,
        page_key: str,
        viewports: Dict[str, Dict[str, int]],
        wait_seconds: float = 2.0,
    ) -> Dict[str, str]:
        """
        Capture screenshots at each viewport size.
        Returns dict: {viewport_name: screenshot_path}
        """
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        paths = {}

        for vp_name, vp_size in viewports.items():
            await page.set_viewport_size(
                {"width": vp_size["width"], "height": vp_size["height"]}
            )
            await page.wait_for_load_state("networkidle", timeout=30000)
            await asyncio.sleep(wait_seconds)

            filename = f"{page_key}_{vp_name}_{timestamp}.png"
            filepath = str(self.screenshots_dir / filename)
            await page.screenshot(path=filepath, full_page=True)

            optimized = self.optimize_for_llm(filepath)
            paths[vp_name] = optimized

        return paths

    async def extract_markdown(self, page, url: str, page_key: str) -> str:
        """Extract page HTML and convert to markdown."""
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")

        html_content = await page.content()
        markdown = self._html_to_markdown(html_content, url)

        filename = f"{page_key}_{timestamp}.md"
        filepath = self.content_dir / filename
        filepath.write_text(markdown, encoding="utf-8")

        return str(filepath)

    def _html_to_markdown(self, html_content: str, url: str) -> str:
        """Convert HTML to clean markdown, stripping noise."""
        soup = BeautifulSoup(html_content, "html.parser")

        # Remove non-content elements
        for tag in soup(["script", "style", "noscript", "svg", "iframe"]):
            tag.decompose()

        # Try to find main content area
        main_content = None
        for selector in ["main", "article", '[role="main"]', "#content", ".content"]:
            main_content = soup.select_one(selector)
            if main_content:
                break

        target = main_content if main_content else soup.body
        if not target:
            target = soup

        markdown = md(str(target), heading_style="ATX", strip=["img"])
        # Clean up excessive whitespace
        lines = [line.rstrip() for line in markdown.splitlines()]
        cleaned = "\n".join(lines)
        # Collapse 3+ blank lines into 2
        while "\n\n\n" in cleaned:
            cleaned = cleaned.replace("\n\n\n", "\n\n")

        return f"# Page Content\n\nURL: {url}\n\n{cleaned}"

    def optimize_for_llm(self, image_path: str, max_size_mb: float = 3.5) -> str:
        """
        Optimize image for LLM vision API constraints.
        - Max 33 megapixels
        - Max aspect ratio 95:1
        - Target file size under max_size_mb
        """
        img = Image.open(image_path)
        width, height = img.size

        # Check megapixel limit
        max_pixels = 33_000_000
        total_pixels = width * height
        if total_pixels > max_pixels:
            scale = (max_pixels / total_pixels) ** 0.5
            new_width = int(width * scale)
            new_height = int(height * scale)
            img = img.resize((new_width, new_height), Image.LANCZOS)

        # Check aspect ratio
        aspect = max(width, height) / max(min(width, height), 1)
        if aspect > 95:
            if width > height:
                new_width = height * 95
                img = img.crop((0, 0, new_width, height))
            else:
                new_height = width * 95
                img = img.crop((0, 0, width, new_height))

        # Save with compression
        img.save(image_path, "PNG", optimize=True)

        # If still too large, convert to JPEG with decreasing quality
        file_size_mb = Path(image_path).stat().st_size / (1024 * 1024)
        if file_size_mb > max_size_mb:
            jpeg_path = image_path.replace(".png", ".jpg")
            for quality in [90, 80, 70, 60, 50]:
                if img.mode in ("RGBA", "P"):
                    img = img.convert("RGB")
                img.save(jpeg_path, "JPEG", quality=quality)
                if Path(jpeg_path).stat().st_size / (1024 * 1024) <= max_size_mb:
                    Path(image_path).unlink()
                    return jpeg_path
            return jpeg_path

        return image_path
