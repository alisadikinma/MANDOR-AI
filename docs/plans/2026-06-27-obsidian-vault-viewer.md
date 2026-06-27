# Obsidian Vault Viewer (read-only)

> Brainstorm: 2026-06-27. View Obsidian vault notes directly inside the Multica UI.

## Design

### Decisions (from brainstorm)

| Question | Decision |
|---|---|
| Data source | **Go backend reads a configured `VAULT_PATH`** and serves notes to web + desktop. Vault must be reachable by the server (local `make server` reads `/Users/alisadikin/Drive-D/Obsidian-Vault`; deployment = per-deploy `VAULT_PATH` env). |
| Scope | **Read-only.** No edit/write — zero conflict handling, zero data-loss risk. Editing stays in Obsidian. |
| Placement | **New full-page workspace route `/:slug/vault`** + a sidebar nav entry. Two-pane (tree + content). |
| v1 features | Wikilink `[[...]]` clickable · folder tree + search · frontmatter rendered · images/`![[...]]` embeds. |

### Architecture

Backend reads one global `VAULT_PATH` and serves it read-only via API. Vault stays on disk. Gating: any workspace member may read (v1 = single global vault; multi-vault per-workspace is future).

### Backend — `server/internal/handler/vault.go`

Four read-only endpoints under `/api/workspaces/{id}/vault` (behind `RequireWorkspaceMemberFromURL`):

| Endpoint | Function |
|---|---|
| `GET .../vault/tree` | Walk `VAULT_PATH`, return folder + `.md` file tree |
| `GET .../vault/note?path=` | Raw markdown split into `{ frontmatter, body }` |
| `GET .../vault/file?path=` | Serve binary (images / embeds) |
| `GET .../vault/search?q=` | Search by filename + content |

- **Config**: add `VaultPath string` to the `Config` struct (`server/internal/handler/handler.go`); read `os.Getenv("VAULT_PATH")` in `server/cmd/server/router.go` (same pattern as existing config fields).
- **Path-traversal guard (trust boundary — MUST NOT be skipped)**: every incoming `path` is `filepath.Clean`-ed and verified to resolve *inside* `VAULT_PATH` (reject `../` escapes). Only hard security requirement.
- **Frontmatter**: parse YAML (`yaml.v3`) → `{ frontmatter: {...}, body: "..." }`.
- **Empty `VAULT_PATH`**: endpoints return empty/404 and the sidebar entry is hidden.

### Frontend

- New shared view in `packages/views/vault/` (web + desktop): 2-pane layout — left tree + search, right content.
- **Markdown**: reuse `ReadonlyContent` (`packages/views/editor/readonly-content.tsx`, react-markdown — **no new dependency**).
  - **Wikilink `[[Note]]`**: regex-transform to a clickable link that navigates within the viewer (resolve name → path).
  - **Embed `![[img]]` / local images**: point `src` at the `vault/file` endpoint.
- **Frontmatter**: render as a small metadata header (tags → chips).
- **Sidebar**: add a "Vault" entry (icon `BookOpen`) in `packages/views/layout/app-sidebar.tsx`.
- **API client**: new methods in `packages/core/api/client.ts` + zod schemas + `parseWithFallback` (per API Response Compatibility rules in CLAUDE.md).
- **Routing**: `apps/web/app/[workspaceSlug]/vault/page.tsx` + desktop `apps/desktop/src/renderer/src/routes.tsx`.

### Data Integration Map

| Component | Data source | Exists? | Notes |
|---|---|---|---|
| Tree pane | `GET vault/tree` | New handler | Walk dir, real FS |
| Content pane | `GET vault/note` | New handler | Reuse `ReadonlyContent` |
| Image/embed | `GET vault/file` | New handler | Binary serve + path guard |
| Search | `GET vault/search` | New handler | Filename + content grep |
| Markdown render | `ReadonlyContent` | ✅ Exists | react-markdown, no new dep |
| API client/schema | `client.ts` / `schema.ts` | ✅ Pattern exists | parseWithFallback + zod |
| Sidebar nav | `app-sidebar.tsx` | ✅ Exists | +1 entry |

### Reuse references (from codebase exploration)

