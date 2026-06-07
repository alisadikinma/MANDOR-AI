import { create } from "zustand";
import type { StorageAdapter } from "../types";
import { createLogger } from "../logger";

const logger = createLogger("theme.accent-store");

/**
 * localStorage key for the selected accent. Mirrored by the anti-FOUC script
 * in apps/web (apps/web/components/accent-script.tsx) — both must agree, but
 * the script cannot import this constant because it runs before any bundle
 * loads. This store is the single source of truth that WRITES the value.
 */
export const ACCENT_STORAGE_KEY = "multica:theme:accent";

/**
 * Accent axis, orthogonal to light/dark. `"default"` leaves the brand tokens
 * untouched; `"blue"` swaps in the Atlassian-blue token overrides keyed off
 * `data-accent="blue"` in packages/ui/styles/tokens.css.
 */
export type Accent = "default" | "blue";

const ACCENTS: readonly Accent[] = ["default", "blue"] as const;

function isAccent(value: string | null): value is Accent {
  return value !== null && (ACCENTS as readonly string[]).includes(value);
}

export interface AccentState {
  accent: Accent;
  setAccent: (accent: Accent) => void;
}

export interface AccentStoreOptions {
  storage: StorageAdapter;
}

/**
 * Accent store factory. Pure client state — persists ONLY through the injected
 * StorageAdapter (zero direct localStorage, zero DOM). The DOM side-effect
 * (stamping `document.documentElement.dataset.accent`) lives in the platform /
 * app layer that already owns the DOM, never here, so this stays
 * react-dom-free and usable on any platform.
 */
export function createAccentStore(options: AccentStoreOptions) {
  const { storage } = options;

  // Read persisted accent at construction. Unknown/forward-drift values
  // downgrade to "default" rather than being trusted blindly.
  const stored = storage.getItem(ACCENT_STORAGE_KEY);
  const initialAccent: Accent = isAccent(stored) ? stored : "default";

  return create<AccentState>((set, get) => ({
    accent: initialAccent,
    setAccent: (accent) => {
      if (accent === get().accent) return;
      logger.info("setAccent", { from: get().accent, to: accent });
      // "default" is the absence of an accent — remove the key so storage
      // doesn't accumulate a redundant default sentinel.
      if (accent === "default") {
        storage.removeItem(ACCENT_STORAGE_KEY);
      } else {
        storage.setItem(ACCENT_STORAGE_KEY, accent);
      }
      set({ accent });
    },
  }));
}
