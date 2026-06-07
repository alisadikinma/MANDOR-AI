# UI Upgrades — Collapsible Sidebar · MCP Connector Directory · Blue Accent Theme

> Status: Design approved (brainstorm). Next: `gaspol-plan` appends `## Implementation Plan`.
> Date: 2026-06-07 · Scope: **web only** (`apps/web`), shared logic in `packages/views` / `packages/core` / `packages/ui` per the no-duplication rule.

## Design

Three independent features bundled by theme (dashboard UX polish). Each can ship separately.

---

### Feature 1 — Side menu collapse / expand (icon rail)

**Decision:** Collapse to an **icon-only rail** (not full off-canvas hide). Wire the *existing* primitive — do not build new.

**What already exists:**
- [packages/ui/components/ui/sidebar.tsx](../../packages/ui/components/ui/sidebar.tsx) — full collapse support: `state: "expanded" | "collapsed"`, `toggleSidebar()`, `collapsible="icon" | "offcanvas" | "none"`, `<SidebarRail/>`, width persisted to `localStorage` (`sidebar_width`), `--sidebar-width-icon: 3rem`.
- [packages/views/layout/app-sidebar.tsx](../../packages/views/layout/app-sidebar.tsx) — renders `<Sidebar variant="inset">` (defaults to `offcanvas`) + `<SidebarRail/>`; already imports `Tooltip`.

**Changes:**
1. `<Sidebar variant="inset" collapsible="icon">`.
2. Ensure `<SidebarProvider>` wraps the dashboard shell with persisted `open` state (cookie or localStorage) so collapse survives reload.
3. Visible toggle: header chevron **and** `⌘/Ctrl+B` keybind (follow the `search-command.tsx` keybind pattern).
4. Icon-mode rendering for the custom regions: workspace switcher, pinned items, DnD groups, footer → icon + `Tooltip` label when `state === "collapsed"`.

**Risk:** Low. **Effort:** S. Only fiddly part is the custom header / pinned / DnD groups under icon mode.

---

### Feature 2 — MCP connector Directory (backend-driven marketplace)

**Decision:** Additive **gallery layer** over the existing per-agent `mcp_config`. Catalog = **seeded curated + workspace-custom** connectors. Raw JSON tab stays as "Advanced".

**What already exists:**
- `server/migrations/046_agent_mcp_config.*` — per-agent `mcp_config` JSON column.
- Daemon consumes it: [server/pkg/agent/opencode_mcp.go](../../server/pkg/agent/opencode_mcp.go).
- UI: raw textarea [packages/views/agents/components/tabs/mcp-config-tab.tsx](../../packages/views/agents/components/tabs/mcp-config-tab.tsx), tab-hosted in [agent-overview-pane.tsx](../../packages/views/agents/components/agent-overview-pane.tsx) (`Plug` icon, gated by `providerSupportsMcpConfig` / `showMcp`).
- Seed-as-embedded-JSON precedent: `server/internal/handler/reserved_slugs.json`.

**Backend (Go):**
- New table `mcp_connectors` — fields: `id`, `workspace_id` (NULL = global/seeded), `slug`, `name`, `icon`, `description`, `popularity`, `input_schema` (JSONB: required env keys / URL the user fills), `mcp_template` (JSONB: the `mcpServers` fragment with `{{placeholders}}`), `created_by`, timestamps.
- Seed global connectors from embedded JSON (GitHub, Slack, Notion, Figma, Atlassian, Gmail, Microsoft 365, …).
- sqlc queries (`server/pkg/db/queries/mcp_connector.sql`): list (global ∪ workspace), get, create-custom, update-custom, delete-custom. All filter by `workspace_id` per multi-tenancy.
- Handler `server/internal/handler/mcp_connector.go`: `GET /api/workspaces/{ws}/mcp-connectors`, `POST` (workspace-custom), `PATCH/DELETE {id}` (custom only; admin-gated). UUID params via `parseUUIDOrBadRequest` / loaders per the handler convention.

**Frontend (`packages/views`):**
- "Browse connectors" button on the MCP tab → **Directory modal** (matches the reference screenshot): search bar, filter, sort, responsive card grid, `+` add per card.
- Pick a connector → **schema-driven form** (renders fields from `input_schema`) → on submit, merge the rendered `mcp_template` into that agent's `mcpServers` JSON → save via existing `onSave({ mcp_config })`.
- "Add custom connector" entry (admin) → form to create a workspace-scoped catalog entry.
- API client method runs through `parseWithFallback` (zod) per **API Response Compatibility**; enum/field drift downgrades, not crashes; one malformed-response test required.

