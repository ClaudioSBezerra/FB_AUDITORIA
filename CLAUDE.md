## Project

**FB_AUDITORIA** — módulo de Controladoria e Auditoria (DRE automatizado, verbas de indústria,
pagamentos a terceiros, trilha/workflow). Mesma ideia do Farol: o sistema vai atrás do gestor.
Nasceu como derivado do FB_FAROL (auth, segurança, multi-empresa e shell do menu copiados); o domínio
de vendas ficou de fora.

### Constraints
- **Stack:** Go (:8088) + React (:3088) + Postgres + Coolify — a mesma do Farol/SmartPick.
- **Menu:** três áreas — Estratégico, Operacional, Administrativo (`frontend/src/lib/navigation.ts`).
- **Autorização:** perfil (`users.perfil`) + áreas (`users.areas`), sempre validadas no backend.
- **Trilha:** toda escrita relevante grava em `auditoria.audit_log` (imutável); use `writeAuditLogTx`
  dentro da transação quando o registro for obrigatório.
- **Multi-tenant desde o início:** toda tabela de domínio tem `empresa_id`.
- Regras de auditoria (Fase 2) devem ficar em configuração (YAML/JSON), não no código.

## Phases
1. DRE + verbas · 2. Pagamentos a terceiros + regras contínuas · 3. Workflow, score de risco, briefing IA.
