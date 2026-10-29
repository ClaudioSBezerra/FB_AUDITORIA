---
title: 'Story 2.18: Comparativo de fechamento (planilha do Marcos × sistema)'
type: 'feature'
created: '2026-10-29'
status: 'done'
baseline_commit: '8220e11'
route: 'dispatch'
review_loop_iteration: 0
context:
  - '{project-root}/_bmad-output/planning-artifacts/epics.md'
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

<frozen-after-approval reason="human-owned intent — do not modify unless human renegotiates">

## Intent

**Problem:** O Marcos fecha o mês numa planilha de análise (abas `MENSAL` e `Real_AA`) e o sistema calcula o mesmo DRE a partir dos lançamentos. Nada mostra, linha a linha, onde os dois não batem; sem isso não há confiança para virar para Produção.

**Approach:** Enviar a planilha de análise, gravar o conteúdo relevante em tabelas próprias (reenviar o ano substitui o escopo) e comparar, no mesmo período, o que veio da planilha com o que o sistema tem: por linha do DRE (contra o `dre.Leitor`) e por conta (contra a soma bruta de `lancamentos`). Cada linha sai `OK`, `DIVERGE`, `SO_PLANILHA` ou `SO_SISTEMA`. Mesmo desenho do comparativo de fechamento do FB_FAROL, com código e nomes próprios.

## Boundaries & Constraints

**Always:**
- Carga: `POST /api/aud/comparativo/carga` (área empresa, capacidade `carregar`), multipart `arquivo` (XLSX) e `ano` opcional (padrão `MENSAL!C2`); limite de `limiteCargaBytes()`. Só as abas `MENSAL` e `Real_AA` do ano; leitura em fluxo (`Rows`/`NextRow`), nunca `GetRows` da pasta inteira; o upload vai a arquivo temporário.
- `Real_AA`: A conta (texto; `123.0` vira `123`), B valor, C mês (AAAAMM), D loja (normalizada: `01` = `1`). Soma por (mês, loja, conta). Linha sem conta, sem mês válido, com mês de outro ano ou valor inválido é ignorada e contada.
- `MENSAL`: C2 ano, C3 visão, D1 filial resolvida; da linha 5 em diante, A chave, C descrição e D..O os doze meses já calculados. Linha sem chave (subtotal) usa o slug da descrição (mesma regra do gerador da estrutura inicial). Linhas vazias ou sem valor numérico são puladas. D1 não numérico: filial `''`, aviso, e as linhas ficam gravadas só para exibição.
- Tabelas `auditoria.comparativo_externo_cargas`, `_linhas` e `_contas` (migration 017), com `empresa_id`; nada disso entra em `lancamentos`. A limpeza de teste as cobre.
- Reenvio do ano: apaga e regrava as contas do ano (todas as lojas) e as linhas do ano da filial da planilha; transação única, advisory lock por empresa, trilha na mesma transação (`comparativo_externo`/`carregar`, só contagens e ano, nunca valores).
- Leitura: `GET /api/aud/comparativo` (capacidade `ler`; o gestor recebe 403, registrado como `negado`): `ano`, `mes`, `filial`, `nivel` (`linha`|`conta`), `tolerancia` (R$, padrão 1,00, aceita vírgula), `origem` (só por conta), `status` (filtro da lista), `limite` (padrão 500) e `offset`, `formato=csv`. `GET /api/aud/comparativo/periodos` lista o que há gravado. Trilha `visualizar`/`exportar` com os parâmetros e as contagens por status.
- DIVERGE quando |planilha − sistema| > tolerância. Ordem: DIVERGE (maior diferença absoluta primeiro), SO_PLANILHA, SO_SISTEMA, OK.
- Por conta: filial obrigatória; sistema = soma de `valor` de `lancamentos` na competência e filial normalizada, por conta, sem o sinal do Mapa, restrito à `origem` pedida. Por linha: sistema = `dre.Leitor.Ler` (mês fechado do snapshot, aberto ao vivo) e a estrutura congelada quando todo o mês vem do snapshot; casamento por código, depois pelo slug do código, depois pelo slug da descrição.
- CSV: BOM, `;`, vírgula decimal (como a 2.12), apóstrofo antes de texto que comece com `=`, `+`, `-`, `@`, tabulação ou retorno; números fora dessa proteção.
- Toda consulta filtra por `empresa_id`.

**Never:** Não escrever em `lancamentos`. Não aplicar o sinal do Mapa por conta. Não ler o Oracle. Nenhum valor, código de conta ou nome do cliente em teste, spec, mensagem de commit ou trilha.

## I/O & Edge-Case Matrix

