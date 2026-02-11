import { useEffect } from 'react';

/**
 * Detects when the mobile virtual keyboard opens/closes and sets a CSS custom
 * property `--keyboard-offset` on <html> equal to the pixel height consumed by
 * the keyboard.  The app layout can then subtract that offset so the composer
 * stays visible above the keyboard.
 *
 * Strategy
 * --------
 * 1. **Primary – `visualViewport` API** (Safari 13+, Chrome Android 62+):
 *    Listen for `resize` events on `window.visualViewport`. When the visual
 *    viewport shrinks below the layout viewport, the difference is the keyboard
 *    height.
 *
 * 2. **Fallback – `resize` + `focusin`/`focusout`** (older browsers):
 *    On `focusin` of an editable element, record `window.innerHeight`. On
 *    subsequent `resize` events, compare the new innerHeight to decide if a
 *    keyboard appeared.
 *
 * Both paths write to the same CSS variable so the rest of the app is agnostic
 * to the detection method.
 */
export default function useVirtualKeyboard() {
  useEffect(() => {
    const root = document.documentElement;

    // Only run on touch devices to avoid false positives on desktop resize
    function isTouchDevice(): boolean {
      return 'ontouchstart' in window || navigator.maxTouchPoints > 0;
    }

    if (!isTouchDevice()) return;

    function setOffset(px: number) {
      // Round to avoid sub-pixel jitter; clamp to 0
      const value = Math.max(0, Math.round(px));
      root.style.setProperty('--keyboard-offset', `${value}px`);

      // Toggle a data attribute so CSS can style keyboard-open state
      if (value > 0) {
        root.setAttribute('data-keyboard-open', '');
      } else {
        root.removeAttribute('data-keyboard-open');
      }
    }

    // ── Primary: visualViewport API ────────────────────────────────
    if (window.visualViewport) {
      const vv = window.visualViewport;

      // We use window.innerHeight (layout viewport height) as the baseline.
      // When the keyboard opens, vv.height shrinks while innerHeight stays
      // the same, so the delta gives us the keyboard height.
      //
      // We also account for vv.offsetTop which shifts on iOS when the
      // address bar collapses/expands.
      function onViewportResize() {
        // On iOS Safari, window.innerHeight may or may not update depending
        // on the interactive-widget viewport meta. Using the initial full
        // height captured once is more reliable.
        const offset = window.innerHeight - vv.height;
        setOffset(offset);
      }

      vv.addEventListener('resize', onViewportResize);

      // Also listen for scroll — iOS shifts the viewport when the URL bar
      // shows/hides, which fires scroll but not always resize.
      vv.addEventListener('scroll', onViewportResize);

      return () => {
        vv.removeEventListener('resize', onViewportResize);
        vv.removeEventListener('scroll', onViewportResize);
        setOffset(0);
        root.removeAttribute('data-keyboard-open');
      };
    }

    // ── Fallback: resize + focus tracking ──────────────────────────
    let baseHeight = window.innerHeight;
    let inputFocused = false;

    function onFocusIn(e: FocusEvent) {
      const target = e.target as HTMLElement | null;
      if (
        target &&
        (target.tagName === 'TEXTAREA' ||
          target.tagName === 'INPUT' ||
          target.isContentEditable)
      ) {
        inputFocused = true;
        baseHeight = window.innerHeight;
      }
    }

    function onFocusOut() {
      inputFocused = false;
      // Small delay to allow keyboard to fully dismiss
      setTimeout(() => {
        if (!inputFocused) setOffset(0);
      }, 100);
    }

    function onResize() {
      if (!inputFocused) return;
      const diff = baseHeight - window.innerHeight;
      // Only treat it as a keyboard if the difference is significant
      // (more than 150px — keyboards are at least ~250px tall)
      if (diff > 150) {
        setOffset(diff);
      } else {
        setOffset(0);
      }
    }

    document.addEventListener('focusin', onFocusIn);
    document.addEventListener('focusout', onFocusOut);
    window.addEventListener('resize', onResize);

    return () => {
      document.removeEventListener('focusin', onFocusIn);
      document.removeEventListener('focusout', onFocusOut);
      window.removeEventListener('resize', onResize);
      setOffset(0);
      root.removeAttribute('data-keyboard-open');
    };
  }, []);
}
