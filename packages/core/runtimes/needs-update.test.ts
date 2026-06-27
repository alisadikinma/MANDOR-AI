import { describe, it, expect } from "vitest";
import { runtimeNeedsUpdate } from "./hooks";
import type { AgentRuntime } from "../types";

const USER = "user-1";

// Minimal local runtime owned by USER with the given cli_version.
function rt(cliVersion: string | null): AgentRuntime {
  return {
    runtime_mode: "local",
    owner_id: USER,
    metadata: cliVersion === null ? {} : { cli_version: cliVersion },
  } as unknown as AgentRuntime;
}

describe("runtimeNeedsUpdate", () => {
  it("never prompts a dev/source build (cli_version 'dev')", () => {
    // Regression: isNewer() parses "dev" to NaN and would otherwise report
    // every dev build as outdated, painting a red dot on a healthy runtime.
    expect(runtimeNeedsUpdate(rt("dev"), "0.3.14", USER)).toBe(false);
  });

  it("prompts when the installed release is older than latest", () => {
    expect(runtimeNeedsUpdate(rt("0.3.13"), "0.3.14", USER)).toBe(true);
  });

  it("does not prompt when already on the latest release", () => {
    expect(runtimeNeedsUpdate(rt("0.3.14"), "0.3.14", USER)).toBe(false);
  });
});