- Markdown renderer: `packages/views/editor/readonly-content.tsx` (react-markdown + remark-gfm/breaks/math, lowlight).
- Go handler template: `server/internal/handler/comment.go` (`ListComments`) — load/gate, query, `writeJSON`.
- Config loading: `server/internal/handler/handler.go` (`Config` struct) + `server/cmd/server/router.go` (env reads).
- Route wiring: `apps/web/app/[workspaceSlug]/layout.tsx`, `apps/desktop/src/renderer/src/routes.tsx`, sidebar `packages/views/layout/app-sidebar.tsx`.
- API client: `packages/core/api/client.ts` + `schema.ts` (`parseWithFallback`) + `schemas.ts` (zod).

### Out of scope (YAGNI for v1)

Edit/write, graph view, backlinks panel, multi-vault per-workspace, persistent index/caching (direct dir-walk is fine for a personal vault; add an index only if thousands of notes feel slow).

---

## Implementation Plan

> **For Claude:** REQUIRED SKILL: Use gaspol-execute to implement this plan.
> **CRITICAL:** This plan specifies real integrations. During execution,
> NEVER substitute placeholders for real data sources without explicit
> user approval. If a data source doesn't exist yet, STOP and ask.

### Goal

Ship a read-only Obsidian vault viewer inside Multica: a new workspace route `/:slug/vault` with a 2-pane (folder tree + rendered note) UI, backed by a Go API that reads a single configured `VAULT_PATH` from disk. Notes render via the existing `ReadonlyContent` markdown component (no new dependency) with clickable `[[wikilinks]]`, rendered frontmatter, and inline `![[image]]`/local-image embeds. The vault menu auto-hides when `VAULT_PATH` is unset. v1 is read-only and serves one global vault.

### Architecture Context (from CLAUDE.md + codebase)

- **Backend**: Chi router (`server/cmd/server/router.go`), handlers in `server/internal/handler/`, `Config` struct in `handler/handler.go` (env read in `router.go`), `writeJSON` helper, `RequireWorkspaceMemberFromURL(queries, "id")` middleware for `/api/workspaces/{id}/...` subtree.
- **Path-traversal is a trust boundary** — CLAUDE.md "When NOT to be lazy": never cut trust-boundary validation. Every `path` query param MUST be cleaned + confined to `VAULT_PATH`.
- **Markdown**: reuse `ReadonlyContent` — `packages/views/editor/readonly-content.tsx` (props: `content: string`, `className?`, `attachments?`). react-markdown already installed.
- **API client**: `packages/core/api/client.ts` — path-per-call, `X-Workspace-ID` header set in private `fetch<T>()`. Responses MUST go through `parseWithFallback` (`packages/core/api/schema.ts`) with a zod schema from `packages/core/api/schemas.ts`. No bare `as` casts (CLAUDE.md API Response Compatibility).
- **Server state** = TanStack Query only (CLAUDE.md State Management). Vault queries key on `wsId`. No Zustand for server data.
- **Nav**: `packages/views/layout/app-sidebar.tsx` — nav items are arrays of `{ key, labelKey, icon }`, paths from the `p` helper (`packages/core/paths/`), labels from `packages/views/locales/<lang>/layout.json` under `nav`.
- **Routes**: web one-liner re-export `apps/web/app/[workspaceSlug]/(dashboard)/<route>/page.tsx`; desktop `apps/desktop/src/renderer/src/routes.tsx` (`{ path, element, handle:{title} }`).

### Tech Stack