**Risk:** Medium. **Effort:** L. This is the one net-new screen — candidate for `gaspol-design` if visual polish needed.
**ADR:** catalog source = *seeded + workspace-custom* (chosen over global-only) — capture via `gaspol-adr`.

---

### Feature 3 — Selectable "Blue / JIRA" accent theme

**Decision:** Blue is a **separate accent axis** (`data-accent`), orthogonal to light/dark — so blue works in *both* modes. Not a 4th mutually-exclusive mode.

**What already exists:**
- [theme-provider.tsx](../../packages/ui/components/common/theme-provider.tsx) — `next-themes`, `attribute="class"`, `light/dark/system`.
- Tokens (oklch) in [packages/ui/styles/tokens.css](../../packages/ui/styles/tokens.css): `--primary`, `--accent`, `--ring`, `--sidebar-primary`, `ring-brand`, defined for `:root` and `.dark`.
- Theme picker (card-radio UI) in [preferences-tab.tsx](../../packages/views/settings/components/preferences-tab.tsx) + quick-switch in `search-command.tsx`.

⚠️ **Gotcha:** `attribute="class"` holds one class — adding `"blue"` to `themes` makes it exclusive with `dark` (no blue-dark). Hence the separate axis.

**Changes:**
1. tokens.css: add a `[data-accent="blue"]` override block setting `--primary` / `--primary-foreground` / `--ring` / `--sidebar-primary` / `--brand` to Atlassian blue (`#0052CC` ≈ `oklch(0.48 0.20 264)`), for **both** `:root[data-accent="blue"]` and `.dark[data-accent="blue"]`.
2. A small accent store/hook (in `packages/core`, no DOM) that sets `document.documentElement.dataset.accent` and persists (localStorage + optional `api.updateMe`), mirroring the locale-persist pattern in `preferences-tab.tsx`.
3. preferences-tab: add an "Accent" card-radio group (Default / Blue), reusing the existing theme-card UI.

**Risk:** Low-medium (the axis decision, now resolved). **Effort:** M.
**ADR:** accent-axis (chosen over 4th-mode) — capture via `gaspol-adr`.

---

## Data Integration Map

| Component | Data source | Existing? | Notes |
|---|---|---|---|
| `app-sidebar` collapse state | `SidebarProvider` `open` + localStorage `sidebar_width` | ✅ primitive exists | Add persisted `open`; wire `collapsible="icon"` |
| Sidebar toggle keybind | `search-command` keybind pattern | ✅ | `⌘/Ctrl+B` |
| MCP Directory list | `GET /api/workspaces/{ws}/mcp-connectors` | ❌ new endpoint + `mcp_connectors` table | global ∪ workspace; `parseWithFallback` |
| MCP custom connector CRUD | `POST/PATCH/DELETE .../mcp-connectors` | ❌ new | admin-gated; UUID via `parseUUIDOrBadRequest` |
| MCP add-to-agent | `onSave({ mcp_config })` (existing) | ✅ | merge rendered template into `mcpServers` |
| MCP daemon consumption | `opencode_mcp.go` reads `mcp_config` | ✅ | no change — same column |
| Accent theme override | tokens.css `[data-accent="blue"]` | ❌ new CSS block | both light + dark |
| Accent persistence | localStorage + `api.updateMe` (optional) | ✅ pattern (locale) | core store sets `dataset.accent` |
| Theme picker UI | `preferences-tab` card-radio | ✅ reuse | add Accent group |

## Implementation feasibility (placeholder check)

- All three buildable on **real** data/infra. No placeholders.
- Sidebar & theme: pure frontend, infra present.
- MCP Directory: only net-new backend (table + handler + seed). `mcp_config` write path already exists end-to-end (UI → API → daemon), so the gallery is a friendly front-end over a proven pipe.
- Seed catalog must ship real connector definitions (not stubs) — define `input_schema` + `mcp_template` per connector at build time.

