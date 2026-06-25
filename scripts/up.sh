#!/usr/bin/env bash
# Start every local service in one shot: postgres + migrations + backend +
# frontend + the local agent daemon (runtime). Ctrl-C stops backend/frontend;
# the daemon is detached and keeps running — stop it with `make daemon-stop`
# (or `multica daemon stop --profile local`).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ---------- Environment file (mirror scripts/dev.sh) ----------
if [ -f .git ]; then ENV_FILE=".env.worktree"; else ENV_FILE=".env"; fi
if [ ! -f "$ENV_FILE" ]; then
  echo "✗ Missing $ENV_FILE — run 'make dev' or 'make setup' first."; exit 1
fi
echo "==> Using $ENV_FILE"
set -a; . "$ENV_FILE"; . scripts/local-env.sh; set +a

# ---------- Database + migrations ----------
bash scripts/ensure-postgres.sh "$ENV_FILE"
echo "==> Running migrations..."
(cd server && go run ./cmd/migrate up)

# ---------- Backend + frontend (foreground; Ctrl-C kills both) ----------
echo ""
echo "✓ Starting services..."
echo "  Backend:  http://localhost:${PORT:-8080}"
echo "  Frontend: http://localhost:${FRONTEND_PORT:-3000}"
echo ""
trap 'kill 0' EXIT
(cd server && go run ./cmd/server) &
pnpm dev:web &

# ---------- Daemon (after backend is reachable) ----------
# The daemon registers with the backend on start, so wait for the port first.
( for _ in $(seq 1 60); do
    if curl -s -o /dev/null "http://localhost:${PORT:-8080}/"; then
      echo "==> Backend up — starting daemon (runtime)..."
      (cd server && go run ./cmd/multica daemon restart --profile local) \
        && echo "✓ Daemon online (logs: ~/.multica/profiles/local/daemon.log)" \
        || echo "✗ Daemon failed — check 'multica login --profile local' and the log above."
      exit 0
    fi
    sleep 1
  done
  echo "✗ Backend never came up on :${PORT:-8080} — daemon not started." ) &

wait
