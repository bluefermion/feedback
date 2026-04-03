"""
Browser-use integration for optional per-page action prompts.
Wraps browser-use Agent to execute natural language tasks against a page.
Uses OpenAI-compatible ChatOpenAI via Demeterics API proxy.
"""

import os
from pathlib import Path
from typing import Any, Dict, Optional


async def run_browser_action(
    action_prompt: str,
    page_url: str,
    storage_state: Optional[str] = None,
) -> Dict[str, Any]:
    """
    Execute a browser-use action with saved session state.

    Args:
        action_prompt: Natural language task to perform
        page_url: URL to navigate to
        storage_state: Path to browser state JSON (cookies/localStorage)

    Returns:
        {success: bool, action: str, result: str, errors: [str]}
    """
    try:
        from browser_use import Agent
        from browser_use.browser.profile import BrowserProfile
        from browser_use.browser.session import BrowserSession
        from browser_use.llm.openai.like import ChatOpenAILike
    except ImportError:
        return {
            "success": False,
            "action": action_prompt,
            "result": "browser-use not installed",
            "errors": ["Install with: pip install browser-use"],
        }

    api_key = os.environ.get("DEMETERICS_API_KEY", "")
    if not api_key:
        return {
            "success": False,
            "action": action_prompt,
            "result": "No API key found",
            "errors": ["Set DEMETERICS_API_KEY env var"],
        }

    model_name = os.environ.get("LLM_BROWSER_USE_MODEL", "meta-llama/llama-4-scout-17b-16e-instruct")
    api_base = os.environ.get("LLM_API_BASE", "https://api.demeterics.com")
    openai_base_url = f"{api_base}/api/groq/v1"

    try:
        llm = ChatOpenAILike(
            model=model_name,
            api_key=api_key,
            base_url=openai_base_url,
            temperature=0.1,
        )

        # Build browser profile with saved session
        profile_kwargs = {
            "headless": True,
            "disable_security": True,
            "args": ["--no-sandbox", "--disable-dev-shm-usage"],
        }
        if storage_state and Path(storage_state).exists():
            profile_kwargs["storage_state"] = storage_state

        profile = BrowserProfile(**profile_kwargs)
        session = BrowserSession(browser_profile=profile)

        # Prepend Demeterics tracking directives for cost attribution
        task = (
            f"/// APP Feedback-UI-Test\n/// FLOW browser-action\n"
            f"You are on {page_url}. {action_prompt}. Report what happened."
        )

        agent = Agent(
            task=task,
            llm=llm,
            browser_session=session,
            use_vision=False,
            max_failures=3,
        )

        result = await agent.run()
        final_result = result.final_result() if hasattr(result, 'final_result') else str(result)

        return {
            "success": True,
            "action": action_prompt,
            "result": final_result or "Action completed",
            "errors": [],
        }

    except Exception as e:
        return {
            "success": False,
            "action": action_prompt,
            "result": f"Action failed: {e}",
            "errors": [str(e)],
        }
