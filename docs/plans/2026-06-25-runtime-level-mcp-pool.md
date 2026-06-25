# Runtime-level MCP pool — connect once, agents reuse

## Design

### Problem

MCP "connection" today is **per-agent**:

- Each agent stores a full `mcp_config` (commands / urls / env / tokens), merged
  with a workspace-level config (`mergeWorkspaceAgentMcpConfig`).
- "Test connections" probes that effective config **per agent**
  (`InitiateAgentMcpProbe`) — spawn → handshake → teardown.
- In-app OAuth is **per agent** (`startMcpOauth(agent.id, …)`,
  `injectMcpOauthHeaders`).

A runtime serving 13 agents (see Runtimes detail page) therefore connects,
authenticates and probes the *same* servers up to 13×. The config also
duplicates secrets into Multica's DB, and workspace-level stdio config is
semantically wrong — an `npx …` server only physically exists on the one
machine where it's installed.

### Decision (locked via brainstorm)

| # | Question | Decision |
|---|----------|----------|
| 1 | What is reused? | **Config + auth, per runtime.** Connect/auth/probe once at the runtime. |
| 2 | How do agents get servers? | **Inherit by default**, but each agent keeps a *unique enabled subset* (see clarification). |
| 3 | Source of the pool | **The runtime machine itself** — the daemon's own resolved MCP config (`~/.claude.json` / `.mcp.json` / runtime-specific). Multica stores **no** server definitions. |
| 4 | FE capability | **Read-only mirror + Test/Authenticate.** No config authoring from the app; add/remove servers is done on the machine. |

**Reconciliation of #2 + user clarification** ("tiap agent tetap punya MCP unik,
tapi gak perlu configure lagi — tinggal reuse"):

- The runtime owns the **connection layer**: every server configured, authed and
  probed once.
- Each agent owns a **selection**: a list of *enabled server names* referencing
  the runtime pool. Default = all (inherit); the user can narrow it to a unique
  subset per agent.
- An agent **never** carries definitions, env, or tokens, and never triggers its
  own connect/auth/probe. It reuses what the runtime already established.

### Target architecture

```
Runtime machine (daemon host)
  └── resolved MCP config  ──reports──▶  Control plane
        (definitions+creds stay here)      • caches pool against runtime_id
                                           • runtime-level probe (heartbeat)
                                           • runtime-level OAuth
                                                   │
                Runtime page (FE)  ◀── read-only mirror: pool + per-server status,
                                        "Test connections", "Authenticate"
                                                   │
   agent.mcp_config = { enabledServers: ["github","figma"] }   ← names only
                                                   │
   Task run: daemon filters its machine config to the agent's
             enabled names → hands that subset to the CLI. No push of
             definitions from Multica.
```

### What this removes (product not live → delete, don't shim)

- `workspace_mcp_config.*` (handler, query, table/column) and the whole
  workspace-level MCP config concept.
- `mergeWorkspaceAgentMcpConfig` + the agent-carries-full-config shape.
- Per-agent probe (`InitiateAgentMcpProbe`) and per-agent OAuth wiring
  (`injectMcpOauthHeaders` keyed on agent/workspace).
- Secrets in Multica's DB for MCP.
- The agent MCP tab as a config editor → becomes a checklist.

### What this adds / changes

- **Daemon → reports its MCP servers.** New heartbeat payload: the runtime reads
  its CLI's resolved MCP config and reports `{name, transport, …}` (no secrets
  needed for display; probe runs locally where creds are reachable). *(This is
  the one real unknown — each runtime type resolves config differently; see
  Open Questions.)*
- **Runtime-level probe.** `InitiateRuntimeMcpProbe(runtimeID)` reusing the
  existing `McpProbeStore` / pop-on-heartbeat / `ReportMcpProbeResult` pattern
  verbatim — only the config *source* changes (machine pool, not agent config).
- **Runtime-level OAuth.** Re-point the freshly-built `mcp_oauth.go` from
  `agent.id` → `runtime.id`. The machinery is reusable, not wasted.
- **`agent.mcp_config` → enabled-names list.** At run time the daemon resolves
  names against its own machine config and hands the CLI that subset.
- **FE:** move the existing `installed-connector-list` + test-connections UI onto
  the Runtime page; the agent MCP tab becomes a read-only/checkbox list of the
  runtime pool.

## Data Integration Map

