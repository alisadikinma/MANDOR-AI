"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plus, Search, Plug } from "lucide-react";
import type { McpConnector } from "@multica/core/types";
import { mcpConnectorsOptions } from "@multica/core/agents";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";

type SortMode = "popularity" | "name";

/**
 * MCP connector directory modal. Browses the backend-driven catalog (global
 * curated connectors UNION workspace-custom ones) and lets an admin add one
 * to the agent. Data comes from TanStack Query keyed on `wsId` — never copied
 * into a store. The directory only renders cards + search/sort; the
 * per-connector add flow (schema-driven form) is owned by the caller via
 * `onSelect`, and the "Add custom connector" affordance by `customAction`.
 */
export function ConnectorDirectory({
  wsId,
  open,
  onOpenChange,
  onSelect,
  customAction,
}: {
  wsId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called when the user clicks the `+` on a connector card. */
  onSelect: (connector: McpConnector) => void;
  /** Optional admin-only "Add custom connector" control, rendered in the
   *  header. The directory itself stays role-agnostic. */
  customAction?: React.ReactNode;
}) {
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<SortMode>("popularity");

  const { data: connectors = [], isLoading } = useQuery({
    ...mcpConnectorsOptions(wsId),
    enabled: !!wsId && open,
  });

  const visible = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const filtered = needle
      ? connectors.filter(
          (c) =>
            c.name.toLowerCase().includes(needle) ||
            c.slug.toLowerCase().includes(needle) ||
            (c.description ?? "").toLowerCase().includes(needle),
        )
      : connectors;
    // Copy before sort — never mutate the query cache array in place.
    return [...filtered].sort((a, b) => {
      if (sort === "name") return a.name.localeCompare(b.name);
      // popularity desc, name asc as a stable tiebreaker (mirrors the server).
      if (b.popularity !== a.popularity) return b.popularity - a.popularity;
      return a.name.localeCompare(b.name);
    });
  }, [connectors, query, sort]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[80vh] w-full max-w-2xl flex-col gap-4">
        <DialogHeader>
          <DialogTitle>Connector directory</DialogTitle>
          <DialogDescription>
            Add a Model Context Protocol server to this agent. Custom values
            you enter are merged into the agent&apos;s existing config.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search connectors"
              aria-label="Search connectors"
              className="pl-8"
            />
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() =>
              setSort((s) => (s === "popularity" ? "name" : "popularity"))
            }
            className="shrink-0"
          >
            Sort: {sort === "popularity" ? "Popular" : "Name"}
          </Button>
          {customAction}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto">
          {isLoading ? (
            <p className="px-1 py-8 text-center text-xs text-muted-foreground">
              Loading connectors…
            </p>
          ) : visible.length === 0 ? (
            <div className="flex flex-col items-center gap-2 px-1 py-10 text-center">
              <Plug className="h-6 w-6 text-muted-foreground" />
              <p className="text-sm font-medium">No connectors found</p>
              <p className="text-xs text-muted-foreground">
                {query.trim()
                  ? "Try a different search term."
                  : "No connectors are available in this workspace yet."}
              </p>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-2 p-1 sm:grid-cols-2">
              {visible.map((c) => (
                <ConnectorCard
                  key={c.id}
                  connector={c}
                  onAdd={() => onSelect(c)}
                />
              ))}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function ConnectorCard({
  connector,
  onAdd,
}: {
  connector: McpConnector;
  onAdd: () => void;
}) {
  return (
    <div className="flex items-start gap-3 rounded-lg border p-3">
      <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-muted/40 text-muted-foreground">
        <Plug className="h-4 w-4" />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1.5">
          <h3 className="truncate text-sm font-medium">{connector.name}</h3>
          {connector.is_custom && (
            <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
              Custom
            </span>
          )}
        </div>
        {connector.description && (
          <p className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
            {connector.description}
          </p>
        )}
      </div>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        className="h-7 w-7 shrink-0"
        onClick={onAdd}
        aria-label={`Add ${connector.name}`}
      >
        <Plus className="h-4 w-4" />
      </Button>
    </div>
  );
}