Go (Chi, sqlc — though vault touches no DB), `gopkg.in/yaml.v3` for frontmatter (verify it's already a server dep; if not, `go get`). Frontend: React + TanStack Query + zod + existing `@multica/ui`/`@multica/views` primitives + `ReadonlyContent`. Tests: Go `httptest`, Vitest (`packages/core` node, `packages/views` jsdom).

### Data Integration Map

| Feature | Data Source | Hook/API | Exists? | Action |
|---|---|---|---|---|
| Vault menu visibility | `GET /api/workspaces/{id}/vault/status` → `{enabled}` | `useVaultStatus(wsId)` | No | Create endpoint + hook |
| Folder tree | `GET .../vault/tree` (walk `VAULT_PATH`) | `useVaultTree(wsId)` | No | Create endpoint + hook |
| Note content | `GET .../vault/note?path=` → `{frontmatter, body}` | `useVaultNote(wsId, path)` | No | Create endpoint + hook |
| Image/embed | `GET .../vault/file?path=` (binary) | `<img src>` → client `vaultFileUrl()` | No | Create endpoint + url helper |
| Search | `GET .../vault/search?q=` | `useVaultSearch(wsId, q)` | No | Create endpoint + hook |
| Markdown render | `ReadonlyContent` | `packages/views/editor/` | Yes | Use existing |
| `[[wikilink]]` resolve | tree/note response | client-side transform | No | Create transform util |
| Sidebar nav item | `app-sidebar.tsx` arrays + `p` paths | existing pattern | Yes | Add 1 entry (gated) |
| i18n labels | `locales/*/layout.json` `nav` | existing | Yes | Add `vault` key per lang |

**Path guard contract:** `safeVaultPath(root, rel) (string, error)` — `filepath.Clean`, join, then verify the result has `root` as prefix (after `filepath.Abs`/symlink-eval); return error on escape. Every endpoint that takes `path` calls it first; on error → 400. This is the single non-negotiable security unit.

---

### Phase A: Backend — config + path-traversal guard (security core)

**Estimated time:** 12 min

**Files:**
- Modify: `server/internal/handler/handler.go` (add `VaultPath string` to `Config`)
- Modify: `server/cmd/server/router.go` (read `os.Getenv("VAULT_PATH")`)
- Create: `server/internal/handler/vault.go` (`safeVaultPath` helper)
- Test: `server/internal/handler/vault_test.go`

**Steps:**
1. Write failing test for `safeVaultPath`. Expected error: `undefined: safeVaultPath`. Cases: normal subpath resolves inside root; `../etc/passwd` returns error; absolute path outside root returns error; symlink escape returns error; empty root returns error.
2. Run `cd server && go test ./internal/handler/ -run TestSafeVaultPath`, confirm it fails to compile (function undefined).
3. Implement `safeVaultPath(root, rel string) (string, error)` in `vault.go` using `filepath.Clean` + `filepath.Abs` + `strings.HasPrefix(abs, root+string(os.PathSeparator))`.
4. Add `VaultPath string` to `Config` (`handler.go`); set `VaultPath: strings.TrimSpace(os.Getenv("VAULT_PATH"))` in `router.go` config block.
5. Run the test, confirm all cases pass.
6. Commit: `feat(vault): add VAULT_PATH config + path-traversal guard`.

**Verification:**
- [ ] `go build ./...` passes
- [ ] `safeVaultPath` rejects `../`, absolute-outside, and symlink-escape; accepts valid subpaths
- [ ] Security: traversal blocked at the helper; no endpoint touches the FS without it
- [ ] No placeholder/TODO comments in new code

---

### Phase B: Backend — `status` + `tree` endpoints

**Estimated time:** 14 min

**Files:**
- Modify: `server/internal/handler/vault.go` (`GetVaultStatus`, `GetVaultTree`)
- Test: `server/internal/handler/vault_test.go`

**Steps:**
1. Write failing test for `GetVaultTree` over a temp dir fixture (a few `.md` files in nested folders). Expected error: `testHandler.GetVaultTree undefined`. Assert JSON tree shape `{ name, path, type, children[] }`, only `.md` + dirs included, sorted.
2. Run `go test ./internal/handler/ -run TestGetVaultTree`, confirm compile failure.
3. Implement `GetVaultStatus` → `{ enabled: VaultPath != "" }`. Implement `GetVaultTree`: if `VaultPath==""` return 404; else `filepath.WalkDir` building the tree (dirs + `.md` only), return via `writeJSON`. Use a temp `VaultPath` on `testHandler` for the test (inject config field).
4. Run tests, confirm pass.
5. Commit: `feat(vault): status + tree endpoints`.

**Verification:**
- [ ] `go test ./internal/handler/ -run TestGetVault` passes
- [ ] `tree` returns only dirs + `.md`, hides dotfiles/`.obsidian`
- [ ] `status` returns `enabled:false` when `VaultPath` empty (no FS access)
- [ ] No placeholder/TODO comments

---

### Phase C: Backend — `note` endpoint (frontmatter split)

**Estimated time:** 12 min

**Files:**
- Modify: `server/internal/handler/vault.go` (`GetVaultNote`)
- Test: `server/internal/handler/vault_test.go`

**Steps:**
1. Write failing test for `GetVaultNote` with `?path=` pointing at a note that has YAML frontmatter. Expected error: `testHandler.GetVaultNote undefined`. Assert response `{ path, frontmatter: {tags:[...]}, body }`; assert `?path=../x` → 400; assert missing file → 404.
2. Run `go test ./internal/handler/ -run TestGetVaultNote`, confirm compile failure.
3. Implement: read `path` query param → `safeVaultPath` (400 on error) → `os.ReadFile` (404 on missing) → split leading `---\n...\n---` frontmatter, `yaml.Unmarshal` into `map[string]any`, return `{path, frontmatter, body}`. Verify `gopkg.in/yaml.v3` is in `server/go.mod`; `go get` if absent.
4. Run tests, confirm pass.
5. Commit: `feat(vault): note endpoint with frontmatter parsing`.

**Verification:**
- [ ] `go test ./internal/handler/ -run TestGetVaultNote` passes
- [ ] Frontmatter split correct; note without frontmatter returns `frontmatter:{}` + full body
- [ ] Security: invalid `path` → 400 via `safeVaultPath` before any read
- [ ] No placeholder/TODO comments

---

### Phase D: Backend — `file` (binary embed) + `search` endpoints

**Estimated time:** 14 min

**Files:**
- Modify: `server/internal/handler/vault.go` (`GetVaultFile`, `SearchVault`)
- Test: `server/internal/handler/vault_test.go`

**Steps:**
1. Write failing test for `GetVaultFile` serving a small PNG fixture (assert 200 + `Content-Type: image/png` + bytes) and for `SearchVault?q=` (assert matches by filename + body substring, returns `[{name,path,snippet}]`). Expected error: handlers undefined.
2. Run `go test ./internal/handler/ -run 'TestGetVaultFile|TestSearchVault'`, confirm compile failure.
3. Implement `GetVaultFile`: `safeVaultPath` (400) → `http.ServeFile`/`http.ServeContent` with detected content-type (404 missing). Implement `SearchVault`: walk `.md`, case-insensitive match on filename + content, cap results (e.g. 50), return snippets.
4. Run tests, confirm pass.
5. Commit: `feat(vault): file serving + search endpoints`.

**Verification:**
- [ ] `go test ./internal/handler/ -run 'TestGetVaultFile|TestSearchVault'` passes
- [ ] Security: `file`/`search` paths confined by `safeVaultPath`; binary served with correct content-type; no directory listing leak
- [ ] Search result count capped; empty `q` returns empty list, not full vault
- [ ] No placeholder/TODO comments

---

### Phase E: Backend — register `/vault` subtree in router

**Estimated time:** 6 min

**Files:**
- Modify: `server/cmd/server/router.go`

**Steps:**
1. Write/extend a router test (or reuse handler tests) asserting `GET /api/workspaces/{id}/vault/status` is reachable and member-gated (401/403 without membership). Expected initial failure: 404 route not found.
2. Inside the existing `r.Route("/api/workspaces", ...)` → `/{id}` member group (`RequireWorkspaceMemberFromURL(queries,"id")`), add `r.Route("/vault", ...)` with `Get("/status",...)`, `Get("/tree",...)`, `Get("/note",...)`, `Get("/file",...)`, `Get("/search",...)`.
3. Run the test, confirm route resolves + is gated.
4. Commit: `feat(vault): register member-gated /vault routes`.

**Verification:**
- [ ] Routes reachable under `/api/workspaces/{id}/vault/*`
- [ ] Security: all vault routes inside the `RequireWorkspaceMemberFromURL` group (non-members blocked)
- [ ] `go build ./...` + handler tests pass

---

### Phase F: Frontend — API client methods + zod schemas

**Estimated time:** 14 min

**Files:**
- Modify: `packages/core/api/schemas.ts` (add `VaultTreeNodeSchema`, `VaultNoteSchema`, `VaultStatusSchema`, `VaultSearchResultSchema` + list schemas, all `.loose()`)
- Modify: `packages/core/api/client.ts` (`getVaultStatus`, `getVaultTree`, `getVaultNote`, `searchVault`, `vaultFileUrl`)
- Test: `packages/core/api/vault.test.ts`

**Steps:**
1. Write failing test that feeds a **malformed** response (missing field, wrong type, null array) through each schema via `parseWithFallback` and asserts the safe fallback (CLAUDE.md mandates a malformed-response test per endpoint). Expected error: schemas/methods undefined.
2. Run `pnpm --filter @multica/core exec vitest run api/vault.test.ts`, confirm fail.
3. Implement schemas + fallback constants (`EMPTY_VAULT_TREE`, etc.). Implement client methods using the private `fetch<unknown>` + `parseWithFallback`. `vaultFileUrl(wsId, path)` returns an absolute URL to the `file` endpoint (for `<img src>`); include workspace routing consistent with how the client builds URLs.
4. Run tests, confirm pass.
5. Commit: `feat(vault): api client methods + zod schemas`.

**Verification:**
- [ ] `pnpm --filter @multica/core exec vitest run api/vault.test.ts` passes
- [ ] Every method runs through `parseWithFallback`; no bare `as` on response bodies
- [ ] Malformed-response test returns fallback, never throws
- [ ] `pnpm typecheck` passes for `@multica/core`

---

### Phase G: Frontend — query hooks + wikilink/image transforms

**Estimated time:** 12 min

**Files:**
- Create: `packages/core/vault/use-vault.ts` (`useVaultStatus`, `useVaultTree`, `useVaultNote`, `useVaultSearch` — TanStack Query, keyed on `wsId`)
- Create: `packages/core/vault/wikilinks.ts` (`transformWikilinks(md, resolve)` + `rewriteEmbeds(md, vaultFileUrl)`)
- Test: `packages/core/vault/wikilinks.test.ts`

**Steps:**
1. Write failing test for `transformWikilinks`: `[[Note Name]]` → markdown link with a `vault:`-scheme href (or resolved path); `[[Note|alias]]` keeps alias; `![[img.png]]` → `![](<vaultFileUrl>)`. Expected error: function undefined.
2. Run `pnpm --filter @multica/core exec vitest run vault/wikilinks.test.ts`, confirm fail.
3. Implement transforms (regex-based, pure functions). Implement hooks wrapping the client methods; `useVaultNote` keyed `["vault","note",wsId,path]`; queries disabled when `wsId`/`path` absent.
4. Run tests, confirm pass.
5. Commit: `feat(vault): query hooks + wikilink/embed transforms`.

**Verification:**
- [ ] `pnpm --filter @multica/core exec vitest run vault/` passes
- [ ] Hooks key on `wsId` (workspace switch swaps cache); no server data copied into Zustand
- [ ] Transforms are pure + cover alias + embed cases
- [ ] `packages/core` stays react-dom/localStorage/process.env-free (Package Boundary Rules)

---

### Phase H: Frontend — VaultPage 2-pane view (UI)

**Estimated time:** 15 min

| Phase | Code Deliverable | Design Deliverable | Verification |
|---|---|---|---|
| H | `VaultPage` (tree pane + content pane) | List/detail 2-pane using existing shell tokens — `bg-background`/`text-muted-foreground`, sidebar-style tree, `ReadonlyContent` for body, frontmatter as `Badge` chips; overflow handled (tree scrolls, body scrolls), empty + loading states | Design-system compliance + tests |

**Files:**
- Create: `packages/views/vault/vault-page.tsx`, `packages/views/vault/vault-tree.tsx`, `packages/views/vault/note-meta.tsx`, `packages/views/vault/index.ts`
- Test: `packages/views/vault/vault-page.test.tsx`

**Steps:**
1. Write failing test (jsdom, `@testing-library/react`, mock `@multica/core` hooks) asserting: tree renders from `useVaultTree`; clicking a file calls `useVaultNote` and renders body via `ReadonlyContent`; frontmatter tags render as chips. Expected error: `VaultPage` undefined.
2. Run `pnpm --filter @multica/views exec vitest run vault/vault-page.test.tsx`, confirm fail.
3. Implement `VaultPage`: left `VaultTree` (collapsible folders, search box bound to `useVaultSearch`), right pane = `NoteMeta` (frontmatter chips) + `ReadonlyContent content={transformWikilinks(rewriteEmbeds(body))}`. Wikilink click → select that note path (local UI state via `useState`, not a store — ephemeral selection). Use design tokens only; truncate long names; handle loading/empty.
4. Run tests, confirm pass.
5. Commit: `feat(vault): 2-pane vault viewer page`.

**Verification:**
- [ ] `pnpm --filter @multica/views exec vitest run vault/` passes
- [ ] Renders real data from core hooks (no mock/placeholder content); reuses `ReadonlyContent`
- [ ] Design: semantic tokens only (no hardcoded colors), overflow/truncation handled, loading + empty states present
- [ ] No `next/*` or `react-router-dom` imports (Package Boundary Rules)
- [ ] No placeholder/TODO comments

---

### Phase I: Frontend — path, i18n label, gated sidebar nav

**Estimated time:** 10 min

**Files:**
- Modify: `packages/core/paths/` (add `vault: (slug) => '/{slug}/vault'` to the `p` helper)
- Modify: `packages/views/locales/en/layout.json` + `zh-Hans/layout.json` (add `nav.vault`)
- Modify: `packages/views/layout/app-sidebar.tsx` (add `{ key:"vault", labelKey:"vault", icon: BookOpen }` to `workspaceNav`, gated by `useVaultStatus`)
- Test: `packages/views/layout/app-sidebar.test.tsx` (extend if exists)

**Steps:**
1. Write failing test: sidebar shows the Vault item when `useVaultStatus` → `{enabled:true}` and hides it when `{enabled:false}`. Expected error: assertion fails (item always shown / missing).
2. Run `pnpm --filter @multica/views exec vitest run layout/app-sidebar.test.tsx`, confirm fail.
3. Add the path, locale keys (EN + zh per Conventions), and nav entry. Gate visibility on `useVaultStatus(wsId)?.enabled === true` (explicit boolean per API Compatibility rules).
4. Run tests + check both locales present (CI glossary/translation rules).
5. Commit: `feat(vault): sidebar nav entry + i18n, gated on vault status`.

**Verification:**
- [ ] `pnpm --filter @multica/views exec vitest run layout/` passes
- [ ] Vault item hidden when `enabled !== true`; visible when `true`
- [ ] EN + zh-Hans `nav.vault` present (no missing-translation drift)
- [ ] Uses `p` paths helper + `AppLink` (no framework link APIs)

---

### Phase J: Wire routes (web + desktop)

**Estimated time:** 6 min

**Files:**
- Create: `apps/web/app/[workspaceSlug]/(dashboard)/vault/page.tsx`
- Modify: `apps/desktop/src/renderer/src/routes.tsx`

**Steps:**
1. Web: create `page.tsx` = `export { VaultPage as default } from "@multica/views/vault";`.
2. Desktop: add `{ path: "vault", element: <VaultPage />, handle: { title: "Vault" } }` under the `:workspaceSlug` children, import `VaultPage` from `@multica/views/vault`.
3. Run `pnpm typecheck`.
4. Commit: `feat(vault): wire /vault route on web + desktop`.

**Verification:**
- [ ] `pnpm typecheck` passes (web + desktop)
- [ ] Web `/{slug}/vault` and desktop vault tab both render `VaultPage` (identical shared view, No-Duplication Rule)
- [ ] `make check` green end-to-end

---

### Execution notes

- Phases A→E (backend) and F→J (frontend) are mostly sequential within each track, but the **backend track (A–E) and the early frontend schema/transform work are independent** until Phase F needs the live API. Backend A–E can run as one focused pass; frontend F–J after.
- Manual smoke: set `VAULT_PATH=/Users/alisadikin/Drive-D/Obsidian-Vault`, `make server`, open `/{slug}/vault`, confirm tree loads, a note with `[[links]]` + an image renders, search works. Unset `VAULT_PATH` → menu disappears.
- Per CLAUDE.md, run `make check` before considering done; update `graphify update .` after.
