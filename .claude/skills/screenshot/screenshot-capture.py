#!/usr/bin/env python3
"""Capture screenshots of a URL at mobile, laptop, and desktop viewports.

Usage:
    screenshot-capture.py <url> [--output-dir DIR] [--viewports mobile,laptop,desktop]
                                [--wait SECONDS] [--full-page] [--no-optimize]
                                [--session NAME]

Outputs screenshot file paths to stdout, one per line, as:
    viewport_name: /path/to/screenshot.png
"""

import argparse
import asyncio
import sys
import tempfile
from pathlib import Path
from urllib.parse import urlparse

from PIL import Image
from playwright.async_api import async_playwright

SESSIONS_DIR = Path.home() / ".claude" / "tools" / "screenshot-sessions"

VIEWPORTS = {
    "mobile": {"width": 375, "height": 667},
    "laptop": {"width": 1366, "height": 768},
    "desktop": {"width": 1920, "height": 1080},
}


def optimize_for_llm(image_path: str, max_size_mb: float = 3.5) -> str:
    """Optimize screenshot for LLM vision constraints."""
    img = Image.open(image_path)
    w, h = img.size

    # Megapixel limit: 33M pixels
    total_pixels = w * h
    if total_pixels > 33_000_000:
        scale = (33_000_000 / total_pixels) ** 0.5
        new_w, new_h = int(w * scale), int(h * scale)
        img = img.resize((new_w, new_h), Image.LANCZOS)

    # Aspect ratio limit: 95:1
    aspect = max(img.width, img.height) / max(min(img.width, img.height), 1)
    if aspect > 95:
        if img.width > img.height:
            img = img.crop((0, 0, img.height * 95, img.height))
        else:
            img = img.crop((0, 0, img.width, img.width * 95))

    # Save as PNG first
    img.save(image_path, "PNG", optimize=True)

    # If still too large, convert to JPEG
    file_size = Path(image_path).stat().st_size
    if file_size > max_size_mb * 1024 * 1024:
        jpeg_path = str(image_path).replace(".png", ".jpg")
        for quality in [90, 80, 70, 60, 50]:
            img.convert("RGB").save(jpeg_path, "JPEG", quality=quality)
            if Path(jpeg_path).stat().st_size <= max_size_mb * 1024 * 1024:
                Path(image_path).unlink()
                return jpeg_path
        return jpeg_path

    return image_path


def find_session(url: str, session_name: str = None) -> str | None:
    """Find a saved session file for the given URL or name."""
    if session_name:
        sp = SESSIONS_DIR / f"{session_name}.json"
        if sp.exists():
            return str(sp)
    # Auto-detect by domain
    domain = urlparse(url).netloc
    sp = SESSIONS_DIR / f"{domain}.json"
    if sp.exists():
        return str(sp)
    # Try without port
    domain_no_port = domain.split(":")[0]
    sp = SESSIONS_DIR / f"{domain_no_port}.json"
    if sp.exists():
        return str(sp)
    return None


async def capture(url: str, output_dir: str, viewports: dict,
                  wait_seconds: float = 2.0, full_page: bool = True,
                  optimize: bool = True, session: str = None) -> dict:
    """Capture screenshots at specified viewports. Returns {viewport: path}."""
    results = {}
    output = Path(output_dir)
    output.mkdir(parents=True, exist_ok=True)

    # Find session file
    session_file = find_session(url, session)
    if session_file:
        print(f"session: {session_file}", file=sys.stderr)

    async with async_playwright() as p:
        browser = await p.chromium.launch(
            headless=True,
            args=["--no-sandbox", "--disable-dev-shm-usage",
                  "--disable-blink-features=AutomationControlled"],
        )
        context_kwargs = dict(
            user_agent=(
                "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
                "(KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
            ),
            viewport={"width": 1920, "height": 1080},
        )
        if session_file:
            context_kwargs["storage_state"] = session_file
        context = await browser.new_context(**context_kwargs)
        page = await context.new_page()

        # Navigate once
        try:
            await page.goto(url, wait_until="networkidle", timeout=30000)
        except Exception:
            # Fallback to domcontentloaded if networkidle times out
            await page.goto(url, wait_until="domcontentloaded", timeout=30000)

        for name, dims in viewports.items():
            await page.set_viewport_size(dims)
            try:
                await page.wait_for_load_state("networkidle", timeout=10000)
            except Exception:
                pass
            await asyncio.sleep(wait_seconds)

            filepath = str(output / f"{name}_{dims['width']}x{dims['height']}.png")
            await page.screenshot(path=filepath, full_page=full_page)

            if optimize:
                filepath = optimize_for_llm(filepath)

            results[name] = filepath

        await browser.close()

    return results


def main():
    parser = argparse.ArgumentParser(description="Capture URL screenshots at multiple viewports")
    parser.add_argument("url", help="URL to capture")
    parser.add_argument("--output-dir", default=None,
                        help="Output directory (default: temp dir)")
    parser.add_argument("--viewports", default="mobile,laptop,desktop",
                        help="Comma-separated viewport names (mobile,laptop,desktop)")
    parser.add_argument("--wait", type=float, default=2.0,
                        help="Seconds to wait after viewport resize (default: 2)")
    parser.add_argument("--full-page", action="store_true", default=True,
                        help="Capture full page height (default: true)")
    parser.add_argument("--viewport-only", action="store_true",
                        help="Capture only the visible viewport, not full page")
    parser.add_argument("--no-optimize", action="store_true",
                        help="Skip LLM optimization")
    parser.add_argument("--session", default=None,
                        help="Session name to load (default: auto-detect by domain)")
    args = parser.parse_args()

    # Parse viewports
    requested = [v.strip() for v in args.viewports.split(",")]
    viewports = {}
    for name in requested:
        if name in VIEWPORTS:
            viewports[name] = VIEWPORTS[name]
        else:
            # Try parsing WxH format
            try:
                w, h = name.split("x")
                viewports[name] = {"width": int(w), "height": int(h)}
            except ValueError:
                print(f"Unknown viewport: {name}", file=sys.stderr)
                sys.exit(1)

    # Output directory
    if args.output_dir:
        output_dir = args.output_dir
    else:
        output_dir = tempfile.mkdtemp(prefix="claude-screenshots-")

    full_page = not args.viewport_only
    optimize = not args.no_optimize

    results = asyncio.run(capture(
        url=args.url,
        output_dir=output_dir,
        viewports=viewports,
        wait_seconds=args.wait,
        full_page=full_page,
        optimize=optimize,
        session=args.session,
    ))

    # Output paths
    for viewport_name, path in results.items():
        print(f"{viewport_name}: {path}")


if __name__ == "__main__":
    main()
