import { describe, expect, it } from "vitest";
import type { StorageAdapter } from "../types";
import { createAccentStore, ACCENT_STORAGE_KEY } from "./accent-store";

function makeStorage(initial: Record<string, string> = {}): StorageAdapter & {
  snapshot: () => Record<string, string>;
} {
  const data = { ...initial };
  return {
    getItem: (k) => data[k] ?? null,
    setItem: (k, v) => {
      data[k] = v;
    },
    removeItem: (k) => {
      delete data[k];
    },
    snapshot: () => ({ ...data }),
  };
}

describe("createAccentStore", () => {
  it("defaults to 'default' when storage is empty", () => {
    const storage = makeStorage();
    const store = createAccentStore({ storage });
    expect(store.getState().accent).toBe("default");
  });

  it("setAccent('blue') updates state and persists through the injected storage", () => {
    const storage = makeStorage();
    const store = createAccentStore({ storage });

    store.getState().setAccent("blue");

    expect(store.getState().accent).toBe("blue");
    expect(storage.snapshot()[ACCENT_STORAGE_KEY]).toBe("blue");
  });

  it("setAccent('default') clears the persisted value (no stray key)", () => {
    const storage = makeStorage({ [ACCENT_STORAGE_KEY]: "blue" });
    const store = createAccentStore({ storage });
    expect(store.getState().accent).toBe("blue");

    store.getState().setAccent("default");

    expect(store.getState().accent).toBe("default");
    expect(ACCENT_STORAGE_KEY in storage.snapshot()).toBe(false);
  });

  it("re-creating with the same storage restores the persisted accent", () => {
    const storage = makeStorage();
    createAccentStore({ storage }).getState().setAccent("blue");

    // Simulate a fresh app boot reading the same persisted storage.
    const restored = createAccentStore({ storage });
    expect(restored.getState().accent).toBe("blue");
  });

  it("ignores an unknown persisted value and falls back to 'default'", () => {
    const storage = makeStorage({ [ACCENT_STORAGE_KEY]: "chartreuse" });
    const store = createAccentStore({ storage });
    expect(store.getState().accent).toBe("default");
  });
});
