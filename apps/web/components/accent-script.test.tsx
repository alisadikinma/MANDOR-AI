import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { render, cleanup } from "@testing-library/react";
import { AccentScript, ACCENT_STORAGE_KEY } from "./accent-script";

// The anti-FOUC accent script must run BEFORE React hydrates, mirroring how
// next-themes stamps the light/dark class. We assert the component emits a
// blocking (non-async, non-deferred) inline <script> whose body reads the
// persisted accent from localStorage and stamps it onto
// document.documentElement.dataset.accent. We then execute the script body in
// a controlled DOM to prove the stamping logic actually works for a stored
// "blue" value and defaults to no attribute when unset.
describe("AccentScript — anti-FOUC accent stamping", () => {
  beforeEach(() => {
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-accent");
  });

  afterEach(() => {
    cleanup();
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-accent");
  });

  function renderScriptBody(): string {
    const { container } = render(<AccentScript />);
    const script = container.querySelector("script");
    expect(script).not.toBeNull();
    // Blocking: must not be async or deferred, otherwise it runs after paint
    // and the accent flashes the default colours first.
    expect(script?.hasAttribute("async")).toBe(false);
    expect(script?.hasAttribute("defer")).toBe(false);
    const body = script?.innerHTML ?? "";
    // The body must target the accent dataset key (dataset.accent maps 1:1 to
    // the data-accent attribute) — that is the contract the CSS overrides key
    // off of.
    expect(body).toContain("dataset.accent");
    return body;
  }

  // Executes the inline script body the same way the browser would when it
  // hits the blocking <script> during initial HTML parse.
  function execScriptBody(body: string): void {
    // eslint-disable-next-line no-new-func
    new Function(body)();
  }

  it("stamps data-accent='blue' on <html> when localStorage holds 'blue'", () => {
    window.localStorage.setItem(ACCENT_STORAGE_KEY, "blue");
    const body = renderScriptBody();

    execScriptBody(body);

    expect(document.documentElement.dataset.accent).toBe("blue");
  });

  it("leaves data-accent unset when no accent is stored (default theme)", () => {
    const body = renderScriptBody();

    execScriptBody(body);

    expect(document.documentElement.hasAttribute("data-accent")).toBe(false);
  });

  it("ignores an unknown stored accent value (enum drift downgrade)", () => {
    window.localStorage.setItem(ACCENT_STORAGE_KEY, "chartreuse");
    const body = renderScriptBody();

    execScriptBody(body);

    expect(document.documentElement.hasAttribute("data-accent")).toBe(false);
  });
});