| Scenario | Input / State | Expected Output / Behavior |
|----------|--------------|---------------------------|
| Dentro da tolerância | diferença menor ou igual à tolerância | `OK` |
| Fora da tolerância | diferença maior | `DIVERGE` com planilha − sistema |
| Só na planilha | linha ou conta sem par no sistema | `SO_PLANILHA` |
| Só no sistema | linha ativa com valor (ou subtotal) sem par na planilha; conta sem par | `SO_SISTEMA` |
| Lado ausente com valor zero | só um lado e valor exatamente zero | `OK` (os dois lados valem zero) |
| Subtotal | linha da planilha sem chave | casa pelo slug da descrição com o subtotal do sistema |
| Visão sem filial | `MENSAL!D1` vazio ou texto | aviso na carga; por linha, aviso e nenhuma linha |
| Sem `MENSAL` ou `Real_AA` | aba ausente | 400 com mensagem clara, nada gravado |
| Reenvio | mesmo ano | substitui; cargas anteriores ficam só no histórico |
| Mês fechado | fechamento ativo | por linha lê do snapshot; `fechada: true` |
| Gestor | perfil sem `ler` | 403 e `negado` na trilha |
| Carga sem `carregar` | diretoria, auditoria, gestor | 403, nada gravado |
| Outra empresa | dados de outra empresa | invisíveis |

</frozen-after-approval>

## Code Map

- `backend/migrations/017_comparativo_externo.sql` -- as três tabelas, índices por (empresa, competência, filial) e a nova `limpar_dados_teste`.
- `backend/dre/fontes/planilha/analise.go` -- leitura em fluxo das abas `MENSAL` e `Real_AA` (sem SQL).
- `backend/dre/comparativo_externo.go` -- carga (`ComparativoExterno.Carregar`, COPY em lote), tipos e `Comparar`.
- `backend/dre/comparativo_externo_ler.go` -- os dois lados, o casamento de linhas, a classificação e a ordenação.
- `backend/handlers/comparativo.go` -- `ComparativoCargaHandler`, `ComparativoHandler`, `ComparativoPeriodosHandler`, CSV seguro.
- `backend/main.go`, `rotas_capacidades_test.go`, `escopo_rotas_test.go` -- rotas (`CapCarregar` na carga, `CapLer` na leitura) e as tabelas de teste.
- `backend/handlers/comparativo_test.go` -- testes com banco e XLSX fictício gerado em código.
- `frontend/src/pages/aud/Comparativo.tsx`, `lib/navigation.ts`, `App.tsx` -- aba "Comparativo" em Administrativo.

## Verification

**Commands:**
- `cd backend && go build ./... && go vet ./... && go test -count=1 -p 1 ./...` -- passam
- `cd frontend && npm run typecheck && npm run build` -- só o erro antigo de `ResetPassword.tsx`

## Implementation Notes

Decisões de detalhe:
- O CSV usa vírgula decimal, como a 2.12 de fato faz (o pedido dizia "ponto", mas mandava seguir a 2.12).
- A tolerância é só absoluta. O percentual (`diferenca_pct`, sobre o valor da planilha, quando diferente de zero) é informativo e não muda o status.
- Lado ausente com valor zero conta como `OK`: nem a planilha nem o sistema têm o que mostrar. Linha do sistema sem par só aparece se tem valor ou é subtotal.
- O resumo (contagens, soma das diferenças absolutas) é sempre sobre todas as linhas; o filtro `status` e a paginação valem só para a lista. Os totais da planilha e do sistema só valem por conta (`totais_validos`); por linha somar subtotais dobraria a conta.
- A planilha só grava as linhas de `MENSAL` se o ano de `C2` for o ano da carga; senão avisa e grava só as contas.
- Reenvio apaga as linhas do ano da mesma filial; filiais diferentes do mesmo ano convivem (a aba `MENSAL` mostra uma visão por arquivo). As contas do ano, de todas as lojas, são sempre substituídas por inteiro.
- Chaves repetidas na `MENSAL` ganham sufixo numérico, com aviso.
- Por linha, sem filial informada vale a filial da carga de linhas mais recente do ano.
- O XLSX é copiado para um arquivo temporário e aberto por caminho; o `excelize` lê as linhas em fluxo e extrai XML grande para disco.
- Pontos frágeis: o casamento por slug da descrição depende de a estrutura do sistema ter sido gerada com a mesma regra (história 3.3) ou ter descrições iguais às da planilha; o sinal das linhas da planilha (despesa negativa) é comparado como vem, então um Mapa com sinais diferentes aparece como divergência; uma planilha salva sem o valor calculado das fórmulas de `MENSAL` não traz valores (a carga avisa que não achou linhas com valor); a comparação por conta é por filial do ERP, não por filial gerencial; `UnzipSizeLimit` de 4 GB protege contra zip bomb só em parte.

## Review Triage Log

| Achado | Veredito | Rota | Evidência |
|--------|----------|------|-----------|
| A varredura de handlers que leem valor acusou a carga (`planilha.Ler`) | low | patch | handler listado em `leSemDevolverValor`, resposta só com contagens |
| Comparar por mês e filial únicos deixa de fora acumulado, filiais lado a lado, centro de custo e trimestre | medium | defer | `deferred-work.md` |
| Comparar contra a origem Oracle quando existir | low | defer | `deferred-work.md` |
