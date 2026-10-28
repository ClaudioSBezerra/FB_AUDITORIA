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

Total de Open Questions: 9. Enquanto houver pergunta aberta, a spec não sai de `draft`. Ao responder, a decisão entra no bloco congelado da spec e a pergunta é apagada.

## Open Questions (numeração única)

1. **(1.1) Sem PostgreSQL acessível, o que `go test ./...` faz?** Opções: (a) falhar com mensagem clara; (b) pular os testes de banco. Recomendação: (a), porque um pulo silencioso deixa passar justamente a regressão que a 1.1 existe para pegar; o dono do produto tem Postgres local.
2. **(1.2) O que impede apagar dado de domínio em Produção pelas rotas `DELETE` de empresa, grupo e ambiente (`/api/config/*`)?** Hoje elas excluem em cascata lançamentos e fechamentos, e só a trilha impede na prática. Opções: (a) gatilho `BEFORE DELETE` em `companies` que recusa `producao`, dentro da migration da 1.2; (b) deixar e registrar em `deferred-work.md`. Recomendação: (a), porque o requisito de Produção diz "nem por chamada direta" e custa um gatilho pequeno.
3. **(1.4) Qual valor de `origem` o CSV grava?** Opções: (a) manter `csv`; (b) `csv.contas`, como no exemplo da arquitetura. Recomendação: (a), porque os dados já gravados têm `csv` e seriam duplicados em vez de substituídos; (b) exigiria migration de dados.
4. **(1.5) O `409` de conta sem mapeamento passa a listar as contas?** Opções: (a) listar (até 20 contas e o total, em texto simples); (b) manter a mensagem fixa. Recomendação: (a), porque o requisito de fechar exige a lista e nenhuma outra história a entrega.
5. **(1.7) Se a produção já tiver o mesmo mês fechado mais de uma vez, ou por filial e consolidado, a migration aborta e a API não sobe. Como tratar?** Opções: (a) aceitar o aborto e o dono do produto conferir a tabela antes do push; (b) a migration marca as versões antigas como reabertas (altera dado de fechamento); (c) índice restrito ao que já é único (sem garantia nos meses antigos). Recomendação: (a), com a consulta de conferência no `Implementation Notes`, porque (b) contraria "nunca tocar fechamentos" e (c) esvazia a garantia.
6. **(1.8) Como fica o DRE de uma filial num mês fechado?** Opções: (a) congelar também por filial (filial na chave de linhas e contas); (b) voltar ao vivo para o filtro de filial; (c) recusar o filtro de filial em mês fechado. Recomendação: (a), porque é a única que mantém "só do snapshot" e o recorte por filial; o custo é gravar mais linhas por fechamento, com volume pequeno.
7. **(1.9) As variáveis `JC_ORACLE_HOST` e `JC_ORACLE_SERVICE` já estão cadastradas no Coolify?** Opções: (a) já existem; (b) não existem, cadastrar antes do push. Recomendação: o dono do produto responde; sem certeza, tratar como (b) e cadastrar antes do push.
8. **(1.9) No compose, host e service ficam obrigatórios com `${VAR:?...}`?** Opções: (a) obrigatórios no compose; (b) sem padrão e sem `:?`, obrigatórios só no `jc-probe` e no código que ler Oracle. Recomendação: (a) se a pergunta 7 for "já existem" e (b) se não, para o deploy não quebrar por uma variável que a API ainda não usa.
9. **(1.9) O padrão de `JC_ORACLE_DRE_VIEWS` também sai do repositório?** Opções: (a) remover e tornar obrigatória; (b) manter. Recomendação: (a), porque o critério cita "schema" e o plano de execução já trata o nome das views como dado a revisar; implica cadastrar mais uma variável no Coolify.

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
