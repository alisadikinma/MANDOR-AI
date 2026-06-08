"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Loader2, Plug, Wifi } from "lucide-react";
import type { McpConnector, McpProbeServerResult } from "@multica/core/types";
import { api } from "@multica/core/api";
import { Button } from "@multica/ui/components/ui/button";
import { toast } from "sonner";
import { ConnectorDirectory } from "./connector-directory";
import { ConnectorConfigForm } from "./connector-config-form";
import { CustomConnectorEntry } from "./custom-connector-form";
import {
  InstalledConnectorList,
  extractInstalledServers,
  type InstalledServer,
} from "./installed-connector-list";
import { removeServerFromConfig, setServerEnabled } from "./config-mutations";

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
const PROBE_POLL_INTERVAL_MS = 1500;
const PROBE_POLL_ATTEMPTS = 30; // ~45s ceiling: covers the ≤15s heartbeat pickup + a 20s cold-npx probe

/**
 * The shared MCP-server management surface used by BOTH the per-agent tab and
 * the workspace-level settings tab (No-Duplication Rule): a "Browse connectors"
 * directory + schema-driven add form, the installed-server list with the
 * detail panel (enable/disable/remove), and a "Test connections" action that
 * asks the runtime to actually handshake each server and reports live status.
 * The caller owns persistence via `onSave(next)` and the probe trigger via
 * `onProbe()` (agent vs workspace endpoint), and supplies any read-only
 * `inheritedServers`.
 */
export function McpServerManager({
  wsId,
  config,
  onSave,
  onProbe,
  inheritedServers,
  savedToast,
  saveFailedToast,
}: {
  wsId: string;
  /** Current `mcp_config` value (possibly null). */
  config: unknown;
  /** Persists the next `mcp_config` value (or null to clear). */
  onSave: (next: unknown | null) => Promise<void>;
  /** Kicks off a connection probe; returns the pending request (or a
   *  runtime_offline status). Omit to hide the Test-connections action. */
  onProbe?: () => Promise<{ id?: string; status: string }>;
  inheritedServers?: InstalledServer[];
  savedToast: string;
  saveFailedToast: string;
}) {
  const [directoryOpen, setDirectoryOpen] = useState(false);
  const [selectedConnector, setSelectedConnector] =
    useState<McpConnector | null>(null);
  const [probing, setProbing] = useState(false);
  const [liveStatus, setLiveStatus] = useState<Record<
    string,
    McpProbeServerResult
  > | null>(null);
  const mounted = useRef(true);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const installedServers = useMemo(
    () => extractInstalledServers(config),
    [config],
  );

  // A config edit invalidates any prior probe result (the server set changed).
  useEffect(() => {
    setLiveStatus(null);
  }, [config]);

  const persist = async (next: unknown | null) => {
    try {
      await onSave(next);
      toast.success(savedToast);
    } catch (err) {
      toast.error(
        err instanceof Error && err.message ? err.message : saveFailedToast,
      );
    }
  };

  const handleProbe = async () => {
    if (!onProbe || probing) return;
    setProbing(true);
    setLiveStatus(null);
    try {
      const start = await onProbe();
      if (start.status === "runtime_offline" || !start.id) {
        toast.error("Runtime is offline — can't test connections right now.");
        return;
      }
      for (let i = 0; i < PROBE_POLL_ATTEMPTS; i++) {
        await sleep(PROBE_POLL_INTERVAL_MS);
        if (!mounted.current) return;
        const res = await api.getMcpProbe(start.id);
        if (res.status === "completed") {
          const map: Record<string, McpProbeServerResult> = {};
          for (const r of res.results ?? []) map[r.name] = r;
          if (mounted.current) setLiveStatus(map);
          return;
        }
        if (res.status === "timeout") {
          toast.error("Connection test timed out before the runtime responded.");
          return;
        }
      }
      toast.error("Connection test timed out.");
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : "Failed to test connections",
      );
    } finally {
      if (mounted.current) setProbing(false);
    }
  };

  return (
    <div className="flex h-full flex-col space-y-3">
      <div className="flex justify-end gap-2">
        {onProbe && installedServers.length > 0 && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => void handleProbe()}
            disabled={probing}
            className="shrink-0"
          >
            {probing ? (
              <Loader2 className="h-3 w-3 animate-spin" />
            ) : (
              <Wifi className="h-3 w-3" />
            )}
            Test connections
          </Button>
        )}
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => setDirectoryOpen(true)}
          className="shrink-0"
        >
          <Plug className="h-3 w-3" />
          Browse connectors
        </Button>
      </div>

      <ConnectorDirectory
        wsId={wsId}
        open={directoryOpen}
        onOpenChange={setDirectoryOpen}
        onSelect={(connector) => {
          setSelectedConnector(connector);
          setDirectoryOpen(false);
        }}
        customAction={<CustomConnectorEntry wsId={wsId} />}
      />

      {selectedConnector && (
        <ConnectorConfigForm
          connector={selectedConnector}
          currentConfig={config}
          open
          onOpenChange={(open) => {
            if (!open) setSelectedConnector(null);
          }}
          onSave={async ({ mcp_config }) => {
            await onSave(mcp_config);
          }}
        />
      )}

      <InstalledConnectorList
        servers={installedServers}
        inherited={inheritedServers}
        liveStatus={liveStatus}
        probing={probing}
        onRemove={(name) => persist(removeServerFromConfig(config, name))}
        onToggle={(name, enabled) =>
          persist(setServerEnabled(config, name, enabled))
        }
      />
    </div>
  );
}
