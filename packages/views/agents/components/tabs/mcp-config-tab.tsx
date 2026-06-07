"use client";

import { useEffect, useMemo, useState } from "react";
import { Lock, Plug } from "lucide-react";
import type { Agent, McpConnector } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { toast } from "sonner";
import { useT } from "../../../i18n";
import { ConnectorDirectory } from "../mcp/connector-directory";
import { ConnectorConfigForm } from "../mcp/connector-config-form";
import { CustomConnectorEntry } from "../mcp/custom-connector-form";
import {
  InstalledConnectorList,
  extractInstalledServers,
} from "../mcp/installed-connector-list";

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

/**
 * Rebuilds an `mcp_config` object from the leftover top-level keys plus the
 * active (`mcpServers`) and disabled (`disabledMcpServers`) server maps. Empty
 * maps are dropped, and a config with nothing left collapses to `null` so the
 * daemon falls back to the CLI default rather than persisting an empty husk.
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
function removeServerFromConfig(config: unknown, name: string): unknown | null {
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
function setServerEnabled(
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

export function McpConfigTab({
  agent,
  onSave,
  onDirtyChange,
}: {
  agent: Agent;
  onSave: (updates: { mcp_config: unknown | null }) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const { t } = useT("agents");
  // The agent always carries its own workspace id; reading it from the prop
  // avoids depending on `WorkspaceIdProvider` context here (rule: workspace-
  // scoped data takes wsId explicitly, not via the provider hook).
  const wsId = agent.workspace_id;

  // Connector directory + schema-driven add flow. Picking a connector in the
  // directory selects it, which opens the per-connector config form; the form
  // merges into the agent's current `mcp_config` and saves through `onSave`.
  // Removal goes through the same path. Both add and remove persist
  // immediately, so this tab has no draft state and is never "dirty".
  const [directoryOpen, setDirectoryOpen] = useState(false);
  const [selectedConnector, setSelectedConnector] =
    useState<McpConnector | null>(null);

  const redacted = agent.mcp_config_redacted === true;
  const installedServers = useMemo(
    () => extractInstalledServers(agent.mcp_config),
    [agent.mcp_config],
  );

  // No raw editor → nothing to lose on tab switch. Keep the parent's dirty
  // tracker pinned to false so it never raises the discard dialog for MCP.
  useEffect(() => {
    onDirtyChange?.(false);
  }, [onDirtyChange]);

  if (redacted) {
    return (
      <div className="space-y-3">
        <p className="flex items-center gap-2 text-sm font-medium">
          <Lock className="h-3.5 w-3.5 text-muted-foreground" />
          {t(($) => $.tab_body.mcp_config.redacted_title)}
        </p>
        <p className="text-xs text-muted-foreground">
          {t(($) => $.tab_body.mcp_config.redacted_hint)}
        </p>
      </div>
    );
  }

  const persist = async (next: unknown | null) => {
    try {
      await onSave({ mcp_config: next });
      toast.success(t(($) => $.tab_body.mcp_config.saved_toast));
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : t(($) => $.tab_body.mcp_config.save_failed_toast),
      );
    }
  };

  const handleRemoveServer = (name: string) =>
    persist(removeServerFromConfig(agent.mcp_config, name));

  const handleToggleServer = (name: string, enabled: boolean) =>
    persist(setServerEnabled(agent.mcp_config, name, enabled));

  return (
    <div className="flex h-full flex-col space-y-3">
      <div className="flex items-start justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {t(($) => $.tab_body.mcp_config.intro)}
        </p>
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
          currentConfig={agent.mcp_config}
          open
          onOpenChange={(open) => {
            if (!open) setSelectedConnector(null);
          }}
          onSave={onSave}
        />
      )}

      <InstalledConnectorList
        servers={installedServers}
        onRemove={handleRemoveServer}
        onToggle={handleToggleServer}
      />
    </div>
  );
}
