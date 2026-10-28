---
title: 'Story 1.5: Fechamento passa pelo serviço `dre` (AD-1)'
type: 'refactor'
created: '2026-10-28'
status: 'draft'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/implementation-artifacts/epic-1-context.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** `DREFechamentoHandler` calcula o snapshot (`somaPorLinha`, `aplicarSubtotais`) fora da transação e depois grava; entre o cálculo e a gravação um lançamento pode entrar e ficar fora do congelado. O cálculo do realizado também está duplicado entre o DRE exibido e o fechamento.

**Approach:** Mover o fechamento para `dre.Fechamento.Fechar` e o cálculo do realizado para `dre.Realizado`, o único caminho que calcula o realizado exibido. Fechar calcula o snapshot dentro da transação. O handler só decodifica, confere o perfil e responde, com o mesmo contrato HTTP de hoje.

## Boundaries & Constraints

**Always:** O handler não tem SQL de fechamento. Só a controladoria fecha (`c.Tem("controladoria")`; o admin de plataforma passa), como hoje, com `403` para os demais. Fechar abre a transação, calcula via `dre.Realizado` sobre essa transação, grava `dre_fechamentos`, `dre_fechamento_linhas` e a trilha `dre_fechamento/fechar` no mesmo `Commit`. `DREHandler`, `DRELancamentosHandler` e o resto do DRE passam a calcular o realizado por `dre.Realizado` e a resolver a conta por `dre.ResolverConta` (movida de `resolverConta`, mesmo comportamento: exato vence; senão o prefixo `%` mais longo). Os testes da 1.1 seguem passando sem mudança (versão, linhas, `409` sem mapeamento, trilha); a listagem de fechamentos devolve o mesmo JSON; `POST` continua `201` com `{id, versao}`.

**Never:** Não adicionar `Guardar` nem lock (1.6), não mudar a versão por filial nem o índice (1.7), não mudar o schema nem criar migration. Não alterar `dre_fechamento_linhas`. Não mexer em `DREOrcadoHandler`, `orcadoPorLinha` nem em ano anterior além de trocar o cálculo do realizado de lugar. Não mudar o JSON de `/api/aud/dre`.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior | Error Handling |
|----------|--------------|---------------------------|----------------|
| Fechar mapeado | controladoria, mês mapeado | `201`, `{id, versao}`, linhas e trilha como hoje | N/A |
| Conta sem mapa | lançamento sem mapeamento | nada gravado | `409` |
| Perfil sem permissão | diretoria | nada gravado | `403` |
| Lançamento durante o fechamento | cálculo e gravação na mesma transação | snapshot consistente com o que a transação leu | N/A |

</frozen-after-approval>

## Open Questions

- O `409` de conta sem mapeamento passa a listar as contas? Hoje a mensagem é fixa e o requisito de fechar diz que o sistema lista as contas — opções: listar na resposta, em texto simples, até 20 contas e o total (cumpre o requisito; muda o texto do erro que o Controller vê) / manter a mensagem fixa (nenhuma mudança visível; o requisito de listar fica sem história que o cubra)

## Code Map

- `backend/handlers/dre.go` -- `DREFechamentoHandler` (linhas ~595-695): origem do POST; `carregarLinhas`, `carregarMapa`, `somaPorLinha` (ignora transação: usa `*sql.DB`), `aplicarSubtotais`, `resolverConta`, `periodo`. Usados também por `DREHandler` (~234, que lê o snapshot por `filial` e depois a soma ao vivo) e `DRELancamentosHandler` (~361, usa `resolverConta`). `DREMapaHandler` e `DRELinhasHandler` usam `carregarLinhas`/`carregarMapa`: podem continuar locais ou usar `dre`.
- Novo `backend/dre/realizado.go` -- `Realizado(ctx, q Consultador, empresa, filial, Periodo, mapa)` devolvendo realizado por linha e as contas sem mapa; `Consultador` é uma interface com `QueryContext`, satisfeita por `*sql.DB` e `*sql.Tx`. `ResolverConta`, `Periodo` e subtotais ficam no mesmo pacote.
- Novo `backend/dre/fechamento.go` -- `Fechamento.Fechar(ctx, FecharIn) (FecharOut, error)`, erros tipados (`ErrContasSemMapa`) que o handler traduz para `409`. Usa `backend/trilha` (da 1.4) para a trilha.
- `backend/dre/` (da 1.4) -- pacote já existente; `dre` não importa `handlers`.
- `backend/handlers/dre_fechamento_test.go` (1.1) -- contrato que não pode mudar.
- `frontend/src/pages/aud/DRE.tsx` e `DREFechamentos.tsx` -- não mudam (contrato igual).

## Tasks & Acceptance

**Execution:**
- [ ] `backend/dre/realizado.go` -- `Realizado`, `ResolverConta`, `Periodo`, subtotais, movidos de `dre.go` -- caminho único do realizado
- [ ] `backend/dre/fechamento.go` -- `Fechar` com cálculo dentro da transação e trilha -- elimina a janela entre cálculo e gravação
- [ ] `backend/handlers/dre.go` -- `DREFechamentoHandler` fino; `DREHandler` e `DRELancamentosHandler` chamam `dre.Realizado` e `dre.ResolverConta`; remover as cópias antigas -- sem regra no handler
- [ ] `backend/dre/fechamento_test.go` -- `Fechar` com contexto já cancelado não deixa fechamento nem trilha (tudo ou nada); `ResolverConta` (exato, prefixo mais longo, sem casamento); `Realizado` sobre `*sql.Tx` e sobre `*sql.DB` dá o mesmo resultado -- prova do cálculo único
- [ ] `backend/arquitetura_test.go` (1.4) -- acrescentar: `handlers` não contém `INSERT INTO auditoria.dre_fechamento` -- trava o escritor

**Acceptance Criteria:**
- Dado o handler de fechamento, quando recebe o POST, então chama `dre.Fechamento.Fechar` sem SQL próprio, e só a controladoria fecha.
- Dado os testes da 1.1, quando rodam, então seguem passando (versão, linhas, `409` sem mapeamento, trilha).
- Dado Fechar, quando calcula o snapshot, então o cálculo acontece dentro da transação, via `dre.Realizado`, e `dre.Realizado` é o único caminho que calcula o realizado exibido.
- Dado a listagem de fechamentos, quando é pedida, então devolve o mesmo JSON de hoje.

## Implementation Notes

Tamanho do plano: cerca de 1850 tokens (estimativa), acima de 1600, porque a história cruza banco, backend e testes; sem divisão proposta.

## Design Notes

`Consultador` deixa o mesmo cálculo servir às leituras (`*sql.DB`) e ao fechamento (`*sql.Tx`). O isolamento fica no padrão do Postgres (`READ COMMITTED`) até a 1.6 explicitá-lo: dentro de uma transação ele ainda deixa um lançamento novo, confirmado por outra conexão, aparecer entre duas consultas; por isso a serialização real só vem com o lock da 1.6. Esta história remove a janela entre cálculo e gravação, não a corrida com a importação.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -p 1 ./...` -- expected: sem erro; os testes da 1.1 passam sem edição
- `grep -n "somaPorLinha\|resolverConta" backend/handlers/*.go` -- expected: vazio