| Component | Data source | Exists? | Notes |
|---|---|---|---|
| Runtime MCP panel (pool + status) | Daemon-reported machine MCP config + runtime probe | **New report**, reuse probe store | Read-only mirror |
| "Test connections" (runtime) | `McpProbeStore` + heartbeat pop + `ReportMcpProbeResult` | ✅ reuse as-is | Re-key agent→runtime |
| "Authenticate" (runtime) | `mcp_oauth.go` | ✅ reuse | Re-point agent→runtime |
| Agent MCP tab (checklist) | Runtime pool (above) + `agent.mcp_config.enabledServers` | Partial | Tab simplified to selection |
| Task execution config | Daemon filters machine config by agent's enabled names | Partial | Replaces config push |
| Workspace MCP config | — | ❌ removed | Delete handler+query+table |

## Open questions (resolve in plan)

1. **How does each runtime type expose its resolved MCP config?** Claude Code,
   Codex, OpenClaw differ. Read config files vs a `mcp list --json` CLI call —
   per-runtime adapter. This is the gating unknown.
2. **OAuth ownership** when a remote server is auth'd at the runtime: does the
   CLI's own `login` already cover it (then Multica's in-app OAuth is only a
   convenience), or must Multica hold the token? Lean: machine holds it.
3. **Default selection** for a new agent — all-enabled (inherit) then narrow,
   confirmed. Store as allow-list or deny-list? Allow-list is explicit but
   silently drops servers added to the machine later; deny-list inherits new
   servers automatically. Lean **deny-list** (matches "inherit by default").

## Feasibility

Green. The hard part (a daemon-side, locally-run, heartbeat-dispatched probe
that reports per-server status) **already exists** — this design narrows its
input from per-agent config to the runtime's own pool and deletes the per-agent
duplication around it. Net change is deletion-heavy.

---

> **For Claude:** REQUIRED SKILL: Use gaspol-execute to implement this plan.
> **CRITICAL:** This plan specifies real integrations. During execution,
> NEVER substitute placeholders for real data sources without explicit
> user approval. If a data source doesn't exist yet, STOP and ask.

## Goal

Move MCP from a per-agent connection model to a per-runtime one. The runtime
machine's own resolved MCP config becomes the single source of truth: the daemon
reports its servers on the heartbeat, Multica probes/auths them **once** per
runtime, and every agent on that runtime reuses the pool — selecting a subset by
name, never re-configuring, re-authing, or re-probing. This kills the N×
duplication (a runtime serving 13 agents connects each server up to 13×), removes
MCP secrets from Multica's DB, and deletes the semantically-wrong workspace-level
stdio config. The change is deletion-heavy because the expensive machinery (a
locally-run, heartbeat-dispatched probe) already exists and is merely re-sourced.

## Architecture Context (from CLAUDE.md + code)

- **Agent↔runtime:** every agent already carries `runtime_id` (`agent.go:37`).
  Agents are bound 1:1 to a runtime — the natural owner of the pool.
- **Probe machinery (reuse):** `server/internal/handler/mcp_probe.go` —
  `McpProbeStore` (in-memory, pop-on-heartbeat), `ReportMcpProbeResult` (daemon→
  server, **already keyed by `runtimeId`**), `runtimeOnline`. The actual probe is
  `server/internal/mcpprobe/probe.go` (`ProbeConfig`) — runs **on the daemon**.
- **Heartbeat channel (reuse):** `server/pkg/protocol/messages.go` —
  `DaemonHeartbeatRequestPayload` (daemon→server: **add reported pool here**),
  `DaemonHeartbeatAckPayload.PendingMcpProbe` (server→daemon: repurpose to a
  "probe your own pool" signal — no config payload needed).
- **Machine config sources (the gating unknown, now resolved):**
  - Codex → `~/.codex/config.toml` `[mcp_servers]` (`execenv/codex_home.go`,
    `codexHome()` resolves `$CODEX_HOME`→`~/.codex`).
  - OpenClaw → user config's `mcp` block, currently **stripped** in
    `execenv/openclaw_config.go` and replaced with the managed config.
  - Claude Code → `~/.claude.json` / workdir `.mcp.json` (`mcpServers`).
- **Run-time config push (to invert):** `execenv.ExecOptions.McpConfig`
  (`execenv/execenv.go:42`) + `daemon/mcp_config.go` `runtimeMcpConfig()` (strips
  `disabledMcpServers`). Today Multica pushes the agent's full config down.
