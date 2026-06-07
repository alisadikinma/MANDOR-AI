"use client";

import { useMemo, useState } from "react";
import { Plug } from "lucide-react";
import type { McpConnector } from "@multica/core/types";
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

/**
 * The shared MCP-server management surface used by BOTH the per-agent tab and
 * the workspace-level settings tab (No-Duplication Rule): a "Browse connectors"
 * directory + schema-driven add form, plus the installed-server list with the
 * detail panel (enable/disable/remove). The caller owns persistence via
 * `onSave(next)` — the agent tab routes it to the agent-save path, the
 * workspace tab to the workspace mcp_config endpoint — and supplies any
 * read-only `inheritedServers` to render as a separate group.
 */
export function McpServerManager({
  wsId,
  config,
  onSave,
  inheritedServers,
  savedToast,
  saveFailedToast,
}: {
  wsId: string;
  /** Current `mcp_config` value (possibly null). */
  config: unknown;
  /** Persists the next `mcp_config` value (or null to clear). */
  onSave: (next: unknown | null) => Promise<void>;
  /**
   * Servers inherited from a higher scope (e.g. workspace servers shown in the
   * agent tab). Rendered read-only above the editable list. Omit for the
   * workspace tab, which has no parent scope.
   */
  inheritedServers?: InstalledServer[];
  savedToast: string;
  saveFailedToast: string;
}) {
  const [directoryOpen, setDirectoryOpen] = useState(false);
  const [selectedConnector, setSelectedConnector] =
    useState<McpConnector | null>(null);

  const installedServers = useMemo(
    () => extractInstalledServers(config),
    [config],
  );

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

  return (
    <div className="flex h-full flex-col space-y-3">
      <div className="flex justify-end">
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
          // ConnectorConfigForm computes the merged config and hands it back;
          // we forward it to the caller's save path (no toast — the form just
          // closes, matching prior behaviour).
          onSave={async ({ mcp_config }) => {
            await onSave(mcp_config);
          }}
        />
      )}

      <InstalledConnectorList
        servers={installedServers}
        inherited={inheritedServers}
        onRemove={(name) => persist(removeServerFromConfig(config, name))}
        onToggle={(name, enabled) =>
          persist(setServerEnabled(config, name, enabled))
        }
      />
    </div>
  );
}
