export {
  createAccentStore,
  ACCENT_STORAGE_KEY,
} from "./accent-store";
export type { Accent, AccentState, AccentStoreOptions } from "./accent-store";

import type { createAccentStore as CreateAccentStoreFn } from "./accent-store";

type AccentStoreInstance = ReturnType<typeof CreateAccentStoreFn>;

/** Module-level singleton — set once at app boot via `registerAccentStore()`. */
let _store: AccentStoreInstance | null = null;

/**
 * Register the accent store instance created by the app. Must be called at
 * boot before any component reads `useAccentStore`.
 */
export function registerAccentStore(store: AccentStoreInstance) {
  _store = store;
}

/**
 * Singleton accessor — a Zustand hook backed by the registered instance.
 * Supports `useAccentStore(selector)` and `useAccentStore.getState()`.
 */
export const useAccentStore: AccentStoreInstance = new Proxy(
  (() => {}) as unknown as AccentStoreInstance,
  {
    apply(_target, _thisArg, args) {
      if (!_store)
        throw new Error(
          "Accent store not initialised — call registerAccentStore() first",
        );
      return (_store as unknown as (...a: unknown[]) => unknown)(...args);
    },
    get(_target, prop) {
      if (!_store) return undefined;
      return Reflect.get(_store, prop);
    },
  },
);
