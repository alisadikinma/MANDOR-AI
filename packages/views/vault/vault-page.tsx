"use client";

import { useCallback, useMemo, useState } from "react";
import { Search, BookOpen, Network, List } from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import { useCurrentWorkspace } from "@multica/core/paths";
import { api } from "@multica/core/api";
import {
  useVaultTree,
  useVaultNote,
  useVaultSearch,
  useVaultGraph,
  transformWikilinks,
  rewriteEmbeds,
  type VaultTreeNode,
} from "@multica/core/vault";
import { ReadonlyContent } from "../editor";
import { VaultTree } from "./vault-tree";
import { NoteMeta } from "./note-meta";
import { VaultGraphView } from "./vault-graph";

// Flatten every file node into a name→path index so [[wikilinks]] resolve. Both
// the full relative path and the basename (sans .md) are keyed; basenames win on
// the common case, full paths disambiguate.
function buildResolver(nodes: VaultTreeNode[]): (name: string) => string | null {
  const byBase = new Map<string, string>();
  const byPath = new Map<string, string>();
  const walk = (list: VaultTreeNode[]) => {
    for (const n of list) {
      if (n.type === "dir") {
        if (n.children) walk(n.children);
        continue;
      }
      byPath.set(n.path.toLowerCase(), n.path);
      const base = n.name.toLowerCase().replace(/\.md$/, "");
      if (!byBase.has(base)) byBase.set(base, n.path);
    }
  };
  walk(nodes);
  return (name: string) => {
    const key = name.toLowerCase().trim();
    return byBase.get(key) ?? byPath.get(key) ?? byPath.get(`${key}.md`) ?? null;
  };
}

export function VaultPage() {
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id;

  const [selectedPath, setSelectedPath] = useState<string | undefined>(undefined);
  const [query, setQuery] = useState("");
  // Graph is the default landing view; opening a node switches to the reader.
  const [view, setView] = useState<"graph" | "files">("graph");

  const tree = useVaultTree(wsId);
  const note = useVaultNote(wsId, selectedPath);
  const search = useVaultSearch(wsId, query);
  const graph = useVaultGraph(wsId);

  const openNote = useCallback((path: string) => {
    setSelectedPath(path);
    setView("files");
  }, []);

  const resolve = useMemo(() => buildResolver(tree.data ?? []), [tree.data]);

  const renderedBody = useMemo(() => {
    if (!note.data || !wsId) return "";
    const withEmbeds = rewriteEmbeds(note.data.body, (target) => api.vaultFileUrl(wsId, target));
    return transformWikilinks(withEmbeds, resolve);
  }, [note.data, wsId, resolve]);

  // Intercept clicks on [[wikilink]] anchors (href "vault:<path>") in capture
  // phase — before ReadonlyContent's own onClick (which would window.open the
  // custom scheme) — and select that note instead.
  const onContentClickCapture = useCallback((e: React.MouseEvent) => {
    const anchor = (e.target as HTMLElement).closest("a");
    const href = anchor?.getAttribute("href");
    if (href && href.startsWith("vault:")) {
      e.preventDefault();
      e.stopPropagation();
      setSelectedPath(href.slice("vault:".length));
    }
  }, []);

  const searching = query.trim().length > 0;

  return (
    <div className="flex h-full min-h-0 flex-col bg-background">
      {/* View toggle: graph (default) ↔ files */}
      <div className="flex shrink-0 items-center gap-1 border-b border-border p-2">
        <ViewToggle active={view === "graph"} onClick={() => setView("graph")} icon={Network} label="Graph" />
        <ViewToggle active={view === "files"} onClick={() => setView("files")} icon={List} label="Files" />
      </div>

      {view === "graph" ? (
        <div className="min-h-0 flex-1">
          {graph.isPending ? (
            <p className="p-6 text-sm text-muted-foreground">Loading graph…</p>
          ) : (graph.data?.nodes.length ?? 0) === 0 ? (
            <EmptyState label="This vault has no notes yet." />
          ) : (
            <VaultGraphView graph={graph.data!} onSelect={openNote} />
          )}
        </div>
      ) : (
        <div className="flex min-h-0 flex-1">
      {/* Left: search + tree */}
      <aside className="flex w-72 shrink-0 flex-col border-r border-border">
        <div className="border-b border-border p-2">
          <div className="relative">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search vault…"
              className="w-full rounded-md border border-border bg-background py-1.5 pl-8 pr-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {searching ? (
            <SearchResults
              isPending={search.isPending}
              results={search.data ?? []}
              selectedPath={selectedPath}
              onSelect={setSelectedPath}
            />
          ) : tree.isPending ? (
            <p className="px-2 py-1 text-sm text-muted-foreground">Loading…</p>
          ) : (tree.data ?? []).length === 0 ? (
            <p className="px-2 py-1 text-sm text-muted-foreground">This vault is empty.</p>
          ) : (
            <VaultTree nodes={tree.data ?? []} selectedPath={selectedPath} onSelect={setSelectedPath} />
          )}
        </div>
      </aside>

      {/* Right: note content */}
      <main className="min-h-0 min-w-0 flex-1 overflow-y-auto">
        {!selectedPath ? (
          <EmptyState />
        ) : note.isPending ? (
          <p className="p-6 text-sm text-muted-foreground">Loading note…</p>
        ) : (
          <div className="mx-auto max-w-3xl p-6" onClickCapture={onContentClickCapture}>
            <NoteMeta frontmatter={note.data?.frontmatter ?? {}} />
            <ReadonlyContent content={renderedBody} />
          </div>
        )}
      </main>
        </div>
      )}
    </div>
  );
}

function ViewToggle({
  active,
  onClick,
  icon: Icon,
  label,
}: {
  active: boolean;
  onClick: () => void;
  icon: typeof Network;
  label: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm",
        active
          ? "bg-accent text-accent-foreground"
          : "text-muted-foreground hover:bg-accent/60",
      )}
    >
      <Icon className="size-3.5" />
      {label}
    </button>
  );
}

function SearchResults({
  isPending,
  results,
  selectedPath,
  onSelect,
}: {
  isPending: boolean;
  results: { name: string; path: string; snippet: string }[];
  selectedPath: string | undefined;
  onSelect: (path: string) => void;
}) {
  if (isPending) return <p className="px-2 py-1 text-sm text-muted-foreground">Searching…</p>;
  if (results.length === 0)
    return <p className="px-2 py-1 text-sm text-muted-foreground">No matches.</p>;
  return (
    <ul className="space-y-0.5">
      {results.map((r) => (
        <li key={r.path}>
          <button
            type="button"
            onClick={() => onSelect(r.path)}
            className={cn(
              "w-full rounded-md px-2 py-1.5 text-left",
              r.path === selectedPath ? "bg-accent text-accent-foreground" : "hover:bg-accent/60",
            )}
          >
            <span className="block truncate text-sm text-foreground">
              {r.name.replace(/\.md$/, "")}
            </span>
            {r.snippet && (
              <span className="mt-0.5 block truncate text-xs text-muted-foreground">{r.snippet}</span>
            )}
          </button>
        </li>
      ))}
    </ul>
  );
}

function EmptyState({ label = "Select a note to read." }: { label?: string }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
      <BookOpen className="size-8 opacity-40" />
      <p className="text-sm">{label}</p>
    </div>
  );
}