## Open items for `gaspol-plan`
- Phase ordering: sidebar (S) → theme (M) → MCP Directory (L). Independent; can parallelize.
- Two ADRs to capture: MCP catalog source, theme accent-axis.
- Decide admin gating mechanism for workspace-custom connectors (reuse membership/role checks).

---

## Implementation Plan

> **For Claude:** REQUIRED SKILL: Use gaspol-execute to implement this plan.
> **CRITICAL:** This plan specifies real integrations. During execution,
> NEVER substitute placeholders for real data sources without explicit
> user approval. If a data source doesn't exist yet, STOP and ask.

## Goal

Ship three independent, web-only dashboard UX upgrades for Multica: (1) a collapsible icon-rail sidebar wired from the existing `ui/sidebar` primitive, (2) a backend-driven MCP connector **Directory** (seeded curated + workspace-custom) layered over the existing per-agent `mcp_config` pipe, and (3) a selectable JIRA-blue **accent** theme implemented as a `data-accent` axis orthogonal to light/dark. All three reuse existing infrastructure; only the MCP catalog is net-new backend.

## Architecture Context

Pulled from `CLAUDE.md` + codebase scan:

- **Sidebar primitive** `packages/ui/components/ui/sidebar.tsx` — already supports `collapsible="icon"`, `state`, `toggleSidebar()`, `<SidebarRail/>`, width persisted to `localStorage` (`sidebar_width`), `--sidebar-width-icon: 3rem`.
- **Web mount** `packages/views/layout/dashboard-layout.tsx:36` — `<SidebarProvider className="h-svh">` + `<SidebarInset>`. `app-sidebar.tsx` renders `<Sidebar variant="inset">` (default `offcanvas`) + `<SidebarRail/>`, already imports `Tooltip`.
- **Agent update path** `api.updateAgent(id, data)` → `PATCH /api/agents/{id}` (`packages/core/api/client.ts:802`). MCP saves already flow `onUpdate(agent.id, { mcp_config })` → `api.updateAgent` (`agent-detail-page.tsx:135`). MCP tab hosted in `agent-overview-pane.tsx` (Plug icon, gated `providerSupportsMcpConfig`/`showMcp`). Daemon consumes column via `server/pkg/agent/opencode_mcp.go`. Table from `server/migrations/046_agent_mcp_config`.
- **Theme** `next-themes` `attribute="class"` (`packages/ui/components/common/theme-provider.tsx`); tokens (oklch) `packages/ui/styles/tokens.css` already expose `--brand`, `--brand-foreground`, `--primary`, `--ring`, `--sidebar-primary`, `ring-brand` for `:root` + `.dark`. Picker `packages/views/settings/components/preferences-tab.tsx` (card-radio). `next-themes` does NOT manage `data-accent` → web needs its own anti-FOUC inline script.
- **Catalog/seed precedent** `//go:embed reserved_slugs.json` in `server/internal/handler/workspace_reserved_slugs.go:18`. **Next migration number = 117.**
- **Stores** live in `packages/core`, created via factory + injected `StorageAdapter` (`packages/core/types/storage.ts`) — no direct `localStorage`. **API responses** parsed via `parseWithFallback` (`packages/core/api/schema.ts:38`) + zod.

## Tech Stack

Go (Chi, sqlc, migrations) backend · Next.js App Router (`apps/web`) · React + Zustand (client state, `packages/core`) · TanStack Query (server state) · zod (response schemas) · Base UI / shadcn (`packages/ui`) · Vitest (TS) / `go test` (Go). No new deps expected (verify `@base-ui` dialog/command primitives already in `packages/ui`).

## Data Integration Map

| Feature | Data Source | Hook/API | Exists? | Action |
|---|---|---|---|---|
| Sidebar collapse state | `SidebarProvider` `open` + cookie/localStorage | `useSidebar()` | Yes | Wire `collapsible="icon"` + persist `open` |
| Sidebar toggle | keybind pattern in `search-command.tsx` | n/a | Yes | Add `⌘/Ctrl+B` + header button |
| Accent value | new `useAccentStore` (StorageAdapter-injected) | `useAccentStore()` | No | Create in `packages/core` |
| Accent persistence | `StorageAdapter` + optional `api.updateMe` | `api.updateMe` | Yes (pattern) | Mirror locale persist in `preferences-tab` |
| Accent FOUC guard | inline script in web root layout | n/a | No | Create in `apps/web` (mirror next-themes) |
| MCP connector list | `GET /api/workspaces/{ws}/mcp-connectors` | `api.listMcpConnectors` | No | New table + handler + client method |
| MCP custom CRUD | `POST/PATCH/DELETE .../mcp-connectors/{id}` | `api.{create,update,delete}McpConnector` | No | New (admin-gated) |
| MCP add-to-agent | `api.updateAgent(id,{mcp_config})` | existing `onUpdate` | Yes | Merge rendered template into `mcpServers` |
| MCP daemon read | `opencode_mcp.go` reads `mcp_config` | n/a | Yes | No change |

