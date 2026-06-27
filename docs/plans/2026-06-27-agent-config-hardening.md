# Agent Config Hardening — routing, MCP least-privilege, plugins, Scrum

> Date: 2026-06-27 · Status: **Design locked, ready to execute** · Follow-on to `2026-06-27-multi-model-agent-routing.md`
> Scope: audit + fix all 14 active MANDOR-AI agents + 4 squads. Pure config/ops (DB + `~/.claude.json` + daemon restart). No code/schema change.

## Design

### Audit findings (ground-truth, 2026-06-27)

| # | Finding | Severity |
|---|---|---|
| A | **4 agents mis-wired**: Project Manager, Security Reviewer, ML/CV Engineer, Mobile Dev have `model=NULL` + `custom_env={}` → fall through to the **real Anthropic API + Claude default**, not Ollama/matrix. Project Manager is a *squad leader* with no model. | BLOCKING |
| B | **Code Reviewer `mcp_config=NULL`** → inherits the **entire 14-server pool** (incl. `ssh-prod-vps`, image/video-gen). The "code reviewer MCP belum benar" issue. | High |
| C | **Deny-lists inconsistent / not least-privilege** — e.g. `obsidian` wrongly denied for UI/UX (breaks its grounding skill); `context7`/`github` arbitrary across coders. | Medium |
| D | **Sprint Scrum entirely absent** — all 4 squad `instructions` empty (len 0); 0/14 agent personas mention DoR/DoD/standup/handoff. | High |
| E | **context-mode MCP tools unreachable** — `claude.go` passes `--strict-mcp-config`, so plugin-provided MCP servers are ignored; agents get context-mode's *hook* but not `ctx_*` tools. | Medium |
| F | **Cleanup** — Product Manager `--settings multica-agent-hooks.json` only adds a leftover debug echo (`PM_HOOK_FIRED`) + a duplicate `rtk hook claude`. | Low |

### Architecture facts that shape the fix

- **Daemon launches `claude` with HOME untouched** (`execenv.go:407` — only Codex gets a per-task HOME). So every agent inherits `~/.claude/settings.json` → **`enabledPlugins` + global hooks apply to all agents automatically**. Plugins are **global, not per-agent**.
  - **ponytail** ✅ enabled globally + embedded in every persona.
  - **rtk** ✅ global `PreToolUse` Bash hook `rtk hook claude`.
  - **context-mode** ✅ hooks fire; ⚠️ MCP tools suppressed by `--strict-mcp-config` (Finding E).
- **MCP pool** = top-level `~/.claude.json → mcpServers` only (`mcp_pool.go`). Per-agent `mcp_config` is a **deny-list** `{"disabledMcpServers":[...]}`; effective = pool − deny-list (`mcp_effective.go`). Adding to pool is opt-OUT.
- **`squad.instructions` = leader briefing content** (not member prompts) per `multica-squads` skill. Scrum coordination policy belongs here + in leader personas; the **leader enforces DoR/DoD on members**.
- **Write path**: direct DB (`docker exec -i multica-postgres-1 psql -U multica -d multica`) — CLI mis-points to :8080 (LabelStudio). Daemon reads agent config from DB per-task; **pool change (`~/.claude.json`) needs a daemon restart**.

### Decisions (locked with user 2026-06-27)