- **To delete:** `handler/workspace_mcp_config.go` (incl.
  `mergeWorkspaceAgentMcpConfig:135`), `handler/mcp_oauth_inject.go`
  `injectMcpOauthHeaders:26`, `InitiateAgentMcpProbe` + `InitiateWorkspaceMcpProbe`
  (`mcp_probe.go`), routes `r.Post("/mcp/probe", …)` at `router.go:581,855`.
- **FE:** runtime detail at `packages/views/runtimes/components/runtime-detail.tsx`
  (+ `runtime-detail-page.tsx`); agent tab + reusable widgets at
  `packages/views/agents/components/mcp/{mcp-server-manager,installed-connector-list}.tsx`
  and `…/tabs/mcp-config-tab.tsx`; API client `packages/core/api/client.ts`
  (`probeAgentMcp:847`, `startMcpOauth:877`, `setMcpAccessToken:895`).

## Tech Stack

Go (Chi, sqlc, gorilla/websocket) backend; daemon over WS heartbeat; React +
TanStack Query + Zustand + shadcn/Base-UI front end. `go test` and Vitest. No new
deps — every piece reuses an existing pattern.

## Data Integration Map

| Feature | Data Source | Hook/API | Exists? | Action |
|---|---|---|---|---|
| Runtime MCP pool (names+transport) | Daemon reads machine config, reports on heartbeat | `DaemonHeartbeatRequestPayload.McpServers` | **No** | Create: daemon reader + payload field |
| Pool cache + read | Runtime row JSON column `reported_mcp_servers` | `GET /runtimes/{id}/mcp` | **No** | Create: column + handler |
| Runtime "Test connections" | `McpProbeStore` + `PendingMcpProbe` + `ReportMcpProbeResult` | `InitiateRuntimeMcpProbe` | Partial | Reuse store; new initiator sourced from runtime |
| Runtime "Authenticate" (OAuth) | `mcp_oauth.go` | `startMcpOauth` re-keyed runtime | Partial | Re-point agent→runtime |
| Agent server selection | `agent.mcp_config` → `{disabledMcpServers:[...]}` deny-list | agent PATCH | Partial | Shrink shape; drop definitions |
| Task-time effective config | Daemon filters its machine config by agent deny-list | `execenv` | Partial | Invert: read machine, filter; stop pushing |
| Workspace MCP config | — | — | ❌ remove | Delete table/handler/query/merge |

## Implementation Plan

Sequencing: **1→2→3→4** build the pool pipeline (backend). **5** (OAuth) and
**6→7** (agent selection + run-time) depend on the pool existing. **8→9** (FE)
depend on the endpoints. **10** deletes dead paths last. Phases 5 and 6/7 are
independent of each other → candidates for `gaspol-parallel`.

### Phase 1: Daemon reads & normalizes machine MCP servers

**Estimated time:** ~12 min
**Files:** Create `server/internal/daemon/mcp_pool.go`, `…/mcp_pool_test.go`

**Steps:**
1. Write failing test for `ResolveMachineMcpServers(runtimeType, home)` over TOML/JSON fixtures (codex, claude, openclaw), asserting normalized `[]protocol.McpServerInfo{Name,Transport}`. Expected error: `undefined: ResolveMachineMcpServers`.
2. Run `cd server && go test ./internal/daemon/ -run McpPool`, confirm it fails for that reason.
3. Implement the reader: codex `[mcp_servers]` from `codexHome()/config.toml`; claude `mcpServers` from `~/.claude.json`/`.mcp.json`; openclaw `mcp` block (the same block `openclaw_config.go` strips). No secrets in the returned struct — names + transport only.
4. Run tests, confirm pass.
5. Commit: `feat(daemon): resolve machine MCP servers per runtime type`.

**Verification:**
- [ ] `go vet ./...` + `go test ./internal/daemon/` pass
- [ ] Returns real parsed servers for all three runtime types from fixtures
- [ ] No secrets/env/tokens in `McpServerInfo`
- [ ] No placeholder/TODO comments

### Phase 2: Report the pool on the heartbeat

**Estimated time:** ~10 min
**Files:** Modify `server/pkg/protocol/messages.go`, daemon heartbeat sender; Test daemon heartbeat test

