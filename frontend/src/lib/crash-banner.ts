/**
 * Phones have no console. An uncaught exception on the phone previously
 * vanished, leaving symptoms like a button that "does nothing" with no way to
 * see why. This banner is raw DOM on purpose: when the crash is inside the
 * reactive runtime, any store-driven toast can be wedged along with the app.
 */
const seen = new Set<string>();
let banner: HTMLElement | null = null;

/**
 * Same-origin asset URLs fill a stack frame with the origin and a 64-character
 * content hash, so a short report held only the first two frames. Keep the
 * function names and positions, which map back onto the deployed bundle.
 */
export function compactStack(message: string): string {
  return message.replace(/https?:\/\/[^\s()]*\/(?:assets\/)?([A-Za-z_-]+)[-.][0-9a-f]{16,}\.js/g, '$1');
}

function show(message: string): void {
  const text = compactStack(message).trim().slice(0, 900);
  if (!text || seen.has(text) || seen.size >= 3) return;
  if (text.includes('ResizeObserver loop')) return;
  seen.add(text);
  try {
    if (!banner) {
      banner = document.createElement('div');
      banner.setAttribute('role', 'alert');
      // Bottom-anchored so the header controls stay reachable while the
      // report is visible; the report itself dismisses on tap.
      banner.style.cssText = 'position:fixed;bottom:0;left:0;right:0;z-index:2147483647;'
        + 'background:#7f1d1d;color:#fff;font:12px/1.4 monospace;padding:.5rem .75rem;'
        + 'white-space:pre-wrap;word-break:break-word;max-height:40vh;overflow-y:auto;';
      banner.addEventListener('click', () => {
        banner?.remove();
        banner = null;
        seen.clear();
      });
      document.body.append(banner);
    }
    banner.textContent = `${banner.textContent ? `${banner.textContent}\n` : 'App error (tap to dismiss)\n'}${text}`;
  } catch {
    // Reporting must never take the app down further.
  }
}

/**
 * Reports an invariant violation the app survived. It rides the same banner
 * as a crash because both mean the same thing to a phone with no console:
 * something is wrong and this text is the only evidence that will exist.
 */
export function reportAppAnomaly(message: string): void {
  show(message);
}

let handlersInstalled = false;
export function reportUncaughtErrors(): void {
  if (handlersInstalled) return;
  handlersInstalled = true;
  window.addEventListener('error', (event) => {
    // Browsers hide everything but "Script error." for scripts from another
    // origin, which on a phone means a Safari extension or content blocker,
    // never the app's own same-origin module. There is nothing to show.
    if (event.message === 'Script error.' && !event.filename) return;
    show(String(event.error?.stack || event.message || event.error || 'Unknown error'));
  });
  window.addEventListener('unhandledrejection', (event) => {
    const reason = event.reason as { stack?: string } | undefined;
    show(`Unhandled rejection: ${String(reason?.stack || reason || 'unknown')}`);
  });
}
