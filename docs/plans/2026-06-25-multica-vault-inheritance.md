> **For Claude:** REQUIRED SKILL: Use gaspol-execute to implement this plan.
> **CRITICAL:** This plan specifies real integrations (live SessionStart hook, real Obsidian vault, real Multica agents). During execution, NEVER substitute placeholders. If a path or endpoint doesn't exist yet, STOP and ask.
> **NOTE:** This is an ops/config plan (shell hook + vault folder + Multica product config), NOT a codebase feature. There is **zero MANDOR-AI source change**. Verification is behavioral (observe hook output / file landing / MCP visibility), not `tsc`/`vitest`.

## Goal

Wariskan knowledge Obsidian second-brain ke agent Multica supaya makin pintar tiap hari, dengan pola **READ-from-vault (selektif per-peran, on-demand) + WRITE-to-quarantine (file biasa ke `90-Inbox/agent-learnings/`)**. Vault inti tetap terkurasi manual oleh Ali; agent tidak pernah menulis ke vault inti. Semua dilakukan runtime-side — tidak ada perubahan kode MANDOR-AI.

## Architecture Context (grounded, file:line)

- **Daemon spawn:** `server/pkg/agent/claude.go:59-66` exec CLI; `buildEnv` (claude.go:585-602) mewariskan `os.Environ()` → **`HOME` tidak di-override**, jadi `~/.claude/*`, plugin, skill, hook, dan SessionStart hook ikut ter-load di agent run.
- **Per-task identity env (kunci plan ini):** `server/internal/daemon/daemon.go:2780-2786` meng-inject `MULTICA_TOKEN`, `MULTICA_WORKSPACE_ID`, **`MULTICA_AGENT_NAME`** (nama/peran agent, mis. "Product Manager"), `MULTICA_AGENT_ID`, `MULTICA_TASK_ID`. → Hook bisa **branch per-peran** dan **deteksi agent-vs-interaktif** dari sini.
- **Vault-inject hook:** `~/.graphify/superbrain-boot.sh` (terdaftar di `~/.claude/settings.json` → `SessionStart`). Saat ini **dump `hot.md` + `INDEX.md` tanpa syarat** ke SEMUA sesi (interaktif & agent). Tidak ada branch env.
- **Obsidian MCP:** stdio, `~/.claude.json` → `mcpServers.obsidian` → `obsidian-mcp` menunjuk `/Users/alisadikin/Drive-D/Obsidian-Vault`. **read+write satu paket** (tidak bisa scope ke subfolder).
- **Per-agent MCP deny-list:** `server/internal/daemon/mcp_config.go` → `{"disabledMcpServers":[...]}`; daemon merakit effective config = pool mesin − deny-list (`server/internal/daemon/execenv/mcp_effective.go`). UI checklist di panel MCP = mekanisme set deny-list ini.
- **Vault path ada di filesystem lokal** (`/Users/alisadikin/Drive-D/Obsidian-Vault`) → agent bisa **WRITE via native Write tool ke path absolut TANPA MCP obsidian**. Ini mendekuplekan WRITE dari MCP: agent engineering (obsidian MCP OFF) tetap bisa setor learning.

## Decisions (from user)

1. **Quarantine WRITE target:** Vault `90-Inbox/agent-learnings/` (Ali lihat & kurasi di Obsidian).
2. **READ default:** Solo / satu akun → kebocoran bukan isu sekarang. Boleh konteks kaya untuk peran planning. Kebocoran HANYA relevan jika ada workspace milik email/akun BERBEDA → sisakan gate yang gampang diaktifkan (YAGNI sampai akun ke-2 muncul).
3. **Scope:** Runtime-only sekarang. Nol kode MANDOR-AI.

## Role tiers (klasifikasi)

- **PLANNING/KNOWLEDGE** (obsidian MCP ON, READ kaya) — 5: Product Manager, Project Manager, System Analyst, Technical Writer, Researcher.
- **ENGINEERING/QA** (obsidian MCP OFF, READ minimal, tetap boleh WRITE quarantine) — 8: DevOps Engineer, ML/CV Engineer, UI/UX Designer, QA Reviewer, Security Reviewer, Backend Dev, Frontend Dev, Mobile Dev.

