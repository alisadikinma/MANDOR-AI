# MCP "Test connections" — npx cold-start timeout fix

## Design

### Problem (root-caused via live verification, 2026-06-08)

"Test connections gagal terus" had a **chain of causes, none of them the MCP config**:

1. **Migration 119 (`workspace_mcp_config`) not applied** → `workspace.mcp_config` column
   missing → `ListWorkspaces` (which SELECTs `w.mcp_config`) → `/api/workspaces` 500 →
   daemon can't list workspaces → daemon exits → no online runtime → probe returns
   `runtime_offline` → toast "can't test right now". **Fixed** with `make migrate-up`.
2. **`local` CLI profile not authenticated** — `multica auth status` was green for the
   *default* profile, but `make daemon` runs `--profile local`, a separate auth store.
   **Fixed** by logging the local profile in with the existing PAT.
3. **Prober per-server timeout (8s) < real `npx` cold start (~15.5s measured).** With 1 & 2
   fixed, the pipeline runs end-to-end (`pending → running → completed`) but every
   `npx -y` stdio server — the most common MCP type in the market, and every stdio entry
   in the ECC reference configs — spuriously reports `failed: "no initialize response"`.
   A *correct* config looks broken. **This file fixes #3.**

Measured evidence (`@modelcontextprotocol/server-sequential-thinking`, warm cache):
```
[15.5s ERR] Sequential Thinking MCP Server running on stdio
[15.5s OUT] {"result":{"protocolVersion":"2025-06-18", ...}}   ← initialize response
```
`npx -y` re-resolves the registry + node cold-starts on every run, so warming the cache
does not help; ~15s is the realistic floor.

### Constraint: the timing budget is a closed system

```
UI poll ceiling ........ 1500ms × 20 = 30s   (mcp-server-manager.tsx:20-21)
Server pending timeout . 30s                  (mcp_probe.go:54)
Server running timeout . 30s                  (mcp_probe.go:55)
Daemon pickup .......... ≤15s (probe rides the heartbeat ack, not a push)
Per-server probe ....... 8s                   (probe.go:55)  ← too small
```
Worst-case end-to-end = pickup (≤15s) + per-server probe + report. Bumping only the
per-server timeout to 20s makes worst case ~35s, **past the 30s UI ceiling** → the UI
would show a false timeout while the daemon is still finishing. Every timer must move
together.

### Chosen approach: A — align the whole budget + honest message

Rejected: **B** (push probe over WS — reworks the task-locked relay, protocol blast radius,
over-built for a cold-start issue) and **C** (async fire-and-forget UI — changes the
interaction model, more UI work, YAGNI for ~15s). ADR-worthy decision: prefer the
smallest internally-consistent timer change over re-architecting delivery.

New budget:
```
Per-server probe ....... 8s  → 20s   (covers ~15s npx cold start + margin)
UI poll ceiling ........ 30s → ~45s  (30 attempts × 1500ms; covers 15s pickup + 20s probe)
Server running timeout . 30s → 40s   (probe 20s + report + jitter, stays > probe, < UI)
Server pending timeout . 30s (unchanged — pickup ≤15s)
```
Ordering invariant preserved: `UI ceiling (45s) > server completes (~35s)`, and
`server running timeout (40s) > per-server probe (20s)`, so neither the UI nor the server
declares a false timeout before the daemon reports.

Honest messaging: a per-server **deadline-exceeded** outcome reports a clear error
("server did not respond before timeout — slow start, e.g. cold npx; try again") instead
of the misleading `"no initialize response"` (which should stay reserved for a server that
streamed non-JSON / closed without ever answering). Stays `status:"failed"` — zero UI/schema
change, the detail panel already renders the error string.

## Implementation Plan

### Phase 1 — prober (Go)
- [ ] `server/internal/mcpprobe/probe.go`
  - `defaultPerServerTimeout` `8` → `20` (update the doc comment too).
  - In `probeStdio`, when the initialize wait fails: if `ctx.Err() == context.DeadlineExceeded`,
    return the honest "did not respond before timeout (slow start, e.g. cold npx) — try again"
    message; otherwise keep `"no initialize response"`.
- [ ] `server/internal/mcpprobe/probe_test.go`
  - Existing tests pass unchanged (each passes its own ctx; `TestProbeStdioNoInitializeResponse`
    uses a 1s ctx → still `failed`). Add one test: a `sh` server that `sleep`s past a short ctx
    asserts `failed` **and** the deadline-exceeded message branch.

### Phase 2 — server timeouts (Go)
- [ ] `server/internal/handler/mcp_probe.go`: `mcpProbeRunningTimeout` `30s` → `40s`.
      `mcpProbePendingTimeout` and `mcpProbeRetention` unchanged.

### Phase 3 — UI poll ceiling (TS)
- [ ] `packages/views/agents/components/mcp/mcp-server-manager.tsx`:
      `PROBE_POLL_ATTEMPTS` `20` → `30` (update the `~30s` comment to `~45s`).
      Verify `packages/views/settings/components/mcp-tab.tsx` (workspace test button) reuses
      this same component/poll — if it has its own ceiling, bump it identically.

### Phase 4 — verify
- [ ] `cd server && go test ./internal/mcpprobe/ ./internal/handler/ -run Mcp`
- [ ] `pnpm --filter @multica/views exec vitest run agents/components/mcp/installed-connector-list.test.tsx`
- [ ] Live: workspace probe with the keyless `sequential-thinking` config → expect
      `connected · N tools` (not `failed`).

### Data Integration Map
| Component | Data Source | Existing? | Notes |
|---|---|---|---|
| Per-server probe | daemon `mcpprobe.ProbeConfig` over JSON-RPC initialize | Yes | timeout constant only |
| Probe lifecycle | `McpProbeStore` (in-mem, pending→running→completed/timeout) | Yes | running timeout only |
| UI status pills | `api.getMcpProbe` poll | Yes | attempt count only |

No new endpoints, schemas, or data sources — timer/message changes only.
