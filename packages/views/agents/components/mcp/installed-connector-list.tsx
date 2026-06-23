"use client";

import { useState } from "react";
import { ChevronLeft, ChevronRight, Loader2, Plug } from "lucide-react";
import type { McpProbeServerResult } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";

/** Live connection status keyed by server name (from a "Test connections"
 *  probe). Absent until the user runs a probe. */
export type LiveStatusMap = Record<string, McpProbeServerResult>;

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

export interface InstalledServer {
  name: string;
  /** One-line, secret-free hint about how the server is launched. */
  summary: string;
  /** False when the server lives in the sidecar `disabledMcpServers` map. */
  enabled: boolean;
}

/**
 * Builds a compact, secret-free summary for one server entry: the launch
 * command (+ first arg) for stdio servers, or the URL / transport for
 * HTTP/SSE-style entries. Env vars and headers are deliberately omitted — they
 * routinely carry tokens, and this string is shown in plain text.
 */
function summarizeServer(entry: unknown): string {
  if (!isPlainObject(entry)) return "";
  if (typeof entry.command === "string") {
    const args = Array.isArray(entry.args) ? entry.args : [];
    const first = typeof args[0] === "string" ? args[0] : "";
    return [entry.command, first].filter(Boolean).join(" ");
  }
  const url = entry.url ?? entry.httpUrl ?? entry.serverUrl;
  if (typeof url === "string") return url;
  const transport = entry.transport ?? entry.type;
  if (typeof transport === "string") return transport;
  return "";
}

/**
 * Extracts the agent's configured MCP servers from a stored `mcp_config`
 * value, reading BOTH the active `mcpServers` map and the sidecar
 * `disabledMcpServers` map (disabled entries are kept verbatim so they can be
 * re-enabled without re-entering config). Tolerates `null`, non-objects, and
 * missing/ malformed maps — every bad shape collapses to an empty list rather
 * than throwing. Active servers sort before disabled ones, name-ascending.
 */