### Full MCP matrix (13 agent, pool: figma firecrawl image-gen video-gen kreuzberg obsidian ssh-prod-vps stitch youtube-transcript)

| Agent | MCP ON (sisanya OFF) |
|---|---|
| Researcher | firecrawl, youtube-transcript, obsidian |
| Product Manager | firecrawl, obsidian |
| Project Manager | obsidian |
| System Analyst | firecrawl, obsidian |
| Technical Writer | firecrawl, obsidian |
| UI/UX Designer | figma, stitch, indusia-image-gen |
| Frontend Dev | figma, firecrawl |
| Mobile Dev | figma, firecrawl |
| Backend Dev | firecrawl |
| ML/CV Engineer | firecrawl, kreuzberg |
| DevOps Engineer | ssh-prod-vps, **github**† |
| Security Reviewer | firecrawl, ssh-prod-vps |
| QA Reviewer | **playwright**† |

† `github` & `playwright` **belum ada di pool** — harus ditambah ke `~/.claude.json` dulu (lihat C.5). Karena pool itu opt-OUT, sesudah ditambah keduanya WAJIB masuk deny-list 12 agent lain (github: semua kecuali DevOps; playwright: semua kecuali QA).

Catatan: `indusia-video-gen` tidak diaktifkan di agent manapun (tidak ada peran video). Aktifkan manual kalau nanti ada peran content/marketing. Backend/Frontend/Mobile Dev juga kandidat `github` kalau nanti mau (PR/issue/commit) — sekarang DevOps saja sesuai permintaan.

## Data Integration Map

| Capability | Source / Target | Mechanism | Exists? | Action |
|---|---|---|---|---|
| Agent role saat runtime | `MULTICA_AGENT_NAME` env | daemon.go:2784 inject | Yes | Baca di hook |
| Deteksi agent-vs-interaktif | ada/tidaknya `MULTICA_AGENT_NAME` | env presence | Yes | Branch di hook |
| Vault state digest | `hot.md` + `INDEX.md` | `sed` di hook | Yes | Reuse, gate per-tier |
| Vault READ on-demand | obsidian MCP → vault | stdio MCP (planning only) | Yes | Gate via deny-list |
| Learning WRITE | `90-Inbox/agent-learnings/*.md` | native Write tool, path absolut | **No (folder)** | **Create folder + README rule** |
| Per-agent MCP gating | `agent.mcp_config.disabledMcpServers` | Multica UI checklist / API | Yes (mekanisme) | Apply matrix |
| Per-agent instructions | `task.Agent.Instructions` → `--append-system-prompt` | claude.go:530 / daemon.go:2618,2633 | Yes | Tulis brief per-agent |
| Per-agent skills | `task.Agent.Skills` → AgentSkills env | daemon.go:2617,2634,3641 | Yes | Set skill emphasis |
| Per-agent env (auth) | `task.Agent.CustomEnv` | daemon.go:2837 | Yes | Inject creds (notebooklm) |
| Leak guard (akun lain) | workspace account vs vault owner | env allowlist gate | N/A (solo) | Tulis sbg `ponytail:` TODO, jangan implement |

## Phases

### Phase A: Vault quarantine folder + rule

**Estimated time:** 4 min

**Files:**
- Create: `/Users/alisadikin/Drive-D/Obsidian-Vault/90-Inbox/agent-learnings/README.md`
- Check (write test): `/Users/alisadikin/Drive-D/Projects/MANDOR-AI/scripts/check-vault-inheritance.sh`

