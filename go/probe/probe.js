// This script injects sightmap ids and computes all component properties
// in a single JavaScript execution, eliminating the need for multiple CDP calls.
function computeCompProps(isLogicalRoot, useScrollOffset) {
    'use strict';

    // NOTE: Sync these to cdp_components.go
    const ATTR_PREFIX = 'data-sightmap-';
    const SIGHTMAP_ATTR = ATTR_PREFIX + 'id';
    const TEXT_NODES_ATTR = ATTR_PREFIX + 'texts';

    /**
     * Generates a sequential sightmap id for an element.
     */
    function genSightmapId(element, idCounter) {
        const id = idCounter.value.toString();
        idCounter.value++;
        return id;
    }

    /**
     * Generates a sequential sightmap id for a text node.
     */
    function genTextSightmapId(textNode, fallbackCounter) {
        const id = fallbackCounter.value.toString();
        fallbackCounter.value++;
        return id;
    }

    /**
     * Removes all sightmap id attributes to prevent state accumulation
     * between events (ids are regenerated on each run).
     */
    function cleanupSightmapIds() {
        try {
            const elemsWithSightmapIds = document.querySelectorAll(`[${SIGHTMAP_ATTR}]`);
            for (const element of elemsWithSightmapIds) {
                element.removeAttribute(SIGHTMAP_ATTR);
            }

            // Remove all text node attributes (they'll be regenerated)
            const elementsWithTextNodes = document.querySelectorAll(`[${TEXT_NODES_ATTR}]`);
            for (const element of elementsWithTextNodes) {
                element.removeAttribute(TEXT_NODES_ATTR);
            }
        } catch (error) {
            // Silently continue if cleanup fails
        }
    }

    /**
     * Computes bounds for an element using getBoundingClientRect
     * Note: getBoundingClientRect returns CSS pixels, same as CDP's GetBoxModel
     *
     * In replay mode, content is scaled to fit the iframe while preserving aspect ratio.
     * We need to account for this scaling to match CDP bounds which are relative to the original session size.
     */
    function computeBounds(element, scaleX, scaleY) {
        try {
            const rect = element.getBoundingClientRect();

            // In replay (logical root in a non-browser-root frame), convert
            // viewport coords to document coords for the session iframe.
            // In live mode and child frames, keep viewport-relative —
            // embedFrames handles rebasing child frame bounds.
            const scrollX = useScrollOffset ? window.pageXOffset : 0;
            const scrollY = useScrollOffset ? window.pageYOffset : 0;
            const bx = rect.left + scrollX;
            const by = rect.top + scrollY;

            // Apply scaling to match CDP coordinates
            // This accounts for iframe scaling in replay mode
            return {
                x: Math.round(bx * scaleX),
                y: Math.round(by * scaleY),
                width: Math.round(rect.width * scaleX),
                height: Math.round(rect.height * scaleY)
            };
        } catch (e) {
            return { x: 0, y: 0, width: 0, height: 0 };
        }
    }

    /**
     * Computes EFFECTIVE visibility for an element.
     *
     * We defer to the browser's own style engine via Element.checkVisibility(),
     * which — unlike a per-element getComputedStyle — accounts for ANCESTORS: it
     * returns false when the element or any ancestor is hidden by display:none,
     * visibility:hidden/collapse, opacity:0, or content-visibility. That is the
     * signal coverage and the tree renderer both need: a control inside a closed
     * opacity:0 overlay (a dismissed dropdown/context menu) has computed
     * opacity:1 of its own, so a per-element check reports it visible while it is
     * really painted nowhere. Keeping the layout/rendering judgment in the real
     * browser avoids re-deriving CSS semantics on the Go side.
     */
    function computeVisibility(element, bounds) {
        try {
            // Zero-size boxes never paint.
            if (bounds.width <= 0 || bounds.height <= 0) return false;

            if (typeof element.checkVisibility === 'function') {
                // Standardized option names (opacityProperty/visibilityProperty/
                // contentVisibilityAuto) plus their pre-standard aliases
                // (checkOpacity/checkVisibilityCSS), so older engines that only
                // know the aliases still honor the opacity/visibility checks.
                // Unknown dictionary members are ignored per WebIDL.
                return element.checkVisibility({
                    opacityProperty: true,
                    visibilityProperty: true,
                    contentVisibilityAuto: true,
                    checkOpacity: true,
                    checkVisibilityCSS: true,
                });
            }

            // Fallback for engines without checkVisibility: per-element checks
            // only (no ancestor awareness).
            const styles = window.getComputedStyle(element);
            if (styles.display === 'none') return false;
            if (styles.visibility === 'hidden') return false;
            if (styles.opacity === '0') return false;
            return true;
        } catch (e) {
            return false;
        }
    }

    /**
     * Computes whether an element's center lies within the visible viewport.
     *
     * Bounds are viewport-relative when useScrollOffset=false (the live-browser
     * default), so y<0 means the element has scrolled above the fold and
     * y>innerHeight means it is below. Center-based testing matches the
     * "click-safe" semantic: if the center is off-screen, clicking the geometric
     * midpoint will land on the wrong element (e.g. a sticky header).
     */
    function computeInViewport(bounds) {
        if (bounds.width <= 0 || bounds.height <= 0) return false;
        const cx = bounds.x + bounds.width / 2;
        const cy = bounds.y + bounds.height / 2;
        return cx >= 0 && cx < window.innerWidth &&
               cy >= 0 && cy < window.innerHeight;
    }

    /**
     * Computes whether an element is interactive
     * Matches the existing isInteractiveElement() function in cdp_components.go
     */
    function computeInteractivity(element) {
        try {
            const tagName = element.tagName.toLowerCase();

            // Interactive tags (exact match to Go implementation)
            const interactiveTags = new Set([
                'button', 'input', 'select', 'textarea', 'a', 'details', 'summary'
            ]);

            if (interactiveTags.has(tagName)) return true;

            // Check for interactive attributes (exact match to Go implementation)
            if (element.hasAttribute('onclick')) return true;
            if (element.hasAttribute('tabindex')) return true;

            // Check for any aria-* attributes (exact match to Go implementation)
            if (element.hasAttributes()) {
                for (const attr of element.attributes) {
                    if (attr.name.startsWith('aria-')) {
                        return true;
                    }
                }
            }

            // Check computed styles
            const styles = window.getComputedStyle(element);
            if (styles.cursor === 'pointer') return true;

            return false;
        } catch (e) {
            return false;
        }
    }

    /**
     * Computes whether an element is "on top" using elementFromPoint
     */
    function computeOnTop(element, bounds, scaleX, scaleY) {
        try {
            if (bounds.width <= 0 || bounds.height <= 0) return false;

            // Convert bounds back to viewport coordinates for elementFromPoint().
            // If scroll offset was applied, subtract it to get back to viewport coords.
            const scrollX = useScrollOffset ? window.pageXOffset : 0;
            const scrollY = useScrollOffset ? window.pageYOffset : 0;
            const viewportX = bounds.x / scaleX - scrollX;
            const viewportY = bounds.y / scaleY - scrollY;
            const viewportWidth = bounds.width / scaleX;
            const viewportHeight = bounds.height / scaleY;

            // Test center point first
            const centerX = viewportX + viewportWidth / 2;
            const centerY = viewportY + viewportHeight / 2;

            const elementAtCenter = document.elementFromPoint(centerX, centerY);
            if (elementAtCenter === element || element.contains(elementAtCenter)) {
                return true;
            }

            // Test corner points if center fails
            const points = [
                [viewportX, viewportY],
                [viewportX + viewportWidth, viewportY],
                [viewportX, viewportY + viewportHeight],
                [viewportX + viewportWidth, viewportY + viewportHeight]
            ];

            for (const [x, y] of points) {
                const elementAtPoint = document.elementFromPoint(x, y);
                if (elementAtPoint === element || element.contains(elementAtPoint)) {
                    return true;
                }
            }

            return false;
        } catch (e) {
            return false;
        }
    }

    /**
     * Extracts metadata for specific element types
     */
    function extractMetadata(element) {
        const metadata = {};
        const tagName = element.tagName.toLowerCase();

        try {
            switch (tagName) {
                case 'input':
                    metadata.type = element.type || '';
                    metadata.value = element.value || '';
                    metadata.placeholder = element.placeholder || '';
                    break;

                case 'img':
                    metadata.alt = element.alt || '';
                    metadata.src = element.src || '';
                    break;
            }
        } catch (e) {
            // Ignore metadata extraction errors
        }

        return metadata;
    }

    /**
     * Generates a CSS selector string for an element
     * Updated to include more attributes (aria-*, data-*) while maintaining string format
     */
    function generateSelector(element) {
        try {
            let selector = element.tagName.toLowerCase();

            if (element.id) {
                selector += '#' + CSS.escape(element.id);
            }

            // Use classList, not className: on SVG elements className is an
            // SVGAnimatedString (no .split), which would throw and lose the whole
            // selector. classList is a DOMTokenList on both HTML and SVG.
            const classList = element.classList;
            if (classList && classList.length) {
                for (const cls of classList) {
                    selector += '.' + CSS.escape(cls);
                }
            }

            // Key attributes that should always be included. placeholder is here
            // so selectors like input[placeholder="..."] match offline too.
            const keyAttrs = ['type', 'name', 'href', 'role', 'for', 'tabindex', 'title', 'alt', 'placeholder'];
            for (const attr of keyAttrs) {
                const value = element.getAttribute(attr);
                if (value) {
                    selector += `[${attr}="${CSS.escape(value)}"]`;
                }
            }

            // Include all aria-* attributes
            if (element.hasAttributes()) {
                for (const attr of element.attributes) {
                    if (attr.name.startsWith('aria-')) {
                        selector += `[${attr.name}="${CSS.escape(attr.value)}"]`;
                    }
                }
            }

            // Include all data-* attributes (except our sightmap ids)
            if (element.hasAttributes()) {
                for (const attr of element.attributes) {
                    if (attr.name.startsWith('data-') && !attr.name.startsWith('data-sightmap-')) {
                        selector += `[${attr.name}="${CSS.escape(attr.value)}"]`;
                    }
                }
            }

            return selector;
        } catch (e) {
            return element.tagName ? element.tagName.toLowerCase() : 'unknown';
        }
    }

    /**
     * Computes all properties for a single element
     * Updated to produce ComponentData structure directly
     */
    // Tags whose text content is not meaningful for accessibility/display
    const noTextTags = new Set(['style', 'script', 'noscript']);

    function computeElemProps(element, sightmapId, scaleX, scaleY) {
        const bounds = computeBounds(element, scaleX, scaleY);
        const isVisible = computeVisibility(element, bounds);
        const inViewport = computeInViewport(bounds);
        const isInteractive = computeInteractivity(element);
        const onTop = computeOnTop(element, bounds, scaleX, scaleY);
        const metadata = extractMetadata(element);
        const selector = generateSelector(element);

        const tag = (element.tagName || '').toLowerCase();

        // For iframe/frame elements, use content box instead of border box so
        // embedFrames offsets child content correctly past the border.
        if (tag === 'iframe' || tag === 'frame') {
            try {
                const s = window.getComputedStyle(element);
                const bl = parseFloat(s.borderLeftWidth) || 0;
                const bt = parseFloat(s.borderTopWidth) || 0;
                const br = parseFloat(s.borderRightWidth) || 0;
                const bb = parseFloat(s.borderBottomWidth) || 0;
                bounds.x += Math.round(bl * scaleX);
                bounds.y += Math.round(bt * scaleY);
                bounds.width -= Math.round((bl + br) * scaleX);
                bounds.height -= Math.round((bt + bb) * scaleY);
            } catch (e) { /* ignore */ }
        }

        // Create ComponentData structure directly
        return {
            id: sightmapId,                                               // Map sightmapId -> id
            role: '',                                                  // Will be filled by accessibility data
            // Rendered text (innerText): reflects text-transform + visibility, is
            // whitespace-collapsed, and — crucially — excludes non-rendered
            // <style>/<script> descendants, matching the accessible name and what
            // runtime consumers read. We deliberately do NOT fall back to
            // textContent (it would re-introduce descendant style/script text on
            // container nodes with empty innerText), and we skip non-rendered
            // elements entirely — innerText is unreliable there (e.g. <head>
            // returns its <style> text). Go-side normalizeText collapses any
            // residual whitespace.
            text: (noTextTags.has(tag) || !isVisible) ? '' : ((element.innerText || '').substring(0, 100)),
            value: '',                                                 // Will be filled by accessibility data
            properties: {},                                            // Will be filled by accessibility data
            bounds: bounds,
            selector: selector,
            onTop: onTop,
            isVisible: isVisible,
            inViewport: inViewport,
            isInteractive: isInteractive,
            isIgnored: false,                                          // Will be filled by accessibility data
            metadata: metadata,
            nthChild: 0,                                               // Will be filled during tree building
            children: [],                                              // Will be filled during tree building
            tagName: element.tagName || '',
        };
    }

    /**
     * Computes bounds for a text node using Range API
     */
    function computeTextBounds(textNode, scaleX, scaleY) {
        try {
            const range = document.createRange();
            range.selectNodeContents(textNode);
            const rect = range.getBoundingClientRect();
            range.detach();

            const scrollX = useScrollOffset ? window.pageXOffset : 0;
            const scrollY = useScrollOffset ? window.pageYOffset : 0;
            const bx = rect.left + scrollX;
            const by = rect.top + scrollY;

            // Apply scaling to match CDP coordinates
            return {
                x: Math.round(bx * scaleX),
                y: Math.round(by * scaleY),
                width: Math.round(rect.width * scaleX),
                height: Math.round(rect.height * scaleY)
            };
        } catch (e) {
            // Fallback: return empty bounds if Range API fails
            return { x: 0, y: 0, width: 0, height: 0 };
        }
    }

    /**
     * Computes properties for text nodes
     * Updated to produce ComponentData structure directly
     */
    function computeTextProps(textNode, sightmapId, scaleX, scaleY) {
        // Text nodes have limited properties
        const textContent = textNode.textContent || '';

        // Compute bounds using Range API for accurate text positioning
        const bounds = computeTextBounds(textNode, scaleX, scaleY);

        // Create ComponentData structure directly for text nodes
        return {
            id: sightmapId,
            role: '',                                  // Text nodes typically don't have roles
            text: textContent.substring(0, 100),
            value: '',
            properties: {},
            bounds: bounds,
            selector: '',
            onTop: false,                              // Text nodes don't block other elements
            isVisible: textContent.trim().length > 0,
            inViewport: computeInViewport(bounds),     // Use computed bounds for viewport check
            isInteractive: false,                      // Text nodes are not interactive
            isIgnored: false,
            metadata: {},
            nthChild: 0,                               // Will be filled during tree building
            children: [],
            tagName: '#text',
        };
    }

    // Inject sightmap ids and compute properties in a single DOM walk
    const results = [];
    const debugInfo = [];
    try {

        // Clean up any existing sightmap ids to prevent state accumulation
        cleanupSightmapIds();

        // In replay the root frame is rendered in a scaled iframe —
        // screen dimensions differ from window dimensions. Compute the
        // scale factor so bounds match the original session size.
        // In live mode these are equal (or very close), so scale ≈ 1.0.
        // Child frames never scale (embedFrames handles rebasing).
        let scaleX = 1.0;
        let scaleY = 1.0;

        if (isLogicalRoot) {
            const currentWidth = window.innerWidth;
            const currentHeight = window.innerHeight;

            if (currentWidth > 0 && currentHeight > 0) {
                scaleX = screen.width / currentWidth;
                scaleY = screen.height / currentHeight;
            }
        }

        // Counter for sightmap ids
        // Using object so it can be passed by reference to maintain state
        const idCounter = { value: 1 };

        // Single-pass DOM traversal: inject ids and compute properties simultaneously
        function procNode(node) {
            if (node.nodeType === Node.ELEMENT_NODE) {
                // Generate sightmap id
                const sightmapId = genSightmapId(node, idCounter);
                node.setAttribute(SIGHTMAP_ATTR, sightmapId);

                // Compute properties for this element
                try {
                    const comp = computeElemProps(node, sightmapId, scaleX, scaleY);
                    results.push(comp);
                } catch (error) {
                    debugInfo.push(`ERROR computing properties for element ${sightmapId}: ${error.message}`);
                }

                // Process child nodes - collect text nodes first
                // Skip text inside style/script/noscript — not meaningful content
                const tag = (node.tagName || '').toLowerCase();
                const textNodeIds = [];
                for (const childNode of node.childNodes) {
                    if (!noTextTags.has(tag) &&
                        childNode.nodeType === Node.TEXT_NODE &&
                        childNode.textContent &&
                        childNode.textContent.trim()) {

                        // Generate stable sightmap id for text node
                        const textSightmapId = genTextSightmapId(childNode, idCounter);
                        textNodeIds.push(textSightmapId);

                        try {
                            const textComp = computeTextProps(childNode, textSightmapId, scaleX, scaleY);
                            results.push(textComp);
                        } catch (error) {
                            debugInfo.push(`error computing properties for text node ${textSightmapId}: ${error.message}`);
                        }
                    }
                }

                // Store text node sightmap ids on parent element
                if (textNodeIds.length > 0) {
                    node.setAttribute(TEXT_NODES_ATTR, textNodeIds.join(','));
                }

                // Recursively process element children
                for (const childNode of node.childNodes) {
                    if (childNode.nodeType === Node.ELEMENT_NODE) {
                        procNode(childNode);
                    }
                }

                // Process shadow DOM if present
                if (node.shadowRoot) {
                    for (const shadowChild of node.shadowRoot.childNodes) {
                        procNode(shadowChild);
                    }
                }
            }
        }

        // Start processing from document element
        if (document.documentElement) {
            procNode(document.documentElement);
        } else {
            debugInfo.push('ERROR: document.documentElement is null!');
        }

        return {
            success: true,
            results: results,
            debug: debugInfo.join('\n'),
        };
    } catch (error) {
        debugInfo.push(`FATAL ERROR: ${error.message}`);
        return {
            success: false,
            error: error.message,
            results: results, // Return partial results
            debug: debugInfo.join('\n'),
        };
    }
}
