"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2, Plug, Wifi } from "lucide-react";
import { toast } from "sonner";
import type { McpProbeServerResult, McpServerInfo } from "@multica/core/types";
import { api } from "@multica/core/api";
import { runtimeMcpOptions } from "@multica/core/runtimes";
import { Button } from "@multica/ui/components/ui/button";

const POLL_INTERVAL_MS = 1500;
const POLL_ATTEMPTS = 30; // ~45s: ≤15s heartbeat pickup + a 20s cold-npx probe
const OAUTH_TIMEOUT_MS = 5 * 60 * 1000;

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

// Waits for the OAuth popup to post back from OUR origin. Resolves false if the
// user closes it or the flow exceeds the TTL.
function waitForOAuthResult(popup: Window | null): Promise<boolean> {
  return new Promise((resolve) => {
    const expected = window.location.origin;
    let done = false;
    const finish = (ok: boolean) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      clearInterval(poll);
      window.removeEventListener("message", onMsg);
      resolve(ok);
    };
    const onMsg = (e: MessageEvent) => {
      if (e.origin !== expected) return;
      const d = e.data as { type?: string; success?: boolean } | null;
      if (d?.type === "mcp-oauth-result") finish(d.success === true);
    };
    window.addEventListener("message", onMsg);
    const poll = setInterval(() => {
      if (popup?.closed) finish(false);
    }, 800);
    const timer = setTimeout(() => finish(false), OAUTH_TIMEOUT_MS);
  });
}

/**
 * Read-only mirror of the runtime host's MCP pool (the daemon reports it on its
 * heartbeat) plus a "Test connections" action and per-server "Authenticate" for
 * remote servers. Server definitions live on the host — this never edits config;
 * agents on the runtime reuse this pool.
 */
export function RuntimeMcpPanel({ runtimeId }: { runtimeId: string }) {
  const { data, isLoading, refetch } = useQuery(runtimeMcpOptions(runtimeId));
  const [testing, setTesting] = useState(false);
  // Live status overrides the persisted probe_results while a test runs.
  const [live, setLive] = useState<Record<string, McpProbeServerResult> | null>(
    null,
  );
  const [authing, setAuthing] = useState<string | null>(null);

  const servers = data?.servers ?? [];
  const statusByName: Record<string, McpProbeServerResult> =
    live ?? Object.fromEntries((data?.probe_results ?? []).map((r) => [r.name, r]));

  async function runProbe(): Promise<Record<string, McpProbeServerResult> | null> {
    const started = await api.probeRuntimeMcp(runtimeId);
    if (started.status === "runtime_offline") {
      toast.error("Runtime is offline — can't test connections.");
      return null;
    }
    if (!started.id) {
      toast.error("Couldn't start the connection test.");
      return null;
    }
    for (let i = 0; i < POLL_ATTEMPTS; i++) {
      await sleep(POLL_INTERVAL_MS);
      const r = await api.getMcpProbe(started.id);
      if (r.status === "completed" || r.status === "timeout") {
        const map = Object.fromEntries(
          (r.results ?? []).map((x) => [x.name, x]),
        );
        return map;
      }
    }
    return {};
  }

  async function onTest() {
    setTesting(true);
    try {
      const map = await runProbe();
      if (map) {
        setLive(map);
        void refetch();
      }
    } finally {
      setTesting(false);
    }
  }

  async function onAuthenticate(server: McpServerInfo) {
    setAuthing(server.name);
    // Open the popup synchronously so the browser doesn't block it.
    const popup = window.open("", "_blank", "width=520,height=680");
    try {
      const { authorize_url } = await api.startMcpOauth(
        runtimeId,
        server.name,
        window.location.origin,
      );
      if (!authorize_url) {
        popup?.close();
        toast.error("Couldn't start sign-in for this server.");
        return;
      }
      if (popup) popup.location.href = authorize_url;
      const ok = await waitForOAuthResult(popup);
      if (ok) {
        toast.success(`Connected ${server.name}.`);
        const map = await runProbe();
        if (map) {
          setLive(map);
          void refetch();
        }
      } else {
        toast.error("Sign-in was not completed.");
      }
    } finally {
      setAuthing(null);
    }
  }

  return (
    <div className="rounded-lg border bg-card">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex items-center gap-2">
          <Plug className="h-4 w-4 text-muted-foreground" />
          <h3 className="text-sm font-medium">MCP servers</h3>
          <span className="text-xs text-muted-foreground">
            configured on this runtime
          </span>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={onTest}
          disabled={testing || servers.length === 0}
        >
          {testing ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Wifi className="h-3.5 w-3.5" />
          )}
          Test connections
        </Button>
      </div>

      {isLoading ? (
        <div className="p-4 text-xs text-muted-foreground">Loading…</div>
      ) : servers.length === 0 ? (
        <div className="p-4 text-xs text-muted-foreground">
          This runtime reports no MCP servers. Configure them on the runtime host
          (e.g. <code>~/.claude.json</code> / <code>~/.codex/config.toml</code>);
          they appear here automatically.
        </div>
      ) : (
        <ul className="divide-y">
          {servers.map((s) => {
            const status = statusByName[s.name];
            const isRemote = s.transport === "http";
            return (
              <li
                key={s.name}
                className="flex items-center justify-between gap-3 px-4 py-2.5"
              >
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium">
                      {s.name}
                    </span>
                    <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase text-muted-foreground">
                      {s.transport}
                    </span>
                  </div>
                  {s.url ? (
                    <p className="truncate text-xs text-muted-foreground">
                      {s.url}
                    </p>
                  ) : null}
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <StatusPill status={status} />
                  {isRemote && status?.status === "needs_auth" ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => onAuthenticate(s)}
                      disabled={authing === s.name}
                    >
                      {authing === s.name ? (
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                      ) : null}
                      Authenticate
                    </Button>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function StatusPill({ status }: { status?: McpProbeServerResult }) {
  if (!status) {
    return <span className="text-xs text-muted-foreground">not tested</span>;
  }
  // Unknown server-driven status downgrades to a neutral label, never crashes.
  const map: Record<string, { label: string; cls: string }> = {
    connected: {
      label:
        status.tool_count > 0
          ? `${status.tool_count} tools`
          : "connected",
      cls: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
    },
    needs_auth: {
      label: "needs sign-in",
      cls: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
    },
    failed: { label: "failed", cls: "bg-red-500/15 text-red-600 dark:text-red-400" },
    skipped: { label: "skipped", cls: "bg-muted text-muted-foreground" },
  };
  const v = map[status.status] ?? {
    label: status.status,
    cls: "bg-muted text-muted-foreground",
  };
  return (
    <span className={`rounded px-2 py-0.5 text-xs font-medium ${v.cls}`}>
      {v.label}
    </span>
  );
}