---

### Feature 1 — Collapsible icon-rail sidebar (web)

#### Phase S1: Persist sidebar open state + enable icon collapse

**Estimated time:** ~12 min
**Files:**
- Modify: `packages/views/layout/dashboard-layout.tsx`, `packages/views/layout/app-sidebar.tsx`
- Test: `packages/views/layout/app-sidebar.test.tsx`

**Steps:**
1. Write failing test for collapsed state: render `AppSidebar` inside `SidebarProvider` with `defaultOpen={false}`, assert `data-state="collapsed"` present. Expected error: `expect(received).toBeInTheDocument()` (element not collapsed — currently offcanvas).
2. Run test, confirm it fails for the expected reason.
3. Set `<Sidebar variant="inset" collapsible="icon">` in `app-sidebar.tsx`; pass persisted `open`/`onOpenChange` from `SidebarProvider` in `dashboard-layout.tsx`, reading initial value from `StorageAdapter` key `sidebar_open` (do NOT use `localStorage` directly in `packages/views`; read via a small prop/adapter the way width is handled in the primitive).
4. Run tests, confirm pass.
5. Commit: `feat(sidebar): enable icon-rail collapse with persisted open state`

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] Sidebar renders `data-state="collapsed"` when `defaultOpen={false}`
- [ ] Open/closed state survives reload (persisted)
- [ ] No placeholder/TODO comments in new code
- [ ] `pnpm --filter @multica/views exec vitest run layout/app-sidebar.test.tsx` passes

#### Phase S2: Toggle affordance + icon-mode rendering for custom regions

**Estimated time:** ~14 min
**Files:**
- Modify: `packages/views/layout/app-sidebar.tsx`
- Test: `packages/views/layout/app-sidebar.test.tsx`

| Phase | Code Deliverable | Design Deliverable | Verification |
|---|---|---|---|
| S2 | Toggle button + keybind + icon/tooltip rendering | Icon-mode spec (tooltip labels, 3rem rail, active states) — reuse existing tokens | Design-system compliance + tests |

**Steps:**
1. Write failing test: with sidebar collapsed, assert each top-level nav item exposes an accessible name via `Tooltip` (query `aria-label`/tooltip trigger) and labels are visually hidden. Expected error: tooltip/aria-label not found.
2. Run test, confirm fail.
3. Add a header toggle button (chevron) calling `toggleSidebar()`; register `⌘/Ctrl+B` following the `search-command.tsx` keybind pattern. In collapsed state, render workspace switcher, pinned items, DnD groups, and footer as icon + `Tooltip` label (use `state` from `useSidebar()`); ensure DnD still works in icon mode or disable drag while collapsed.
4. Run tests, confirm pass.
5. Commit: `feat(sidebar): add collapse toggle, ⌘B keybind, icon-mode tooltips`

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] `⌘/Ctrl+B` toggles; header button toggles
- [ ] Collapsed nav items have accessible names (tooltip/aria)
- [ ] No hardcoded colors — semantic tokens only
- [ ] vitest for `app-sidebar.test.tsx` passes

---

### Feature 2 — Blue accent theme (web)

#### Phase T1: tokens.css blue accent override + anti-FOUC script

**Estimated time:** ~12 min
**Files:**
- Modify: `packages/ui/styles/tokens.css`, `apps/web/app/layout.tsx` (root layout)
- Test: `apps/web/app/layout.test.tsx` (or a focused token-presence test)

| Phase | Code Deliverable | Design Deliverable | Verification |
|---|---|---|---|
| T1 | `[data-accent="blue"]` token block + FOUC script | Blue palette spec: `#0052CC` ≈ `oklch(0.48 0.20 264)` mapped to `--brand/--primary/--ring/--sidebar-primary` for light+dark | Token coverage both modes |

