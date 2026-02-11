import { useEffect } from 'react';

/**
 * Keeps CSS vars in sync with the visible viewport on touch devices:
 * - `--app-height`: pixel height currently visible to the user.
 * - `--keyboard-offset`: estimated keyboard height when an editable is focused.
 *
 * This avoids brittle UI hiding rules and keeps the composer visible without
 * unmounting header controls.
 */
export default function useVirtualKeyboard() {
  useEffect(() => {
    const root = document.documentElement;
    const KEYBOARD_THRESHOLD_PX = 120;
    let isEditableFocused = false;
    let baselineVisibleHeight = 0;
    let blurTimeout: number | null = null;

    function isTouchDevice(): boolean {
      return 'ontouchstart' in window || navigator.maxTouchPoints > 0;
    }

    if (!isTouchDevice()) return;

    function isEditableElement(node: EventTarget | Element | null): node is HTMLElement {
      if (!(node instanceof HTMLElement)) return false;
      return node.tagName === 'TEXTAREA' || node.tagName === 'INPUT' || node.isContentEditable;
    }

    function getVisibleViewportHeight(): number {
      if (!window.visualViewport) return window.innerHeight;
      const vv = window.visualViewport;
      return vv.height + vv.offsetTop;
    }

    function setAppHeight(px: number) {
      const value = Math.max(0, Math.round(px));
      root.style.setProperty('--app-height', `${value}px`);
    }

    function setKeyboardOffset(px: number) {
      const value = Math.max(0, Math.round(px));
      root.style.setProperty('--keyboard-offset', `${value}px`);
      if (value > 0) {
        root.setAttribute('data-keyboard-open', '');
      } else {
        root.removeAttribute('data-keyboard-open');
      }
    }

    function syncViewport() {
      const visibleHeight = getVisibleViewportHeight();
      setAppHeight(visibleHeight);

      if (!isEditableFocused) {
        baselineVisibleHeight = visibleHeight;
        setKeyboardOffset(0);
        return;
      }

      const keyboardDelta = baselineVisibleHeight - visibleHeight;
      setKeyboardOffset(keyboardDelta > KEYBOARD_THRESHOLD_PX ? keyboardDelta : 0);
    }

    function handleFocusIn(event: FocusEvent) {
      if (!isEditableElement(event.target)) return;
      isEditableFocused = true;
      baselineVisibleHeight = getVisibleViewportHeight();
      syncViewport();
    }

    function handleFocusOut() {
      isEditableFocused = false;
      if (blurTimeout !== null) {
        window.clearTimeout(blurTimeout);
        blurTimeout = null;
      }
      blurTimeout = window.setTimeout(() => {
        if (!isEditableElement(document.activeElement)) {
          syncViewport();
        }
      }, 80);
    }

    function handleViewportChange() {
      syncViewport();
    }

    isEditableFocused = isEditableElement(document.activeElement);
    baselineVisibleHeight = getVisibleViewportHeight();
    syncViewport();

    document.addEventListener('focusin', handleFocusIn);
    document.addEventListener('focusout', handleFocusOut);
    window.addEventListener('resize', handleViewportChange);

    const vv = window.visualViewport;
    if (vv) {
      vv.addEventListener('resize', handleViewportChange);
      vv.addEventListener('scroll', handleViewportChange);
    }

    return () => {
      document.removeEventListener('focusin', handleFocusIn);
      document.removeEventListener('focusout', handleFocusOut);
      window.removeEventListener('resize', handleViewportChange);
      if (vv) {
        vv.removeEventListener('resize', handleViewportChange);
        vv.removeEventListener('scroll', handleViewportChange);
      }
      if (blurTimeout !== null) {
        window.clearTimeout(blurTimeout);
      }

      root.style.removeProperty('--app-height');
      root.style.removeProperty('--keyboard-offset');
      root.removeAttribute('data-keyboard-open');
    };
  }, []);
}