export function extractInstalledServers(config: unknown): InstalledServer[] {
  if (!isPlainObject(config)) return [];
  const collect = (raw: unknown, enabled: boolean): InstalledServer[] =>
    isPlainObject(raw)
      ? Object.entries(raw).map(([name, entry]) => ({
          name,
          summary: summarizeServer(entry),
          enabled,
        }))
      : [];
  const servers = [
    ...collect(config.mcpServers, true),
    ...collect(config.disabledMcpServers, false),
  ];
  return servers.sort((a, b) => {
    if (a.enabled !== b.enabled) return a.enabled ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
}

/**
 * The Claude-Code-`/mcp`-style manager for an agent's MCP servers: a list of
 * rows that drill into a per-server detail panel. The detail panel toggles a
 * server between enabled/disabled (without losing its config) and removes it
 * after an inline confirm. All mutations are delegated to the parent (which
 * rewrites `mcp_config` and persists through the agent-save path); this
 * component owns only selection + the per-row pending/confirm UI state.
 */
export function InstalledConnectorList({
  servers,
  inherited,
  liveStatus,
  probing,
  onAuthenticate,
  onRemove,
  onToggle,
}: {
  servers: InstalledServer[];
  /**
   * Read-only servers inherited from a higher scope (workspace servers shown in
   * the agent tab). When present, the list splits into a "Workspace" group and
   * a "This agent" group, mirroring Claude Code's scoped `/mcp` view. Omit for
   * single-scope surfaces (e.g. the workspace settings tab).
   */
  inherited?: InstalledServer[];
  /** Live connection status per server name from the last probe; null until a
   *  probe completes. Overrides the enabled/disabled pill when present. */
  liveStatus?: LiveStatusMap | null;
  /** True while a probe is in flight — rows show a "Checking…" pill. */
  probing?: boolean;
  /** Starts OAuth for a `needs_auth` server (opens a popup, re-probes on
   *  success). Omit to fall back to the runtime-CLI sign-in hint. */
  onAuthenticate?: (name: string) => Promise<void> | void;
  onRemove: (name: string) => Promise<void> | void;
  onToggle: (name: string, enabled: boolean) => Promise<void> | void;
}) {
  const [selected, setSelected] = useState<string | null>(null);

  const active = selected
    ? (servers.find((s) => s.name === selected) ?? null)
    : null;

  if (active) {
    return (
      <ServerDetail
        server={active}
        live={liveStatus?.[active.name]}
        probing={probing}
        onAuthenticate={onAuthenticate}
        onBack={() => setSelected(null)}
        onRemove={async (name) => {
          await onRemove(name);
          setSelected(null);
        }}
        onToggle={onToggle}
      />
    );
  }

  const hasInherited = (inherited?.length ?? 0) > 0;

  if (servers.length === 0 && !hasInherited) {
    return (
      <p className="text-xs text-muted-foreground">
        No connectors yet. Use “Browse connectors” to add one.
      </p>
    );
  }

  return (
    <div className="space-y-3">
      {hasInherited && (
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">Workspace</p>
          <ul
            className="divide-y rounded-md border"
            aria-label="Inherited connectors"
          >
            {inherited!.map((server) => (
              <li
                key={server.name}
                className="flex items-center gap-3 px-3 py-2"
              >
                <Plug className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">{server.name}</p>
                  {server.summary && (
                    <p className="truncate font-mono text-xs text-muted-foreground">
                      {server.summary}
                    </p>
                  )}
                </div>
                <InheritedPill
                  live={liveStatus?.[server.name]}
                  probing={probing}
                />
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="space-y-1.5">
        {hasInherited && (
          <p className="text-xs font-medium text-muted-foreground">
            This agent
          </p>
        )}
        {servers.length === 0 ? (
          <p className="text-xs text-muted-foreground">
            No agent-specific connectors. Use “Browse connectors” to add one.
          </p>
        ) : (
          <ul
            className="divide-y rounded-md border"
            aria-label="Installed connectors"
          >
            {servers.map((server) => (
              <li key={server.name}>
                <button
                  type="button"
                  className="flex w-full items-center gap-3 px-3 py-2 text-left hover:bg-muted/40"
                  onClick={() => setSelected(server.name)}
                  aria-label={`Manage ${server.name}`}
                >
                  <Plug className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">
                      {server.name}
                    </p>
                    {server.summary && (
                      <p className="truncate font-mono text-xs text-muted-foreground">
                        {server.summary}
                      </p>
                    )}
                  </div>
                  <ConnectionPill
                    live={liveStatus?.[server.name]}
                    probing={probing}
                    enabled={server.enabled}
                  />
                  <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function StatusPill({ enabled }: { enabled: boolean }) {
  return (
    <Badge variant={enabled ? "default" : "secondary"} className="shrink-0">
      {enabled ? "Enabled" : "Disabled"}
    </Badge>
  );
}

/** Live connection status from a probe. `status` is server-driven — an unknown
 *  value downgrades to "Not testable" rather than crashing (enum drift). */
function LivePill({ result }: { result: McpProbeServerResult }) {
  switch (result.status) {
    case "connected":
      return (
        <Badge variant="default" className="shrink-0">
          Connected
          {result.tool_count > 0 ? ` · ${result.tool_count} tools` : ""}
        </Badge>
      );
    case "failed":
      return (
        <Badge variant="destructive" className="shrink-0">
          Failed
        </Badge>
      );
    case "needs_auth":
      return (
        <Badge variant="secondary" className="shrink-0">
          Needs auth
        </Badge>
      );
    default:
      return (
        <Badge variant="outline" className="shrink-0">
          Not testable
        </Badge>
      );
  }
}

function CheckingPill() {
  return (
    <Badge variant="secondary" className="shrink-0 gap-1">
      <Loader2 className="h-3 w-3 animate-spin" />
      Checking…
    </Badge>
  );
}

/** Picks the right pill for an editable server row: live probe result wins,
 *  then an in-flight "Checking…", else the static enabled/disabled state. */
function ConnectionPill({
  live,
  probing,
  enabled,
}: {
  live?: McpProbeServerResult;
  probing?: boolean;
  enabled: boolean;
}) {
  if (live) return <LivePill result={live} />;
  if (probing) return <CheckingPill />;
  return <StatusPill enabled={enabled} />;
}

/** Pill for a read-only inherited (workspace) row: falls back to an "Inherited"
 *  badge instead of an enabled/disabled toggle state. */
function InheritedPill({
  live,
  probing,
}: {
  live?: McpProbeServerResult;
  probing?: boolean;
}) {
  if (live) return <LivePill result={live} />;
  if (probing) return <CheckingPill />;
  return (
    <Badge variant="outline" className="shrink-0">
      Inherited
    </Badge>
  );
}

function ServerDetail({
  server,
  live,
  probing,
  onAuthenticate,
  onBack,
  onRemove,
  onToggle,
}: {
  server: InstalledServer;
  live?: McpProbeServerResult;
  probing?: boolean;
  onAuthenticate?: (name: string) => Promise<void> | void;
  onBack: () => void;
  onRemove: (name: string) => Promise<void> | void;
  onToggle: (name: string, enabled: boolean) => Promise<void> | void;
}) {
  const [busy, setBusy] = useState<"toggle" | "remove" | null>(null);
  const [authing, setAuthing] = useState(false);
  const [confirming, setConfirming] = useState(false);

  const run = async (
    kind: "toggle" | "remove",
    fn: () => Promise<void> | void,
  ) => {
    setBusy(kind);
    try {
      await fn();
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="space-y-3 rounded-md border p-3">
      <button
        type="button"
        className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        onClick={onBack}
      >
        <ChevronLeft className="h-3.5 w-3.5" />
        Back to list
      </button>

      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <Plug className="h-4 w-4 shrink-0 text-muted-foreground" />
          <h3 className="truncate text-sm font-semibold">{server.name}</h3>
        </div>
        <ConnectionPill live={live} probing={probing} enabled={server.enabled} />
      </div>

      {live?.status === "failed" && live.error && (
        <p className="rounded bg-destructive/10 px-2 py-1.5 text-xs text-destructive">
          {live.error}
        </p>
      )}

      {live?.status === "needs_auth" &&
        (onAuthenticate ? (
          <div className="space-y-1.5 rounded bg-muted/40 px-2 py-2">
            <p className="text-xs text-muted-foreground">
              This server needs sign-in. Authenticate once and it connects for
              everyone in this workspace.
            </p>
            <Button
              type="button"
              size="sm"
              disabled={authing}
              onClick={async () => {
                setAuthing(true);
                try {
                  await onAuthenticate(server.name);
                } finally {
                  setAuthing(false);
                }
              }}
            >
              {authing ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : null}
              Authenticate
            </Button>
          </div>
        ) : (
          <p className="rounded bg-muted/40 px-2 py-1.5 text-xs text-muted-foreground">
            Sign-in happens on the runtime, not here. Authenticate once on the
            runtime host with the agent&apos;s CLI (e.g. run{" "}
            <code className="font-mono">claude</code> there and complete the
            sign-in), then re-test.
          </p>
        ))}

      {server.summary && (
        <div className="space-y-1">
          <p className="text-xs font-medium text-muted-foreground">
            Configuration
          </p>
          <p className="break-all rounded bg-muted/40 px-2 py-1.5 font-mono text-xs">
            {server.summary}
          </p>
        </div>
      )}

      <div className="flex flex-col gap-2 pt-1">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={busy !== null}
          onClick={() =>
            void run("toggle", () => onToggle(server.name, !server.enabled))
          }
        >
          {busy === "toggle" ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : null}
          {server.enabled ? "Disable" : "Enable"}
        </Button>

        {confirming ? (
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="destructive"
              size="sm"
              className="flex-1"
              disabled={busy !== null}
              onClick={() => void run("remove", () => onRemove(server.name))}
            >
              {busy === "remove" ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : null}
              Remove
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="flex-1"
              disabled={busy !== null}
              onClick={() => setConfirming(false)}
            >
              Cancel
            </Button>
          </div>
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="text-muted-foreground hover:text-destructive"
            disabled={busy !== null}
            onClick={() => setConfirming(true)}
            aria-label={`Remove ${server.name}`}
          >
            Remove connector
          </Button>
        )}
      </div>
    </div>
  );
}