1. **Model mapping = recommended matrix**: Project Manager → `claude-opus-4-8`; Security Reviewer → `glm-5.2:cloud`; ML/CV Engineer → `glm-5.2:cloud`; Mobile Dev → `qwen3.6:35b`.
2. **Scrum = squads + leaders (full)**: populate all 4 `squad.instructions` + add Scrum block to the 3 leader personas (Product Manager, Project Manager, QA Reviewer).
3. **MCP = fix all (least-privilege matrix)** below.
4. **context-mode = add to pool** (then deny per role that doesn't need it).

### Model + env target state (14 agents)

| Agent | model | custom_env (base URL) | Note |
|---|---|---|---|
| Product Manager (leader) | `claude-opus-4-8` | `{}` (real Claude) | ✅ unchanged |
| **Project Manager** (leader) | `claude-opus-4-8` | `{}` | **FIX** |
| **Security Reviewer** | `glm-5.2:cloud` | ollama | **FIX** |
| **ML/CV Engineer** | `glm-5.2:cloud` | ollama | **FIX** |
| **Mobile Dev** | `qwen3.6:35b` | ollama | **FIX** |
| System Analyst | `glm-5.2:cloud` | ollama | ✅ |
| Researcher / UI-UX / QA Reviewer | `minimax-m3:cloud` | ollama | ✅ |
| Backend / Frontend / Tech Writer / Code Reviewer / DevOps | `qwen3.6:35b` | ollama | ✅ |

`ollama` env = `{"ANTHROPIC_BASE_URL":"http://localhost:11434","ANTHROPIC_API_KEY":"ollama"}`.

### MCP least-privilege matrix

Pool after adding context-mode (15): `chrome-devtools, context7, context-mode, figma, firecrawl, github, indusia-image-gen, indusia-video-gen, obsidian, playwright, semgrep, ssh-prod-vps, stitch, xberg, youtube-transcript`.
**Universal allow (never deny):** `obsidian` (WHY brain — grounding skill needs it), `context-mode` (session memory), `context7` (docs).

| Agent | ALLOW (beyond universal) | → deny = pool − allow |
|---|---|---|
| Product / Project Manager, System Analyst, Backend, Mobile, ML/CV, Technical Writer | `github` | chrome-devtools, figma, firecrawl, indusia-image-gen, indusia-video-gen, playwright, semgrep, ssh-prod-vps, stitch, xberg, youtube-transcript |
| Frontend Dev | `github, chrome-devtools, playwright` | figma, firecrawl, indusia-image-gen, indusia-video-gen, semgrep, ssh-prod-vps, stitch, xberg, youtube-transcript |
| UI/UX Designer | `figma, stitch, chrome-devtools, playwright, indusia-image-gen` | firecrawl, github, indusia-video-gen, semgrep, ssh-prod-vps, xberg, youtube-transcript |
| Researcher | `github, firecrawl, youtube-transcript, xberg` | chrome-devtools, figma, indusia-image-gen, indusia-video-gen, playwright, semgrep, ssh-prod-vps, stitch |
| DevOps Engineer | `github, ssh-prod-vps` | chrome-devtools, figma, firecrawl, indusia-image-gen, indusia-video-gen, playwright, semgrep, stitch, xberg, youtube-transcript |
| QA Reviewer (leader) | `github, chrome-devtools, playwright, semgrep` | figma, firecrawl, indusia-image-gen, indusia-video-gen, ssh-prod-vps, stitch, xberg, youtube-transcript |
| Security Reviewer, Code Reviewer | `github, semgrep` | chrome-devtools, figma, firecrawl, indusia-image-gen, indusia-video-gen, playwright, ssh-prod-vps, stitch, xberg, youtube-transcript |

`ssh-prod-vps` (prod shell) → **DevOps only**. Image/video/figma/stitch → **UI/UX only** (figma/stitch + image). Browser (chrome-devtools/playwright) → Frontend, UI/UX, QA.

### Scrum template (leader briefing → `squad.instructions`)

Each squad gets a leader-facing coordination policy:
- **Role split**: PO/reporter sets the goal; **leader coordinates, does NOT do the build work**; members are the doers.
- **DoR before delegation**: leader confirms goal, acceptance criteria, and inputs exist before creating/assigning a child issue.
- **Binary DoD** per discipline (code compiles + tests pass + reviewed; design = spec+states; docs = published) — no "half-done".
- **Leader MUST verify** each member's output against DoD **before accepting handoff**; reject + re-delegate if it fails.
- **Heartbeat**: leader re-checks mid-flight issues; a silent member ≠ a done issue.

The 3 leader personas (Product/Project Manager, QA Reviewer) get a condensed version of the same block.

---

## Implementation Plan

> Ops plan — RED→GREEN command gates (no TDD). Each phase: baseline that currently fails → action → verification that passes → rollback.

### Phase 1 — Fix the 4 mis-wired agents (model + env)

**RED:** `SELECT name,model,custom_env FROM agent WHERE archived_at IS NULL AND model IS NULL;` → returns the 4 rows.
**Action (DB):** per the model+env target table — `UPDATE agent SET model=$m, custom_env=$env WHERE name=$n AND archived_at IS NULL;` (Project Manager → opus + `{}`; the other 3 → their tag + ollama env).
**GREEN:** the RED query returns **0 rows**; `SELECT name,model,custom_env FROM agent WHERE name IN (...)` shows correct values.
**Rollback:** `UPDATE ... SET model=NULL, custom_env='{}'`.

### Phase 2 — Add context-mode to the MCP pool

**RED:** `python3 -c "import json;print('context-mode' in json.load(open('~/.claude.json'))['mcpServers'])"` → `False`.
**Action:** add to `~/.claude.json → mcpServers`:
```json
"context-mode": { "command": "node", "args": ["/Users/alisadikin/.claude/plugins/cache/context-mode/context-mode/1.0.151/start.mjs"] }
```
<!-- ponytail: version-pinned path; replace with a stable symlink if the plugin auto-updates and breaks this. -->
**GREEN:** the RED check returns `True`; JSON still parses.
**Rollback:** remove the key.
**Note:** interactive sessions already load context-mode via the plugin → harmless duplicate-name (Claude dedupes); only agents (strict mode) gain the tools.

### Phase 3 — Apply the MCP least-privilege matrix (all 14)

**RED:** `SELECT name FROM agent WHERE archived_at IS NULL AND (mcp_config IS NULL OR NOT (mcp_config ? 'disabledMcpServers'));` → returns ≥ Code Reviewer.
**Action (DB):** per the matrix — `UPDATE agent SET mcp_config=$denylist::jsonb WHERE name=$n AND archived_at IS NULL;` with `{"disabledMcpServers":[...]}` for each of the 14.
**GREEN:** RED returns 0 rows; spot-check Code Reviewer denies ssh-prod-vps + image/video and allows github+semgrep; UI/UX no longer denies obsidian.
**Rollback:** restore prior `mcp_config` values (snapshot before write).

### Phase 4 — Scrum (4 squads + 3 leaders)

**RED:** `SELECT name,length(instructions) FROM squad;` → all len 0; leader personas lack Scrum markers.
**Action:** (a) `UPDATE squad SET instructions=$scrum WHERE name=$n;` for all 4 (leader briefing template). (b) append the condensed Scrum block to Product Manager, Project Manager, QA Reviewer `agent.instructions`.
**GREEN:** all 4 `squad.instructions` non-empty + contain DoR/DoD/handoff; the 3 leader personas match `~* 'definition of (ready|done)|handoff|DoR|DoD'`.
**Rollback:** `UPDATE squad SET instructions=''`; revert the 3 personas (snapshot first).

### Phase 5 — Cleanup Product Manager custom_args

**RED:** `SELECT custom_args FROM agent WHERE name='Product Manager';` → contains the debug `--settings` file.
**Action:** `UPDATE agent SET custom_args='[]'::jsonb WHERE name='Product Manager';` (rtk + ponytail + context-mode already global; the file only added a debug echo + duplicate rtk). Optionally delete `~/.claude/multica-agent-hooks.json` + `~/multica-hook-test.log`.
**GREEN:** `custom_args = []`; no PM_HOOK_FIRED appended to the log on next PM run.
**Rollback:** restore the custom_args array.

### Phase 5b — Squad consolidation (amendment, user-directed 2026-06-27)

The 4-squad layout had a redundant **AI Dev Team** (9-member everything-squad under Product Manager) overlapping the 3 functional squads, and **Code Reviewer** had no home in the functional split.
**Action (DB, one txn):** add Code Reviewer to **Engineering** + **Quality & Security**; cancel the leftover "Routing smoke test" issue + null its squad assignee; `DELETE` the AI Dev Team squad (cascades its members).
**GREEN:** 3 squads remain (Engineering / Product / Quality & Security); Code Reviewer ∈ {Engineering, Quality & Security}; **zero orphan agents**; smoke issue `cancelled`.
**Rollback:** `squad_member` snapshot (`*-squad_members.json`) + squad snapshot restore.

Final structure:
- **Engineering** (lead Project Manager·Opus) — Backend, Frontend, Mobile, ML/CV, DevOps, **Code Reviewer**
- **Product** (lead Product Manager·Opus) — System Analyst, Researcher, UI/UX, Technical Writer
- **Quality & Security** (lead QA Reviewer) — Security Reviewer, **Code Reviewer**

### Phase 6 — Reload + smoke test

**Action:** restart the daemon (picks up the new `~/.claude.json` pool); refresh UI. Assign a small issue to a squad → confirm leader (Opus) delegates, a member runs its pinned model, and `disabledMcpServers` takes effect (e.g. Code Reviewer can't see ssh-prod-vps).
**GREEN:** one local-model + one cloud-model task run via the right role agent; deny-list honored; context-mode `ctx_*` callable by an agent.
**Rollback:** `multica daemon stop`; revert `~/.claude.json`.

### Snapshot before writing (safety)

`COPY (SELECT id,name,model,custom_env,custom_args,mcp_config,instructions FROM agent) TO STDOUT` + squad equivalent → save to scratchpad so every phase is reversible.