**Steps:**
1. Write failing test asserting the root layout emits a blocking inline script that sets `documentElement.dataset.accent` from storage before paint. Expected error: script string not found in rendered layout.
2. Run test, confirm fail.
3. Add `:root[data-accent="blue"]{…}` and `.dark[data-accent="blue"]{…}` blocks in `tokens.css` overriding `--brand`, `--brand-foreground`, `--primary`, `--primary-foreground`, `--ring`, `--sidebar-primary` to the Atlassian-blue oklch values. Add an inline anti-FOUC `<script>` in `apps/web/app/layout.tsx` (mirror next-themes) reading `accent` from `localStorage` and stamping `data-accent`.
4. Run tests, confirm pass.
5. Commit: `feat(theme): add blue accent token axis + anti-FOUC script`

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] `[data-accent="blue"]` overrides resolve in both light and dark
- [ ] No FOUC: accent applied pre-hydration
- [ ] No hardcoded hex in components (only in tokens.css)

#### Phase T2: Core accent store (StorageAdapter-injected)

**Estimated time:** ~10 min
**Files:**
- Create: `packages/core/theme/accent-store.ts`, `packages/core/theme/accent-store.test.ts`
- Modify: `packages/core/platform/*` (register store factory, inject `StorageAdapter`)

**Steps:**
1. Write failing test: `createAccentStore({storage})` defaults to `"default"`, `setAccent("blue")` persists via storage and updates state. Expected error: `Cannot find module './accent-store'`.
2. Run test, confirm fail.
3. Implement Zustand store in `packages/core` (zero DOM): state `accent: "default" | "blue"`, `setAccent()` writes through injected `StorageAdapter`. A thin platform effect stamps `document.documentElement.dataset.accent` (effect lives where other DOM-touching platform code lives, NOT in the store).
4. Run tests, confirm pass.
5. Commit: `feat(core): accent store with StorageAdapter persistence`

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] Store has zero react-dom / direct `localStorage` (boundary rule)
- [ ] `setAccent` persists + restores
- [ ] `pnpm --filter @multica/core exec vitest run theme/accent-store.test.ts` passes

#### Phase T3: Accent picker in preferences

**Estimated time:** ~10 min
**Files:**
- Modify: `packages/views/settings/components/preferences-tab.tsx`, locale files `packages/views/locales/*/settings.json`
- Test: `packages/views/settings/components/preferences-tab.test.tsx`

| Phase | Code Deliverable | Design Deliverable | Verification |
|---|---|---|---|
| T3 | Accent card-radio group | Reuse existing theme-card radio UI; Default vs Blue swatches | Design-system compliance |

**Steps:**
1. Write failing test: preferences renders an "Accent" radiogroup with Default + Blue; clicking Blue calls `setAccent("blue")`. Expected error: radiogroup not found.
2. Run test, confirm fail.
3. Add an Accent section reusing the theme card-radio markup; wire to `useAccentStore`; add i18n keys to all `settings.json` locales (en, zh-Hans, ko, ja).
4. Run tests, confirm pass.
5. Commit: `feat(settings): add blue accent picker`

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] Selecting Blue flips `data-accent` and persists across reload
- [ ] All locale files updated (no missing-key console warnings)
- [ ] vitest for preferences passes

---

### Feature 3 — MCP connector Directory (backend + web)

#### Phase M1: `mcp_connectors` table + seed JSON

**Estimated time:** ~12 min
**Files:**
- Create: `server/migrations/117_mcp_connectors.up.sql`, `server/migrations/117_mcp_connectors.down.sql`, `server/internal/handler/mcp_connectors_seed.json`
- Test: `server/internal/handler/mcp_connectors_seed_test.go`