**Steps:**
1. Write failing check for quarantine: append to `scripts/check-vault-inheritance.sh` an assertion `test -f "$VAULT/90-Inbox/agent-learnings/README.md"`. Run it. Expected error: check fails (file absent, exit 1).
2. Run check, confirm it fails for the expected reason (README belum ada).
3. Create `90-Inbox/agent-learnings/README.md` berisi: tujuan (tempat setor learning agent, UNCURATED), aturan (agent hanya WRITE di sini, JANGAN edit vault inti), konvensi frontmatter (`agent`, `task_id`, `date`, `type: feedback|insight|gotcha`, `curated: false`), dan catatan untuk Ali (kurasi naik ke vault inti lalu hapus/`curated: true`).
4. Run check, confirm pass.
5. Commit (vault repo kalau ada git; kalau tidak, skip): "chore(vault): add agent-learnings quarantine inbox".

**Verification:**
- [ ] `90-Inbox/agent-learnings/README.md` ada dan menjelaskan aturan WRITE-only-here
- [ ] `scripts/check-vault-inheritance.sh` step quarantine = PASS
- [ ] Tidak ada perubahan di vault inti (folder selain `90-Inbox/`)

---

### Phase B: Role-aware SuperBrain hook

**Estimated time:** 12 min

**Files:**
- Modify: `/Users/alisadikin/.graphify/superbrain-boot.sh`
- Check: `/Users/alisadikin/Drive-D/Projects/MANDOR-AI/scripts/check-vault-inheritance.sh`

**Design (branch logic):**
```
VAULT=/Users/alisadikin/Drive-D/Obsidian-Vault
if [ -z "$MULTICA_AGENT_NAME" ]; then
  # interaktif (Ali) → perilaku LAMA: full hot.md + INDEX dump. JANGAN diubah.
else
  # Multica agent run → role-aware
  case "$MULTICA_AGENT_NAME" in
    *Manager*|*Analyst*|*Writer*|*Researcher*)  TIER=planning ;;
    *)                                          TIER=lean ;;
  esac
  # SEMUA tier: aturan WRITE quarantine (path absolut) + larangan edit vault inti.
  # planning: + trimmed hot.md (state) + INDEX pointer + "tarik detail on-demand via obsidian MCP".
  # lean: minimal — cuma write-rule + 1 baris "vault tersedia kalau perlu" (MCP mereka OFF).
  # ponytail: leak-gate OFF (solo, 1 akun). Saat ada akun/email ke-2, gate di sini
  #           pakai allowlist MULTICA_WORKSPACE_ID. Jangan implement sekarang (YAGNI).
fi
```

**Steps:**
1. Write failing check: append assertions ke `check-vault-inheritance.sh`:
   - `MULTICA_AGENT_NAME="Product Manager" zsh superbrain-boot.sh` output mengandung string write-rule (`agent-learnings`) DAN `hot.md` snippet.
   - `MULTICA_AGENT_NAME="DevOps Engineer" zsh superbrain-boot.sh` output mengandung write-rule TAPI **tidak** mengandung dump `hot.md` penuh.
   - `MULTICA_AGENT_NAME=` (kosong) `zsh superbrain-boot.sh` output mengandung `=== AI SUPERBRAIN` (perilaku interaktif lama). Run → fail (hook belum branch).
2. Run check, confirm fail (hook saat ini sama untuk semua).
3. Edit `superbrain-boot.sh` sesuai design di atas. Pertahankan blok interaktif lama persis (regresi-safe).
4. Run check, confirm 3 skenario pass.
5. Sanity: jalankan interaktif sekali (`zsh superbrain-boot.sh` tanpa env) → output identik dgn sebelumnya.
6. Commit: "feat(superbrain): role-aware vault inject for Multica agent runs".

**Verification:**
- [ ] Interaktif (env kosong): output **byte-identik** dengan versi lama (no regression untuk sesi Ali)
- [ ] `MULTICA_AGENT_NAME=Product Manager`: ada write-rule + trimmed state + on-demand pointer
- [ ] `MULTICA_AGENT_NAME=DevOps Engineer`: ada write-rule, TANPA dump hot.md penuh
- [ ] Ada `ponytail:` TODO untuk leak-gate akun ke-2
- [ ] `check-vault-inheritance.sh` semua PASS

