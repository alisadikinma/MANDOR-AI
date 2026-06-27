# Multi-Model Agent Routing — Opus 4.8 orchestrator + Ollama role workers

> Date: 2026-06-27 · Status: **Stack LOCKED** (Opus + 2 cloud + 1 local) · wiring deferred
> Goal: keep **Opus 4.8** as the task orchestrator + complex-work executor; route **simple/bounded** work to Ollama models, picked per software-team role. **Quality is the top priority** — where a local model can't hit the quality bar, the role escalates to a cloud model (and this doc says so explicitly).
>
> **Constraints (2026-06-27):** Ollama cloud capped at 3 models (we use **2**); everything else runs **local** on the MacBook Pro M5 + 64GB. Still planning — no wiring yet.
>
> **Final stack (locked):** **Opus 4.8** (complex/critical) · **Cloud ×2** — `glm-5.2` (generalist) + `minimax-m3` (vision/computer-use) · **Local ×1** — `qwen3.6:35b`. Add-a-model rule: a model earns a slot only by adding a **capability or efficiency the next-cheaper tier lacks**, never just "more power."

### Decisive benchmark (Artificial Analysis Intelligence Index v4.1)

Among all Ollama-available models, **`glm-5.2` is the single strongest** — it beats both other cloud candidates on every composite index, which is why it's the lone cloud *generalist*:

| Model | Intelligence | Coding | Agentic | Vision | Computer-use |
|---|---|---|---|---|---|
| **glm-5.2 (max)** | **51** | **68.8** | **43.1** | ❌ | ❌ |
| minimax-m3 | 44 | 58.6 | 35.4 | ✅ | ✅ |
| kimi-k2.7-code | 42 | 60.8 | 29.6 | ✅ | ❌ |
| *(ceiling) Claude Fable 5* | *60* | *76.5* | *52.8* | — | — |