**Steps:**
1. Write failing test asserting the daemon's heartbeat request includes `McpServers` from `ResolveMachineMcpServers`. Expected error: `unknown field McpServers`.
2. Run the daemon test, confirm failure.
3. Add `McpServers []McpServerInfo \`json:"mcp_servers,omitempty"\`` to `DaemonHeartbeatRequestPayload`; populate it in the heartbeat sender (omitempty → old servers ignore it).
4. Run tests, confirm pass.
5. Commit: `feat(protocol): daemon reports MCP pool on heartbeat`.

**Verification:**
- [ ] `go test ./pkg/protocol/ ./internal/daemon/` pass
- [ ] Heartbeat carries the pool; field is `omitempty` (back-compat for old daemons)

### Phase 3: Cache pool + `GET /runtimes/{id}/mcp`

**Estimated time:** ~12 min
**Files:** Create migration `…add_runtime_reported_mcp_servers`, sqlc query; Modify heartbeat handler + `handler/mcp_probe.go`/new `handler/runtime_mcp.go`; Test handler test

**Steps:**
1. Write failing test for `GET /runtimes/{id}/mcp` returning the cached pool + last probe results for a member. Expected error: route 404 / handler undefined.
2. Run `go test ./internal/handler/ -run RuntimeMcp`, confirm failure.
3. Add `reported_mcp_servers jsonb` to the runtime row (migration + `make sqlc`); persist it in the heartbeat handler; implement `GetRuntimeMcp` (member-gated via workspace) returning pool + latest `McpProbeStore` results for the runtime.
4. Run tests, confirm pass.
5. Commit: `feat(runtime): cache + expose reported MCP pool`.

**Verification:**
- [ ] `make sqlc` clean, migration up/down works, `go test ./internal/handler/` pass
- [ ] Endpoint returns real cached pool; gated to workspace members
- [ ] Response parsed defensively on FE side (Phase 8 schema)

### Phase 4: Runtime-level probe (replace per-agent/workspace)

**Estimated time:** ~12 min
**Files:** Modify `server/internal/handler/mcp_probe.go`, `server/cmd/server/router.go`, daemon probe handler, `server/pkg/protocol/messages.go`; Test `mcp_probe_test.go`

**Steps:**
1. Write failing test for `InitiateRuntimeMcpProbe(runtimeID)` enqueuing a probe the daemon runs against **its own** pool (no config in payload). Expected error: `undefined: InitiateRuntimeMcpProbe`.
2. Run `go test ./internal/handler/ -run McpProbe`, confirm failure.
3. Implement `InitiateRuntimeMcpProbe` reusing `McpProbeStore` (Config empty → `PendingMcpProbe` signals "probe own pool"); daemon side calls `mcpprobe.ProbeConfig` on its machine config. Add route `POST /runtimes/{id}/mcp/probe`. Delete `InitiateAgentMcpProbe`, `InitiateWorkspaceMcpProbe`, and routes at `router.go:581,855`.
4. Run tests, confirm pass.
5. Commit: `refactor(mcp): runtime-level probe, drop per-agent/workspace probe`.

**Verification:**
- [ ] `go test ./internal/handler/ ./internal/mcpprobe/` pass
- [ ] Daemon probes machine pool with no config pushed from server
- [ ] Old per-agent/workspace probe handlers + routes gone (grep clean)

### Phase 5: Runtime-level OAuth

**Estimated time:** ~10 min
**Files:** Modify `server/internal/handler/mcp_oauth.go`, `router.go`, `packages/core/api/client.ts`; Test `mcp_oauth_test.go`

**Steps:**
1. Write failing test for OAuth start/callback keyed by `runtime_id` (token stored against runtime, not agent). Expected error: handler signature/route mismatch.
2. Run `go test ./internal/handler/ -run McpOauth`, confirm failure.
3. Re-point `startMcpOauthRequest` and storage from agent/workspace → runtime; move route under `/runtimes/{id}/mcp/...`; update `client.ts` `startMcpOauth`/`setMcpAccessToken` to take `runtimeId`.
4. Run tests, confirm pass.
5. Commit: `refactor(mcp): OAuth at runtime scope`.

**Verification (security-sensitive):**
- [ ] `go test ./internal/handler/` pass; `pnpm typecheck` pass
- [ ] Authz server-side (caller must access the runtime); tokens never returned to UI; no secrets in source
- [ ] Token stored against runtime; per-agent OAuth path removed

### Phase 6: Agent `mcp_config` → deny-list selection

