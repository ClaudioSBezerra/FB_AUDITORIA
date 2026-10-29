---
title: 'Story 3.2: Carregar estrutura e Mapa por arquivo'
type: 'feature'
created: '2026-10-29'
status: 'review'
route: 'dispatch'
context:
  - '{project-root}/_bmad-output/planning-artifacts/respostas-cliente-2026-10-28.md'
---

## Intent

**Problem:** o cliente importou lançamentos, mas o DRE sai vazio: não há estrutura (`dre_linhas`) nem Mapa (`dre_mapa`) em produção e não existe como criar a estrutura (a rota de linhas é só leitura; o Mapa só tem PUT por conta).

**Approach:** duas cargas por arquivo (CSV ou XLSX), tudo ou nada, e um aviso no DRE com a lista das contas sem Mapa.

## Contrato

- `POST /api/aud/dre/estrutura/carga` (capacidade `editar_estrutura`) e `POST /api/aud/dre/mapa/carga` (`editar_mapa`), multipart `arquivo`, limite `limiteCargaBytes()`. Perfil sem a capacidade: 403 (diretoria, auditoria, gestor); controladoria e analista passam.
- `GET /api/aud/dre/mapa/sem-mapa?ano=&mes=&visao=&limite=&formato=csv` (capacidade `ler`; o gestor recebe 403, registrado, porque a resposta traz valor). Lista conta, descrição e soma por conta, maiores valores em módulo primeiro; `limite` padrão 200, máximo 5000; CSV traz todas.
- Uma transação por carga, com advisory lock por empresa (`estrutura-mapa:<empresa>`, o mesmo para as duas cargas) e a trilha no mesmo commit: entidades `dre_estrutura` e `dre_mapa`, acao `carregar`; payload só com contagens, códigos (até 200) e o aviso de que meses fechados não mudam. Nunca valores.

### Estrutura

Colunas `codigo`, `descricao`, `tipo` (`conta` ou `subtotal`) obrigatórias; `nivel` (padrão 1), `ordem` (padrão = posição x 10), `soma_de` e `papel` opcionais. Cabeçalho normalizado (acento, caixa, `_`, `-` e espaço equivalentes; aliases `chave`, `nome`, `soma de`). `soma_de` separa por vírgula, `|` ou `;`; `-COD` subtrai (o motor de subtotais já aceita).

Valida: código único e sem `-` no início; tipo válido; subtotal com `soma_de` não vazio e só de códigos do arquivo; sem ciclo (nem autorreferência); conta sem `soma_de`; `papel` só `faturamento_liquido` e no máximo uma linha. Qualquer erro: 400 com até 20 problemas e o total, nada gravado.

Semântica: **substitui** a estrutura da empresa. Upsert pelo código (reativa linha inativa) e **marca `ativo=false`** as linhas que não vieram; nunca apaga (histórico e snapshots dependem delas).

### Mapa

Colunas `conta_padrao` (aceita `%` só no fim) e `linha_codigo` obrigatórias; `sinal` (1 ou -1, vazio = 1). Valida conta única no arquivo e `linha_codigo` existente, ativa e do tipo conta. Semântica: **substitui o Mapa inteiro**: upsert do que vem, remoção do que não vem, na mesma transação. Resposta: `inseridas`, `atualizadas`, `removidas`, `meses_fechados`.

## Decisões

- **Linha que sai com conta no Mapa: recusar (409)**, listando `codigo (N conta(s))`. Alternativa de aceitar e reportar deixaria o Mapa apontando para linha inativa e contas caindo fora do DRE sem aviso; recusar é a mais segura. O mesmo vale para linha que passa de conta a subtotal. Para tirar a linha, carregue antes um Mapa sem essas contas.
- **Papel:** se o arquivo traz um papel, ele substitui o da empresa. Se não traz nenhum, o papel marcado na tela fica nas linhas que permanecem (reenviar uma estrutura sem a coluna `papel` não desfaz a base dos percentuais). A linha que sai perde o papel (o índice único é por empresa, inclusive inativas).
- **Meses fechados** não mudam (leitura do snapshot, 1.8); a resposta e a trilha dizem isso.
- **FR-9:** o DRE já devolvia `nao_mapeado` (meses ao vivo). A faixa passou a "N conta(s) sem Mapa (R$ X) ficaram fora do DRE — ver Mapa"; o link abre o Mapa em `?sem=AAAA-MM&visao=` com a lista (`ContasSemMapa`). Mês fechado: omitido, porque o fechamento recusa conta sem Mapa e o snapshot não tem o que listar.
- **Leitura:** novo `csv.LerEstrutura` e `csv.LerMapa` sobre `lerTabela` (mesmo leitor de CSV e XLSX do importador e do dicionário). O serviço fica em `dre.EstruturaCarga`.

## Tela

Administrativo > Mapa do DRE (`DREMapa.tsx`): blocos "Carregar estrutura" e "Carregar Mapa" (upload, resultado, erros, botão de modelo CSV gerado no navegador com 3 linhas fictícias) para quem edita; lista de contas sem Mapa quando se chega pelo aviso do DRE.

## Verificação

Testes com banco em `handlers/dre_carga_test.go`: estrutura válida, subtotal inexistente, ciclo, autorreferência, dois papéis, linha removida com conta no Mapa (409) e liberada depois, Mapa que substitui, linha inexistente, `%` específico, 403 por perfil, trilha sem valores, outra empresa isolada, sem-mapa com soma correta (limite, CSV, acumulado). Rotas acrescentadas às tabelas de `rotas_capacidades_test.go`.
