# Épico 1 — índice das especificações para a revisão da manhã

Todas as specs estão em `draft`, rota `dispatch`, sem nenhuma linha de código alterada. O contexto do épico está em `epic-1-context.md`. Tamanhos são estimativas (caracteres divididos por 3,6); todas passam de 1600 tokens, e cada spec declara isso no `Implementation Notes`. Nenhuma divisão foi proposta, porque as histórias cruzam banco, backend e testes.

| História | Arquivo da spec | Tokens (aprox.) | Open Questions |
|----------|-----------------|-----------------|----------------|
| 1.1 Testes do fechamento e da importação | `spec-1-1-testes-fechamento-importacao.md` | 1900 | 1 |
| 1.2 Marcador Teste/Produção e apagar | `spec-1-2-ambiente-teste-producao-apagar.md` | 2000 | 1 |
| 1.3 Virar para Produção e faixa | `spec-1-3-virar-producao-faixa-ambiente.md` | 1700 | 0 |
| 1.4 Importação CSV pelo Ingestor | `spec-1-4-importacao-csv-ingestor.md` | 2050 | 1 |
| 1.5 Fechamento no serviço `dre` | `spec-1-5-fechamento-servico-dre.md` | 1850 | 1 |
| 1.6 Guarda de período | `spec-1-6-guarda-de-periodo.md` | 1750 | 0 |
| 1.7 Fechamento ativo único e reabertura | `spec-1-7-fechamento-unico-reabertura.md` | 2400 | 1 |
| 1.8 Mês fechado lido do snapshot | `spec-1-8-mes-fechado-do-snapshot.md` | 2050 | 1 |
| 1.9 Sem endereço do banco do cliente | `spec-1-9-sem-endereco-do-cliente.md` | 2000 | 3 |

Respondidas pelo dono do produto em 28/10/2026 (já no bloco congelado das specs): 2, 5, 6, 7, 8 e 9. As 1, 3 e 4 foram aprovadas como recomendado. Todas as specs estão `approved`. Enquanto houver pergunta aberta, a spec não sai de `draft`. Ao responder, a decisão entra no bloco congelado da spec e a pergunta é apagada.

## Open Questions (numeração única)


## Inconsistências que encontrei entre epics.md e o código real

- **1.7, índice único e dados existentes.** Hoje não existe a noção de reabertura: todo fechamento já gravado tem `reaberto_em` nulo, então qualquer mês fechado duas vezes (versões 1 e 2) ou por filial e consolidado viola o índice parcial. O critério "aborta sem alterar dado" derruba o boot da API (o runner faz `log.Fatal`), e a regra de nunca tocar fechamentos impede marcar as antigas como reabertas. Pergunta 5.
- **1.7, mensagem de aborto e o runner de migrations.** O runner em `main.go` trata como "já aplicada" qualquer erro com `already exists` ou `duplicate`. Uma migration que aborta com essas palavras seria registrada como executada, sem o índice. A spec exige mensagem sem elas e um teste.
- **1.8, snapshot sem filial.** O critério diz que o DRE de mês fechado lê só do snapshot, mas `dre_fechamento_contas` e as linhas descritas não têm filial, e o DRE de hoje aceita `?filial=`. Os subtotais também são valores congelados, então não dá para derivar uma filial do snapshot do grupo. Pergunta 6.
- **1.8, detalhe do mês fechado.** Hoje o detalhe lista lançamentos (com documento e histórico); com a leitura só do snapshot, o detalhe de mês fechado passa a mostrar contas, sem documento. O critério implica isso, mas não o diz.
- **FR-9 e 1.5.** O requisito manda listar as contas sem mapeamento; o código devolve mensagem fixa e nenhuma história de 1.x cobre a lista. Pergunta 4.
- **1.2 e requisito de Produção.** O critério cobre só a função de limpeza; as rotas `DELETE` de empresa, grupo e ambiente excluem em cascata e hoje só param porque a cascata esbarra na trilha imutável. Pergunta 2.
- **1.5, tamanho.** O título fala só do fechamento, mas "`dre.Realizado` único caminho do realizado exibido" obriga a mexer também em `DREHandler` e `DRELancamentosHandler`. Está declarado na spec.
- **1.1, runner fora de alcance.** As migrations rodam no boot, no pacote `main`; os testes em `handlers` não conseguem chamá-lo, então o helper repete a regra (aplicar `migrations/*.sql` em ordem). Se o runner mudar, o helper precisa acompanhar.
- **1.2 e a função `imutavel_salvo_em_teste`.** Ela é criada na 1.2, mas nenhum gatilho de imutabilidade a usa até a 1.7 (hoje só `audit_log` tem gatilho, e este nunca é liberado). Não é erro, mas a 1.2 não consegue testar o efeito completo da função.
- **1.9 e o deploy.** Tornar as variáveis obrigatórias no compose faz o deploy falhar se não estiverem cadastradas, embora a API ainda não leia Oracle. Perguntas 7 e 8.
- **Risco para a 1.7 e o gatilho.** `fechado_por` tem `ON DELETE SET NULL`, que é um `UPDATE`; o gatilho de imutabilidade passará a bloquear a exclusão de um usuário que já fechou um mês. É coerente com "usuários nunca são apagados", mas o `DeleteUserHandler` atual vai falhar nesse caso (já registrado como dívida AR-16 para a trilha).
- **Fora do escopo, achado.** O endereço do banco do cliente também aparece em um arquivo de revisão dentro de `_bmad-output/planning-artifacts`, fora dos três arquivos que a 1.9 cobre.