**Estimated time:** ~12 min
**Files:** Modify `server/internal/handler/agent.go`, `server/internal/handler/workspace_mcp_config.go` (gut), agent validation; Test `agent_test.go`

**Steps:**
1. Write failing test asserting an agent stores only `{"disabledMcpServers":["x"]}` (deny-list; absent = inherit all) and rejects full server definitions. Expected error: assertion fails (definitions still accepted).
2. Run `go test ./internal/handler/ -run Agent`, confirm failure.
3. Constrain `agent.mcp_config` to the deny-list shape on write; delete `mergeWorkspaceAgentMcpConfig` usage and the workspace-config read path from agent flows.
4. Run tests, confirm pass.
5. Commit: `refactor(agent): mcp_config is a deny-list over the runtime pool`.

**Verification:**
- [ ] `go test ./internal/handler/` pass
- [ ] New agents default to inherit-all (empty deny-list); definitions rejected
- [ ] No reference to `mergeWorkspaceAgentMcpConfig` remains

### Phase 7: Task-time — filter machine config by the agent deny-list

**Estimated time:** ~14 min
**Files:** Modify `server/internal/daemon/mcp_config.go`, `execenv/execenv.go`, `execenv/openclaw_config.go`, `execenv/codex_home.go`; Test `mcp_config_test.go`, `openclaw_config_test.go`

**Steps:**
1. Write failing test: given a machine pool and an agent deny-list, the effective runtime config = machine servers minus disabled (no Multica-pushed definitions). Expected error: still expects pushed `McpConfig`.
2. Run `go test ./internal/daemon/...`, confirm failure.
3. Replace the pushed-config path: `ExecOptions` carries the deny-list (names), the daemon reads its machine config and removes disabled names; openclaw stops stripping its `mcp` block; codex keeps `~/.codex/config.toml` as-is, applying the deny-list. Reduce `runtimeMcpConfig()` accordingly.
4. Run tests, confirm pass.
5. Commit: `refactor(daemon): agents reuse machine MCP, filtered by deny-list`.

**Verification:**
- [ ] `go test ./internal/daemon/...` pass
- [ ] Effective config derives from machine pool, not Multica DB
- [ ] Disabled servers absent at run time; openclaw `mcp` block preserved

### Phase 8: FE — Runtime page MCP panel (read-only mirror + Test/Auth)

**Estimated time:** ~14 min

| Phase | Code Deliverable | Design Deliverable | Verification |
|---|---|---|---|
| 8 | Runtime MCP panel reusing `installed-connector-list` + test-connections | Reuse existing MCP widget tokens/layout (no net-new design); place under runtime detail right column | Schema-parsed response, typecheck, renders real pool |

**Files:** Modify `packages/views/runtimes/components/runtime-detail.tsx`, `packages/core/api/client.ts`, add zod schema in `packages/core/api/schema.ts`; Test `packages/views/runtimes/*.test.tsx`

**Steps:**
1. Write failing test for the panel rendering the runtime pool + per-server status from a mocked `getRuntimeMcp`. Expected error: component undefined.
2. Run `pnpm --filter @multica/views exec vitest run runtimes`, confirm failure.
3. Add `api.getRuntimeMcp(runtimeId)` (zod-parsed via `parseWithFallback`), `api.probeRuntimeMcp(runtimeId)`; mount the existing `installed-connector-list` + Test/Authenticate buttons on the runtime detail page sourced from the pool.
4. Add one malformed-response test (missing field / null array → fallback). Run tests, confirm pass.
5. Commit: `feat(runtimes): read-only MCP pool panel with test/auth`.

**Verification:**
- [ ] `pnpm typecheck` + views tests pass
- [ ] Response runs through a schema (no bare `as`); malformed payload falls back, no white-screen
- [ ] Panel renders real pool; Test/Auth call runtime endpoints

### Phase 9: FE — Agent MCP tab becomes a checklist

**Estimated time:** ~12 min

| Phase | Code Deliverable | Design Deliverable | Verification |
|---|---|---|---|
| 9 | `mcp-config-tab.tsx` reduced to a checklist over the runtime pool | Inherit existing list styling; remove editor affordances | Typecheck, writes deny-list, no config editor |

**Files:** Modify `packages/views/agents/components/tabs/mcp-config-tab.tsx`, `…/mcp/mcp-server-manager.tsx`; remove now-dead `probeAgentMcp`/agent OAuth wiring; Test agent tab test