---

### Phase C: Per-agent capability profiles (MCP + Instructions + Skills + Env)

**Estimated time:** 20 min

**Mechanism (semua product config via UI tab per-agent, no code):**
- **MCP** tab → `agent.mcp_config.disabledMcpServers` (deny-list).
- **Instructions** tab → `task.Agent.Instructions` → `--append-system-prompt` (claude.go:530).
- **Skills** tab → `task.Agent.Skills` (daemon.go:2617,2634,3641) — **HANYA workspace skills**. ⚠️ **Local runtime skills (notebooklm, gaspol-*, figma-*, ponytail, dll. dari `~/.claude`) OTOMATIS tersedia di semua agent** — tidak perlu di-add; cukup di-trigger lewat Instructions.
- **Environment** tab → `task.Agent.CustomEnv` (daemon.go:2837) — utk creds/auth.

**C.0 — Workspace-skill matrix** (skill custom di tab Skills, saat ini ke-assign seragam ke semua agent — tailor per-peran):

| Workspace skill | Pasang di |
|---|---|
| `coding-standards` | Backend, Frontend, Mobile, ML/CV, Security Reviewer (yang baca/tulis app code). **Cabut dari DevOps** (app/frontend-leaning, bukan IaC) + PM/PgM/SysAnalyst/TechWriter/Researcher/UI-UX |
| `verification-loop` | Semua engineering + QA (output = code/test) |
| `superbrain-grounding` | Semua (grounding murah; kedalaman vault sudah digate hook Fase B). Opsional cabut dari DevOps/QA utk ekstra lean |

**C.1 — MCP deny-list matrix (semua 13 agent):**
1. Tarik daftar agent live → klasifikasi tier.
2. PLANNING (PM, Project Manager, System Analyst, Technical Writer, Researcher): `obsidian` **ON**.
3. ENGINEERING/QA + 3 unseen (default): `obsidian` **OFF** (WRITE quarantine tetap jalan via file tool, bukan MCP).
4. Konfirmasi 3 agent unseen sebelum finalize.

**C.2 — Researcher → NotebookLM:**
- MCP: obsidian ON, firecrawl ON, youtube-transcript ON.
- Skills: `notebooklm` (+ `deep-research`).
- Instructions (paste ke tab): "Untuk riset mendalam, pakai skill `/notebooklm`: buat notebook, tambah sumber (URL/paste/transcript), generate Audio Overview / summary / FAQ / study guide. Kombinasikan dgn firecrawl (web) + youtube-transcript (video). Selalu sitasi sumber. Simpan temuan durable ke quarantine `90-Inbox/agent-learnings/`."
- Env: NotebookLM auth (Google) — **pastikan creds ter-warisi di HOME runtime**. (User confirm: `/notebooklm` works.) Jika headless butuh creds eksplisit, set di Environment tab.

**C.3 — UI/UX Designer → Figma design→prototype (bukan static doang):**
- MCP: figma ON, stitch ON, indusia-image-gen ON. obsidian OFF.
- Skills: `figma:figma-use`, `figma:figma-generate-design`, `figma:figma-implement-motion`.
- Instructions (paste ke tab): "JANGAN berhenti di static frame. Wajib lanjut ke PROTOTYPE: wire flows, interactions, transitions, overlays via figma `use_figma` (reactions, smart-animate) — pakai skill figma-use + figma-generate-design. Deliverable akhir = Figma file dgn prototype yang BISA DI-PLAY (klik antar screen), bukan kumpulan frame mati."
- **Dependency (caveat):** `use_figma` (write/prototype) butuh Figma desktop terbuka + file aktif di mesin runtime (bridge Plugin API). Read tools jalan via API. → Pastikan Figma desktop hidup di `Alis-MacBook-Pro.local` saat task UI berjalan.