**Steps:**
1. Write failing Go test that parses the embedded `mcp_connectors_seed.json` into the connector struct and asserts ≥1 entry with non-empty `slug`, `mcp_template`, `input_schema`. Expected error: build fails / `embed` file missing.
2. Run `cd server && go test ./internal/handler/ -run TestMcpConnectorSeed`, confirm fail.
3. Write migration 117: `mcp_connectors(id uuid pk, workspace_id uuid null references workspaces, slug text, name text, icon text, description text, popularity int, input_schema jsonb, mcp_template jsonb, created_by uuid null, created_at, updated_at)`; unique `(workspace_id, slug)`; index on `workspace_id`. Author seed JSON with real curated entries (GitHub, Slack, Notion, Figma, Atlassian, Gmail, Microsoft 365) including real `input_schema` + `mcp_template` placeholders. Add `//go:embed mcp_connectors_seed.json`.
4. Run `make migrate-up` then the test, confirm pass.
5. Commit: `feat(mcp): add mcp_connectors table + curated seed`

**Verification:**
- [ ] `make migrate-up` and `make migrate-down` both succeed
- [ ] Seed parses; entries have real templates (no stubs)
- [ ] `go test ./internal/handler/ -run TestMcpConnectorSeed` passes

#### Phase M2: sqlc queries

**Estimated time:** ~10 min
**Files:**
- Create: `server/pkg/db/queries/mcp_connector.sql`
- Generated: `server/pkg/db/*` (via `make sqlc`)
- Test: `server/pkg/db/mcp_connector_test.go`

**Steps:**
1. Write failing Go test exercising `ListMcpConnectors(ws)` returns global (`workspace_id IS NULL`) ∪ workspace rows. Expected error: method undefined.
2. Run `cd server && go test ./pkg/db/ -run TestMcpConnector`, confirm fail.
3. Write queries: `ListMcpConnectors` (`workspace_id IS NULL OR workspace_id = $1`), `GetMcpConnector`, `CreateMcpConnector`, `UpdateMcpConnector` (custom only), `DeleteMcpConnector` (custom only). Run `make sqlc`.
4. Run test, confirm pass.
5. Commit: `feat(mcp): sqlc queries for mcp_connectors`

**Verification:**
- [ ] `make sqlc` clean (no drift)
- [ ] List returns global ∪ workspace
- [ ] `go test ./pkg/db/ -run TestMcpConnector` passes

#### Phase M3: Go handler + routes (admin-gated writes)

**Estimated time:** ~15 min
**Files:**
- Create: `server/internal/handler/mcp_connector.go`, `server/internal/handler/mcp_connector_test.go`
- Modify: router registration
**Steps:**
1. Write failing handler test: `GET /api/workspaces/{ws}/mcp-connectors` returns seeded list (200); `POST` as non-admin → 403; `POST` with invalid UUID path → 400. Expected error: route 404.
2. Run `go test ./internal/handler/ -run TestMcpConnector`, confirm fail.
3. Implement handlers: list (global∪workspace), create/update/delete custom (membership/admin gate via existing role check), all UUID path params via `parseUUIDOrBadRequest`, writes use resolved `entity.ID`. Register routes.
4. Run test, confirm pass.
5. Commit: `feat(mcp): connector catalog endpoints with admin gating`

**Verification:**
- [ ] `pnpm`/`go vet` + `gofmt` clean
- [ ] Non-admin write → 403; bad UUID → 400 (per handler convention)
- [ ] List filters by `workspace_id`
- [ ] `go test ./internal/handler/ -run TestMcpConnector` passes

#### Phase M4: API client method + zod schema + malformed test

**Estimated time:** ~12 min
**Files:**
- Modify: `packages/core/api/client.ts`, `packages/core/api/schema.ts` (or new `packages/core/api/mcp-connector-schema.ts`)
- Test: `packages/core/api/mcp-connector.test.ts`
**Steps:**
1. Write failing test feeding a malformed response (missing `slug`, `input_schema: null`) through the schema; assert `parseWithFallback` returns the fallback (empty list) and does NOT throw. Expected error: method/schema undefined.
2. Run `pnpm --filter @multica/core exec vitest run api/mcp-connector.test.ts`, confirm fail.
3. Add zod `McpConnectorSchema` + `listMcpConnectors`/`createMcpConnector`/`updateMcpConnector`/`deleteMcpConnector` on the client, each list/get parsed via `parseWithFallback` with explicit fallback. No bare `as` casts.
4. Run test, confirm pass.
5. Commit: `feat(core): mcp connector api client + zod schema`

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] Malformed response → fallback, no throw (per API Response Compatibility)
- [ ] No bare `as` on response bodies
- [ ] vitest passes

#### Phase M5: Directory modal UI (search / filter / sort / cards)

