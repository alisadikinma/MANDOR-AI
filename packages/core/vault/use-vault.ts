/**
 * TanStack Query hooks for the read-only vault viewer. Every query keys on
 * `wsId` so switching workspace swaps the cache automatically (CLAUDE.md State
 * Management). Server data lives only in the Query cache — never copied into a
 * store.
 */

import { useQuery } from "@tanstack/react-query";
import { api } from "../api";

export const vaultKeys = {
  all: ["vault"] as const,
  status: (wsId: string) => ["vault", "status", wsId] as const,
  tree: (wsId: string) => ["vault", "tree", wsId] as const,
  note: (wsId: string, path: string) => ["vault", "note", wsId, path] as const,
  search: (wsId: string, q: string) => ["vault", "search", wsId, q] as const,
  graph: (wsId: string) => ["vault", "graph", wsId] as const,
};

export function useVaultStatus(wsId: string | undefined) {
  return useQuery({
    queryKey: wsId ? vaultKeys.status(wsId) : ["vault", "status", "disabled"],
    queryFn: () => api.getVaultStatus(wsId!),
    enabled: !!wsId,
  });
}

export function useVaultTree(wsId: string | undefined) {
  return useQuery({
    queryKey: wsId ? vaultKeys.tree(wsId) : ["vault", "tree", "disabled"],
    queryFn: () => api.getVaultTree(wsId!),
    enabled: !!wsId,
  });
}

export function useVaultNote(wsId: string | undefined, path: string | undefined) {
  return useQuery({
    queryKey: wsId && path ? vaultKeys.note(wsId, path) : ["vault", "note", "disabled"],
    queryFn: () => api.getVaultNote(wsId!, path!),
    enabled: !!wsId && !!path,
  });
}

export function useVaultSearch(wsId: string | undefined, q: string) {
  const query = q.trim();
  return useQuery({
    queryKey: wsId ? vaultKeys.search(wsId, query) : ["vault", "search", "disabled"],
    queryFn: () => api.searchVault(wsId!, query),
    enabled: !!wsId && query.length > 0,
  });
}

export function useVaultGraph(wsId: string | undefined) {
  return useQuery({
    queryKey: wsId ? vaultKeys.graph(wsId) : ["vault", "graph", "disabled"],
    queryFn: () => api.getVaultGraph(wsId!),
    enabled: !!wsId,
  });
}
