"use client";

import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import type { Agent } from "@multica/core/types";
import { workspaceMcpConfigOptions } from "@multica/core/agents";
import { useT } from "../../../i18n";
import { McpServerManager } from "../mcp/mcp-server-manager";
import {
  extractInstalledServers,
  type InstalledServer,
} from "../mcp/installed-connector-list";

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

  const redacted = agent.mcp_config_redacted === true;

  // Workspace-level servers every agent inherits. Owner/admin-gated; for a
  // redacted (non-privileged) view we skip the fetch entirely. The agent's own
  // servers override inherited ones by name, so drop any inherited entry the
  // agent also defines — it shows (editable) in the agent group instead.
  const { data: workspaceConfig } = useQuery({
    ...workspaceMcpConfigOptions(wsId),
    enabled: !!wsId && !redacted,
  });

  const inheritedServers = useMemo<InstalledServer[]>(() => {
    const own = new Set(
      extractInstalledServers(agent.mcp_config).map((s) => s.name),
    );
    return extractInstalledServers(workspaceConfig).filter(
      (s) => !own.has(s.name),
    );
  }, [workspaceConfig, agent.mcp_config]);

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

  return (
    <div className="flex h-full flex-col space-y-3">
      <p className="text-xs text-muted-foreground">
        {t(($) => $.tab_body.mcp_config.intro)}
      </p>
      <McpServerManager
        wsId={wsId}
        config={agent.mcp_config}
        onSave={(next) => onSave({ mcp_config: next })}
        inheritedServers={inheritedServers}
        savedToast={t(($) => $.tab_body.mcp_config.saved_toast)}
        saveFailedToast={t(($) => $.tab_body.mcp_config.save_failed_toast)}
      />
    </div>
  );
}