**Estimated time:** ~15 min
**Files:**
- Create: `packages/views/agents/components/mcp/connector-directory.tsx`, `.test.tsx`
**Steps:**
1. Write failing test: modal renders connector cards from a mocked `@multica/core/api` list, search filters by name, sort reorders by popularity. Expected error: component undefined.
2. Run vitest, confirm fail.
3. Build the Directory dialog (reuse `packages/ui` dialog + command/input primitives) matching the reference: search bar, filter, sort, responsive card grid, `+` per card. Data via TanStack Query keyed on `wsId`. Semantic tokens only.
4. Run tests, confirm pass.
5. Commit: `feat(mcp): connector directory modal`

| Phase | Code Deliverable | Design Deliverable | Verification |
|---|---|---|---|
| M5 | `ConnectorDirectory` modal | Layout spec from reference screenshot (card grid, search/filter/sort, empty state) — run `gaspol-design` | Design-system compliance + tests |

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] Cards render from real query; search + sort work
- [ ] Query keyed on `wsId`; no server data copied into Zustand
- [ ] No hardcoded colors; overflow/truncation handled
- [ ] vitest passes

#### Phase M6: Schema-driven add form + merge into agent mcp_config

**Estimated time:** ~14 min
**Files:**
- Create: `packages/views/agents/components/mcp/connector-config-form.tsx`, `.test.tsx`
- Modify: `packages/views/agents/components/tabs/mcp-config-tab.tsx`
**Steps:**
1. Write failing test: rendering a connector with `input_schema` produces the matching fields; submit merges the rendered `mcp_template` into existing `mcpServers` and calls `onSave({ mcp_config })` without dropping prior servers. Expected error: component undefined.
2. Run vitest, confirm fail.
3. Implement schema-driven form (fields from `input_schema`), substitute values into `mcp_template`, deep-merge into current `mcp_config.mcpServers`, call existing `onSave`. Add a "Browse connectors" button to `mcp-config-tab.tsx` opening the Directory; relabel the raw textarea as "Advanced (JSON)".
4. Run tests, confirm pass.
5. Commit: `feat(mcp): schema-driven connector add merges into agent config`

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] Adding a connector preserves existing `mcpServers` entries
- [ ] Save flows through existing `api.updateAgent` (no new agent endpoint)
- [ ] Raw JSON editor still reachable as Advanced
- [ ] vitest passes

#### Phase M7: Workspace-custom connector authoring (admin)

**Estimated time:** ~12 min
**Files:**
- Create: `packages/views/agents/components/mcp/custom-connector-form.tsx`, `.test.tsx`
- Modify: `connector-directory.tsx` (add "Add custom" entry, admin-only)
**Steps:**
1. Write failing test: admin sees "Add custom connector"; submitting calls `api.createMcpConnector`; non-admin does not see the entry. Expected error: control not found.
2. Run vitest, confirm fail.
3. Implement custom-connector form (name, icon, description, `input_schema`, `mcp_template`) → `api.createMcpConnector`; gate visibility on workspace role. Invalidate the connectors query on success.
4. Run tests, confirm pass.
5. Commit: `feat(mcp): workspace-custom connector authoring (admin)`

**Verification:**
- [ ] `pnpm typecheck` passes
- [ ] Non-admin cannot see/POST custom connectors (UI + 403 already enforced server-side)
- [ ] New custom connector appears in Directory after invalidation
- [ ] vitest passes

---

## Final Verification (whole plan)

- [ ] `make check` green (typecheck, TS unit, Go tests, E2E)
- [ ] `make sqlc` and reserved-slug-style generators show no drift
- [ ] No placeholder/TODO/stub in any new file
- [ ] Two ADRs captured (`gaspol-adr`): MCP catalog source = seeded+custom; theme = accent-axis
- [ ] `graphify update .` run after merge

## Execution Handoff

- **Option 1 — This session:** start Phase S1 with `gaspol-execute` (per-phase checkpoints + TDD gate).
- **Option 2 — Parallel:** the three features are independent — `gaspol-parallel` (mode: plan-phases) can run Feature 1 / Feature 2 / Feature 3 concurrently (Feature 3 phases M1→M7 are sequential within the feature). Use `gaspol-worktree` to isolate.
- **Option 3 — New session:** this file has full context; resume anytime.
