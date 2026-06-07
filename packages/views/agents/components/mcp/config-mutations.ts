// Pure helpers that rewrite an `mcp_config` value (`{ mcpServers, ...,
// disabledMcpServers }`) for the MCP server manager. Shared by the per-agent
// and workspace-level surfaces so both produce identical config shapes. No
// React, no I/O — every function returns a new value and never mutates input.

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

/**
 * Rebuilds an `mcp_config` object from leftover top-level keys plus the active
 * (`mcpServers`) and disabled (`disabledMcpServers`) server maps. Empty maps
 * are dropped, and a config with nothing left collapses to `null` so the
 * backend clears the column and the runtime falls back to its own default.
 */
function reassembleConfig(
  rest: Record<string, unknown>,
  active: Record<string, unknown>,
  disabled: Record<string, unknown>,
): unknown | null {
  const out = { ...rest };
  if (Object.keys(active).length > 0) out.mcpServers = active;
  if (Object.keys(disabled).length > 0) out.disabledMcpServers = disabled;
  return Object.keys(out).length === 0 ? null : out;
}

/** Splits a config into its leftover keys + copies of both server maps. */
function splitConfig(config: Record<string, unknown>) {
  const active = isPlainObject(config.mcpServers) ? { ...config.mcpServers } : {};
  const disabled = isPlainObject(config.disabledMcpServers)
    ? { ...config.disabledMcpServers }
    : {};
  const rest = { ...config };
  delete rest.mcpServers;
  delete rest.disabledMcpServers;
  return { rest, active, disabled };
}

/**
 * Returns a copy of `config` with the named server removed from BOTH the active
 * and disabled maps (a name lives in exactly one, but deleting from both keeps
 * the operation idempotent regardless of current state).
 */
export function removeServerFromConfig(
  config: unknown,
  name: string,
): unknown | null {
  if (!isPlainObject(config)) return config ?? null;
  const { rest, active, disabled } = splitConfig(config);
  delete active[name];
  delete disabled[name];
  return reassembleConfig(rest, active, disabled);
}

/**
 * Moves the named server between the active `mcpServers` map and the sidecar
 * `disabledMcpServers` map. Disabling preserves the entry verbatim so it can be
 * re-enabled without re-entering config; the dispatch layer strips
 * `disabledMcpServers` before the runtime ever sees it. A no-op (name absent
 * from the source map) returns the config unchanged.
 */
export function setServerEnabled(
  config: unknown,
  name: string,
  enabled: boolean,
): unknown | null {
  if (!isPlainObject(config)) return config ?? null;
  const { rest, active, disabled } = splitConfig(config);
  const from = enabled ? disabled : active;
  const to = enabled ? active : disabled;
  if (!(name in from)) return config;
  to[name] = from[name];
  delete from[name];
  return reassembleConfig(rest, active, disabled);
}