**C.4 — Product Manager → brainstorm ala /gaspol-brainstorm:**
- MCP: obsidian ON, firecrawl ON.
- Skills: `gaspol-dev:gaspol-brainstorm`, `gaspol-dev:gaspol-plan`.
- Instructions (paste ke tab): "Untuk kerja fitur/produk, MULAI dengan metodologi gaspol-brainstorm: eksplorasi intent, tanya balik yang ambigu, hasilkan spec + Data Integration Map sebelum implementasi. KARENA jalan headless (tanpa AskUserQuestion), lakukan brainstorm SECARA ASINKRON lewat komentar issue/task: tulis pertanyaan klarifikasi sbg komentar, tunggu balasan Ali, baru lanjut. Jangan lompat ke solusi sebelum intent jelas."
- **Caveat:** brainstorm di Multica = async via issue comments, bukan dialog real-time (daemon pakai `--disallowedTools AskUserQuestion`, `--permission-mode bypassPermissions`). Metodologi gaspol-brainstorm dipakai, channel-nya komentar.

**C.5 — Extend runtime pool: github + playwright:**
- Kenapa: keduanya bukan anggota pool `~/.claude.json`, dan `--strict-mcp-config` memblok plugin MCP di agent run. Harus ditambah ke pool dulu.
- **github** (DevOps): tambah entry stdio ke `~/.claude.json` mcpServers, mis. `npx -y @modelcontextprotocol/server-github` + env `GITHUB_PERSONAL_ACCESS_TOKEN` (scope: repo, workflow, read:org). Headless-friendly (PAT, bukan OAuth interaktif).
- **playwright** (QA): tambah entry stdio `npx @playwright/mcp@latest` (tanpa auth) + `npx playwright install chromium` di mesin runtime. Browser jalan di Alis-MacBook-Pro.
- Sesudah ditambah: deny-list `github` di semua agent kecuali DevOps; deny-list `playwright` di semua agent kecuali QA (opt-out).
- Catatan: entry github di `~/.claude.json` juga muncul di sesi interaktif Ali (di samping github plugin) — duplikat kecil, harmless; cabut plugin github kalau mau bersih.

**Verification:**
- [ ] C.5: `jq '.mcpServers|keys' ~/.claude.json` memuat `github` & `playwright`; `claude mcp list` di run DevOps menampilkan github (connected), di run QA menampilkan playwright
- [ ] C.5: github absent di 12 agent non-DevOps; playwright absent di 12 agent non-QA
- [ ] C.1: matrix obsidian benar (planning ON, sisanya OFF), 3 unseen dikonfirmasi
- [ ] C.2: Researcher Instructions menyebut `/notebooklm`; skill notebooklm aktif; firecrawl+youtube-transcript ON; 1 test task riset benar memanggil notebooklm
- [ ] C.3: UI/UX Instructions mewajibkan prototype; figma skills aktif; 1 test task menghasilkan Figma file dgn interaction (bukan static) — dgn Figma desktop hidup
- [ ] C.4: PM Instructions menyetir gaspol-brainstorm async; 1 test task → PM menulis pertanyaan klarifikasi sbg komentar issue, bukan langsung ngoding

---

### Phase D: End-to-end verification (live agent task)

**Estimated time:** 10 min

**Steps:**
1. Trigger 1 task ke agent **planning** (mis. Product Manager) yang menyentuh konteks "kenapa/keputusan". Amati: agent log/output menunjukkan ia menerima konteks vault (write-rule + state) dari hook; dan kalau perlu, memanggil obsidian MCP `search-vault`/`read-note`.
2. Minta (atau biarkan instruksi hook memicu) agent menulis 1 learning → cek file muncul di `/Users/alisadikin/Drive-D/Obsidian-Vault/90-Inbox/agent-learnings/`.
3. Trigger 1 task ke agent **engineering** (mis. DevOps Engineer). Konfirmasi: tidak ada dump hot.md di konteksnya; obsidian MCP TIDAK tersedia (cek `mcp_status` / `claude mcp list` di run itu); tetap bisa WRITE learning ke quarantine.
4. Konfirmasi vault inti (`hot.md`, `10-Identity/`, dll.) **tidak berubah** oleh agent (`git status` di vault, atau diff mtime).

