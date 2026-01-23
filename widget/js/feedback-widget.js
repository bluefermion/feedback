/**
 * Feedback Widget with Screenshot Annotation
 *
 * EDUCATIONAL CONTEXT:
 * This script demonstrates how to build a self-contained, drop-in frontend widget
 * using vanilla JavaScript (ES6+). It avoids external framework dependencies (like React/Vue)
 * to ensure it works on ANY website with minimal footprint.
 *
 * Key Concepts:
 * 1. IIFE (Immediately Invoked Function Expression) for scope isolation.
 * 2. Proxying/Monkey-patching 'console.log' to capture debugging data.
 * 3. Using Canvas API for client-side image manipulation (screenshots).
 * 4. Dynamic CSS injection to avoid requiring a separate .css file.
 */
(function() {
    'use strict'; // Enforce stricter parsing and error handling

    // -------------------------------------------------------------------------
    // CONFIGURATION & STATE
    // -------------------------------------------------------------------------

    // Default configuration. Can be overridden via init().
    const config = {
        endpoint: '/api/feedback', // Where to POST data
        debug: false,
        maxScreenshotSize: 1024 * 1024, // 1MB limit to prevent payload rejection
        maxConsoleLogs: 50,             // Limit memory usage
        screenshotScale: 0.5,           // Downscale retina screens to save bandwidth
        // User Identity (for Self-Healing features)
        userEmail: '',
        userName: '',
    };

    // Internal state management
    // We use simple variables instead of a state management library.
    let feedbackModal = null;
    let screenshotModal = null;
    let selectedType = 'bug';
    let screenshotDataUrl = null;        // Final image to submit
    let originalScreenshotDataUrl = null; // Clean image for re-editing
    let annotations = [];                // List of drawing objects ({x,y,w,h,type})
    let currentTool = 'highlight';       // 'highlight' or 'hide'
    let isDrawing = false;
    let drawStart = { x: 0, y: 0 };
    let html2canvasLoaded = false;       // Lazy loading flag

    // Form persistence: Keep text when switching between modal views
    let savedTitle = '';
    let savedDescription = '';

    // -------------------------------------------------------------------------
    // CONSOLE LOG CAPTURE
    // -------------------------------------------------------------------------

    // Circular buffer for logs.
    const consoleLogs = [];
    // Keep references to the native console methods so we can still print to devtools.
    const originalConsole = {
        log: console.log,
        warn: console.warn,
        error: console.error,
        info: console.info,
        debug: console.debug,
    };

    /**
     * Intercepts browser console methods to record logs internally.
     * This allows developers to see what happened *before* the user reported a bug.
     */
    function captureConsoleLogs() {
        ['log', 'warn', 'error', 'info', 'debug'].forEach(level => {
            console[level] = function(...args) {
                // 1. Store the log
                consoleLogs.push({
                    level,
                    // Safe stringify: Handles circular references or non-string objects
                    message: args.map(arg => {
                        try {
                            return typeof arg === 'object' ? JSON.stringify(arg) : String(arg);
                        } catch (e) {
                            return String(arg);
                        }
                    }).join(' '),
                    timestamp: new Date().toISOString(),
                });

                // Maintain fixed buffer size (FIFO)
                while (consoleLogs.length > config.maxConsoleLogs) {
                    consoleLogs.shift();
                }

                // 2. Pass through to the real console
                originalConsole[level].apply(console, args);
            };
        });

        // Global Error Handler: Catches uncaught exceptions
        window.addEventListener('error', (event) => {
            consoleLogs.push({
                level: 'error',
                message: `Uncaught: ${event.message} at ${event.filename}:${event.lineno}`,
                timestamp: new Date().toISOString(),
            });
        });

        // Promise Rejection Handler: Catches async errors
        window.addEventListener('unhandledrejection', (event) => {
            consoleLogs.push({
                level: 'error',
                message: `Unhandled Promise Rejection: ${event.reason}`,
                timestamp: new Date().toISOString(),
            });
        });
    }

    // -------------------------------------------------------------------------
    // DEVICE METADATA
    // -------------------------------------------------------------------------

    /**
     * Collects technical context about the user's environment.
     * Essential for reproducing device-specific bugs.
     */
    function getDeviceInfo() {
        const ua = navigator.userAgent;
        let browserName = 'Unknown';
        let browserVersion = '';
        let os = 'Unknown';
        let deviceType = 'Desktop';

        // Primitive UA parsing (Production apps might use a library like UAParser.js)
        if (ua.includes('Chrome') && !ua.includes('Edg')) {
            browserName = 'Chrome';
            browserVersion = ua.match(/Chrome\/(\d+)/)?.[1] || '';
        } else if (ua.includes('Firefox')) {
            browserName = 'Firefox';
            browserVersion = ua.match(/Firefox\/(\d+)/)?.[1] || '';
        } else if (ua.includes('Safari') && !ua.includes('Chrome')) {
            browserName = 'Safari';
            browserVersion = ua.match(/Version\/(\d+)/)?.[1] || '';
        } else if (ua.includes('Edg')) {
            browserName = 'Edge';
            browserVersion = ua.match(/Edg\/(\d+)/)?.[1] || '';
        }

        if (ua.includes('Windows')) os = 'Windows';
        else if (ua.includes('Mac OS')) os = 'macOS';
        else if (ua.includes('Linux')) os = 'Linux';
        else if (ua.includes('Android')) os = 'Android';
        else if (ua.includes('iOS') || ua.includes('iPhone') || ua.includes('iPad')) os = 'iOS';

        const isMobile = /Mobile|Android|iPhone|iPad/i.test(ua);
        if (isMobile) {
            deviceType = /iPad|Tablet/i.test(ua) ? 'Tablet' : 'Mobile';
        }

        return {
            screenWidth: window.screen.width,
            screenHeight: window.screen.height,
            viewportWidth: window.innerWidth,
            viewportHeight: window.innerHeight,
            screenResolution: `${window.screen.width}x${window.screen.height}`,
            pixelRatio: window.devicePixelRatio || 1, // 2+ for Retina/HiDPI
            browserName,
            browserVersion,
            os,
            deviceType,
            isMobile,
            language: navigator.language,
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
            url: window.location.href,
        };
    }

    // -------------------------------------------------------------------------
    // SCREENSHOT CAPTURE (html2canvas)
    // -------------------------------------------------------------------------

    /**
     * Dynamically loads html2canvas from CDN only when needed.
     * This keeps the initial bundle size small.
     */
    function loadHtml2Canvas() {
        return new Promise((resolve, reject) => {
            if (html2canvasLoaded || window.html2canvas) {
                html2canvasLoaded = true;
                resolve();
                return;
            }

            const script = document.createElement('script');
            script.src = 'https://cdnjs.cloudflare.com/ajax/libs/html2canvas/1.4.1/html2canvas.min.js';
            script.onload = () => {
                html2canvasLoaded = true;
                resolve();
            };
            script.onerror = () => reject(new Error('Failed to load html2canvas'));
            document.head.appendChild(script);
        });
    }

    /**
     * Captures the visible viewport as an image.
     */
    async function captureScreenshot() {
        try {
            await loadHtml2Canvas();

            // UI UX: Hide our own modals so they don't appear in the screenshot.
            if (feedbackModal) feedbackModal.style.display = 'none';
            if (screenshotModal) screenshotModal.style.display = 'none';

            // Workaround: Bootstrap 5.3+ uses new CSS color functions that can crash html2canvas.
            // We temporarily disable known problematic stylesheets.
            const stylesheets = Array.from(document.styleSheets);
            const disabledSheets = [];

            stylesheets.forEach(sheet => {
                try {
                    if (sheet.href && sheet.href.includes('bootstrap')) {
                        sheet.disabled = true;
                        disabledSheets.push(sheet);
                    }
                } catch (e) {
                    // Ignore CORS errors for external stylesheets
                }
            });

            // The Core Capture Logic
            const canvas = await html2canvas(document.body, {
                scale: config.screenshotScale,
                useCORS: true,       // Allow loading cross-origin images
                allowTaint: false,   // Security: don't allow tainted canvas
                imageTimeout: 5000,
                logging: false,
                ignoreElements: (element) => {
                    // Double check we ignore our widget elements
                    return element.classList?.contains('feedback-trigger-btn') ||
                           element.classList?.contains('feedback-modal-overlay');
                },
            });

            // Restore stylesheets
            disabledSheets.forEach(sheet => sheet.disabled = false);

            // Restore UI
            if (feedbackModal) feedbackModal.style.display = 'flex';
            if (screenshotModal) screenshotModal.style.display = 'flex';

            let dataUrl = canvas.toDataURL('image/png');

            // Compression Strategy
            if (dataUrl.length > config.maxScreenshotSize) {
                // Fallback to JPEG 70% quality if PNG is too big
                dataUrl = canvas.toDataURL('image/jpeg', 0.7);
            }

            if (dataUrl.length > config.maxScreenshotSize) {
                // If still too big, abandon ship.
                dataUrl = 'screenshot-too-large';
            }

            return dataUrl;
        } catch (error) {
            console.error('Screenshot capture failed:', error);
            // Always restore UI even on error
            if (feedbackModal) feedbackModal.style.display = 'flex';
            if (screenshotModal) screenshotModal.style.display = 'flex';
            return null;
        }
    }

    // -------------------------------------------------------------------------
    // CANVAS ANNOTATION LOGIC
    // -------------------------------------------------------------------------

    /**
     * Redraws the base screenshot + all annotation layers.
     * This is the "Render Loop" for the canvas editor.
     */
    function redrawAnnotations(ctx, img) {
        // Clear canvas
        ctx.clearRect(0, 0, ctx.canvas.width, ctx.canvas.height);
        // Draw base image
        ctx.drawImage(img, 0, 0, ctx.canvas.width, ctx.canvas.height);

        // Draw each annotation
        annotations.forEach(ann => {
            if (ann.type === 'highlight') {
                // Yellow semi-transparent box
                ctx.fillStyle = 'rgba(255, 235, 59, 0.3)';
                ctx.strokeStyle = 'rgba(255, 193, 7, 0.8)';
                ctx.lineWidth = 2;
            } else if (ann.type === 'hide') {
                // Black opaque box (Redaction)
                ctx.fillStyle = 'rgba(0, 0, 0, 0.9)';
                ctx.strokeStyle = 'rgba(0, 0, 0, 1)';
                ctx.lineWidth = 1;
            }
            ctx.fillRect(ann.x, ann.y, ann.width, ann.height);
            ctx.strokeRect(ann.x, ann.y, ann.width, ann.height);
        });
    }

    // -------------------------------------------------------------------------
    // UI COMPONENTS & RENDERING
    // -------------------------------------------------------------------------

    function showToast(message, type = 'info') {
        const toast = document.createElement('div');
        toast.className = 'feedback-toast feedback-toast-' + type;
        toast.textContent = message;
        // Inline styles allow self-containment
        toast.style.cssText = `
            position: fixed;
            bottom: 80px;
            left: 1.5rem;
            padding: 12px 20px;
            border-radius: 8px;
            color: white;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            font-size: 14px;
            z-index: 10003;
            animation: feedbackFadeIn 0.3s ease;
            background: ${type === 'error' ? '#f44336' : type === 'success' ? '#4caf50' : '#2196f3'};
            box-shadow: 0 4px 12px rgba(0,0,0,0.15);
        `;
        document.body.appendChild(toast);
        // Auto-dismiss
        setTimeout(() => {
            toast.style.animation = 'feedbackFadeOut 0.3s ease';
            setTimeout(() => toast.remove(), 300);
        }, 3000);
    }

    function createWidget() {
        // Inject CSS via JS.
        // This avoids forcing the user to include a separate <link rel="stylesheet"> tag.
        const styles = document.createElement('style');
        styles.textContent = `
            @keyframes feedbackFadeIn {
                from { opacity: 0; transform: translateY(10px); }
                to { opacity: 1; transform: translateY(0); }
            }
            @keyframes feedbackFadeOut {
                from { opacity: 1; transform: translateY(0); }
                to { opacity: 0; transform: translateY(10px); }
            }
            @keyframes feedbackSpin {
                to { transform: rotate(360deg); }
            }
            .feedback-trigger-btn {
                position: fixed;
                bottom: 1.5rem;
                left: 1.5rem;
                width: 48px;
                height: 48px;
                border-radius: 50%;
                background: #FF9800;
                border: none;
                cursor: pointer;
                display: flex;
                align-items: center;
                justify-content: center;
                box-shadow: 0 3px 8px rgba(255, 152, 0, 0.3);
                z-index: 10000;
                transition: transform 0.2s, background-color 0.2s;
            }
            .feedback-trigger-btn:hover {
                background: #F57C00;
                transform: scale(1.1);
            }
            .feedback-icon {
                color: white;
                font-size: 24px;
                font-weight: bold;
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            }
            .feedback-modal-overlay {
                position: fixed;
                inset: 0;
                background: rgba(0,0,0,0.5);
                z-index: 10001;
                display: flex;
                align-items: center;
                justify-content: center;
                animation: feedbackFadeIn 0.2s ease;
            }
            .feedback-modal-overlay.screenshot-modal {
                z-index: 10002;
            }
            .feedback-modal {
                background: white;
                border-radius: 12px;
                width: 90%;
                max-width: 500px;
                max-height: 90vh;
                overflow-y: auto;
                box-shadow: 0 20px 60px rgba(0,0,0,0.3);
                font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            }
            .feedback-modal.screenshot-editor-modal {
                max-width: 800px;
            }
            .feedback-modal-header {
                padding: 20px;
                border-bottom: 1px solid #eee;
                display: flex;
                justify-content: space-between;
                align-items: center;
            }
            .feedback-modal-title {
                margin: 0;
                font-size: 18px;
                font-weight: 600;
                color: #333;
            }
            .feedback-modal-close {
                background: none;
                border: none;
                font-size: 24px;
                cursor: pointer;
                color: #999;
                padding: 0;
                line-height: 1;
            }
            .feedback-modal-close:hover {
                color: #333;
            }
            .feedback-modal-body {
                padding: 20px;
            }
            .feedback-form-group {
                margin-bottom: 16px;
            }
            .feedback-form-label {
                display: block;
                margin-bottom: 6px;
                font-weight: 500;
                color: #333;
                font-size: 14px;
            }
            .feedback-form-input,
            .feedback-form-textarea,
            .feedback-form-select {
                width: 100%;
                padding: 10px 12px;
                border: 1px solid #ddd;
                border-radius: 6px;
                font-size: 14px;
                font-family: inherit;
                transition: border-color 0.2s;
                box-sizing: border-box;
            }
            .feedback-form-input:focus,
            .feedback-form-textarea:focus,
            .feedback-form-select:focus {
                outline: none;
                border-color: #FF9800;
            }
            .feedback-form-textarea {
                min-height: 100px;
                resize: vertical;
            }
            .feedback-type-buttons {
                display: flex;
                gap: 8px;
                margin-bottom: 16px;
                flex-wrap: wrap;
            }
            .feedback-type-btn {
                flex: 1;
                min-width: 70px;
                padding: 12px 8px;
                border: 2px solid #ddd;
                border-radius: 8px;
                background: white;
                cursor: pointer;
                text-align: center;
                transition: all 0.2s;
                font-family: inherit;
            }
            .feedback-type-btn:hover {
                border-color: #FF9800;
            }
            .feedback-type-btn.active {
                border-color: #FF9800;
                background: #FFF3E0;
            }
            .feedback-type-btn-icon {
                font-size: 24px;
                margin-bottom: 4px;
            }
            .feedback-type-btn-label {
                font-size: 11px;
                color: #666;
            }
            .feedback-btn {
                padding: 12px 24px;
                border: none;
                border-radius: 6px;
                font-size: 14px;
                font-weight: 500;
                cursor: pointer;
                transition: all 0.2s;
                font-family: inherit;
            }
            .feedback-btn-primary {
                background: #FF9800;
                color: white;
            }
            .feedback-btn-primary:hover {
                background: #F57C00;
            }
            .feedback-btn-primary:disabled {
                background: #ccc;
                cursor: not-allowed;
            }
            .feedback-btn-secondary {
                background: #f5f5f5;
                color: #333;
                border: 1px solid #ddd;
            }
            .feedback-btn-secondary:hover {
                background: #eee;
            }
            .feedback-btn-group {
                display: flex;
                gap: 12px;
                justify-content: flex-end;
                margin-top: 20px;
            }
            .feedback-screenshot-section {
                margin-bottom: 16px;
                padding: 16px;
                background: #f9f9f9;
                border-radius: 8px;
                border: 1px dashed #ddd;
            }
            .feedback-screenshot-btn {
                display: flex;
                align-items: center;
                gap: 8px;
                padding: 10px 16px;
                background: white;
                border: 1px solid #ddd;
                border-radius: 6px;
                cursor: pointer;
                font-size: 14px;
                transition: all 0.2s;
                font-family: inherit;
            }
            .feedback-screenshot-btn:hover {
                border-color: #FF9800;
                background: #FFF8E1;
            }
            .feedback-screenshot-preview {
                position: relative;
                display: inline-block;
            }
            .feedback-screenshot-preview img {
                max-width: 100%;
                max-height: 150px;
                border-radius: 6px;
                border: 1px solid #ddd;
            }
            .feedback-screenshot-actions {
                display: flex;
                gap: 8px;
                margin-top: 10px;
            }
            .feedback-screenshot-action-btn {
                padding: 6px 12px;
                font-size: 13px;
                border: 1px solid #ddd;
                border-radius: 4px;
                background: white;
                cursor: pointer;
                display: flex;
                align-items: center;
                gap: 4px;
                font-family: inherit;
            }
            .feedback-screenshot-action-btn:hover {
                border-color: #FF9800;
                background: #FFF8E1;
            }
            .feedback-screenshot-action-btn.remove:hover {
                border-color: #f44336;
                background: #ffebee;
                color: #f44336;
            }
            .feedback-screenshot-tools {
                display: flex;
                gap: 8px;
                margin-bottom: 12px;
                flex-wrap: wrap;
            }
            .feedback-tool-btn {
                padding: 8px 12px;
                border: 1px solid #ddd;
                border-radius: 6px;
                background: white;
                cursor: pointer;
                display: flex;
                align-items: center;
                gap: 6px;
                font-size: 13px;
                transition: all 0.2s;
                font-family: inherit;
            }
            .feedback-tool-btn:hover {
                border-color: #FF9800;
            }
            .feedback-tool-btn.active {
                border-color: #FF9800;
                background: #FFF3E0;
            }
            .feedback-screenshot-canvas {
                width: 100%;
                max-height: 400px;
                object-fit: contain;
                border: 1px solid #ddd;
                border-radius: 8px;
                cursor: crosshair;
            }
            .feedback-loading {
                display: flex;
                flex-direction: column;
                align-items: center;
                padding: 40px;
                color: #666;
            }
            .feedback-spinner {
                width: 32px;
                height: 32px;
                border: 3px solid #f3f3f3;
                border-top: 3px solid #FF9800;
                border-radius: 50%;
                animation: feedbackSpin 1s linear infinite;
                margin-bottom: 12px;
            }
            .feedback-screenshot-hint {
                font-size: 12px;
                color: #888;
                margin-top: 8px;
            }
            .feedback-editor-instructions {
                font-size: 13px;
                color: #666;
                margin-bottom: 16px;
                padding: 12px;
                background: #f5f5f5;
                border-radius: 6px;
            }
        `;
        document.head.appendChild(styles);

        // Create Floating Action Button (FAB)
        const button = document.createElement('button');
        button.className = 'feedback-trigger-btn';
        button.setAttribute('aria-label', 'Send Feedback');
        button.innerHTML = '<span class="feedback-icon">!</span>';
        button.onclick = openFeedbackModal;
        document.body.appendChild(button);
    }

    // ========================================
    // MODAL LOGIC (Feedback Form)
    // ========================================

    function openFeedbackModal() {
        if (feedbackModal) return;

        // Reset state only if opening fresh (not returning from screenshot editor)
        if (!screenshotModal) {
            screenshotDataUrl = null;
            originalScreenshotDataUrl = null;
            annotations = [];
            selectedType = 'bug';
            savedTitle = '';
            savedDescription = '';
        }

        // Create overlay
        feedbackModal = document.createElement('div');
        feedbackModal.className = 'feedback-modal-overlay';
        // Close on backdrop click
        feedbackModal.onclick = (e) => {
            if (e.target === feedbackModal) closeFeedbackModal();
        };

        renderFeedbackForm();
        document.body.appendChild(feedbackModal);
    }

    function renderFeedbackForm() {
        // Template Literals allow easy HTML construction within JS.
        feedbackModal.innerHTML = `
            <div class="feedback-modal">
                <div class="feedback-modal-header">
                    <h2 class="feedback-modal-title">Send Feedback</h2>
                    <button class="feedback-modal-close" aria-label="Close">&times;</button>
                </div>
                <div class="feedback-modal-body">
                    <div class="feedback-type-buttons">
                        <button class="feedback-type-btn ${selectedType === 'bug' ? 'active' : ''}" data-type="bug">
                            <div class="feedback-type-btn-icon">🐛</div>
                            <div class="feedback-type-btn-label">Bug</div>
                        </button>
                        <button class="feedback-type-btn ${selectedType === 'feature' ? 'active' : ''}" data-type="feature">
                            <div class="feedback-type-btn-icon">💡</div>
                            <div class="feedback-type-btn-label">Feature</div>
                        </button>
                        <button class="feedback-type-btn ${selectedType === 'improvement' ? 'active' : ''}" data-type="improvement">
                            <div class="feedback-type-btn-icon">⚡</div>
                            <div class="feedback-type-btn-label">Improve</div>
                        </button>
                        <button class="feedback-type-btn ${selectedType === 'question' ? 'active' : ''}" data-type="question">
                            <div class="feedback-type-btn-icon">❓</div>
                            <div class="feedback-type-btn-label">Question</div>
                        </button>
                        <button class="feedback-type-btn ${selectedType === 'other' ? 'active' : ''}" data-type="other">
                            <div class="feedback-type-btn-icon">💬</div>
                            <div class="feedback-type-btn-label">Other</div>
                        </button>
                    </div>
                    <div class="feedback-form-group">
                        <label class="feedback-form-label" for="feedback-title">Title *</label>
                        <input type="text" id="feedback-title" class="feedback-form-input"
                               placeholder="Brief description of your feedback" maxlength="200"
                               value="${escapeHtml(savedTitle)}">
                    </div>
                    <div class="feedback-form-group">
                        <label class="feedback-form-label" for="feedback-description">Description *</label>
                        <textarea id="feedback-description" class="feedback-form-textarea"
                                  placeholder="Please provide more details..." maxlength="5000">${escapeHtml(savedDescription)}</textarea>
                    </div>
                    <div class="feedback-screenshot-section" id="feedback-screenshot-section">
                        ${renderScreenshotSection()}
                    </div>
                    <div class="feedback-btn-group">
                        <button class="feedback-btn feedback-btn-primary" id="feedback-submit">Submit Feedback</button>
                    </div>
                </div>
            </div>
        `;

        setupFeedbackFormListeners();
    }

    function renderScreenshotSection() {
        if (screenshotDataUrl && screenshotDataUrl !== 'screenshot-too-large') {
            return `
                <div class="feedback-screenshot-preview">
                    <img src="${screenshotDataUrl}" alt="Screenshot">
                </div>
                <div class="feedback-screenshot-actions">
                    <button class="feedback-screenshot-action-btn" id="feedback-edit-screenshot">
                        <span>✏️</span> Edit
                    </button>
                    <button class="feedback-screenshot-action-btn remove" id="feedback-remove-screenshot">
                        <span>✕</span> Remove
                    </button>
                </div>
            `;
        } else {
            return `
                <button class="feedback-screenshot-btn" id="feedback-add-screenshot">
                    <span>📸</span>
                    <span>Add Screenshot</span>
                </button>
                <div class="feedback-screenshot-hint">
                    Capture the current page and annotate to highlight issues
                </div>
            `;
        }
    }

    function setupFeedbackFormListeners() {
        feedbackModal.querySelector('.feedback-modal-close').onclick = closeFeedbackModal;

        // Type selection logic
        feedbackModal.querySelectorAll('.feedback-type-btn').forEach(btn => {
            btn.onclick = () => {
                feedbackModal.querySelectorAll('.feedback-type-btn').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                selectedType = btn.dataset.type;
            };
        });

        // Screenshot buttons
        const addBtn = feedbackModal.querySelector('#feedback-add-screenshot');
        if (addBtn) addBtn.onclick = startScreenshotCapture;

        const editBtn = feedbackModal.querySelector('#feedback-edit-screenshot');
        if (editBtn) editBtn.onclick = openScreenshotEditorModal;

        const removeBtn = feedbackModal.querySelector('#feedback-remove-screenshot');
        if (removeBtn) {
            removeBtn.onclick = () => {
                screenshotDataUrl = null;
                originalScreenshotDataUrl = null;
                annotations = [];
                updateScreenshotSectionInForm();
            };
        }

        // Submit
        feedbackModal.querySelector('#feedback-submit').onclick = submitFeedback;

        // Auto-focus logic for better UX
        setTimeout(() => {
            const titleInput = feedbackModal.querySelector('#feedback-title');
            if (titleInput && !titleInput.value) {
                titleInput.focus();
            }
        }, 100);
    }

    function updateScreenshotSectionInForm() {
        const section = feedbackModal.querySelector('#feedback-screenshot-section');
        if (!section) return;

        section.innerHTML = renderScreenshotSection();

        // Re-attach event listeners since we replaced HTML
        const addBtn = section.querySelector('#feedback-add-screenshot');
        if (addBtn) addBtn.onclick = startScreenshotCapture;

        const editBtn = section.querySelector('#feedback-edit-screenshot');
        if (editBtn) editBtn.onclick = openScreenshotEditorModal;

        const removeBtn = section.querySelector('#feedback-remove-screenshot');
        if (removeBtn) {
            removeBtn.onclick = () => {
                screenshotDataUrl = null;
                originalScreenshotDataUrl = null;
                annotations = [];
                updateScreenshotSectionInForm();
            };
        }
    }

    function saveFormState() {
        if (feedbackModal) {
            const titleInput = feedbackModal.querySelector('#feedback-title');
            const descInput = feedbackModal.querySelector('#feedback-description');
            if (titleInput) savedTitle = titleInput.value;
            if (descInput) savedDescription = descInput.value;
        }
    }

    function closeFeedbackModal() {
        if (feedbackModal) {
            // Animation for smooth exit
            feedbackModal.style.animation = 'feedbackFadeOut 0.2s ease';
            setTimeout(() => {
                feedbackModal.remove();
                feedbackModal = null;
                // Cleanup
                savedTitle = '';
                savedDescription = '';
                screenshotDataUrl = null;
                originalScreenshotDataUrl = null;
                annotations = [];
            }, 200);
        }
    }

    // ========================================
    // SCREENSHOT EDITOR MODAL
    // ========================================

    async function startScreenshotCapture() {
        saveFormState();

        // Temporarily hide current modal to reveal page
        if (feedbackModal) {
            feedbackModal.style.display = 'none';
        }

        // Show "Capturing..." spinner
        screenshotModal = document.createElement('div');
        screenshotModal.className = 'feedback-modal-overlay screenshot-modal';
        screenshotModal.innerHTML = `
            <div class="feedback-modal screenshot-editor-modal">
                <div class="feedback-modal-header">
                    <h2 class="feedback-modal-title">Capture Screenshot</h2>
                </div>
                <div class="feedback-modal-body">
                    <div class="feedback-loading">
                        <div class="feedback-spinner"></div>
                        <div>Capturing screenshot...</div>
                    </div>
                </div>
            </div>
        `;
        document.body.appendChild(screenshotModal);

        // Async capture
        const dataUrl = await captureScreenshot();

        if (dataUrl && dataUrl !== 'screenshot-too-large') {
            originalScreenshotDataUrl = dataUrl;
            screenshotDataUrl = dataUrl;
            annotations = [];
            renderScreenshotEditor();
        } else {
            showToast('Screenshot capture failed', 'error');
            closeScreenshotModal(false);
        }
    }

    function openScreenshotEditorModal() {
        saveFormState();
        if (feedbackModal) feedbackModal.style.display = 'none';

        screenshotModal = document.createElement('div');
        screenshotModal.className = 'feedback-modal-overlay screenshot-modal';
        document.body.appendChild(screenshotModal);

        renderScreenshotEditor();
    }

    function renderScreenshotEditor() {
        currentTool = 'highlight';

        screenshotModal.innerHTML = `
            <div class="feedback-modal screenshot-editor-modal">
                <div class="feedback-modal-header">
                    <h2 class="feedback-modal-title">Annotate Screenshot</h2>
                    <button class="feedback-modal-close" aria-label="Close">&times;</button>
                </div>
                <div class="feedback-modal-body">
                    <div class="feedback-editor-instructions">
                        Draw on the screenshot to highlight areas or hide sensitive information.
                    </div>
                    <div class="feedback-screenshot-tools">
                        <button class="feedback-tool-btn active" data-tool="highlight">
                            <span>✏️</span>
                            <span>Highlight</span>
                        </button>
                        <button class="feedback-tool-btn" data-tool="hide">
                            <span>🔒</span>
                            <span>Hide Sensitive</span>
                        </button>
                        <button class="feedback-tool-btn" data-action="clear">
                            <span>✕</span>
                            <span>Clear All</span>
                        </button>
                    </div>
                    <canvas class="feedback-screenshot-canvas" id="feedback-canvas"></canvas>
                    <div class="feedback-btn-group">
                        <button class="feedback-btn feedback-btn-secondary" id="feedback-retake">Retake</button>
                        <button class="feedback-btn feedback-btn-secondary" id="feedback-cancel-screenshot">Cancel</button>
                        <button class="feedback-btn feedback-btn-primary" id="feedback-done-screenshot">Done</button>
                    </div>
                </div>
            </div>
        `;

        setupScreenshotEditorListeners();
        displayScreenshotInCanvas();
    }

    function setupScreenshotEditorListeners() {
        screenshotModal.querySelector('.feedback-modal-close').onclick = () => closeScreenshotModal(false);

        // Toolbar logic
        screenshotModal.querySelectorAll('.feedback-tool-btn[data-tool]').forEach(btn => {
            btn.onclick = () => {
                screenshotModal.querySelectorAll('.feedback-tool-btn[data-tool]').forEach(b => b.classList.remove('active'));
                btn.classList.add('active');
                currentTool = btn.dataset.tool;
            };
        });

        screenshotModal.querySelector('[data-action="clear"]').onclick = () => {
            annotations = [];
            updateCanvasWithAnnotations();
        };

        // Retake Logic
        screenshotModal.querySelector('#feedback-retake').onclick = async () => {
            // Show loading state again
            screenshotModal.querySelector('.feedback-modal-body').innerHTML = `
                <div class="feedback-loading">
                    <div class="feedback-spinner"></div>
                    <div>Capturing screenshot...</div>
                </div>
            `;

            const dataUrl = await captureScreenshot();
            if (dataUrl && dataUrl !== 'screenshot-too-large') {
                originalScreenshotDataUrl = dataUrl;
                screenshotDataUrl = dataUrl;
                annotations = [];
                renderScreenshotEditor();
            } else {
                showToast('Screenshot capture failed', 'error');
                closeScreenshotModal(false);
            }
        };

        screenshotModal.querySelector('#feedback-cancel-screenshot').onclick = () => closeScreenshotModal(false);

        screenshotModal.querySelector('#feedback-done-screenshot').onclick = () => {
            finalizeScreenshot();
            closeScreenshotModal(true);
        };
    }

    function displayScreenshotInCanvas() {
        const canvas = screenshotModal.querySelector('#feedback-canvas');
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const img = new Image();

        img.onload = () => {
            // Fit image within modal limits while preserving aspect ratio
            const maxWidth = 750;
            const maxHeight = 400;
            let width = img.width;
            let height = img.height;

            if (width > maxWidth) {
                height = (maxWidth / width) * height;
                width = maxWidth;
            }
            if (height > maxHeight) {
                width = (maxHeight / height) * width;
                height = maxHeight;
            }

            canvas.width = width;
            canvas.height = height;
            canvas.style.width = width + 'px';
            canvas.style.height = height + 'px';

            ctx.drawImage(img, 0, 0, width, height);
            canvas._img = img; // Cache for redraws

            redrawAnnotations(ctx, img);
            setupCanvasDrawing(canvas, ctx, img);
        };

        img.src = originalScreenshotDataUrl || screenshotDataUrl;
    }

    function setupCanvasDrawing(canvas, ctx, img) {
        // Mouse Events for Desktop
        canvas.onmousedown = (e) => {
            isDrawing = true;
            const rect = canvas.getBoundingClientRect();
            drawStart = {
                x: e.clientX - rect.left,
                y: e.clientY - rect.top
            };
        };

        canvas.onmousemove = (e) => {
            if (!isDrawing) return;

            const rect = canvas.getBoundingClientRect();
            const currentX = e.clientX - rect.left;
            const currentY = e.clientY - rect.top;

            // Clear and redraw base + previous annotations
            redrawAnnotations(ctx, img);

            // Draw current incomplete rectangle
            const width = currentX - drawStart.x;
            const height = currentY - drawStart.y;

            if (currentTool === 'highlight') {
                ctx.fillStyle = 'rgba(255, 235, 59, 0.3)';
                ctx.strokeStyle = 'rgba(255, 193, 7, 0.8)';
            } else {
                ctx.fillStyle = 'rgba(0, 0, 0, 0.9)';
                ctx.strokeStyle = 'rgba(0, 0, 0, 1)';
            }
            ctx.lineWidth = 2;
            ctx.fillRect(drawStart.x, drawStart.y, width, height);
            ctx.strokeRect(drawStart.x, drawStart.y, width, height);
        };

        canvas.onmouseup = (e) => {
            if (!isDrawing) return;
            isDrawing = false;

            const rect = canvas.getBoundingClientRect();
            const endX = e.clientX - rect.left;
            const endY = e.clientY - rect.top;

            const width = endX - drawStart.x;
            const height = endY - drawStart.y;

            // Only save if significant size (ignore accidental clicks)
            if (Math.abs(width) > 5 && Math.abs(height) > 5) {
                annotations.push({
                    type: currentTool,
                    x: Math.min(drawStart.x, endX),
                    y: Math.min(drawStart.y, endY),
                    width: Math.abs(width),
                    height: Math.abs(height)
                });
            }

            redrawAnnotations(ctx, img);
        };

        canvas.onmouseleave = () => {
            if (isDrawing) {
                isDrawing = false;
                redrawAnnotations(ctx, img);
            }
        };

        // Touch Events for Mobile (Mapping touch to mouse events)
        canvas.ontouchstart = (e) => {
            e.preventDefault();
            const touch = e.touches[0];
            canvas.onmousedown({ clientX: touch.clientX, clientY: touch.clientY });
        };

        canvas.ontouchmove = (e) => {
            e.preventDefault();
            const touch = e.touches[0];
            canvas.onmousemove({ clientX: touch.clientX, clientY: touch.clientY });
        };

        canvas.ontouchend = (e) => {
            e.preventDefault();
            const touch = e.changedTouches[0];
            canvas.onmouseup({ clientX: touch.clientX, clientY: touch.clientY });
        };
    }

    function updateCanvasWithAnnotations() {
        const canvas = screenshotModal?.querySelector('#feedback-canvas');
        if (!canvas || !canvas._img) return;
        const ctx = canvas.getContext('2d');
        redrawAnnotations(ctx, canvas._img);
    }

    function finalizeScreenshot() {
        const canvas = screenshotModal?.querySelector('#feedback-canvas');
        if (!canvas) return;

        // Bake annotations into the image
        screenshotDataUrl = canvas.toDataURL('image/png');

        if (screenshotDataUrl.length > config.maxScreenshotSize) {
            screenshotDataUrl = canvas.toDataURL('image/jpeg', 0.7);
        }

        if (screenshotDataUrl.length > config.maxScreenshotSize) {
            screenshotDataUrl = 'screenshot-too-large';
            showToast('Screenshot too large, will be excluded', 'info');
        }
    }

    function closeScreenshotModal(keepScreenshot) {
        if (!keepScreenshot) {
            // Cancel pressed: Revert to previous state
            if (!originalScreenshotDataUrl) {
                screenshotDataUrl = null;
                annotations = [];
            }
        }

        if (screenshotModal) {
            screenshotModal.remove();
            screenshotModal = null;
        }

        // Return to main form
        if (feedbackModal) {
            feedbackModal.style.display = 'flex';
            updateScreenshotSectionInForm();
        } else {
            openFeedbackModal();
        }
    }

    // ========================================
    // SUBMISSION LOGIC
    // ========================================

    async function submitFeedback() {
        const title = feedbackModal.querySelector('#feedback-title').value.trim();
        const description = feedbackModal.querySelector('#feedback-description').value.trim();
        const submitBtn = feedbackModal.querySelector('#feedback-submit');

        if (!title) {
            showToast('Please enter a title', 'error');
            feedbackModal.querySelector('#feedback-title').focus();
            return;
        }
        if (!description) {
            showToast('Please enter a description', 'error');
            feedbackModal.querySelector('#feedback-description').focus();
            return;
        }

        // Prevent double submit
        submitBtn.disabled = true;
        submitBtn.textContent = 'Submitting...';

        try {
            // Collect all data
            const deviceInfo = getDeviceInfo();
            if (config.userEmail) deviceInfo.userEmail = config.userEmail;
            if (config.userName) deviceInfo.userName = config.userName;

            const payload = {
                title,
                description,
                type: selectedType,
                consoleLogs: JSON.stringify(consoleLogs),
                screenshotUrl: screenshotDataUrl || '',
                annotations: JSON.stringify(annotations), // Send annotations separately too (for potential future re-editing)
                metadata: deviceInfo,
            };

            const response = await fetch(config.endpoint, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(payload),
            });

            if (!response.ok) {
                const error = await response.json();
                throw new Error(error.error || 'Failed to submit feedback');
            }

            const result = await response.json();

            if (config.debug) {
                console.log('Feedback submitted:', result);
            }

            closeFeedbackModal();
            showToast('Thank you for your feedback!', 'success');

        } catch (error) {
            console.error('Feedback submission error:', error);
            showToast(error.message || 'Failed to submit feedback', 'error');
            submitBtn.disabled = false;
            submitBtn.textContent = 'Submit Feedback';
        }
    }

    // ========================================
    // UTILITIES & EXPORT
    // ========================================

    // Simple XSS prevention for user inputs re-rendered in form
    function escapeHtml(str) {
        if (!str) return '';
        return str
            .replace(/&/g, '&amp;')
            .replace(/</g, '&lt;')
            .replace(/>/g, '&gt;')
            .replace(/"/g, '&quot;')
            .replace(/'/g, '&#039;');
    }

    // Expose public API
    window.FeedbackWidget = {
        init: function(options = {}) {
            Object.assign(config, options);
            captureConsoleLogs();

            // Lazy init UI when DOM is ready
            if (document.readyState === 'loading') {
                document.addEventListener('DOMContentLoaded', createWidget);
            } else {
                createWidget();
            }
        },
        open: openFeedbackModal,
        close: closeFeedbackModal,
        config: config,
    };

    // Auto-init mechanism: allows script tag to just work without extra JS
    // <script src="..." data-auto-init="true"></script>
    const script = document.currentScript;
    if (script && script.dataset.autoInit !== 'false') {
        FeedbackWidget.init();
    }
})();