"use client";

import { useState } from "react";
import { Loader2, Plug, Trash2 } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

export interface InstalledServer {
  name: string;
  /** One-line, secret-free hint about how the server is launched. */
  summary: string;
}

/**
 * Builds a compact, secret-free summary for one `mcpServers` entry: the launch
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
 * Extracts the agent's currently-configured MCP servers from a stored
 * `mcp_config` value. Tolerates the value being `null`, a non-object, or
 * missing `mcpServers` — every malformed shape collapses to an empty list
 * rather than throwing (the raw JSON editor remains the escape hatch).
 */
export function extractInstalledServers(config: unknown): InstalledServer[] {
  if (!isPlainObject(config)) return [];
  const servers = config.mcpServers;
  if (!isPlainObject(servers)) return [];
  return Object.entries(servers).map(([name, entry]) => ({
    name,
    summary: summarizeServer(entry),
  }));
}

/**
 * The Claude-Desktop-style list of installed MCP servers: one row per
 * `mcpServers` entry with an inline-confirmed remove. Removal is delegated to
 * the parent (which mutates `mcp_config` and persists through the agent-save
 * path); this component owns only the per-row confirm + pending state.
 */
export function InstalledConnectorList({
  servers,
  onRemove,
}: {
  servers: InstalledServer[];
  onRemove: (name: string) => Promise<void> | void;
}) {
  const [confirming, setConfirming] = useState<string | null>(null);
  const [removing, setRemoving] = useState<string | null>(null);

  if (servers.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">
        No connectors yet. Use “Browse connectors” to add one.
      </p>
    );
  }

  const handleRemove = async (name: string) => {
    setRemoving(name);
    try {
      await onRemove(name);
    } finally {
      setRemoving(null);
      setConfirming(null);
    }
  };

  return (
    <ul className="divide-y rounded-md border" aria-label="Installed connectors">
      {servers.map((server) => {
        const isConfirming = confirming === server.name;
        const isRemoving = removing === server.name;
        return (
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
            {isConfirming ? (
              <div className="flex shrink-0 items-center gap-1.5">
                <Button
                  type="button"
                  variant="destructive"
                  size="sm"
                  disabled={isRemoving}
                  onClick={() => handleRemove(server.name)}
                >
                  {isRemoving ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : null}
                  Remove
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={isRemoving}
                  onClick={() => setConfirming(null)}
                >
                  Cancel
                </Button>
              </div>
            ) : (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="shrink-0 text-muted-foreground hover:text-destructive"
                aria-label={`Remove ${server.name}`}
                onClick={() => setConfirming(server.name)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            )}
          </li>
        );
      })}
    </ul>
  );
}
