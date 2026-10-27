#!/usr/bin/env bash
# dev.sh — sobe backend (8088) + frontend (3088) em localhost.
# Pré-requisito: backend/.env (copie de backend/.env.example) e Postgres local.
set -e
ROOT="$(cd "$(dirname "$0")" && pwd)"
set -a; source "$ROOT/backend/.env"; set +a
pg_isready -h localhost -p 5432 -q || { echo "[ERRO] PostgreSQL não responde em localhost:5432"; exit 1; }
fuser -k "${PORT:-8088}/tcp" 3088/tcp 2>/dev/null || true
(cd "$ROOT/backend" && go run .) &
(cd "$ROOT/frontend" && npm run dev) &
wait