- **`kimi-k2.7-code` DROPPED** — `glm-5.2` beats it on **all three** indices incl. coding (68.8 vs 60.8). A coding model that loses to the generalist we already run has no slot. (Earlier "SWE-Verified 80.2" was a K2.6 single-bench figure the unified index overturns.)
- **`minimax-m3` KEPT** — *not* for intelligence (it's below glm-5.2) but for the **capability glm-5.2 lacks: vision + computer-use**. It's the only model in the set that can *see* a rendered UI and *drive* a browser → UI/UX & QA.

## Design

### Routing principle (quality-first, 3 tiers)

A task lands in exactly one tier. Pick the **lowest** tier that still meets the quality bar for that role — never trade quality for cost, but don't pay for cloud when a local model is genuinely good enough.

1. **Tier 0 — Opus 4.8 (orchestrator + complex):** multi-agent orchestration/planning, ambiguous greenfield architecture, security-critical review, cross-cutting refactors, anything needing taste/judgement where a wrong call is expensive. *Decided: complex → always Opus.*
2. **Tier 1 — Cloud Ollama (2 models):** high quality needed but bounded. **The two slots:**
   - `glm-5.2` — **the cloud generalist** (top AA scores among Ollama models): System Analyst, Researcher (text), heavy Backend, critical Code Review — every text/coding/agentic need below Opus.
   - `minimax-m3` — **vision + computer-use only** (glm-5.2 can't see/click): UI/UX visual critique, QA E2E that drives and *sees* the browser. Kept for capability, not score.
   - *Dropped:* `kimi-k2.7-code` (beaten by glm-5.2 on all indices) and `nemotron-3-super` (its 1M-retrieval niche is covered by `minimax-m3`). Re-add `nemotron-3-super` as a 3rd slot **only** if you do frequent massive-corpus retrieval research.
3. **Tier 2 — Local Ollama (M5 64GB) — 1 model:** everything routine. `qwen3.6:35b` is the single local default (coding, docs, devops). Zero marginal cost, private, offline. One model = no model-swap thrash in unified memory.

### Model verification (your list → real Ollama tags, June 2026)

All seven names exist in the current Ollama library. Mapping + headline strength + benchmark:

| Your name | Real tag | Tier | Strength (evidence) |
|---|---|---|---|
| glm-5.2:cloud | `glm-5.2` | **cloud ✅ KEEP** | **Strongest Ollama model** (AA 51/68.8/43.1). Cloud generalist for all text/coding/agentic below Opus |
| Minimax-m3:cloud | `minimax-m3` | **cloud ✅ KEEP** | **Vision + computer-use** (70% OSWorld, MCP Atlas 74.2%) — the only "eyes & hands" model. Kept for capability, not score (AA 44/58.6/35.4) |
| Kimi-k2.7-code:cloud | `kimi-k2.7-code` | ❌ **DROP** | Beaten by glm-5.2 on every index incl. coding (60.8 < 68.8). No unique slot |
| Nemotron-3-super:cloud | `nemotron-3-super` | ⏸️ optional | 1M retrieval (RULER@1M 91.8). Re-add only if research/large-corpus heavy |
| qwen3.6 | `qwen3.6:35b` (`:27b` dense alt) | **local ✅ PRIMARY** | Single local default. Repo-level + frontend coding, **SWE-Verified 77.2**, vision+tools, 256K, fast 3B-active MoE |
| Gemma4:31b | `gemma4:31b` | ⏸️ optional (audio) | Only unique edge vs qwen3.6 = **audio input**. Keep only if you need audio understanding |
| Qwen3-coder:30b-a3b-q8_0 | `qwen3-coder:30b` | ❌ superseded | Replaced by qwen3.6 (newer/smaller/better) |

### Local roster for MacBook Pro M5 + 64GB unified memory

M5 Max 64GB = 460 GB/s, Ollama auto-uses the **MLX** backend. Hard fit facts (verified June 2026):
- **30–35B Q4 is the sweet spot** → 20–28 tok/s, leaves room for big context. ← target this.
- 70B Q4 *runs* (8–12 tok/s) but eats most of RAM and is slower → **not worth it**; qwen3.6:35b gives better quality-per-second.
- 70B Q5 / 122B → needs 128GB. Out.
- `qwen3-coder-next` (80B-A3B) is on Ollama but has **local quantization issues** (GH #14049) → skip for now.

Usable budget after macOS ≈ 48GB → keep **one** big model resident at a time; don't expect two 24GB models concurrent with context.

**Download set (final — 1 primary, rest optional):**

```bash
# THE local default — handles all local roles (coding, docs, devops). MLX auto.
ollama pull qwen3.6:35b              # ~24GB  (or qwen3.6:35b-mlx ~22GB)

# OPTIONAL — only if you need them:
ollama pull qwen3.6:27b             # ~17GB  dense, deeper reasoning than the MoE
ollama pull gemma4:31b              # ~19GB  ONLY if you need audio input
ollama pull nemotron-3-nano:30b     # ~18-20GB  local long-context, offline research
```

### Role → model matrix (quality-first)

"Local OK?" = does the best M5-64GB local model meet the quality bar for this role. When **No**, the role runs cloud (or Opus) — quality wins.

| Role | Quality-optimal pick | Local OK on M5 64GB? | Routing verdict |
|---|---|---|---|
| **UI/UX Designer** | `minimax-m3` (cloud) | **No** — needs computer-use to critique live rendered UI | **Cloud** `minimax-m3` |
| **Frontend Dev** | `qwen3.6:35b` (local) | **Yes** — near-frontier (matches Opus 4.5 Terminal-Bench) | **Local** `qwen3.6:35b`; complex → **Opus** |
| **Backend Dev** | `qwen3.6:35b` (local) routine | **Yes** for routine | **Local** `qwen3.6:35b`; heavy systems → **Cloud** `glm-5.2`; critical → **Opus** |
| **Researcher** | `minimax-m3` (cloud) | **No** — needs long-ctx + multimodal sources | **Cloud** `minimax-m3` (text-only research can also use `glm-5.2`) |
| **System Analyst** | `glm-5.2` (cloud) | **No** for greenfield/ambiguous | **Cloud** `glm-5.2`; greenfield architecture → **Opus** |
| **QA / Tester** | `minimax-m3` (cloud) | **No** — computer-use to drive+see E2E | **Cloud** `minimax-m3`; unit-test gen can stay **local** `qwen3.6:35b` |
| **Tech Writer** | `qwen3.6:35b` (local) | **Yes** — docs don't need frontier | **Local** `qwen3.6:35b` |
| **Code Reviewer** | `qwen3.6:35b` (local) routine | **Yes** for routine PRs (SWE-Verified 77.2) | **Local** `qwen3.6:35b`; security/architecture-critical → **Cloud** `glm-5.2` or **Opus** |
| **DevOps** | `qwen3.6:35b` (local) | **Yes** — configs/scripts/tool-calling | **Local** `qwen3.6:35b`; big-corpus log analysis → **Cloud** `glm-5.2` |

**Final routing:** **`minimax-m3`** → UI/UX, QA, Researcher (vision/computer-use/long-ctx). **`glm-5.2`** → System Analyst + every heavy/critical escalation below Opus. **`qwen3.6:35b`** (local) → Frontend, Backend, Tech Writer, Code Reviewer, DevOps (routine). **Opus 4.8** → orchestration + greenfield architecture + security-critical.

### Can Multica route models by role? — capability check (code-verified)

**Yes — via agent assignment, not an automatic model-router.** Multica has no `role` column and no "classify task → pick model" engine. But the primitives compose into exactly the routing we want:

- **Agent** (`server/pkg/db/generated/models.go` → `type Agent struct`) carries its own `RuntimeID`, `Instructions` (the role persona / system prompt), `Description`, and `CustomArgs` — and the model is set per-agent via `multica agent create --model <tag>` (stored in the agent's runtime config/custom_args; empty → runtime default). So **one agent = one role = one pinned model.**
- **Squad** (`squad.sql`: `squad` has `leader_id`; `squad_member` lists members) groups agents under a leader. **Assigning an issue to a squad routes to the squad leader**, and child-issue routing goes to the assigned agent / squad-leader (`issue_child_done.go`).
- **Runtimes are CLI-agent runtimes** (`claude`/`codex`/`openclaw`); Ollama backs the same CLIs (`ollama launch claude|codex|openclaw --model <tag>`). **One runtime can host many models** — agents sharing a runtime each pass their own `--model`, so one local-Ollama runtime + one cloud-Ollama runtime is enough.

**Therefore the routing pattern is:**

1. **Orchestrator** = an Opus 4.8 agent on a Claude runtime, set as the **squad leader**.
2. **9 role agents** = one per role, each pinned to its matrix model via `--model`, role persona in `Instructions`. Cloud roles → cloud-Ollama runtime (2 models: `glm-5.2`, `minimax-m3`); local roles → local-Ollama runtime (`localhost:11434`, `qwen3.6:35b`).
3. Work is assigned to the **squad** → the Opus leader decomposes it and creates child issues assigned to the right role agent → each runs on its own model. **The "model routing" is the leader's assignment decision** — exactly the Opus-orchestrates-simple-work-to-Ollama design.

Limitation to accept: **no automatic task→model classifier** — the Opus leader (or a human) decides which role-agent gets each child issue. An auto-classifier would be net-new work and is **not** required.

### Wiring plan (deferred — planning only)

When approved: one local-Ollama runtime + one cloud-Ollama runtime + one Claude runtime; a `role → model` config + an idempotent `multica agent create --model <tag>` script for the 9 role agents; one squad with the Opus agent as leader. No schema change.

## Sources

- Ollama cloud library — https://ollama.com/search?c=cloud (model cards, tags, sizes)
- Ollama `qwen3.6` card — https://ollama.com/library/qwen3.6 (27b/35b-a3b, 256K, MLX)
- Kilo.ai 2026 open-weight rankings — https://kilo.ai/open-source-models (SWE-Bench/Terminal-Bench/LiveCodeBench)
- Morph LLM Ollama benchmarks — https://www.morphllm.com/best-ollama-models (VRAM, tool-calling)
- PromptQuorum Apple M5 local-LLM — https://www.promptquorum.com/local-llms/apple-silicon-m5-local-llm (M5 64GB tok/s, MLX)
- APXML best local LLMs per Mac — https://apxml.com/posts/best-local-llm-apple-silicon-mac (RAM tiers)
- Ollama GH #14049 — qwen3-coder-next local quantization issue

---

## Implementation Plan

> **For Claude:** REQUIRED SKILL: Use gaspol-execute to implement this plan.
> **CRITICAL:** Real integrations only — never substitute placeholders. If a real integration can't be made (mechanism unknown, service down, missing creds), STOP and ask.
> **OPS PLAN — TDD N/A:** This is operational wiring on a live backend, not unit-testable code. Each phase uses an **ops RED→GREEN verification gate**: step 1 is a *baseline check that currently fails* (RED, proving the thing isn't done), then the action, then a *verification check that passes* (GREEN). Treat the RED baseline as this plan's TDD-step equivalent; if gaspol-execute's TDD gate prompts, select "skip TDD (ops) — ADR" since the RED/GREEN command gates substitute for tests.

### Goal

Wire the locked stack into MANDOR-AI: **Opus 4.8** (Claude runtime, squad leader) orchestrating **9 role agents** that run on **`glm-5.2` / `minimax-m3` (Ollama cloud)** or **`qwen3.6:35b` (local Ollama)** per the role→model matrix above. No code/schema change — pure config/ops via the `multica` CLI + Ollama.

### Architecture Context (verified this session)

- **Backend** UP on `http://localhost:8080` (`{"status":"UP"}`). ⚠️ Memory gotcha: `multica` CLI can mis-point to 8080 when LabelStudio occupies it — Phase 0 pins `MULTICA_SERVER_URL` and proves the CLI talks to MANDOR-AI before any mutation.
- **Ollama** serving on `:11434`. Pulled: `qwen3-coder:30b-a3b-q8_0`, `gemma4:31b` (OLD picks). **`qwen3.6:35b` NOT pulled.**
- **`OLLAMA_API_KEY`** staged in gitignored root `.env`.
- **CLI surface (verified):** `multica agent create|update|get|list|env`, `multica runtime list|update` (**no `runtime create` — daemon registers runtimes**), `multica squad create|get|list|member|update`, `multica daemon start|status|logs` (runs local agent CLIs: Claude, Codex), `multica issue create`.
- `agent create` flags: `--name --runtime-id --model --instructions --custom-env-stdin --visibility`. Per-agent model via `--model`; secrets via `--custom-env-stdin`.
- **Open mechanism (Phase 3 discovers):** how a daemon-registered runtime is pointed at Ollama (local `:11434` vs cloud + key). Likely via the agent CLI's base-URL env (`ANTHROPIC_BASE_URL` / `OPENAI_BASE_URL`) or `ollama launch <cli>`. Unverified → Phase 3 is discovery-first and STOPs if the mechanism isn't confirmable.

### Tech Stack

`multica` CLI 0.3.14 · Ollama (local + cloud) · existing MANDOR-AI daemon/runtime/agent/squad model. No new code.

### Data Integration Map

| Feature | Real source | Command / mechanism | Exists? | Action |
|---|---|---|---|---|
| CLI → correct server | MANDOR-AI on :8080 | `MULTICA_SERVER_URL` + `multica auth status` | Backend yes; CLI target unverified | Pin + verify (Phase 0) |
| Local model `qwen3.6:35b` | Ollama local :11434 | `ollama pull qwen3.6:35b` | **No** | Pull (Phase 1) |
| Cloud models `glm-5.2`,`minimax-m3` | Ollama cloud + key | `OLLAMA_API_KEY` + `ollama` cloud auth | Key staged, unverified | Verify (Phase 2) |
| Local-Ollama runtime | daemon + CLI→:11434 | `multica daemon start` + base-URL env | **No** | Discover+create (Phase 3) |
| Cloud-Ollama runtime | daemon + CLI→cloud+key | daemon env / second daemon | **No** | Discover+create (Phase 3) |
| Opus orchestrator agent | Claude runtime | `multica agent create --model claude-opus-4-8` | **No** | Create (Phase 4) |
| Squad + leader | `squad`/`squad_member` | `multica squad create` + `squad member` | **No** | Create (Phase 4) |
| 9 role agents | `agents` | `multica agent create --model <tag> --runtime-id` | **No** | Create (Phase 5) |
| E2E routing | issue→squad→leader→agent | `multica issue create` + assign squad | n/a | Verify (Phase 6) |

### Phase 0 — Pre-flight: pin CLI to the right server (the 8080 gotcha guard)

**Est:** 5 min. **Files:** none (env/CLI only).
**Steps:**
1. RED baseline: run `multica runtime list --output json` and confirm it either errors or returns a non-MANDOR-AI payload if mis-pointed. Expected-until-fixed: ambiguous/empty or wrong-server response.
2. Export `MULTICA_SERVER_URL=http://localhost:8080`; run `multica auth status` (or `multica config`) — confirm authenticated against MANDOR-AI and a workspace is selected (`MULTICA_WORKSPACE_ID`).
3. GREEN: `multica workspace list` and `multica runtime list` return MANDOR-AI data.
**Verification:** `multica runtime list --output json` returns valid MANDOR-AI JSON; workspace id resolved. **Rollback:** none (read-only). **STOP if:** CLI cannot reach MANDOR-AI (do not fall through to LabelStudio) — fix server URL / port conflict first.

### Phase 1 — Pull the local model

**Est:** 15–40 min (network, ~24GB). **Files:** none.
**Steps:**
1. RED baseline: `ollama list | grep qwen3.6:35b` → expected failure: no output (not pulled).
2. `ollama pull qwen3.6:35b` (or `qwen3.6:35b-mlx`).
3. GREEN: `ollama run qwen3.6:35b "reply READY"` returns a completion.
**Verification:** `ollama list | grep qwen3.6:35b` shows the model; a test prompt completes locally. **Rollback:** `ollama rm qwen3.6:35b`. **STOP if:** disk insufficient (~24GB) or pull fails repeatedly.

### Phase 2 — Verify Ollama cloud auth

**Est:** 10 min. **Files:** none (reads `.env`).
**Steps:**
1. RED baseline: call the Ollama cloud endpoint for `glm-5.2` **without** auth → expected failure: 401/unauthorized.
2. Load `OLLAMA_API_KEY` from `.env`; authenticate (`ollama` cloud signin or `Authorization: Bearer $OLLAMA_API_KEY`), keeping the key out of shell history/transcript (read from env, never echo).
3. GREEN: a minimal authed completion from `glm-5.2` **and** `minimax-m3` succeeds.
**Verification:** authed calls to both cloud models return 200 + content; unauthed returns 401. **Rollback:** none (no state). **STOP if:** key rejected → ask user to re-issue/rotate.

### Phase 3 — Stand up Ollama-backed runtime(s) (DISCOVERY-FIRST)

**Est:** 30–60 min. **Files:** daemon env / launch config only.
**Steps:**
1. RED baseline: `multica runtime list --output json` → expected: no Ollama-backed runtime present.
2. **Discover** the real mechanism (no inventing): inspect `multica daemon start` env + how a runtime gets its provider/base-URL; determine how to point the agent CLI at Ollama local (`:11434`) and at cloud (+`OLLAMA_API_KEY`) — candidates: `ANTHROPIC_BASE_URL`/`OPENAI_BASE_URL` env on the daemon, or `ollama launch <cli> --model`. Confirm against MANDOR-AI daemon/runtime code before acting.
3. Start the daemon(s) so a **local-Ollama** runtime and a **cloud-Ollama** runtime register (plus a normal **Claude** runtime for Opus). Use `--runtime-name` to label them clearly.
4. GREEN: `multica runtime list` shows the expected runtimes ONLINE with correct providers.
**Verification:** ≥1 local-Ollama + ≥1 cloud-Ollama + 1 Claude runtime appear ONLINE; a trivial task on each resolves to the right backend. **Rollback:** `multica daemon stop`; remove added daemon env. **STOP if:** the daemon has no supported path to point a runtime at Ollama — report to user; do NOT fake a runtime.

### Phase 4 — Opus orchestrator agent + squad

**Est:** 10 min. **Files:** none (DB via CLI).
**Steps:**
1. RED baseline: `multica squad list --output json` → expected: routing squad absent.
2. `multica agent create --name "Orchestrator (Opus 4.8)" --runtime-id <claude-rt> --model claude-opus-4-8 --instructions "<orchestrator persona: decompose, delegate per role→model matrix, escalate complex/critical to self>"`.
3. `multica squad create --name "Dev Team" --leader-id <opus-agent-id>` (confirm exact flags via `multica squad create --help`).
4. GREEN: `multica squad get <id>` shows Opus as leader.
**Verification:** squad exists with the Opus agent as `leader_id`. **Rollback:** `multica squad delete <id>`; `multica agent archive <id>`. **STOP if:** no Claude runtime available for Opus (depends on Phase 3).

### Phase 5 — Create the 9 role agents (pinned models) + add to squad

**Est:** 20 min. **Files:** none (DB via CLI).
**Steps (repeat per role from the matrix):**
1. RED baseline: `multica agent list --output json` → expected: role agent absent.
2. Create each agent with its matrix model + persona, on the matching runtime:
   - UI/UX Designer, Researcher, QA Tester → `--model minimax-m3 --runtime-id <cloud-rt>`
   - System Analyst → `--model glm-5.2 --runtime-id <cloud-rt>`
   - Frontend, Backend, Tech Writer, Code Reviewer, DevOps → `--model qwen3.6:35b --runtime-id <local-rt>`
   - persona via `--instructions`; if any agent needs the cloud key inline, pass via `--custom-env-stdin` (never on argv).
3. `multica squad member add <squad-id> --agent <agent-id>` (confirm exact flags via `multica squad member --help`) for all 9.
4. GREEN: `multica squad get <id>` lists 1 leader + 9 members with the right models.
**Verification:** 9 agents exist, each `multica agent get <id>` shows the correct `--model`; all are squad members. No agent left on a wrong/default model. **Rollback:** `multica agent archive <id>` per agent; `multica squad member remove`. **STOP if:** `agent create` writes to the wrong server (8080 gotcha) — re-confirm Phase 0 pin; if CLI proves unreliable for writes, fall back to the documented DB path and flag to user before proceeding.

### Phase 6 — End-to-end smoke test

**Est:** 15 min. **Files:** none.
**Steps:**
1. RED baseline: no issue routed through the squad yet.
2. `multica issue create --title "Routing smoke test: tweak a button label" --assign-squad <squad-id>` (confirm assign flag via `multica issue create --help`); set status so the daemon picks it up.
3. Observe `multica daemon logs` + `multica squad activity` / `multica agent tasks`: leader (Opus) should decompose and a **local** role agent (e.g. Frontend → `qwen3.6:35b`) should execute.
4. Repeat with a vision/UX-flavored task to confirm a **cloud** agent (`minimax-m3`) is selected.
**Verification:** at least one task runs on a **local** model and one on a **cloud** model, each via the intended role agent; `multica runtime usage` attributes tokens to the right runtime. **Rollback:** close/delete the test issues. **STOP if:** leader doesn't delegate (no auto-classifier — confirm orchestrator persona instructs it to, per the capability note above).

### Execution handoff

Ready to start **Phase 0** with gaspol-execute (per-phase checkpoints). Phases are sequential (each depends on the prior); no parallelization. Phase 1's ~24GB pull and Phase 3's discovery are the long poles.
