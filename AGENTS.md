<!-- bmad:context -->
<!-- Verified 2026-10-27 against ba52699. Managed by bmad-project-context; edits inside this block are replaced on refresh. Keep anything you want preserved outside the markers. -->

## FB_AUDITORIA

Módulo de Controladoria e Auditoria da família FB (derivado do FB_FAROL), integrado ao ERP Winthor/Oracle: DRE gerencial, verbas, descontos e precificação, fretes, produtividade logística. Três frentes (Administrativo, Comercial, Operação), entregues em releases a partir do DRE. Go + React + Postgres. Planejamento BMAD em `_bmad-output/planning-artifacts/`.

## Policy

- O repositório é público: nunca commite contratos, propostas, CNPJs, valores comerciais, extrações do Winthor nem dados reais de clientes. Escopo do cliente vai para o planejamento só em resumo, sem esses dados.
- Commits vão direto no `main`; não abra PR.
- Valide perfil (`users.perfil`) e áreas (`users.areas`) sempre no backend, via `AuditoriaAuthMiddleware`; o menu do frontend só reflete.
- Toda escrita relevante grava em `auditoria.audit_log` (imutável); dentro da transação use `writeAuditLogTx` quando o registro for obrigatório.
- Toda tabela de domínio tem `empresa_id`.
- Regras de auditoria (Fase 2) ficam em configuração YAML/JSON, nunca no código.

## Where things are

- Menu por área: `frontend/src/lib/navigation.ts`
- Rotas, autorização e trilha: `backend/handlers/` (`auditoria_auth.go`, `audit_log.go`)
- Migrations: `backend/migrations/` (rodam sozinhas no boot da API)

## Running and verifying

- `./dev.sh` exige `backend/.env` (copie de `.env.example`, preencha `JWT_SECRET`) e Postgres em localhost:5432; ele mata qualquer processo nas portas 8088 e 3088.
- Não há testes Go nem CI; antes de commitar rode `npm run typecheck` e `npm run lint` em `frontend/` e `go build ./...` em `backend/`. Se mexeu em `epics.md` ou `sprint-status.yaml`, rode também `cd backend && go run ./cmd/gerar-acompanhamento` e commite o `acompanhamento/dados.json` (o teste do pacote falha se estiver desatualizado).
<!-- /bmad:context -->
