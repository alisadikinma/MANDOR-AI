"use client";

import { useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { memberListOptions } from "@multica/core/workspace/queries";
import {
  workspaceMcpConfigOptions,
  workspaceMcpConfigKeys,
} from "@multica/core/agents";
import { api } from "@multica/core/api";
import { McpServerManager } from "../../agents/components/mcp/mcp-server-manager";
import { useT } from "../../i18n";

/**
 * Workspace-level MCP servers. These are inherited by every agent in the
 * workspace (the agent's own config overrides by server name), so they live in
 * workspace settings rather than per-agent. Owner/admin only — the value
 * carries secrets and the endpoint 403s for everyone else, so non-admins get a
 * read-only notice instead of the manager. Reuses the same McpServerManager as
 * the agent tab (No-Duplication Rule); the difference is purely the persistence
 * target (workspace mcp_config vs agent mcp_config).
 */
export function McpTab() {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const user = useAuthStore((s) => s.user);
  const qc = useQueryClient();

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const isAdmin = useMemo(() => {
    const member = members.find((m) => m.user_id === user?.id) ?? null;
    return member?.role === "owner" || member?.role === "admin";
  }, [members, user?.id]);

  const { data: config } = useQuery({
    ...workspaceMcpConfigOptions(wsId),
    enabled: !!wsId && isAdmin,
  });

  // Optimistic by default (architecture rule): apply locally, roll back on
  // failure, invalidate on settle. The query cache stays the single source of
  // truth — no copy into a store.
  const mutation = useMutation({
    mutationFn: (next: unknown | null) =>
      api.updateWorkspaceMcpConfig(wsId, next),
    onMutate: async (next) => {
      await qc.cancelQueries({ queryKey: workspaceMcpConfigKeys.all(wsId) });
      const prev = qc.getQueryData(workspaceMcpConfigKeys.all(wsId));
      qc.setQueryData(workspaceMcpConfigKeys.all(wsId), next);
      return { prev };
    },
    onError: (_err, _next, ctx) => {
      if (ctx) qc.setQueryData(workspaceMcpConfigKeys.all(wsId), ctx.prev);
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: workspaceMcpConfigKeys.all(wsId) });
    },
  });

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h2 className="text-sm font-semibold">{t(($) => $.mcp.title)}</h2>
        <p className="text-xs text-muted-foreground">
          {t(($) => $.mcp.description)}
        </p>
      </div>

      {isAdmin ? (
        <McpServerManager
          wsId={wsId}
          config={config}
          onSave={async (next) => {
            await mutation.mutateAsync(next);
          }}
          savedToast={t(($) => $.mcp.saved_toast)}
          saveFailedToast={t(($) => $.mcp.save_failed_toast)}
        />
      ) : (
        <p className="flex items-center gap-2 text-xs text-muted-foreground">
          <Lock className="h-3.5 w-3.5" />
          {t(($) => $.mcp.admin_only)}
        </p>
      )}
    </div>
  );
}
