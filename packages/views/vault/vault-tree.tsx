"use client";

import { useState } from "react";
import { ChevronRight, FileText } from "lucide-react";
import { cn } from "@multica/ui/lib/utils";
import type { VaultTreeNode } from "@multica/core/vault";

// Strip the .md extension for display (Obsidian-style); other files keep theirs.
function displayName(name: string): string {
  return name.toLowerCase().endsWith(".md") ? name.slice(0, -3) : name;
}

interface TreeProps {
  nodes: VaultTreeNode[];
  selectedPath: string | undefined;
  onSelect: (path: string) => void;
}

export function VaultTree({ nodes, selectedPath, onSelect }: TreeProps) {
  return (
    <ul className="space-y-0.5">
      {nodes.map((node) => (
        <TreeItem key={node.path} node={node} selectedPath={selectedPath} onSelect={onSelect} depth={0} />
      ))}
    </ul>
  );
}

function TreeItem({
  node,
  selectedPath,
  onSelect,
  depth,
}: {
  node: VaultTreeNode;
  selectedPath: string | undefined;
  onSelect: (path: string) => void;
  depth: number;
}) {
  const [open, setOpen] = useState(true);
  const indent = { paddingLeft: `${depth * 12 + 8}px` };

  if (node.type === "dir") {
    return (
      <li>
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          style={indent}
          className="flex w-full items-center gap-1.5 rounded-md py-1 pr-2 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
        >
          <ChevronRight className={cn("size-3.5 shrink-0 transition-transform", open && "rotate-90")} />
          <span className="min-w-0 flex-1 truncate text-left">{node.name}</span>
        </button>
        {open && node.children && node.children.length > 0 && (
          <ul className="space-y-0.5">
            {node.children.map((child) => (
              <TreeItem
                key={child.path}
                node={child}
                selectedPath={selectedPath}
                onSelect={onSelect}
                depth={depth + 1}
              />
            ))}
          </ul>
        )}
      </li>
    );
  }

  const active = node.path === selectedPath;
  return (
    <li>
      <button
        type="button"
        onClick={() => onSelect(node.path)}
        style={indent}
        className={cn(
          "flex w-full items-center gap-1.5 rounded-md py-1 pr-2 text-sm",
          active
            ? "bg-accent text-accent-foreground"
            : "text-muted-foreground hover:bg-accent/60 hover:text-accent-foreground",
        )}
      >
        <FileText className="size-3.5 shrink-0 opacity-70" />
        <span className="min-w-0 flex-1 truncate text-left">{displayName(node.name)}</span>
      </button>
    </li>
  );
}