**Verification:**
- [ ] Learning file dari agent benar-benar mendarat di `90-Inbox/agent-learnings/`
- [ ] Agent engineering: obsidian MCP absent, hot.md tidak ke-dump
- [ ] Agent planning: dapat konteks vault + bisa baca on-demand
- [ ] Vault inti 0 perubahan dari agent
- [ ] Token/context agent engineering turun (tidak ada vault dump) vs baseline

## Appendix: Per-role tool expansion (deep-research 2026)

**Prinsip pasang (hemat deny-list — pool itu opt-OUT):**
- Tool 1-peran berupa **CLI** (lighthouse, axe-core, gitleaks, vale, markdownlint, k6, `flutter analyze`, `psql`, kubectl) → install di runtime, agent panggil via Bash. **Nol pool entry, nol deny-list.** Scope via Instructions.
- **Pool MCP** hanya untuk yang tak bisa CLI DAN lintas-peran/layak ongkos deny: github, Context7, Postgres, Semgrep, Playwright, chrome-devtools.

**Tambahan per peran (top pick; ⚠️ = OAuth interaktif, risky headless):**

| Peran | MCP (pool) | CLI (Bash, no pool) |
|---|---|---|
| DevOps | Kubernetes, Grafana, AWS | kubectl, helm, terraform, docker |
| Backend | **Postgres MCP Pro**, Redis | psql, redis-cli, oasdiff |
| ML/CV | **W&B** (/MLflow), Qdrant/Pinecone, HuggingFace, arxiv | — |
| Frontend | **chrome-devtools-mcp**, Storybook | axe-core, lighthouse |
| Mobile | **Dart/Flutter MCP** | flutter analyze/test |
| UI/UX | Storybook | axe-core, Style Dictionary |
| QA | k6 MCP (+ playwright ✓) | axe-core, k6, ⚠️BrowserStack |
| Security | **Semgrep**, Trivy, Snyk(token) | gitleaks |
| Product Mgr | github(token) | ⚠️Linear/Notion (OAuth) |
| Project Mgr | github(token) | ⚠️Jira/Linear (OAuth) |
| System Analyst | Postgres(read-only), **Mermaid** | redocly/openapi |
| Tech Writer | **Context7**, Mermaid | Vale, markdownlint |
| Researcher | **Exa**, arxiv, Context7 | — |

**Lintas-peran juara:** `Context7 MCP` (no-auth, docs library up-to-date) → Backend/Frontend/Mobile/SysAnalyst/TechWriter/Researcher.
**Headless caveat:** Sentry, Linear, Notion, Jira, BrowserStack = OAuth interaktif → interaktif-only, jangan diandalkan di agent run.

Status: roadmap, BUKAN bagian Fase A–D inti. Implement bertahap sesudah core jalan (mulai dari Context7 + Postgres + Semgrep — paling tinggi leverage, semua no/token-auth).

## Out of scope (sengaja ditunda — YAGNI)

- Leak-gate untuk workspace akun/email lain (implement saat akun ke-2 muncul; titik gate sudah ditandai di hook).
- Curation routine otomatis (agent "Archivist" yang promote learning → vault inti). Sekarang kurasi manual oleh Ali.
- Fitur produk MANDOR-AI (setting "knowledge access" per-agent di UI/daemon) — opsi B yang ditolak; runtime-only dulu.
- Role-aware via flag CLI khusus — tidak perlu, `MULTICA_AGENT_NAME` sudah cukup.

## Red-flag self-check

- Data Integration Map: ✅ ada. Per-phase verification: ✅ ada. CLAUDE.md/arsitektur grounded dgn file:line: ✅. Data source spesifik (bukan "connect to backend"): ✅. Runnable check (`check-vault-inheritance.sh`): ✅. Tidak ada placeholder/TODO-wire-later di deliverable: ✅ (TODO yang ada = leak-gate, sengaja YAGNI dengan `ponytail:`).
