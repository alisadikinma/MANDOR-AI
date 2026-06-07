// localStorage key holding the selected accent. Must stay in sync with the
// core accent store (packages/core/theme/accent-store.ts), which is the single
// source of truth that writes this value. This duplicated literal is the price
// of an anti-FOUC script: it runs before any module loads, so it cannot import
// the constant.
export const ACCENT_STORAGE_KEY = "multica:theme:accent";

// The set of accent values this build understands. An unknown stored value
// (forward/back version drift) is ignored rather than stamped, so a future
// accent never paints garbage onto an older client.
const KNOWN_ACCENTS = ["blue"];

// Blocking inline script, mirroring how next-themes stamps the light/dark
// class before paint. `data-accent` is an axis ORTHOGONAL to light/dark
// (blue works in both), so next-themes does not manage it — we do. Running
// this synchronously in <head> (before <body> and hydration) means the correct
// accent tokens are resolved on the very first paint, with no flash of the
// default brand colour.
//
// Kept dependency-free and stringified by hand: it executes during HTML parse,
// long before any bundle is evaluated, so it must not reference imports.
function buildAccentScript(): string {
  return `(function(){try{var a=localStorage.getItem(${JSON.stringify(
    ACCENT_STORAGE_KEY,
  )});if(a&&${JSON.stringify(
    KNOWN_ACCENTS,
  )}.indexOf(a)!==-1){document.documentElement.dataset.accent=a;}}catch(e){}})();`;
}

/**
 * Anti-FOUC accent stamper. Render once in the root layout `<head>` (before
 * `<body>`). Emits a blocking `<script>` that reads the persisted accent and
 * sets `data-accent` on `<html>` before the first paint.
 *
 * Direct `localStorage` / DOM access is intentional and allowed here: this is
 * the `apps/web` boundary, the only place such access belongs (same exception
 * next-themes relies on). The core store never touches the DOM.
 */
export function AccentScript() {
  return (
    <script
      // suppressHydrationWarning: the script mutates <html> before React
      // hydrates, so the server-rendered markup and the post-script DOM differ
      // by design — identical to next-themes' own injected script.
      suppressHydrationWarning
      dangerouslySetInnerHTML={{ __html: buildAccentScript() }}
    />
  );
}
