# Writing Agent → Google Drive (native Google Docs, auto-save)

## Design

### Goal
The "technical writer" (the **Writing Agent** template) should save every document it
produces to Google Drive **as a Google Doc** (downloadable as `.docx` on demand).

### Decisions (from brainstorm)
- **Approach:** MCP connector — reuse the existing connector/runtime-MCP infrastructure, no
  new backend Go code, no docx-generation pipeline.
- **"docx" means:** a native, editable **Google Doc** (File → Download → `.docx` anytime).
  No docx-binary generation in the pipeline. *(If literal `.docx`-in-Drive is ever wanted,
  that's a separate conversion step — upgrade path, not built.)*
- **Trigger:** auto on **every** document, enforced at the **prompt level** (agent
  instructions). Hard, guaranteed auto would need a backend post-run hook — **not built**
  (YAGNI; the directive covers the stated need).

### MCP server chosen
`google-workspace-mcp` (npm, npx, stdio) — OAuth 2.0, supports **creating Google Docs**
(`https://www.googleapis.com/auth/documents` + Drive scopes), `serve` subcommand,
one-time host OAuth via its own CLI. Mirrors the existing **Gmail** connector pattern.

**Alternative considered — Google's official remote MCP**
`https://drivemcp.googleapis.com/mcp/v1` (HTTP, scopes `drive.readonly` + `drive.file`).
Rejected as primary because it needs a **manually-created GCP OAuth client per deployment**
(no Dynamic Client Registration), so it does not fit the field-less remote-OAuth slot the
way Notion/Figma do.
Sources: <https://developers.google.com/workspace/drive/api/guides/configure-mcp-server>,
<https://developers.google.com/workspace/guides/configure-mcp-servers>

### Architecture finding (important)
Two MCP models coexist in the repo:
- **Older:** connector seed (`mcp_connectors_seed.json`) → `mcp_template` deep-merged into an
  agent's `mcp_config` (`server/internal/handler/mcp_connector.go:16`).
- **Newer (current):** MCP servers are configured on the **runtime host** (`~/.claude.json` /
  `~/.codex/config.toml`), reported via heartbeat; an agent's `mcp_config` is just a
  **deny-list** over that pool (`packages/views/agents/components/tabs/mcp-config-tab.tsx:11`,
  read-only mirror in `runtime-mcp-panel.tsx`).
- The seed brand-icon strings (`figma`, `atlassian`, `microsoft`, …) have **no consuming
  frontend component** — the connector catalog UI is not present yet.

**Therefore the functional path today is the runtime host pool + agent instructions.** The
seed entry is forward-looking catalog data (inert until the catalog UI ships).

### Data Integration Map
| Component | Data Source | Existing? | Notes |
|---|---|---|---|
| Doc creation | `google-workspace-mcp` tools (create Doc) | 🌐 external (Google) | agent calls MCP tool |
| MCP server availability | runtime host pool (`~/.claude.json`) | ✅ | **operational step**, see below |
| Auto-save-every-doc | Writing Agent template instructions | ✅ `onboarding.json` (4 locales) | prompt-level directive |
| Connector catalog entry | `mcp_connectors_seed.json` | ✅ backend | added; no catalog UI yet |

## Implementation (done)
1. **Writing Agent instructions** — appended a "save to Google Drive as a Google Doc" directive,
   all 4 locales: `packages/views/locales/{en,ja,ko,zh-Hans}/onboarding.json` (`step_agent.templates.writing.instructions`).
   Affects **newly created** agents from the template; for an existing Writing Agent, edit its
   instructions in agent settings.
2. **Seed catalog entry** — added `google-drive` connector to
   `server/internal/handler/mcp_connectors_seed.json` (field-less, `google-workspace-mcp` via npx).
   Passes `mcp_connectors_seed_test.go`.

## Operational step — turn it on for a runtime (required to actually work)
On the runtime host (the daemon/cloud runtime the Writing Agent runs on):

```bash
# 1. One-time OAuth (creates ~/.google-mcp/credentials.json; needs a GCP project +
#    OAuth consent screen with Docs + Drive scopes — see the package README)
npx google-workspace-mcp setup
npx google-workspace-mcp accounts add personal

# 2. Register the server in the host MCP pool, e.g. ~/.claude.json:
#    "mcpServers": {
#      "google-workspace": { "command": "npx", "args": ["-y", "google-workspace-mcp", "serve"] }
#    }
```

Then the server appears in the runtime's MCP panel; the Writing Agent will create Google Docs
and reply with links.

## Open / upgrade paths (not built)
- **Hard auto-save guarantee** (backend post-run hook intercepting document outputs).
- **Literal `.docx` files in Drive** (markdown/HTML→docx conversion step).
- **Valid brand icon** for the `google-drive` seed entry once a connector catalog UI exists.
