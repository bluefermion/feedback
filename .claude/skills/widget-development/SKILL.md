---
name: widget-development
description: Frontend feedback widget patterns, JavaScript, UI components, floating button, screenshots
---

# Feedback Widget Development

## Widget Architecture

The widget (`widget/js/feedback-widget.js`) is a self-contained JavaScript module:
- Floating action button (FAB) in bottom-right corner
- Console.log/warn/error interception (last 50 entries)
- Device metadata collection
- Screenshot capture with annotation tools
- Submits to `POST /api/feedback`

## Styling Constants

```javascript
// Material Orange theme - MUST maintain consistency
const WIDGET_COLOR = '#FF9800';
const WIDGET_HOVER = '#F57C00';
const WIDGET_ACTIVE = '#EF6C00';

// System fonts - no external font loading
const FONT_STACK = '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
```

## Feedback Types

**YOU MUST** validate against these exact values:
- `Bug` — Something is broken
- `Feature` — New functionality request
- `Improvement` — Enhancement to existing feature
- `Other` — General feedback

## Console Interception

```javascript
const originalLog = console.log;
const logs = [];
const MAX_LOGS = 50;

console.log = function(...args) {
    logs.push({
        type: 'log',
        message: args.map(a => String(a)).join(' '),
        timestamp: Date.now()
    });
    if (logs.length > MAX_LOGS) logs.shift();
    originalLog.apply(console, args);
};
// Repeat for console.warn, console.error
```

## Metadata Collection

Automatically collected with each submission:
```javascript
const metadata = {
    url: window.location.href,
    userAgent: navigator.userAgent,
    screenWidth: window.innerWidth,
    screenHeight: window.innerHeight,
    timestamp: new Date().toISOString(),
    consoleLogs: logs.slice(-50)
};
```

## Screenshot Capture

Uses html2canvas (loaded from CDN when needed):
```javascript
async function captureScreenshot() {
    if (!window.html2canvas) {
        await loadScript('https://cdnjs.cloudflare.com/ajax/libs/html2canvas/1.4.1/html2canvas.min.js');
    }
    const canvas = await html2canvas(document.body);
    return canvas.toDataURL('image/png');
}
```

## IIFE Pattern

Widget uses immediately-invoked function expression for isolation:
```javascript
(function() {
    'use strict';

    // All widget code here
    // No global namespace pollution

    // Auto-init on DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }
})();
```

## Dynamic CSS Injection

```javascript
function injectStyles() {
    const style = document.createElement('style');
    style.textContent = `
        .feedback-widget-btn {
            position: fixed;
            bottom: 20px;
            right: 20px;
            width: 56px;
            height: 56px;
            border-radius: 50%;
            background: ${WIDGET_COLOR};
            /* ... */
        }
    `;
    document.head.appendChild(style);
}
```

## Fetch API for Submission

```javascript
async function submitFeedback(data) {
    const response = await fetch('/api/feedback', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
    });

    if (!response.ok) {
        const error = await response.json();
        throw new Error(error.error || 'Submission failed');
    }

    return response.json();
}
```

## Testing the Widget

1. Run server: `go run ./cmd/server`
2. Open http://localhost:8080/demo
3. Click yellow "!" button
4. Select feedback type
5. Enter message
6. Optionally capture screenshot
7. Submit and verify in `/feedback` admin view
