---
title: 'Story 1.3: Virar para Produção e faixa de ambiente (FR-21)'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '89ab235624cb66a72ba80924b51251553d0c9875'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** Com o marcador da 1.2, ainda falta o admin virar a empresa para Produção, e nenhuma tela diz em que ambiente a pessoa está. Sem isso, teste e produção se confundem e a limpeza de teste não tem interface.

**Approach:** Rota de virada (senha mais a frase "VIRAR PARA PRODUCAO") que muda `ambiente` para `producao` e devolve o aviso das contas ainda vindas de arquivo; `/api/aud/me` passa a informar o ambiente; o frontend mostra a faixa fixa "AMBIENTE DE TESTE" e uma tela de administração do ambiente com os botões de apagar (só em teste) e de virar.

## Boundaries & Constraints

**Always:** A virada reaproveita a senha de novo e o admin de plataforma da 1.2 (`CheckPasswordHash`, `AudContext.Admin`). A frase tem de ser exatamente `VIRAR PARA PRODUCAO`; senha errada ou ausente, ou frase diferente, devolve `403` e nada muda. A virada acontece numa transação com a trilha (`writeAuditLogTx`: quem, quando, empresa). O aviso (não bloqueia) lista as contas distintas com lançamento de origem arquivo, isto é, `origem` igual a `csv` ou começando por `csv.`. A faixa vem do campo `ambiente` de `/api/aud/me` e some em Produção; a tela mostra "Produção" e não mostra botão de apagar. O frontend só reflete: o backend recusa tudo igualmente.

**Never:** Nenhuma rota ou botão para voltar a `teste`. A migration não é alterada (o gatilho de mão única é da 1.2). Não esconder a faixa por configuração do usuário. Nenhuma lógica de e-mail de teste aqui (Épico 5). A virada em produção real só ocorre por ação humana na tela; nada nesta história a executa.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Virar | admin, `teste`, senha certa, frase certa | `producao`; trilha registra; resposta traz `contas_de_arquivo` | N/A |
| Senha errada ou frase errada | admin, `teste` | nada muda | `403` |
| Já em produção | admin, `producao` | `producao` continua; sem novo registro | `409` |
| Não admin | perfil `controladoria` sem `role admin` | nada muda | `403` |
| Faixa | `/api/aud/me` com `ambiente: teste` | faixa fixa em toda tela; em `producao`, sem faixa | N/A |

</frozen-after-approval>

## Code Map

- `backend/handlers/ambiente.go` (da 1.2) -- acrescentar `AmbienteVirarHandler` (POST `/api/aud/ambiente/virar`, corpo `{senha, confirmacao}`); mesma conferência de senha e de admin do handler de limpeza, extraída para uma função comum.
- `backend/handlers/auditoria_auth.go` -- `AudMeHandler` devolve `perfil`, `areas`, `admin`, `can_manage_users`; incluir `ambiente` (consulta a `companies.ambiente` pela empresa do contexto). Não mexer em `trial_ends_at` (é do Épico 4).
- `backend/main.go` -- registrar a rota junto da de limpeza da 1.2.
- `backend/handlers/ambiente_test.go` (da 1.2) -- estender.
- `frontend/src/contexts/AuthContext.tsx` -- `fetchAcesso` lê `/api/aud/me`; guardar `ambiente` no contexto. Não mexer no interceptor de fetch.
- `frontend/src/App.tsx` -- `AppLayout` (linha ~121) renderiza `AppRail` e as rotas: incluir a faixa aqui; incluir a rota nova dentro do bloco `/config` com `PlatformAdminRoute`.
- `frontend/src/lib/navigation.ts` -- módulo `config`: nova aba "Dados e ambiente" com `masterOnly: true`.
- Novo `frontend/src/pages/ConfigAmbienteEmpresa.tsx` -- tela de administração: mostra o ambiente, botão de apagar (só em teste, com senha) e de virar (senha mais frase). Usa `api` de `pages/aud/util.ts` e `sonner`, como `AdmCarga.tsx`.
- `frontend/src/pages/GestaoAmbiente.tsx` -- é a tela de "ambientes" no sentido de inquilino (grupos e empresas); não é esta. Não mudar.

## Tasks & Acceptance

**Execution:**
- [ ] `backend/handlers/ambiente.go` -- `AmbienteVirarHandler` e a função comum de senha e admin -- reaproveita a 1.2
- [ ] `backend/handlers/auditoria_auth.go` -- campo `ambiente` em `/api/aud/me` -- faixa e tela dependem dele
- [ ] `backend/main.go` -- registrar a rota -- exposição
- [ ] `backend/handlers/ambiente_test.go` -- casos da matriz (virar, `403` por senha, por frase, não admin, `409` já em produção, `ambiente` em `/me`) -- prova
- [ ] `frontend/src/contexts/AuthContext.tsx`, `App.tsx`, `lib/navigation.ts`, `pages/ConfigAmbienteEmpresa.tsx` -- faixa, aba e tela -- interface

**Acceptance Criteria:**
- Dado empresa em `teste`, quando o admin pede para virar informando a senha e digitando "VIRAR PARA PRODUCAO", então `ambiente` passa a `producao`, a trilha registra quem e quando, e a resposta lista as contas com lançamento de origem arquivo (aviso, não bloqueia).
- Dado ambiente `teste`, quando qualquer tela carrega, então a faixa "AMBIENTE DE TESTE" aparece fixa (via `/api/aud/me`).
- Dado `producao`, quando o admin abre a tela de administração, então não há faixa, a tela mostra "Produção" e não há botão de apagar.
- Dado senha errada, quando pede para virar, então `403` e nada muda.

## Implementation Notes

Tamanho do plano: cerca de 1700 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

## Design Notes

A tela fica numa aba nova do módulo Configurações, porque "Ambiente" já nomeia a tela de inquilinos (`GestaoAmbiente.tsx`) e misturar os dois confundiria. A 1.2 só tem backend: o botão de apagar nasce aqui.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -p 1 ./...` -- expected: sem erro
- `cd frontend && npm run typecheck` -- expected: só o erro anterior em `ResetPassword.tsx(13,9)`, que não é desta história (`npm run lint` não roda por falta de configuração do ESLint)

**Manual checks (if no CLI):**
- Com `npm run dev` e o banco local: faixa visível em teste, some em produção, e a tela troca o botão de apagar por "Produção".

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| `/api/aud/me` devolve 500 se a consulta do ambiente falhar e derruba o menu | medium | patch | agora registra e devolve `ambiente` vazio |
| `ambienteEmpresa` não zera no logout, na troca de empresa nem na falha do `/me` | medium | patch | corrigido em `AuthContext.tsx` |
| Tela presa em Teste após virada ou 409; duplo clique | medium | patch | estado local para Produção, 409 recarrega, `useRef` contra duplo clique |
| `Scan` falha se `conta` for NULL e bloquearia a virada | low | patch | `COALESCE(conta,'')` |
| Atomicidade da virada sem teste | medium | patch | `TestAmbienteVirar_FalhaNaTrilhaDesfazTudo` |
| Teste de virada concorrente; ramos de erro de `conferirSenha` | low | rejeitado | `FOR UPDATE` presente; dano é só uma linha de trilha duplicada |
| Sem limite de tentativas de senha nem trilha de falha | low | defer | já registrado na 1.2; Épico 4 |
| Aviso das contas de arquivo some ao sair da tela | low | defer | a resposta da virada é a única fonte; registrar |
| Frase sem trim, `limpar` sem frase, trilha só com a contagem, nome da aba, faixa fora do layout | low | rejeitado | a spec define frase exata e escopo; sem dano real |
