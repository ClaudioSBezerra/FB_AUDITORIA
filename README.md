# FB_AUDITORIA

Módulo de Controladoria e Auditoria da família FB (irmão do FB_FAROL e do FB_SMARTPICK):
**o sistema vai atrás do gestor, não o contrário.**

## Stack
Go (API, :8088) · React + Vite + Tailwind/shadcn (:3088) · PostgreSQL · Docker/Coolify.
Autenticação, segurança (JWT + refresh em cookie, rate-limit, headers/CORS, licença, bloqueio de
empresa) e multi-empresa (ambiente → grupo → empresa) são os mesmos do FB_FAROL.

## Menu — três áreas de gestão
| Área | Para quê | Hoje |
|---|---|---|
| **Estratégico** | visão da diretoria/controladoria | DRE, Fechamentos, Mapa do DRE |
| **Operacional** | acompanhamento do dia a dia | Verbas, Pagamentos a terceiros, Exceções (em breve) |
| **Administrativo** | cadastros, carga, usuários, trilha | Carga de lançamentos, Usuários e perfis, Trilha de auditoria |

Cada usuário tem um **perfil** (`auditoria`, `controladoria`, `diretoria`, `gestor`) e as **áreas** do menu
que enxerga. O backend valida ambos em cada rota (`AuditoriaAuthMiddleware`); o menu só reflete.

## DRE (Fase 1)
- Estrutura do DRE e de-para conta→linha são **dados** (editáveis na tela, sem deploy).
- Lançamentos analíticos em `auditoria.lancamentos` (carga CSV hoje; ingestão Oracle/Winthor grava na mesma tabela).
- Visões mensal/acumulada, por filial, com orçado e ano anterior; drill-down até o lançamento.
- Fechamento mensal versionado (congela o realizado); recusa fechar com conta sem mapeamento.
- Trilha de auditoria imutável (`auditoria.audit_log`, trigger bloqueia UPDATE/DELETE).

## Rodar local
```
cp backend/.env.example backend/.env   # preencha JWT_SECRET
./dev.sh
```
Migrations em `backend/migrations` rodam sozinhas no boot.
