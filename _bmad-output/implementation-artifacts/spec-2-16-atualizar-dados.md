---
title: 'Story 2.16: Botão "Atualizar dados" para a gestão'
type: 'feature'
created: '2026-10-28'
status: 'done'
baseline_commit: '3842898'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** A carga é semanal; depois de uma correção no ERP o Controller espera a próxima carga para ver o DRE certo. O adaptador Oracle (2.1, 2.3 a 2.6) ainda não existe e depende de resposta do cliente.

**Approach:** Implementar o mecanismo, não a fonte: `POST /api/aud/dados/atualizar` dispara as fontes atualizáveis registradas no pacote `dre`, devolve resultado e hora e registra tudo na trilha. Sem fonte registrada, responde que não há fonte configurada, nunca falso sucesso. A fonte Oracle real se registra quando a 2.1 sair.

## Boundaries & Constraints

**Always:** Interface `dre.Atualizavel { Nome(); Atualizar(ctx, EscopoAtualizacao) (Resultado, error) }`, registro `dre.Registro` (`RegistroPadrao`, sem nenhuma fonte por padrão; nome repetido recusado) e adaptador `dre.ViaIngestor(ing, fonte)`, que grava pelo `Ingestor` e, portanto, pela guarda de período (1.6). `dre.Atualizador.Atualizar` registra o pedido (`solicitar`) antes de rodar e o resultado (`concluir`) depois, entidade `atualizacao_dados`, com quem pediu, as fontes e o resultado por fonte, também quando recusado, com erro ou sem fonte; falha ao gravar o pedido impede a execução. Falha ou recusa de uma fonte não impede as demais. Competência com fechamento ativo: fonte `recusada` com a orientação de reabrir, atualizar e fechar de novo. Rota `POST /api/aud/dados/atualizar` na área `empresa`, só `controladoria` e admin de plataforma (`403` e trilha `negado` para os demais; `405` para outros métodos). Respostas, sempre o JSON de `dre.Atualizacao` (`hora` UTC, `status`, `mensagem`, `fontes[]`): `200` para `ok` e `sem_fonte`; `409` se alguma fonte foi recusada pela guarda; `502` para erro de fonte ou parcial sem recusa; `500` (texto) só se a trilha falhar. Erro desconhecido de fonte não vaza detalhe interno (vai para o log). Frontend: botão "Atualizar dados" na tela do DRE só para controladoria e admin, com o resultado e a hora na tela e o aviso de "nenhuma fonte configurada".

**Never:** Nenhum dado ou endereço do cliente em arquivo versionado. Não escrever em `lancamentos` fora do Ingestor. Não criar tabela nova.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Sem fonte | controladoria, registro vazio | `200`, `status=sem_fonte`, "nenhuma fonte configurada" | trilha: solicitar e concluir |
| Fonte fake | controladoria | `200`, `ok`, linhas gravadas; repetir não duplica | N/A |
| Mês fechado | fonte traz mês com fechamento ativo | `409`, `recusada`, orientação; nada muda | trilha com a recusa |
| Falha de fonte | fonte devolve erro | `502`, `erro`, mensagem sem detalhe interno | log |
| Duas fontes, uma falha | N/A | `parcial`; a boa gravou | `502` (ou `409`) |
| Outros perfis | auditoria, diretoria, gestor | `403`, nada gravado | trilha `negado` |
| Admin de plataforma | `role=admin` | permitido | N/A |

</frozen-after-approval>

## Code Map

- `backend/dre/atualizar.go` -- interface, registro, `ViaIngestor`, `Atualizador`; guarda de período vem de `backend/dre/ingest.go` e `periodo.go`.
- `backend/handlers/dados_atualizar.go`, `backend/main.go` -- handler fino (`GetAudContext`, `c.Tem("controladoria")`) e rota `withAud(..., est, "controladoria")`.
- `frontend/src/pages/aud/DRE.tsx` -- botão e painel de resultado.
- Testes: `backend/dre/atualizar_test.go` (fonte fake, mês fechado, parcial), `backend/handlers/dados_atualizar_test.go`.

## Tasks & Acceptance

**Execution:**
- [x] Interface, registro, adaptador e `Atualizador` com trilha
- [x] Handler e rota
- [x] Testes com fonte fake
- [x] Botão e resultado na tela do DRE

**Acceptance Criteria:**
- Given nenhuma fonte registrada, when o Controller atualiza, then `200` com "nenhuma fonte configurada" e trilha.
- Given uma fonte fake, when atualiza, then as linhas entram uma vez só; e mês com fechamento ativo é recusado com a orientação de reabrir.
- Given outro perfil, when tenta, then `403`.
- Given `go build`, `go vet`, `go test -p 1 ./...` e `npm run typecheck` (exceto o erro antigo em `ResetPassword.tsx`), when rodam, then passam.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- expected: passam
- `cd frontend && npm run typecheck && npm run build` -- expected: só o erro anterior em `ResetPassword.tsx(13,9)`; build passa

## Implementation Notes

Decisão: sem fonte, `200` com `status=sem_fonte` (o corpo e a tela deixam claro que nada foi atualizado) em vez de `409`, porque não é conflito de estado. O escopo desta versão é a empresa inteira; filtro por competência ou fonte fica para quando houver fonte real. O orçado do Oracle (2.6) entra como outra `Atualizavel` cuja escrita passe pela guarda de período do orçado. O registro de produção está vazio por decisão: nada em `main.go` registra fonte.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| Dois cliques/pedidos simultâneos rodam as fontes em paralelo | low | defer | `deferred-work.md`; o Ingestor serializa por (empresa, origem), mas não há trava do pedido |
| Fonte lenta prende a requisição (sem timeout próprio) | low | defer | `deferred-work.md`; definir ao entrar a fonte Oracle |
| Trilha `concluir` falha depois de a fonte gravar | low | aceito | `500` com mensagem; a trilha `solicitar` já existe |
| Erro desconhecido de fonte vazaria detalhe | medium | patch | mensagem genérica, detalhe no log; coberto em `TestAtualizar_FonteFakeGravaERecusaMesFechado` |
