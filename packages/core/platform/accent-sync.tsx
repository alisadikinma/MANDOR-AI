"use client";

import { useEffect } from "react";
import { useAccentStore } from "../theme";

/**
 * Reflects the accent store onto `document.documentElement.dataset.accent`.
 *
 * The anti-FOUC script (apps/web) stamps the attribute on first paint from
 * persisted storage; this component keeps it in sync afterwards when the user
 * changes the accent live (no reload). The DOM write lives here in the
 * platform layer — which already owns DOM access guarded for SSR — and NOT in
 * the core store, keeping the store pure client state.
 */
export function AccentSync() {
  const accent = useAccentStore((s) => s.accent);

  useEffect(() => {
    if (typeof document === "undefined") return;
    const root = document.documentElement;
    if (accent === "default") {
      delete root.dataset.accent;
    } else {
      root.dataset.accent = accent;
    }
  }, [accent]);

  return null;
}
