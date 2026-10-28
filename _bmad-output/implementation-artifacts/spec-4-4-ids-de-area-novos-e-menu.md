---
title: 'Story 4.4: Ids de área novos e menu (AD-12)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '0f0438f0dd3bf13dfff1d606d592089bd6bfd15a'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-4-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O menu ainda está na primeira versão (áreas Estratégico, Operacional, Administrativo; o Administrativo traz carga, usuários e trilha). O cliente definiu: Administrativo é a área da empresa (DRE, fechamentos, Mapa), Configurações traz carga, usuários e trilha, e entram Comercial e Operação ("em breve").

**Approach:** Ids de área novos `empresa` (rótulo "Administrativo"), `configuracoes`, `comercial` e `operacao`, migrados por um único `UPDATE` com `CASE` sobre `users.areas`. Backend, rotas e frontend saem juntos. O módulo de administração da plataforma passa a se chamar "Plataforma".

## Boundaries & Constraints

**Always:** Migration `005_areas_novas.sql`: `UPDATE users SET areas = ARRAY(SELECT CASE a WHEN 'estrategico' THEN 'empresa' WHEN 'administrativo' THEN 'configuracoes' WHEN 'operacional' THEN 'operacao' ELSE a END FROM unnest(areas) AS a)`, preservando a ordem; idempotente (rodar de novo não altera nada); `ALTER COLUMN areas SET DEFAULT ARRAY['empresa','configuracoes','comercial','operacao']`. Mensagens de erro da migration sem `already exists` nem `duplicate`. Backend: constantes `AreaEmpresa`, `AreaConfiguracoes`, `AreaComercial`, `AreaOperacao` com os ids novos; `areasValidas` e a validação do `PUT /api/aud/usuarios` aceitam só os novos e recusam os antigos (`400`); `/api/aud/me` do admin de plataforma devolve as quatro. Rotas do DRE (`/api/aud/dre*`) exigem `empresa`; `/api/aud/usuarios` e `/api/aud/audit-log` exigem `configuracoes`. Frontend: rail e abas com Administrativo (`/empresa/dre`, `/empresa/fechamentos`, `/empresa/mapa`), Configurações (`/configuracoes/carga`, `/configuracoes/usuarios`, `/configuracoes/trilha`), Comercial (`/comercial`) e Operação (`/operacao`), as duas últimas com abas "em breve" desabilitadas; o seletor de áreas da tela de usuários lista os quatro rótulos (Administrativo, Configurações, Comercial, Operação). O módulo `config` (ambiente, dados e ambiente, usuários da plataforma) passa a ter o rótulo "Plataforma"; seus caminhos `/config/*` não mudam. Os caminhos antigos `/estrategico/*`, `/administrativo/*` e `/operacional` redirecionam para os novos (`/estrategico/*`→`/empresa/*`; `/administrativo/*`→`/configuracoes/*`; `/operacional`→`/operacao`). A landing leva à primeira área liberada na ordem empresa, comercial, operacao, configuracoes.

**Never:** Não reaproveitar id antigo com significado novo. Não alterar perfis, matriz de capacidades nem escopo por departamento (4.2, 4.3, 4.7). Não conceder `comercial` a usuário existente (a migração só renomeia; o admin de plataforma vê todas). Não editar migration já aplicada. Produção só recebe por push; decisão humana.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Três áreas antigas | `{estrategico,operacional,administrativo}` | `{empresa,operacao,configuracoes}` | N/A |
| Só uma área | `{administrativo}` | `{configuracoes}` | N/A |
| Vazia | `{}` | `{}` | N/A |
| Já migrada | `{empresa,configuracoes}` | inalterada ao rodar de novo | N/A |
| Usuário PUT com id antigo | `areas:["estrategico"]` | `400`, nada gravado | mensagem de áreas inválidas |
| Rota do DRE sem `empresa` | usuário só com `configuracoes` | `403` | N/A |
| Rota de usuários sem `configuracoes` | usuário só com `empresa` | `403` | N/A |
| URL antiga | `/estrategico/dre` | redireciona para `/empresa/dre` | N/A |

</frozen-after-approval>

## Code Map

