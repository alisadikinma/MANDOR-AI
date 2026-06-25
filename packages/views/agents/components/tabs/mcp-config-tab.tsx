"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import type { Agent } from "@multica/core/types";
import { runtimeMcpOptions } from "@multica/core/runtimes";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { useT } from "../../../i18n";

// The agent's mcp_config is a deny-list over its runtime's pool:
// `{ disabledMcpServers: string[] }` (absent = inherit every server).
function parseDisabled(config: unknown): string[] {
  if (config && typeof config === "object" && "disabledMcpServers" in config) {
    const v = (config as { disabledMcpServers?: unknown }).disabledMcpServers;
    if (Array.isArray(v)) return v.filter((x): x is string => typeof x === "string");
  }
  return [];
}

/**
 * MCP servers are configured on the runtime now; an agent only picks which of
 * its runtime's servers it uses. This tab is a checklist over the runtime's
 * reported pool — no server definitions, no per-agent connect/auth. Toggling a
 * server writes the agent's deny-list.
 */
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
  const redacted = agent.mcp_config_redacted === true;

  const { data, isLoading } = useQuery({
    ...runtimeMcpOptions(agent.runtime_id),
    enabled: !!agent.runtime_id && !redacted,
  });

  const [disabled, setDisabled] = useState<string[]>(() =>
    parseDisabled(agent.mcp_config),
  );
  const [saving, setSaving] = useState<string | null>(null);

  // Re-sync if the agent's stored deny-list changes underneath us.
  useEffect(() => {
    setDisabled(parseDisabled(agent.mcp_config));
  }, [agent.mcp_config]);

  // No dirty editor here — every toggle persists immediately.
  useEffect(() => {
    onDirtyChange?.(false);
  }, [onDirtyChange]);

  const servers = useMemo(() => data?.servers ?? [], [data]);

  async function toggle(name: string, enabled: boolean) {
    const next = enabled
      ? disabled.filter((n) => n !== name)
      : [...disabled, name];
    setDisabled(next);
    setSaving(name);
    try {
      await onSave({
        mcp_config: next.length ? { disabledMcpServers: next } : null,
      });
    } catch {
      // Roll back on failure so the checkbox reflects the persisted truth.
      setDisabled(disabled);
    } finally {
      setSaving(null);
    }
  }

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

      {isLoading ? (
        <p className="text-xs text-muted-foreground">Loading…</p>
      ) : servers.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          This agent&apos;s runtime reports no MCP servers. Configure them on the
          runtime host; they appear here automatically.
        </p>
      ) : (
        <ul className="divide-y rounded-md border">
          {servers.map((s) => {
            const enabled = !disabled.includes(s.name);
            return (
              <li
                key={s.name}
                className="flex items-center gap-3 px-3 py-2.5"
              >
                <Checkbox
                  checked={enabled}
                  disabled={saving === s.name}
                  onCheckedChange={(v) => toggle(s.name, v === true)}
                />
                <div className="min-w-0 flex-1">
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
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