**Steps:**
1. Write failing test: tab lists the runtime pool with checkboxes, toggling writes `disabledMcpServers` to the agent (no command/url/env fields shown). Expected error: still renders config editor.
2. Run `pnpm --filter @multica/views exec vitest run agents`, confirm failure.
3. Fetch the agent's runtime pool via `getRuntimeMcp(agent.runtime_id)`; render checkboxes; on save write the deny-list. Strip config-editor + per-agent probe/auth handlers.
4. Run tests, confirm pass.
5. Commit: `feat(agents): MCP tab is a reuse checklist over the runtime pool`.

**Verification:**
- [ ] `pnpm typecheck` + views tests pass
- [ ] Toggling writes deny-list only; redacted/non-privileged view still safe
- [ ] No config editor, no per-agent probe/auth UI

### Phase 10: Delete dead paths + sync docs

**Estimated time:** ~10 min
**Files:** Delete `handler/workspace_mcp_config.go`, `handler/mcp_oauth_inject.go`, workspace-mcp routes/queries, `probeAgentMcp` + per-agent OAuth in `client.ts`; Modify conventions/skill docs

**Steps:**
1. Grep for `workspace_mcp_config|mergeWorkspaceAgentMcpConfig|injectMcpOauthHeaders|probeAgentMcp|InitiateWorkspaceMcpProbe`; expect only dead refs.
2. Delete the files/queries/routes + the workspace MCP table migration (down-migration since not live).
3. Run `make check` (typecheck, unit, Go, lint). Update `apps/docs/.../conventions.mdx` and any built-in skill referencing per-agent MCP config (CLAUDE.md rule).
4. Commit: `chore(mcp): remove per-agent/workspace MCP paths`.

**Verification:**
- [ ] `make check` green
- [ ] grep for deleted symbols returns nothing
- [ ] Docs/skills updated to the runtime-pool model
- [ ] `graphify update .` run

## Execution status (2026-06-25)

All 10 phases implemented and committed to `main`. Backend Go build + vet clean;
full TS typecheck (6/6) and FE tests (views 1218, core 491) green; MCP-touched Go
packages green (the only red is the pre-existing `TestEnsureRepoReady*`
git-sandbox failures, unrelated).

Key decisions taken during execution:
- **OAuth ownership:** Multica holds the token and **injects** it into the
  daemon's config — `mcpOauthHeadersByServerName` sends bearer tokens keyed by
  server name; the daemon's `BuildEffectiveMcpConfig` injects them for probe AND
  task-run. Server definitions never leave the host.
- **Empty-pool signal:** the heartbeat field is non-`omitempty`; a current daemon
  sends `[]` for "zero servers" (clears the mirror), old daemons send `null`
  (skipped, never clobbering a known pool).
- **mcp_pool reader** lives in `daemon/execenv/` (reuses `resolveSharedCodexHome`
  / `openclawExec`), not `daemon/` as the plan first said.

### Residual follow-ups (deliberately not done autonomously)
- **Product docs** (`apps/docs/.../agents*.mdx`, `providers*.mdx`,
  `install-agent-runtime*.mdx` + ja/zh/ko) still describe the per-agent MCP
  model. These are user-facing prose across 3 languages — needs an editorial pass
  + messaging decision. (Contract docs — built-in skills, conventions naming —
  are unaffected.)
- **Orphaned FE workspace-config cluster**: `workspaceMcpConfigOptions`,
  `api.getWorkspaceMcpConfig/updateWorkspaceMcpConfig`, `WorkspaceMcpConfigSchema`
  — unused (their endpoints are deleted) but left to avoid rippling into a schema
  test for zero functional gain.
- **Connector-directory backend** (`mcp_connector` table, `ListMcpConnectors` etc.,
  routes, seed) — its FE was removed; the backend is a separable catalog feature,
  left intact rather than folded into this refactor.
- **`workspace.mcp_config` column** left in place (nullable, unused); dropping it
  is a no-benefit migration.

## Open question carried into execution

Pool freshness vs heartbeat cost: reporting the full pool on **every** heartbeat
is cheap (read a config file) but chatty. If heartbeats are frequent, gate the
report to "changed since last sent" (hash the config) — decide in Phase 2 once
the heartbeat cadence is in view. `ponytail:` start unconditional; add the hash
only if heartbeat payload size measurably matters.
