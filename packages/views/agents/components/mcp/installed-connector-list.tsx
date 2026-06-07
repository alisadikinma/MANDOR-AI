"use client";

import { useState } from "react";
import { ChevronLeft, ChevronRight, Loader2, Plug } from "lucide-react";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";

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
  onRemove,
  onToggle,
}: {
  servers: InstalledServer[];
  onRemove: (name: string) => Promise<void> | void;
  onToggle: (name: string, enabled: boolean) => Promise<void> | void;
}) {
  const [selected, setSelected] = useState<string | null>(null);

  if (servers.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No connectors yet. Use “Browse connectors” to add one.
      </p>
    );
  }

  const active = selected
    ? (servers.find((s) => s.name === selected) ?? null)
    : null;

  if (active) {
    return (
      <ServerDetail
        server={active}
        onBack={() => setSelected(null)}
        onRemove={async (name) => {
          await onRemove(name);
          setSelected(null);
        }}
        onToggle={onToggle}
      />
    );
  }

  return (
    <ul className="divide-y rounded-md border" aria-label="Installed connectors">
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
              <p className="truncate text-sm font-medium">{server.name}</p>
              {server.summary && (
                <p className="truncate font-mono text-xs text-muted-foreground">
                  {server.summary}
                </p>
              )}
            </div>
            <StatusPill enabled={server.enabled} />
            <ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
          </button>
        </li>
      ))}
    </ul>
  );
}

function StatusPill({ enabled }: { enabled: boolean }) {
  return (
    <Badge variant={enabled ? "default" : "secondary"} className="shrink-0">
      {enabled ? "Enabled" : "Disabled"}
    </Badge>
  );
}

function ServerDetail({
  server,
  onBack,
  onRemove,
  onToggle,
}: {
  server: InstalledServer;
  onBack: () => void;
  onRemove: (name: string) => Promise<void> | void;
  onToggle: (name: string, enabled: boolean) => Promise<void> | void;
}) {
  const [busy, setBusy] = useState<"toggle" | "remove" | null>(null);
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
        <StatusPill enabled={server.enabled} />
      </div>

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