- `backend/migrations/005_areas_novas.sql` -- novo; o runner aplica no boot por ordem de nome.
- `backend/handlers/auditoria_auth.go:30-32,148` -- constantes das áreas e lista do admin em `AudMeHandler`.
- `backend/handlers/aud_usuarios.go:15` -- `areasValidas`.
- `backend/main.go:425-438` -- áreas exigidas pelas rotas (`handlers.AreaAdministrativo`, `est`).
- `frontend/src/lib/navigation.ts` -- módulos, abas, `getActiveModule`; módulo `config` vira "Plataforma".
- `frontend/src/components/AppRail.tsx:39-46,53,172-183` -- itens do rail (área e rótulo) e o botão de Configurações da plataforma (rótulo/tooltip "Plataforma").
- `frontend/src/App.tsx:63-65,146-158` -- landing e rotas por área; acrescentar redirecionamentos e as páginas "em breve".
- `frontend/src/pages/aud/AdmUsuarios.tsx:7` -- `AREAS` do seletor.
- `frontend/src/contexts/AuthContext.tsx:21` -- comentário dos ids (atualizar).
- `backend/internal/testdb`, `backend/handlers/ambiente_test.go` -- helper e padrão de testes com banco; reusar.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/migrations/005_areas_novas.sql` -- `UPDATE ... CASE` e novo `DEFAULT` -- AD-12
- [ ] `backend/handlers/auditoria_auth.go`, `aud_usuarios.go`, `main.go` -- ids novos, validação e áreas das rotas -- backend e frontend no mesmo release
- [ ] `backend/handlers/areas_test.go` -- testes com banco: a matriz de migração (aplicar o `UPDATE` do arquivo `005` sobre usuários de cada combinação e rodar duas vezes), `PUT` com id antigo (400), `403` entre `empresa` e `configuracoes` -- cobrir a matriz
- [ ] `frontend/src/lib/navigation.ts`, `components/AppRail.tsx`, `App.tsx`, `pages/aud/AdmUsuarios.tsx`, `contexts/AuthContext.tsx` -- menu novo, rotas, redirecionamentos, páginas "em breve", seletor e rótulo "Plataforma"

**Acceptance Criteria:**
- Given um usuário com as três áreas antigas, when a migration roda, then ele fica com `empresa`, `operacao` e `configuracoes`, e rodar de novo não muda nada.
- Given o menu, when um usuário com todas as áreas o abre, then vê Administrativo (DRE, Fechamentos, Mapa), Configurações (Carga, Usuários, Trilha), Comercial e Operação ("em breve"), e o admin de plataforma vê também "Plataforma".
- Given `go build ./...`, `go vet ./...`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- expected: passam
- `cd frontend && npm run typecheck` -- expected: só o erro anterior em `ResetPassword.tsx(13,9)`
- `grep -rn "estrategico\|administrativo\|operacional" backend frontend/src --include=*.go --include=*.ts --include=*.tsx` -- expected: só nos redirecionamentos, na migration e em comentários

## Implementation Notes

Tamanho: acima de 1600 tokens, porque a história cruza banco, backend e frontend; sem divisão proposta. A migration é irreversível depois de aplicada em produção. Usuários existentes não recebem `comercial`; para ver Comercial (em breve) é preciso o admin de plataforma ou liberar a área na tela de usuários.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Rail e landing de Configurações levam a `/configuracoes/carga` (exige `controladoria`) e o perfil `auditoria` cai em página negada | medium | patch | rota `/configuracoes` com `AreaHome`; rail e landing apontam para ela |
| Redirecionamento perde query string e hash | low | patch | `RedirecionaPrefixo` preserva `search` e `hash` |
| `updateMigracao` fatia antes de checar `i < 0` | low | patch | checagem antes do fatiamento |
| Nenhum teste envia os quatro ids novos pelo `PUT` | medium | patch | `TestAreas_PutComIdsNovos` |
| Migration pode duplicar id se o usuário tiver id antigo e novo ao mesmo tempo | low | rejeitado | impossível antes desta versão |
| `PUT` aceita ids duplicados | low | rejeitado | comportamento anterior; sem dano |
| Default inclui `comercial` e `configuracoes` | low | rejeitado | é o que o AD-12 e a spec definem |
| API antiga recebe só ids novos durante o deploy (403 breve) | low | defer | troca de container curta; observar no push |
| Migration sem registro na trilha | low | rejeitado | migrations não passam pela trilha de aplicação |
| Rotas `em-breve` sem página; `/estrategico/briefing` sem destino; ordem de áreas do admin; renomear componentes `Adm*` | low | rejeitado | abas desabilitadas; ajuste cosmético ou fora da spec |
| Sem testes de frontend | low | defer | o repositório não tem harness de frontend |
