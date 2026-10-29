---
status: final
stepsCompleted: [1, 2, 3, 4]
inputDocuments:
  - _bmad-output/planning-artifacts/prds/prd-FB_AUDITORIA-2026-10-27/prd.md
  - _bmad-output/planning-artifacts/architecture/architecture-FB_AUDITORIA-2026-10-27/ARCHITECTURE-SPINE.md
---

# FB_AUDITORIA — Release A1 (DRE gerencial) - Epic Breakdown

## Overview

Este documento decompõe os requisitos do PRD e as decisões da arquitetura da Release A1 em épicos e histórias implementáveis. O projeto é brownfield: a Fase 1 já existe em produção, e as histórias dizem o que muda em relação a ela. Não há documento de UX (a única referência visual é o protótipo de painel que o cliente já usa, fora do repositório).

## Requirements Inventory

### Functional Requirements

FR-1: Importar o Realizado consolidado do banco do cliente, por Competência e Filial, sem escrever no banco do cliente; reimportar substitui, não duplica; competência fechada é recusada.
FR-2: Importar o Orçado por ano, Filial e Conta gerencial, convertendo as 12 colunas de mês em um Lançamento de Orçado por Competência; nulo é "sem orçado", não zero.
FR-3: Carregar o Realizado por Conta gerencial por arquivo CSV (provisório), com validação antes de gravar e recusa em competência fechada.
FR-4: Registrar os Ajustes que vêm do ERP junto do realizado (o sistema não lança ajuste manual), exigir a Justificativa dos Gerentes afetados e registrar cada justificativa em log; o Ajuste não é editável no sistema.
FR-5: Manter a estrutura de Linhas do DRE (descrição, ordem, nível, tipo conta ou subtotal, linhas somadas).
FR-6: Manter o Mapa de contas (Conta gerencial para Linha do DRE com sinal, e para Departamento); a associação mais específica prevalece.
FR-7: Exibir o DRE mensal e acumulado, por Filial ou grupo, com subtotais exatos e percentual sobre o Faturamento Líquido; competência fechada usa os valores congelados.
FR-8: Comparar Realizado com Orçado e Ano anterior, em valor e percentual, por Linha do DRE e Conta gerencial, descendo até a Filial; "sem orçado" é indicado.
FR-9: Fechar a Competência do grupo inteiro (fechamento sempre consolidado), recusando se houver conta sem mapeamento, só pelo Controller, com versão e trilha.
FR-10: Reabrir o Fechamento com motivo obrigatório, criando nova versão e preservando as anteriores.
FR-11: Bloquear carga, importação e Ajuste em Competência com Fechamento ativo.
FR-12: Registrar Justificativa de Desvio ou de Ajuste, no escopo do Departamento do Gerente, imutável e versionada.
FR-13: Definir o limite de Desvio que exige Justificativa (Controller); mudar o limite não reclassifica meses fechados.
FR-14: Enviar por e-mail a cada Gerente o resumo do seu Departamento, com link para justificar; o envio é registrado na trilha.
FR-15: Configurar e aplicar o Rateio das despesas de Departamentos centralizados entre as Filiais; a soma rateada é igual ao valor do departamento.
FR-16: Exibir o ranking dos maiores Desvios, com filtros por ano, Filial, Departamento e Gerente, e o status da Justificativa.
FR-17: Exibir tendência da despesa sobre o Faturamento Líquido, ponte do resultado e projeção de fechamento do ano.
FR-18: Controlar acesso por perfil e, para o Gerente, por Departamento, em toda rota do backend.
FR-19: Restringir o cadastro de usuários ao administrador; cadastro público desativado.
FR-20: Carregar o dicionário de contas (conta, descrição, grupo, Departamento) por arquivo.
FR-21: Marcar o Ambiente da Empresa como Teste ou Produção: em Teste o administrador apaga os dados de domínio (com a senha de novo); em Produção nada pode ser apagado; a virada é de mão única; e-mail só ao administrador em Teste; faixa visível de ambiente.
FR-22: Bloquear e desbloquear usuário, com motivo; nenhum usuário expira por tempo; o acesso só termina por bloqueio.

### NonFunctional Requirements

O PRD não tem uma seção de requisitos não funcionais transversais; os itens abaixo foram **derivados** do PRD e da arquitetura, e precisam de confirmação.

NFR1: Toda escrita relevante grava na trilha de auditoria imutável, dentro da transação (FR-1, FR-3, FR-4, FR-9, FR-10, FR-12, FR-19).
NFR2: A autorização é validada no backend em toda rota; a interface só reflete (FR-18).
NFR3: Ajustes, justificativas e o snapshot do fechamento não podem ser editados nem apagados (FR-4, FR-9, FR-12).
NFR4: Valores monetários são exatos (`NUMERIC(18,2)`); todo subtotal é igual à soma de seus componentes e o rateio soma exatamente o valor de origem (FR-7, FR-15).
NFR5: Nenhum dado real do cliente nem referência a contrato no repositório ou no quadro do projeto.
NFR6: Backup do banco funcionando e **restauração testada** antes do go-live com dado real.
NFR7: Segredos (Oracle, SMTP, JWT, banco) só em variáveis de ambiente; nenhum endereço do banco do cliente no repositório.
NFR8: O banco do cliente é só lido, com timeout, e nunca consultado sem filtro.
NFR9: O servidor nunca devolve a um Gerente dado fora do seu Departamento, inclusive subtotais e percentuais (FR-18).
NFR11: Em Produção nenhum dado de domínio pode ser apagado, nem por chamada direta à API (FR-21).
NFR10: Disponibilidade, desempenho e retenção da trilha **não estão definidos** no PRD; ficam como pergunta aberta.

### Additional Requirements

Brownfield, sem starter template: a Fase 1 existe em produção; o trabalho é incremental.

- AR-1 (AD-1, AD-2): migrar o fechamento (`dre.go`) e a importação CSV (`dre_import.go`) para o pacote `backend/dre`; nenhum handler escreve em lançamento, orçado, ajuste ou fechamento; um teste falha o build se outro pacote o fizer.
- AR-2 (AD-3, AD-16): `origem` em `lancamentos` e `orcado_conta`; reimportação por escopo; `conta_fonte` (uma conta, uma fonte); leitura vazia nunca apaga; `filial` vazia rejeitada.
- AR-3 (AD-5): `orcado_conta` como única fonte de orçado do DRE; escrita de `dre_orcado` removida.
- AR-4 (AD-6, AD-7): tabelas `ajustes` e `justificativas` append-only por trigger; chave canônica de desvio (`dre.ChaveDesvio`).
- AR-5 (AD-8): `dre.Periodo.Guardar` com `pg_advisory_xact_lock(dre.ChaveLock)` em toda escrita; isolamento READ COMMITTED explícito.
- AR-6 (AD-9, AD-17): `dre_fechamentos` com `reaberto_*`, `limite_desvio` e índice único parcial em `(empresa, competência)`; Fechar dentro do lock via `dre.Realizado`; snapshot em `dre_fechamento_linhas` e novo `dre_fechamento_contas`, imutáveis.
- AR-7 (AD-10): `departamentos`, `conta_departamento`, `usuario_departamentos`; escopo fail closed na leitura e na escrita; `dre.FiltroEscopo`; prefixo em Go, nunca `LIKE`.
- AR-8 (AD-11): `perfis.go` com capacidades; middleware por capacidade; perfil `analista` no `CHECK`.
- AR-9 (AD-12): migração única de `users.areas` (`estrategico`→`empresa`, `administrativo`→`configuracoes`, `operacional`→`operacao`), novo `DEFAULT`, rotas e `navigation.ts` no mesmo release.
- AR-10 (AD-13): `outbox_email` genérica, worker com `SKIP LOCKED`, `lista_aviso` em `config_empresa`; e-mail do DRE enfileirado no Fechamento; em Teste, só ao administrador.
- AR-11 (AD-14): `rateio_regras`, percentuais e `rateio_aplicado`; resto de centavos à maior filial; reabrir invalida.
- AR-12 (AD-15): gatilho manual "Importar agora" obrigatório; agendador opcional e última história, com `pg_try_advisory_lock` em `*sql.Conn` dedicada, `America/Sao_Paulo`, `auditoria.execucoes`, trilha com `user_id` nulo e origem agendador.
- AR-13: migrations `004+` aditivas e idempotentes, nunca tocam fechamentos; usuários desativados e não apagados.
- AR-14: `ALLOW_SELF_REGISTER=false` por padrão; `JC_EMPRESA_ID` e `JC_ORACLE_*` sem valores padrão no repositório (remover host padrão de compose, `.env.example` e `jc-probe`).
- AR-15: backup do Postgres da Auditoria com restauração testada. Verificado no servidor: o cron diário `pg-backup-gdrive.sh` (23:00 UTC, `pg_dump -Fc`, `rclone` para o OneDrive, retenção de 56 dias, alerta por e-mail) tem lista fixa de stacks e usa `-U postgres`; a Auditoria não está na lista e seu usuário é outro. Trabalho: incluir a Auditoria no script (usuário lido de `POSTGRES_USER`), versionar uma cópia sem segredos no repositório, documentar e executar o teste de restauração; confirmar com o cliente o destino dos dados no OneDrive.
- AR-17 (AD-18): `companies.ambiente` (padrão `teste`, trigger de mão única), função `limpar_dados_teste`, função `imutavel_salvo_em_teste` nos gatilhos de imutabilidade, senha de novo para apagar e virar, faixa de ambiente, e-mail em Teste só ao administrador.
- AR-18 (AD-19): `users.bloqueado_em`, `bloqueado_por` e `motivo_bloqueio`; verificação no login, na renovação de sessão e no middleware; **remover** a verificação por `trial_ends_at` do login (`auth.go`) e do middleware (`auditoria_auth.go`), a extensão de teste do `PromoteUserHandler`, o prazo de 14 dias do `CreateUserHandler` e, no frontend, a coluna Trial de `AdminUsers.tsx` e o "Licença vence" do `AppRail.tsx`.
- AR-19 (AD-20): `dre_linhas.papel` e `dre.LinhaPorPapel` para percentual sobre o Faturamento Líquido, ponte e projeção.
- AR-16 (dívida, não bloqueia): `node:18-alpine`, tags fixas de `cloudflared` e `alpine`, `go mod tidy`; `DeleteUserHandler` versus o trigger de `audit_log`.

### UX Design Requirements

Não há documento de UX. As telas seguem os padrões de componentes e o `navigation.ts` existentes; o protótipo de painel do cliente é referência visual para FR-16 e FR-17.

### FR Coverage Map

FR-1: Epic 2 - Importar o Realizado consolidado do banco do cliente
FR-2: Epic 2 - Importar o Orçado por conta
FR-3: Epic 2 - Carregar o Realizado por conta por arquivo (validação e provisório)
FR-4: Epic 5 - Ajuste vindo do ERP, Justificativa e log
FR-5: Epic 3 - Estrutura de Linhas do DRE
FR-6: Epic 3 - Mapa de contas com Departamento
FR-7: Epic 3 - DRE mensal e acumulado
FR-8: Epic 3 - Comparação com Orçado e Ano anterior
FR-9: Epic 1 - Fechar a Competência (sempre consolidado)
FR-10: Epic 1 - Reabrir o Fechamento com motivo
FR-11: Epic 1 - Bloquear escrita em Competência fechada
FR-12: Epic 5 - Justificativa no escopo do Departamento
FR-13: Epic 5 - Limite de Desvio
FR-14: Epic 5 - E-mail do DRE por Departamento
FR-15: Epic 7 - Rateio entre Filiais
FR-16: Epic 6 - Ranking dos principais Desvios
FR-17: Epic 6 - Tendência, ponte do resultado e projeção
FR-18: Epic 4 - Acesso por perfil e por Departamento
FR-19: Epic 4 - Cadastro público desativado (primeira história a sair)
FR-20: Epic 2 - Dicionário de contas por arquivo
FR-21: Epic 1 - Ambiente Teste ou Produção
FR-22: Epic 4 - Bloqueio de usuário no lugar da expiração por tempo

## Epic List

### Epic 1: Fechar o mês com segurança
O Controller fecha o mês do grupo e o reabre com motivo, sabendo que ninguém altera números fechados; a equipe valida o sistema num ambiente de Teste que pode ser apagado e que vira Produção de forma irreversível.
**FRs covered:** FR-9, FR-10, FR-11, FR-21
**Notas:** abre com testes de caracterização do fechamento e da importação atuais (não há nenhum teste Go); migra os handlers para o pacote `dre` (AD-1, AD-2); guarda de período e fechamento ativo único (AD-8, AD-9, AD-17); marcador Teste/Produção, apagar com senha, faixa de ambiente e virada (AD-18); remove o host padrão do Oracle do repositório. Fechamento só consolidado.

### Epic 2: Os dados do cliente entram sozinhos
O analista importa o realizado consolidado e o orçado do banco do cliente com um clique, carrega por arquivo o que ainda não vem de lá, e confere que o resultado bate com a planilha atual.
**FRs covered:** FR-1, FR-2, FR-3, FR-20
**Notas:** "Importar agora" manual primeiro; agendador é opcional e a última história (AD-15); guardar a UF no lançamento (AD-3); orçado por conta (AD-5); dono da conta (AD-16); dicionário inicial derivado da planilha e validado com a área de negócio; história de paridade com a planilha e de Oracle × planilha.

#### Story 2.14: Mapa de filiais (filial do ERP para filial gerencial)

As a analista,
I want cadastrar quais filiais do ERP compõem cada filial gerencial,
So that o realizado, o orçado e o rateio sejam lidos por filial gerencial.

**Nota:** requisito do cliente de 28/10/2026 (ver `respostas-cliente-2026-10-28.md`). Deve sair **antes** da ingestão (2.3 e 2.6). Os códigos reais entram pela tela, nunca no repositório ou em seed.

**Acceptance Criteria:**

**Given** a migration
**Then** `auditoria.filiais_gerenciais (id, empresa_id, codigo, nome)` e `auditoria.filiais_gerenciais_erp (empresa_id, filial_gerencial_id, filial_erp)`, com `filial_erp` única por empresa
**And** ela entra em `limpar_dados_teste` se a regra de limpeza (1.2) exigir

**Given** o analista ou o Controller
**When** cria, edita, lista ou exclui uma filial gerencial e as filiais do ERP que a compõem
**Then** grava e registra na trilha
**And** gestor e diretoria recebem `403` na escrita

**Given** uma filial do ERP já ligada a outra filial gerencial
**When** se tenta ligá-la de novo
**Then** recusa com `409` e diz a qual filial gerencial ela pertence

**Given** uma filial do ERP sem ligação
**When** o realizado é lido
**Then** ela vale por si (filial solta), sem precisar de cadastro

**Given** uma filial gerencial usada por lançamento, orçado ou fechamento
**When** se tenta excluí-la
**Then** recusa com `409`

**Given** um mês fechado
**When** o mapa é alterado depois
**Then** o fechamento não muda (o snapshot da 1.8 guarda a filial gerencial do momento)

### Story 2.15: Agrupamentos de filiais gerenciais (Balcões, Distribuição)

As a analista,
I want agrupar filiais gerenciais em visões nomeadas (por exemplo, Balcões e Distribuição),
So that o DRE possa mostrar a soma do agrupamento, como a planilha atual.

**Nota:** resposta do cliente (28/10/2026): na planilha, `Grupo` é a soma de todas as filiais e há visões nomeadas que somam algumas filiais gerenciais (Balcões, Distribuição). `AC` pede o acumulado do ano (`AC` com filial, com agrupamento ou com o grupo todo). A composição real entra pela tela, nunca no repositório.

**Acceptance Criteria:**

**Given** a migration
**Then** `auditoria.agrupamentos_filiais (id, empresa_id, codigo, nome)` e `auditoria.agrupamentos_filiais_membros (empresa_id, agrupamento_id, filial_gerencial_id)`, com membros da mesma empresa
**And** uma filial gerencial pode estar em mais de um agrupamento

**Given** o analista ou o Controller
**When** cria, edita ou exclui um agrupamento
**Then** grava e registra na trilha, e os demais perfis recebem `403` na escrita

**Given** um agrupamento
**When** o resolvedor recebe seu código
**Then** devolve as filiais gerenciais (e, por elas, as filiais do ERP)

**Given** a visão "Grupo" (todas as filiais)
**When** o DRE a pede
**Then** é a soma de todas as filiais gerenciais, sem precisar de cadastro

### Story 2.16: Botão "Atualizar dados" para a gestão

As a Controller,
I want buscar de novo os dados do ERP depois de uma correção,
So that o DRE reflita lançamentos corrigidos sem esperar a próxima carga.

**Nota:** resposta do cliente (28/10/2026): a carga é semanal; não existe lançamento com data retroativa, mas pode haver correção de lançamento no primeiro dia útil do mês e estorno de pagamento do mês anterior (que entra no mês atual). Só a gestão da área pede a atualização. Corrigir um mês já fechado exige reabrir (1.7), atualizar e fechar de novo.

**Acceptance Criteria:**

**Given** o Controller (ou admin de plataforma)
**When** aciona "Atualizar dados"
**Then** dispara as importações de realizado e de orçado e mostra o resultado e a hora

**Given** um mês com fechamento ativo
**When** a atualização o atinge
**Then** é recusada pela guarda de período (1.6), com a orientação de reabrir antes

**Given** qualquer outro perfil
**When** tenta atualizar
**Then** `403`

**Given** uma atualização
**When** termina
**Then** a trilha registra quem pediu, o escopo e o resultado

### Story 2.17: Importar a Entrada_Dados do Marcos

As a analista,
I want carregar o arquivo de carga mensal do Marcos (a aba `Entrada_Dados`) sem reformatar,
So that o realizado por conta da rotina 124 entre no sistema com a mesma planilha que o cliente já usa.

**Nota:** estrutura apurada em 29/10/2026 (ver `respostas-cliente-2026-10-28.md`): duas tabelas na mesma aba, cada uma com o seu cabeçalho (`Código` e `Conta` na primeira; `CONTA` e `DESCRIÇÃO` na segunda), subtotais sem código, linhas de conferência sem `Mês` e `Loja`, e `Mês`/`Loja` por fórmula. A 2.2 rejeitava este arquivo. Subtotais não são importados (seria dupla contagem). Detalhes e decisões na spec `spec-2-17-entrada-dados.md`.

**Acceptance Criteria:**

**Given** um arquivo com duas tabelas e dois cabeçalhos diferentes
**When** importa
**Then** as duas são importadas e o cabeçalho repetido nunca vira dado

**Given** as colunas `Código` e `Conta`
**Then** `Código` é a conta e `Conta` é a descrição; com só `Conta` (e `DESCRIÇÃO`), `Conta` é o código, como na 2.2

**Given** linhas sem código (subtotais) ou com código e valor mas sem `Mês` e sem `Loja` (conferência, `TOTAL`, provas)
**When** importa
**Then** são ignoradas e contadas em `linhas_ignoradas`
**And** linha com só um dos dois vazio, ou valor não numérico, rejeita o arquivo com o número da linha

**Given** uma linha `TOTAL` com valor
**When** importa
**Then** a resposta traz `conferencia_total` (esperado, calculado, ok) comparando com a soma das contas da mesma tabela
**And** divergência é só aviso, mostrado na tela

**Given** um XLSX salvo sem o valor calculado das fórmulas (`Mês` e `Loja` vazios em todas as linhas)
**When** importa
**Then** a mensagem diz que as fórmulas não têm valor calculado e orienta salvar de novo no Excel ou preencher `Mês` e `Loja`

**Given** `Ajuste` ou `Ajustes`
**Then** ambos são aceitos e o ajuste continua não importado (só contado)

**Given** os formatos da 2.2 e do sistema
**Then** seguem como antes

### Story 2.18: Comparativo de fechamento (planilha do Marcos × sistema)

As a controladoria,
I want enviar a planilha de análise do Marcos e ver, linha a linha, onde ela e o sistema não batem,
So that eu confie no fechamento antes de virar para Produção e ache a conta ou a linha que diverge.

**Nota:** mesmo desenho do comparativo de fechamento do FB_FAROL. A planilha de análise (abas `MENSAL` e `Real_AA`) fica **gravada em tabelas próprias** (`auditoria.comparativo_externo_*`): nada entra em `lancamentos`, então o DRE não a soma. Duas comparações: por linha do DRE (aba `MENSAL`, uma filial por vez, contra o DRE do sistema) e por conta (aba `Real_AA`, contra a soma bruta dos lançamentos da filial no mês). Decisões e pontos frágeis na spec `spec-2-18-comparativo-fechamento.md`.

**Acceptance Criteria:**

**Given** o XLSX da análise e a capacidade `carregar`
**When** envia (`POST /api/aud/comparativo/carga`, ano opcional, padrão `MENSAL!C2`)
**Then** a aba `Real_AA` do ano vira soma por (mês, loja, conta) e a aba `MENSAL` vira as linhas do DRE por mês, na filial resolvida da planilha
**And** a leitura é em fluxo, só das duas abas necessárias, e o arquivo não é carregado inteiro na memória
**And** a resposta traz contagens (contas, linhas, meses, filiais, ignoradas) e avisos

**Given** o reenvio do mesmo ano
**Then** substitui o que estava gravado daquele ano (contas de todas as lojas e linhas da filial da planilha), na mesma transação, com a trilha só de contagens

**Given** a aba `MENSAL` numa visão sem filial resolvida (grupo, filial sem número)
**Then** a carga avisa; o comparativo por linha precisa de uma filial resolvida, o por conta continua valendo

**Given** `GET /api/aud/comparativo` com ano, mês, filial, nível (`linha` ou `conta`), tolerância (R$, padrão 1,00) e origem (só por conta)
**When** compara
**Then** cada linha ou conta sai como `OK`, `DIVERGE`, `SO_PLANILHA` ou `SO_SISTEMA`, ordenada por maior divergência, com resumo, `fechada`, a carga de origem, paginação e avisos
**And** por linha, o lado do sistema é o DRE (mês fechado do snapshot, aberto ao vivo) casado por código ou pela descrição normalizada; por conta, a soma bruta de `lancamentos`, sem o sinal do Mapa

**Given** `?formato=csv`
**Then** exporta tudo (BOM, `;`, apóstrofo antes de `=`, `+`, `-`, `@`), e `visualizar` e `exportar` vão à trilha com os parâmetros

**Given** o gestor
**Then** `403` registrado na leitura, e a carga exige `carregar`

**Given** a tela "Comparativo" em Administrativo
**Then** tem a carga da planilha, seletores de ano, mês e filial, "Por linha | Por conta", tolerância, cartões de resumo, tabela com filtro por status e busca, exportar CSV e o painel "Como funciona"

### Story 2.19: Salvaguarda de período na carga de lançamentos

As a analista,
I want ver, antes de gravar, de qual competência e filial o arquivo realmente é,
So that um arquivo com a célula de mês ou de filial desatualizada não seja gravado no período errado sem ninguém notar.

**Nota:** caso real de 29/10/2026: a Entrada_Dados trazia `Mês` e `Loja` desatualizados e o importador confiou neles. A regra de leitura não muda; a salvaguarda é mostrar o que foi lido, em duas etapas, e avisar o que parece fora do lugar. Decisões na spec `spec-2-19-salvaguarda-de-periodo.md`.

**Acceptance Criteria:**

**Given** um arquivo de lançamentos
**When** carrega (com ou sem simulação)
**Then** a resposta traz `pares` (competência, filial, quantidade de lançamentos de cada par), o total, os `avisos` e a conferência do TOTAL
**And** a tela mostra isso em destaque, num cartão acima do resto ("Gravado em 2026-05, filial 11: 433 lançamentos"), um item por par

**Given** `POST /api/aud/dre/lancamentos/importar?simular=1`
**When** o arquivo é lido
**Then** devolve o mesmo resumo sem gravar nada (nem carga, nem lançamentos, nem trilha)
**And** competência fechada já é recusada com 409 na simulação

**Given** a tela de carga
**When** escolhe o arquivo
**Then** só "Conferir arquivo" fica disponível; "Confirmar carga" aparece depois da simulação, com o resumo visível, e envia o arquivo de novo sem `simular`
**And** a gravação direta pela API continua possível (compatível com o CSV do sistema)

**Given** competência a mais de 12 meses no passado ou depois do mês corrente
**Or** competência que não é a do nome do arquivo (AAAAMM, MM-AAAA, MM_AAAA ou MMAAAA)
**Or** filial que não está no Mapa de filiais nem em lançamentos anteriores
**Or** competência e filial que já têm lançamentos de outra origem
**Then** a simulação e a gravação trazem o aviso correspondente, sem bloquear

### Story 2.20: Carga de departamentos por arquivo

As a analista,
I want carregar os departamentos por um arquivo CSV ou XLSX,
So that eu cadastre a lista inteira de uma vez, em vez de um por um.

**Nota:** mesma capacidade do cadastro manual (`editar_estrutura`). O arquivo cria e atualiza por código; departamento que não vem no arquivo não é apagado (pode estar ligado a contas e usuários). Decisões na spec `spec-2-20-carga-de-departamentos.md`.

**Acceptance Criteria:**

**Given** um arquivo com `codigo`, `nome` e `grupo` (Comercial, Logística ou Administrativo, sem acento ou maiúsculas também)
**When** carrega (`POST /api/aud/departamentos/carga`)
**Then** cria os novos e atualiza nome e grupo dos existentes por código, e a resposta diz quantos foram criados, atualizados e inalterados

**Given** código vazio ou repetido no arquivo, grupo inválido, nome vazio ou acima do tamanho do cadastro manual
**Then** nada é gravado e a resposta lista até 20 erros de linha mais o total

**Given** departamentos já cadastrados que o arquivo não traz
**Then** continuam como estão

**Given** o arquivo dá certo
**Then** a trilha (`departamento`/`carga`) é gravada na mesma transação com só contagens e códigos

**Given** quem não tem `editar_estrutura`
**Then** 403, e a tela tem o bloco "Carregar departamentos" com "Baixar modelo CSV"

**Given** um arquivo do dicionário (2.10) com departamento inexistente
**Then** a mensagem lista os códigos de departamento distintos que não existem e quantas linhas os usam

### Story 2.21: Carga do Mapa de filiais e dos agrupamentos por arquivo

As a analista,
I want carregar o Mapa de filiais e os agrupamentos por arquivo CSV ou XLSX,
So that a estrutura de filiais da empresa entre de uma vez, em vez de um cadastro por vez.

**Nota:** mesma capacidade do cadastro manual (`editar_estrutura`). As duas cargas só criam e acrescentam; nada é apagado. O Mapa vem antes: os membros de um agrupamento são filiais gerenciais que já existem, e uma filial que vale só por si também precisa de uma linha no mapa (a ligada a ela mesma) para entrar num agrupamento. Decisões na spec `spec-2-21-carga-do-mapa-de-filiais-e-agrupamentos.md`.

**Acceptance Criteria:**

**Given** um arquivo do mapa com `filial_gerencial` e `filial_erp` (e `nome` opcional)
**When** carrega (`POST /api/aud/filiais-gerenciais/carga`)
**Then** cria as filiais gerenciais que faltam, liga as filiais do ERP ainda não ligadas e a resposta diz quantas foram criadas, atualizadas, ligadas, movidas e sem mudança

**Given** uma filial do ERP que já pertence a outra filial gerencial
**Then** ela é movida para a do arquivo e a resposta lista a movimentação (filial, de, para)

**Given** um arquivo de agrupamentos com `agrupamento` e `filial` (e `nome` opcional)
**When** carrega (`POST /api/aud/agrupamentos-filiais/carga`)
**Then** cria os agrupamentos que faltam e acrescenta os membros, sem tirar nenhum

**Given** uma filial de agrupamento que não existe como filial gerencial
**Then** nada é gravado e a mensagem lista as filiais que faltam e pede o Mapa de filiais antes

**Given** linha vazia ou inválida, filial do ERP em duas filiais gerenciais no mesmo arquivo, código igual ao de outra coisa ou a palavra GRUPO
**Then** nada é gravado e a resposta lista até 20 erros de linha mais o total

**Given** códigos de filial com zeros à esquerda ("01")
**Then** valem como "1", como na ingestão

**Given** a carga dá certo
**Then** a trilha (`filial_gerencial` ou `agrupamento_filiais`, ação `carga`) é gravada na mesma transação, e as telas têm o bloco de carga com "Baixar modelo CSV"

## Epic 3: DRE por departamento, com orçado e ano anterior
Qualquer usuário autorizado lê o DRE do grupo, por filial e por departamento, mensal e acumulado, comparando Realizado, Orçado e Ano anterior.
**FRs covered:** FR-5, FR-6, FR-7, FR-8
**Notas:** Mapa e estrutura iniciais derivados da planilha; paridade completa com o DRE da planilha (os subtotais da planilha são somas simples de linhas já com sinal: Venda Líquida = Bruta + Devoluções; Margem Bruta = Venda Líquida + CMV + Devolução de CMV + Recomposição + Dedução; Margem Composta = Margem Bruta + Desconto de custo + ST; Margem de Contribuição = Margem Composta + Impostos; Total de Despesas = Comercial + Logística + Administrativo; Margem Líquida = Margem de Contribuição + Total de Despesas; Resultado antes da tributação = Margem Líquida + Receitas/Despesas; Resultado após = antes + IRPJ + CSLL; ver `respostas-cliente-2026-10-28.md`); visões da planilha ainda não cobertas (3.11 a 3.15); o perfil `gestor` não vê dado de departamento até o épico 4; o fim deste épico é a primeira demonstração ao cliente.

#### Story 3.9: Detalhe até o lançamento em mês fechado

As a Gerente,
I want descer do grupo à conta e da conta ao lançamento, também em mês fechado,
So that eu veja data, fornecedor, nota, valor e filial de cada lançamento.

**Nota:** resposta do cliente (28/10/2026): o DRE mostra por grupo; clicar no grupo abre as contas; clicar na conta abre os lançamentos (data, fornecedor, nota, valor, filial). O histórico com nome de pessoa pode aparecer na tela e no e-mail. A 1.8 deixou o detalhe de mês fechado só até a conta; como a guarda de período (1.6) impede escrita em mês fechado, ler os lançamentos ao vivo é consistente com o snapshot.

**Acceptance Criteria:**

**Given** a migration
**Then** `lancamentos` ganha data de pagamento e fornecedor (e documento, se ainda não houver), preenchidos pelo Ingestor a partir da fonte

**Given** um mês fechado
**When** o gerente abre a conta
**Then** lista os lançamentos da conta, da filial e da competência (data, fornecedor, nota, valor, filial)
**And** a soma bate com o valor congelado da conta; se não bater, a tela avisa

**Given** um mês reaberto e atualizado
**When** o gerente abre a conta
**Then** vê os lançamentos novos

**Given** um gerente
**When** abre contas fora do seu departamento
**Then** `403` (escopo da 4.7)

### Story 3.10: Visão por trimestre (FR-7)

As a Gerente,
I want ver o DRE por trimestre do ano (T1 a T4 e o total do ano),
So that eu compare os trimestres sem somar meses na mão.

**Nota:** pedido do Marcos (28/10/2026): a planilha enviada não tem a visão dos trimestres. Um trimestre é a soma de três meses (T1 de janeiro a março etc.); vale para realizado, orçado e ano anterior, e para o percentual sobre o Faturamento Líquido. O período por intervalo já existe no `dre.Leitor` (cada mês fechado vem do snapshot e cada mês aberto, ao vivo), então esta história é consulta e tela.

**Acceptance Criteria:**

**Given** um ano e uma filial (ou agrupamento, ou o grupo todo)
**When** o usuário escolhe "Trimestres"
**Then** vê colunas T1, T2, T3, T4 e Ano, cada uma com realizado, orçado, ano anterior e percentual sobre o Faturamento Líquido

**Given** um trimestre com algum mês aberto
**When** a tela o mostra
**Then** marca o trimestre como "parcial" e diz quais meses estão fechados

**Given** um trimestre com os três meses fechados
**When** o Mapa ou a estrutura mudam depois
**Then** o valor do trimestre não muda (vem do snapshot, mês a mês)

**Given** um gerente
**When** abre a visão
**Then** vê só os grupos do seu departamento (escopo da 4.7)

**Given** o ano em curso
**When** um trimestre ainda não começou
**Then** a coluna aparece vazia, sem erro

### Story 3.11: Comparativo de vários anos (acumulado multi-ano)

As a Diretor,
I want ver o DRE acumulado até o mês escolhido, ano a ano (2020 em diante, quando houver dado),
So that eu compare a evolução de vários anos sem abrir uma aba por ano.

**Nota:** a planilha de análise do cliente tem uma aba de acumulado com um ano por coluna. Acumulado (AC) = do início do ano até o mês escolhido. Ano sem dado aparece vazio, não zero.

**Acceptance Criteria:**

**Given** um mês, uma filial (ou agrupamento, ou o grupo todo) e um intervalo de anos
**When** o usuário escolhe "Vários anos"
**Then** vê uma coluna por ano com o acumulado do ano até o mês, com valor e percentual (base da 3.15)

**Given** um ano sem dado
**Then** a coluna aparece vazia, sem erro

**Given** meses fechados e abertos no acumulado
**Then** o que está fechado vem do snapshot e o que está aberto, ao vivo (como na 3.10), e a coluna marca o que é parcial

**Given** um gerente
**Then** vê só os grupos do seu departamento (escopo da 4.7)

### Story 3.12: Filiais lado a lado

As a Controller,
I want comparar as filiais gerenciais lado a lado no mesmo mês ou acumulado,
So that eu veja qual filial destoa sem trocar o filtro a cada vez.

**Nota:** a planilha de análise tem uma aba com uma coluna por filial. Usa a filial gerencial (2.14) e os agrupamentos (2.15) como colunas, além do grupo todo.

**Acceptance Criteria:**

**Given** um mês (ou o acumulado até ele) e a lista de filiais gerenciais e agrupamentos
**When** o usuário escolhe "Filiais lado a lado"
**Then** vê uma coluna por filial gerencial, uma por agrupamento escolhido e o total do grupo, com valor e percentual

**Given** uma filial sem lançamento no período
**Then** a coluna aparece vazia, não zero

**Given** o escopo do usuário (4.7)
**Then** o gerente vê só as filiais e os grupos que lhe cabem

### Story 3.13: DRE por centro de custo (natureza do gasto)

As a Controller,
I want abrir o DRE por centro de custo, a natureza do gasto,
So that eu veja, por exemplo, quanto de despesa vai para transporte, manutenção ou vendas.

**Nota:** o dicionário do cliente liga cada conta a duas classificações: o grupo (a linha do DRE) e o centro de custo (a natureza do gasto). Comercial soma compras e vendas; Logística soma transporte e entrega, centro de distribuição e manutenção; Administrativo soma os demais grupos. O dicionário de contas (2.10) ganha a coluna de centro de custo.

**Acceptance Criteria:**

**Given** o dicionário por arquivo (2.10)
**When** o arquivo traz a coluna de centro de custo
**Then** ela é gravada por conta e validada contra os centros cadastrados; sem a coluna, o comportamento atual não muda

**Given** um mês e uma filial
**When** o usuário escolhe "Centro de custo"
**Then** vê o realizado por centro de custo, com o total por departamento (Comercial, Logística, Administrativo) e o percentual

**Given** uma conta sem centro de custo
**Then** aparece em "sem centro de custo", e a soma do DRE por centro bate com a do DRE por grupo

### Story 3.14: Visão condensada "Fechamento da diretoria"

As a Diretor,
I want uma visão condensada do DRE (poucas linhas e os subtotais),
So that eu leia o fechamento do mês em uma tela.

**Nota:** a planilha do cliente tem uma aba "Fechamento diretoria" com o DRE condensado. A lista de linhas condensadas vem da estrutura do DRE (3.1), marcada como "aparece na diretoria", e não de código.

**Acceptance Criteria:**

**Given** a estrutura do DRE
**When** a controladoria marca quais linhas aparecem na visão da diretoria
**Then** a escolha é gravada e registrada na trilha

**Given** um mês fechado
**When** a diretoria abre "Fechamento"
**Then** vê só as linhas marcadas e os subtotais, com realizado, orçado, ano anterior e percentual, lidos do snapshot

**Given** o perfil gestor
**Then** recebe `403` (capacidade de ler irrestrita)

### Story 3.15: Percentual com a base certa (Venda Bruta e Venda Líquida)

As a Gerente,
I want que o percentual de cada linha use a base da planilha do cliente,
So that o DRE do sistema mostre os mesmos percentuais do Excel.

**Nota:** na planilha, a Venda Bruta e as Devoluções são percentuais **sobre a Venda Bruta**; todas as demais linhas são percentuais sobre a Venda Líquida. A 3.4 usa o Faturamento Líquido como base de todas as linhas; esta história ajusta as duas linhas de venda.

**Acceptance Criteria:**

**Given** o DRE de uma filial
**When** é mostrado
**Then** a Venda Bruta e as Devoluções têm percentual sobre a Venda Bruta, e as demais linhas sobre a Venda Líquida

**Given** Venda Bruta zero
**Then** o percentual dessas duas linhas aparece vazio, sem erro

**Given** a visão por departamento, por trimestre ou por vários anos
**Then** usam a mesma regra de base

## Epic 4: Acesso seguro por perfil e departamento
Cada pessoa vê e faz só o que cabe ao seu papel, o Gerente enxerga apenas o seu departamento, e o cadastro público deixa de existir.
**FRs covered:** FR-18, FR-19, FR-22
**Notas:** a história do FR-19 sai antes de todas as outras (o site já é público); `perfis.go` e middleware por capacidade (AD-11); escopo que nega por padrão (AD-10); migração dos ids de área (AD-12).

### Epic 5: Gerentes justificam ajustes e desvios
Os ajustes vêm do ERP, os gerentes justificam (com log), e cada gerente recebe por e-mail o DRE do seu departamento, toda sexta (acumulado) e no fechamento.
**FRs covered:** FR-4, FR-12, FR-13, FR-14
**Notas:** outbox de e-mail (AD-13); limite de desvio e Lista de aviso começam com padrão provisório até o cliente responder; em Teste o e-mail vai só ao administrador.

#### Story 5.10: Disparo semanal do acumulado do mês (FR-14)

As a Gerente,
I want receber toda sexta-feira o DRE do meu departamento com o acumulado do mês,
So that eu acompanhe o mês antes do fechamento.

**Nota:** requisito do cliente (28/10/2026): toda sexta-feira, acumulado do mês até aquele dia (semana 1 de 01 a 07, semana 2 de 01 a 14 etc.), em nível de lançamento. Depende de a fonte atualizar com essa frequência (hoje as views atualizam no início do mês). O disparo do **fechamento** continua manual (5.8).

**Acceptance Criteria:**

**Given** o dia e a hora em `config_empresa` (padrão sexta-feira, `America/Sao_Paulo`)
**When** chega o horário
**Then** enfileira para cada gerente o DRE do seu departamento com o acumulado do mês, marcado "parcial, mês não fechado"

**Given** `pg_try_advisory_lock` em conexão dedicada
**When** outra execução está em andamento
**Then** a segunda não roda e registra

**Given** a chave `semanal:<AAAA-MM-DD>:user:<uid>`
**When** o disparo repete na mesma sexta
**Then** não duplica

**Given** o ambiente em Teste
**When** chega o horário
**Then** o e-mail vai só ao administrador

**Given** o analista ou o Controller
**When** altera o dia, a hora ou desliga o disparo
**Then** grava na trilha, e gestor e diretoria recebem `403`

## Epic 6: Dashboards de desvios e tendência
A diretoria vê os maiores desvios por departamento, gerente e filial, a tendência da despesa sobre o Faturamento Líquido, a ponte do resultado e a projeção do ano.
**FRs covered:** FR-16, FR-17

### Epic 7: Rateio entre filiais
O DRE por filial mostra o custo real, com as despesas dos departamentos centralizados rateadas entre as unidades.
**FRs covered:** FR-15
**Notas:** depende de respostas do cliente (quais departamentos e se o critério é fixo ou mensal); só começa depois delas.

### Epic 8: Go-live com dado real (trilha paralela)
A equipe do cliente passa a usar o sistema com dado real sabendo que há backup restaurável, que o Oracle cobre as contas do DRE e que não restou dívida de segurança.
**FRs covered:** nenhum; cobre NFR6, NFR7, AR-15 e AR-16
**Notas:** corre em paralelo aos demais; o pedido de autorização do cliente sobre o destino do backup é aberto desde já; a virada para Produção avisa quando o Oracle ainda não cobre contas do DRE.

### Ordem e dependências
1 → 2 → 3 → 4 → 5 → 6. O épico 7 depende dos épicos 1, 2 e 3 e das respostas do cliente. O épico 8 corre em paralelo e deve terminar antes do go-live.

**Consolidação considerada e recusada.** Vários épicos mexem no pacote `dre` e nos mesmos handlers, mas cada um separa uma fronteira de risco ou um ciclo de retorno: o épico 1 troca a base de código em produção, o 2 depende da fonte do cliente, o 4 muda segurança e menu, o 5 e o 7 dependem de respostas do cliente, e o 8 depende de decisões externas.

**Épicos com carga técnica.** O épico 1 abre com fundação (testes e migração dos handlers) porque corrige falhas que já existem na Fase 1; o épico 8 é uma trilha de prontidão, e não um épico de valor para o usuário.

**Histórias sem FR próprio.** Sustentam requisitos sem implementá-los: 1.1 e 1.5 (FR-9 a FR-11), 1.4 (FR-3), 1.9 (NFR7), 2.4 (FR-1), 2.8 (FR-1 e FR-3), 2.11 (FR-20), 2.12 (FR-1), 3.3 (FR-5 e FR-6), 3.7 (FR-7), 4.3 (FR-18), 4.4 (AR-9), 5.6 (FR-4 e FR-14) e as do épico 8 (NFR6, NFR7, AR-15, AR-16). A 3.8 é um marco, sem requisito.

## Epic 1: Fechar o mês com segurança

O Controller fecha o mês do grupo e o reabre com motivo, sabendo que ninguém altera números fechados; a equipe valida o sistema num ambiente de Teste que pode ser apagado e que vira Produção de forma irreversível.

**Nota de perfis:** até o épico 4, o analista e o Controller usam o mesmo perfil `controladoria`; o perfil `analista` e a matriz de capacidades chegam no épico 4. Toda história futura que criar tabela de domínio a inclui em `auditoria.limpar_dados_teste` e usa `auditoria.imutavel_salvo_em_teste` nos seus gatilhos (AD-18).

### Story 1.1: Testes que registram o fechamento e a importação de hoje

As a dono do produto,
I want testes automáticos que registram como o fechamento e a importação CSV funcionam hoje,
So that eu possa mover esse código sem quebrar o que o Controller já usa.

**Acceptance Criteria:**

**Given** um banco PostgreSQL de teste com as migrations 001 a 003
**When** `go test ./...` roda em `backend/`
**Then** os testes usam um banco cujo nome termina em `_test`
**And** abortam com mensagem clara se o nome não terminar assim
**And** o banco local `fb_auditoria_test` é criado se não existir e recriado limpo a cada execução

**Given** lançamentos mapeados para as linhas do DRE
**When** o teste fecha uma competência
**Then** confirma a versão 1, as linhas congeladas com os valores esperados e o registro na trilha

**Given** uma conta com lançamento sem mapeamento
**When** o teste tenta fechar
**Then** recebe `409` e nada é gravado

**Given** um CSV com `;` e outro com `,`
**When** o teste importa
**Then** as linhas entram com `carga_id` e a trilha registra a importação

**Given** que hoje a importação ignora o fechamento
**When** o teste importa para uma competência fechada
**Then** o comportamento atual (aceita) fica registrado e marcado "muda na 1.6"
**And** `go build ./...` e `go vet ./...` seguem passando

### Story 1.2: Marcador Teste/Produção e apagar os dados de teste (FR-21, AD-18)

As a administrador de plataforma,
I want marcar a empresa como Teste ou Produção e apagar os dados de teste com a minha senha,
So that eu valide com planilhas e depois comece do zero sem risco de apagar dado real.

**Acceptance Criteria:**

**Given** as empresas existentes
**When** a migration roda
**Then** `companies.ambiente` existe com padrão `teste` e todas ficam em `teste`

**Given** uma empresa em `producao`
**When** alguém tenta voltar para `teste` (por SQL ou API)
**Then** o banco recusa (gatilho de mão única)

**Given** o admin de plataforma logado
**When** pede para apagar informando a senha correta
**Then** `auditoria.limpar_dados_teste` apaga lançamentos, orçado, fechamentos e cargas, mantém estrutura do DRE, Mapa, usuários e `audit_log`, e grava na trilha

**Given** senha errada ou ausente
**When** pede para apagar
**Then** `403` e nada é apagado

**Given** empresa em `producao`
**When** chama apagar (inclusive direto na API)
**Then** a função recusa e nada é apagado

**Given** um perfil que não é admin de plataforma
**When** chama apagar
**Then** `403`
**And** existe `auditoria.imutavel_salvo_em_teste(empresa_id)`, verdadeira só em `teste` com a sessão aberta pela função de limpeza

### Story 1.3: Virar para Produção e faixa de ambiente (FR-21)

As a administrador de plataforma,
I want virar a empresa para Produção e ver em toda tela em que ambiente estou,
So that ninguém confunda teste com produção.

**Acceptance Criteria:**

**Given** empresa em `teste`
**When** o admin pede para virar informando a senha e digitando "VIRAR PARA PRODUCAO"
**Then** `ambiente` passa a `producao` e a trilha registra quem e quando
**And** a resposta lista as contas com lançamento de origem arquivo (aviso, não bloqueia)

**Given** ambiente `teste`
**When** qualquer tela carrega
**Then** a faixa "AMBIENTE DE TESTE" aparece fixa (via `/api/aud/me`)

**Given** `producao`
**When** o admin abre a tela de administração
**Then** não há faixa, a tela mostra "Produção" e não há botão de apagar

**Given** senha errada
**When** pede para virar
**Then** `403` e nada muda

### Story 1.4: Importação CSV passa pelo Ingestor (AD-1, AD-2, AD-3)

As a analista,
I want que reenviar um CSV substitua os dados em vez de duplicá-los,
So that a carga de validação seja repetível.

**Acceptance Criteria:**

**Given** o pacote `backend/dre` com a porta `Fonte` e `dre/fontes/csv`
**When** o handler recebe um CSV
**Then** só chama `dre.Ingestor`, sem `INSERT` em lançamentos

**Given** um escopo `(empresa, origem, competência, filial)` já importado
**When** o CSV é reenviado
**Then** o escopo é substituído numa transação e não duplica

**Given** lançamentos de outra origem na mesma competência e filial
**When** reimporta
**Then** eles permanecem

**Given** linha inválida ou `filial` vazia
**When** importa
**Then** rejeita listando as linhas e nada é gravado

**Given** arquivo sem linhas
**When** importa
**Then** não apaga o escopo e informa
**And** um teste falha o build se outro pacote tiver `INSERT` ou `DELETE` em `lancamentos`

### Story 1.5: Fechamento passa pelo serviço `dre` (AD-1)

As a Controller,
I want que o fechamento continue funcionando como hoje, agora num único lugar,
So that as garantias seguintes tenham onde morar.

**Acceptance Criteria:**

**Given** o handler de fechamento
**When** recebe o POST
**Then** chama `dre.Fechamento.Fechar` sem SQL próprio, e só a controladoria fecha

**Given** os testes da 1.1
**When** rodam
**Then** seguem passando (versão, linhas, `409` sem mapeamento, trilha)

**Given** Fechar
**When** calcula o snapshot
**Then** o cálculo acontece dentro da transação, via `dre.Realizado` (hoje é fora dela)
**And** `dre.Realizado` é o único caminho que calcula o realizado exibido
**And** a listagem de fechamentos devolve o mesmo JSON de hoje

### Story 1.6: Guarda de período e bloqueio de escrita em mês fechado (FR-11, AD-8)

As a Controller,
I want que nenhuma carga entre em mês fechado,
So that o detalhe nunca mostre dado posterior ao fechamento.

**Acceptance Criteria:**

**Given** competência com fechamento (nesta história "ativo" significa que existe fechamento; a reabertura chega na 1.7)
**When** o analista importa para ela
**Then** `409` listando os escopos recusados, com a mensagem de que é preciso reabrir, e nada é gravado

**Given** competência aberta
**When** importa
**Then** funciona

**Given** `dre.Periodo.Guardar(tx, empresa, competência)` com `pg_advisory_xact_lock(dre.ChaveLock)` e isolamento `READ COMMITTED`
**When** fechar e importar a mesma competência rodam juntas
**Then** são serializadas: a carga entra antes do snapshot ou é recusada depois, nunca fica de fora do snapshot e dentro dos lançamentos
**And** um teste garante que toda escrita em `lancamentos` passa por `Guardar`
**And** o teste da 1.1 que registrava "aceita em mês fechado" é invertido

### Story 1.7: Um fechamento ativo por mês e reabertura com motivo (FR-9, FR-10, AD-9)

As a Controller,
I want fechar o mês do grupo uma única vez e reabri-lo com motivo,
So that eu corrija sem perder a história do fechamento anterior.

**Acceptance Criteria:**

**Given** a migration
**Then** `dre_fechamentos` ganha `reaberto_em`, `reaberto_por`, `motivo_reabertura` e um índice único parcial em `(empresa, competência) WHERE reaberto_em IS NULL`
**And** se houver duplicatas, a migration aborta com mensagem, sem alterar dado

**Given** competência fechada e ativa
**When** o Controller tenta fechar de novo
**Then** `409`

**Given** motivo informado
**When** o Controller reabre
**Then** o ativo recebe `reaberto_*`, a competência volta a aceitar escrita, a versão anterior fica legível e a trilha registra

**Given** reabrir sem motivo
**When** o Controller envia
**Then** recusa

**Given** competência reaberta
**When** o Controller fecha de novo
**Then** a versão é `MAX+1` por `(empresa, competência)`, mantendo as anteriores

**Given** um usuário sem o perfil `controladoria` (por exemplo, diretoria)
**When** tenta fechar ou reabrir
**Then** `403`
**And** `reaberto_*` mudam uma só vez, por gatilho que respeita `imutavel_salvo_em_teste`
**And** a tela de fechamento deixa de oferecer escolha de filial; fechamentos por filial já gravados continuam legíveis

### Story 1.8: Mês fechado lido do que foi congelado (AD-17)

As a Controller,
I want que o mês fechado seja lido sempre do snapshot,
So that mudar o Mapa depois não altere um mês fechado.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `dre_fechamento_contas` (conta, linha, realizado, ano anterior) e `dre_fechamento_linhas` ganha orçado e ano anterior
**And** ambas são imutáveis por gatilho que respeita `imutavel_salvo_em_teste` e entram em `limpar_dados_teste`

**Given** Fechar
**When** grava o snapshot
**Then** grava linhas e contas; as colunas de orçado por conta, departamento e ajustes são acrescentadas pelas histórias 2.7, 3.5 e 5.4

**Given** competência fechada
**When** o DRE (mensal, acumulado, detalhe por conta) é consultado
**Then** lê só do snapshot, e mudar Mapa ou estrutura depois não altera o resultado

**Given** competência aberta
**When** o DRE é consultado
**Then** lê ao vivo como hoje
**And** o aviso da tela "o detalhe mostra lançamentos posteriores ao fechamento" deixa de existir

### Story 1.9: Nenhum endereço do banco do cliente no repositório (NFR7)

As a dono do produto,
I want que o repositório não carregue o endereço do banco do cliente,
So that uma cópia do código não revele a infraestrutura dele.

**Acceptance Criteria:**

**Given** `docker-compose.yml`, `backend/.env.example` e `backend/cmd/jc-probe`
**When** revisados
**Then** nenhum traz IP, service name ou schema do cliente como valor padrão
**And** `JC_ORACLE_HOST` e `JC_ORACLE_SERVICE` ficam obrigatórios

**Given** o `jc-probe` sem variáveis
**When** roda
**Then** falha pedindo-as, sem endereço embutido

**Given** o deploy
**When** a história é entregue
**Then** ela lista as variáveis a cadastrar no Coolify antes do push
**And** `go mod tidy` marca o `go-ora` como dependência direta
**And** o endereço continua no histórico do git; a mitigação decidida é o repositório privado, sem reescrever histórico

## Epic 2: Os dados do cliente entram sozinhos

O analista importa o realizado consolidado e o orçado do banco do cliente, carrega por arquivo o que ainda não vem de lá, e confere que o resultado bate com a planilha atual.

**Nota:** a história 2.13 é opcional; nenhuma outra depende dela. Na 2.2, a coluna `Ajustes` da planilha não é importada: ajuste nasce no épico 5.

### Story 2.1: Uma conta, uma fonte (AD-16)

As a Controller,
I want que cada conta tenha uma única fonte dona,
So that a mesma conta vinda do Oracle e do arquivo nunca seja somada em dobro.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `auditoria.conta_fonte (empresa_id, conta_padrao, origem)`
**And** ela é mantida por `limpar_dados_teste` (é configuração)

**Given** um CSV com conta sem dona
**When** importa
**Then** o Ingestor registra `csv.contas` como dona e grava

**Given** conta cuja dona é outra origem
**When** o CSV a traz
**Then** a linha é rejeitada citando a dona
**And** nada é gravado se houver erro bloqueante

**Given** a posse de uma conta
**When** é registrada
**Then** ela vai à trilha

### Story 2.2: CSV e XLSX no formato da planilha atual (FR-3)

As a analista,
I want carregar o bloco de contas da planilha atual (CSV ou XLSX) sem reformatar,
So that eu valide o ambiente com os mesmos dados que o Excel usa.

**Acceptance Criteria:**

**Given** um arquivo CSV ou XLSX (decisão do dono do produto, 28/10/2026) com `CONTA, DESCRIÇÃO, VL. Realizado, Ajustes, Saldo, Grupo, Mês, Loja`
**When** importa
**Then** `Mês` (AAAAMM) vira competência, `Loja` vira filial e `VL. Realizado` vira o valor, com origem `csv.contas`

**Given** linhas com `Ajustes` preenchido
**When** importa
**Then** o ajuste não é importado e a resposta avisa quantas linhas o tinham
**And** `Saldo` é ignorado

**Given** `Mês` fora de AAAAMM ou `Loja` vazia
**When** importa
**Then** a linha é rejeitada com o seu número

**Given** o formato antigo do importador
**When** importa
**Then** continua aceito
**And** competência fechada é recusada (1.6)

### Story 2.3: Ler o realizado consolidado do Oracle (FR-1, AD-2, AD-4)

As a analista,
I want trazer o realizado consolidado do banco do cliente para dentro do sistema,
So that faturamento, devoluções, CMV de devolução, ST e desconto de custo venham do ERP sem digitação.

**Acceptance Criteria:**

**Given** a migration
**Then** `lancamentos` ganha a coluna `uf`

**Given** `dre/fontes/oracle` (go-ora v2, só leitura) e as variáveis `JC_ORACLE_*` e `JC_EMPRESA_ID`
**When** a importação é disparada
**Then** lê `VW_DRE_CONSOLIDADO` por competência (`WHERE MES_ANO = :1`), com ping de 30 s e timeout de consulta, nunca sem filtro

**Given** cada coluna de valor da view
**When** importa
**Then** vira lançamento com conta `ERP.<COLUNA>`, origem `oracle.consolidado`, a UF na coluna própria e o valor como a fonte entrega
**And** valor nulo não é gravado

**Given** `MES_ANO` em `MM/AAAA`
**When** importa
**Then** vira competência
**And** formato inválido rejeita a linha

**Given** reimportação do mesmo escopo
**When** importa
**Then** substitui sem duplicar nem apagar outra origem
**And** leitura vazia nunca apaga

**Given** competência fechada
**When** importa
**Then** é pulada e informada, sem falhar as demais

**Given** que o IP das máquinas de desenvolvimento não está liberado no Oracle do cliente
**When** os testes rodam
**Then** a leitura, a conversão de `MES_ANO` e a transformação das colunas em contas `ERP.*` são testadas com uma `Fonte` falsa, sem Oracle
**And** a leitura real é verificada no servidor de produção, com um comando de sonda, e não nos testes automáticos

### Story 2.4: Registrar cada importação e a posse das contas (FR-1, AD-15, AD-16)

As a analista,
I want que cada importação fique registrada e que as contas do Oracle tenham dona,
So that eu saiba o que foi importado e nada seja somado em dobro.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `auditoria.execucoes`
**And** ela entra em `limpar_dados_teste`

**Given** uma importação do Oracle (2.3)
**When** termina
**Then** a execução grava início, fim, status e resumo (aplicadas, puladas por mês fechado, rejeitadas), e a trilha registra quem a disparou

**Given** as contas `ERP.*`
**When** são importadas pela primeira vez
**Then** ganham dona `oracle.consolidado` (2.1)
**And** uma conta `ERP.*` cuja dona é outra origem é rejeitada

**Given** falha de conexão
**When** a importação roda
**Then** devolve mensagem clara (rede, serviço ou credencial), registra a execução com status de falha e mantém os dados anteriores

### Story 2.5: Tela "Importar do banco do cliente"

As a analista,
I want um botão "Importar agora" e ver o resultado,
So that eu acompanhe cada importação.

**Acceptance Criteria:**

**Given** Configurações, Carga
**When** clico em "Importar do banco do cliente" e escolho o período
**Then** a importação roda e a tela mostra aplicadas, puladas por mês fechado e rejeitadas

**Given** execuções anteriores
**When** abro a tela
**Then** ela lista as últimas (início, fim, status, resumo)

**Given** uma falha
**When** a importação termina
**Then** a tela mostra a causa e os dados anteriores continuam

**Given** um perfil sem permissão (diretoria, gestor)
**When** abre a tela ou chama a API
**Then** não vê o botão e a API devolve `403`

### Story 2.6: Orçado do Oracle por conta (FR-2, AD-5)

As a analista,
I want importar o orçado do banco do cliente por conta,
So that o DRE compare com o planejado.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `auditoria.orcado_conta (empresa_id, competencia, filial, conta, valor, origem, carga_id)`, escrita só pelo Ingestor
**And** o teste de arquitetura passa a cobri-la e ela entra em `limpar_dados_teste`

**Given** uma linha de `VW_DRE_ORCAMENTO` (ano, filial, conta, 12 meses)
**When** importa
**Then** gera até 12 registros, origem `oracle.orcamento`

**Given** um mês nulo
**When** importa
**Then** não grava ("sem orçado", não zero)

**Given** reimportação do ano
**When** importa
**Then** substitui por escopo
**And** competências fechadas são puladas e informadas

**Given** o `CODCONTA` do orçamento
**When** é guardado
**Then** fica como texto

### Story 2.7: O DRE lê o orçado por conta (AD-5)

As a Controller,
I want que o DRE use o orçado vindo do banco do cliente,
So that exista uma só fonte de orçado.

**Acceptance Criteria:**

**Given** orçado em `orcado_conta`
**When** o DRE é consultado
**Then** o orçado é resolvido pelo Mapa (mesma função do realizado) e somado por linha

**Given** conta sem Mapa
**When** o DRE é consultado
**Then** aparece como "sem mapeamento"

**Given** a rota que grava orçado por linha
**When** chamada
**Then** foi removida, e `dre_orcado` fica só para leitura (legado)

**Given** conta ou filial sem orçado
**When** o DRE é consultado
**Then** o DRE não inventa zero

**Given** o snapshot do fechamento
**When** Fechar roda
**Then** `dre_fechamento_contas` ganha a coluna `orcado` e grava o orçado por conta
**And** os testes da 1.1 seguem passando

### Story 2.8: Trocar a dona da conta (AD-16)

As a Controller,
I want passar uma conta de arquivo para o Oracle quando ele passar a cobri-la,
So that a virada não duplique nem perca dados.

**Acceptance Criteria:**

**Given** conta com dona `csv.contas`
**When** o Controller troca a dona
**Then** a troca vai à trilha e vale para as importações seguintes

**Given** lançamentos da origem antiga em competência aberta
**When** tenta trocar
**Then** recusa listando as competências (em Teste dá para apagar antes; em Produção primeiro se fecha o mês)

**Given** competências fechadas
**When** a dona é trocada
**Then** elas mantêm os lançamentos da origem antiga

**Given** um perfil sem `controladoria`
**When** tenta trocar
**Then** `403`

### Story 2.9: Cadastro de departamentos

As a analista,
I want cadastrar os departamentos e o grupo de cada um,
So that contas e gerentes tenham a quem se ligar.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `departamentos (empresa_id, codigo, nome, grupo)` com `grupo` em Comercial, Logística ou Administrativo, único por `(empresa, codigo)`
**And** escrita só por `dre.Dicionario` e mantida por `limpar_dados_teste`

**Given** a tela de Configurações
**When** o analista cria, edita ou lista
**Then** funciona e grava na trilha

**Given** departamento ligado a contas
**When** tenta excluir
**Then** `409`

### Story 2.10: Dicionário de contas por arquivo (FR-20, AD-2)

> **Regra confirmada pelo cliente (28/10/2026):** o departamento de uma conta é o do **grupo da conta** (por exemplo, o grupo 402 é Centro de Distribuição). O dicionário guarda o grupo; o vínculo conta-departamento (2.9) deriva dele e pode ser corrigido conta a conta.

As a analista,
I want carregar o dicionário (conta, descrição, grupo, departamento) por arquivo,
So that o sistema saiba de qual departamento é cada conta.

**Acceptance Criteria:**

**Given** a migration
**Then** existem `conta_dicionario (empresa_id, conta, descricao, grupo)` e `conta_departamento (empresa_id, conta_padrao, departamento_id)`, escritas só por `dre.Dicionario`

**Given** linhas com conta repetida ou departamento inexistente
**When** carrega
**Then** são listadas
**And** nada é gravado se houver erro bloqueante

**Given** reenvio do arquivo
**When** carrega
**Then** atualiza descrição, grupo e departamento sem apagar a associação à linha do DRE no Mapa

**Given** conta com prefixo `%`
**When** resolvida
**Then** vale a mesma resolução do Mapa (a mais específica vence)
**And** a carga vai à trilha

### Story 2.11: Dicionário inicial validado com o cliente (tarefa de dados)

As a dono do produto,
I want gerar o arquivo do dicionário a partir da planilha do cliente e validá-lo com a área de negócio,
So that os gerentes tenham departamento desde o primeiro dia.

**Acceptance Criteria:**

**Given** a aba de gerentes da planilha (contas por departamento)
**When** o arquivo é gerado
**Then** traz conta, descrição, grupo e departamento
**And** as contas sem departamento vêm listadas à parte

**Given** o arquivo gerado
**When** o repositório é verificado
**Then** o arquivo não está nele

**Given** a revisão com o responsável do cliente
**When** termina
**Then** quem validou e quando fica registrado

**Given** o arquivo validado
**When** é carregado em Teste pela 2.10
**Then** a contagem de contas por departamento é conferida

### Story 2.12: Relatório de conferência entre origens

As a dono do produto,
I want comparar o que cada origem de lançamentos trouxe para a mesma competência (Oracle × planilha, planilha × planilha),
So that eu confie na conexão e na planilha antes de virar para Produção.

**Nota:** reescrita em 29/10/2026. O relatório lê `lancamentos` de qualquer origem registrada e compara o valor **bruto** contra o valor bruto (o ERP e a planilha não têm sinal diferente; o sinal do Mapa só vale no DRE). Não depende de o adaptador Oracle existir: hoje serve para `csv` × `csv.contas` ou planilha × planilha, e quando o adaptador gravar a sua origem, o mesmo relatório a compara. A correspondência de conta entre origens com planos diferentes (arquivo com fator) ficou para quando a view do Oracle existir (ver `deferred-work.md`). Enquanto duas origens estiverem carregadas ao mesmo tempo para a mesma competência, o DRE as soma (a 2.1, uma conta uma fonte, ainda não existe); o relatório avisa.

**Acceptance Criteria:**

**Given** a competência (AAAA-MM), duas origens existentes na empresa, uma filial opcional e a tolerância (padrão 0,01)
**When** o usuário pede a conferência (`GET /api/aud/conferencia`, área empresa)
**Then** recebe os totais por origem (soma, contas e linhas), o número de pares comparados, a lista só das diferenças acima da tolerância (filial, conta, descrição, valor A, valor B, diferença), as contas que existem só numa origem e o resumo por linha do DRE (pelo Mapa, mesma resolução do Mapa)
**And** a filial é normalizada como na ingestão (zeros à esquerda: `01` e `1` são a mesma)
**And** as listas têm limite e paginação, e `formato=csv` exporta tudo (BOM, `;`, células que começam com `=`, `+`, `-` ou `@` ganham apóstrofo)

**Given** a competência fechada
**Then** a resposta diz que está fechada

**Given** as duas origens com lançamentos na mesma competência
**Then** a resposta e a tela trazem o aviso de que o DRE as soma

**Given** `GET /api/aud/conferencia/origens`
**Then** lista as origens existentes na empresa, com linhas e intervalo de competências (a tela preenche as duas escolhas com elas)

**Given** o gestor
**When** pede o relatório
**Then** `403` (a capacidade `ler` irrestrita; o menu mostra a aba a controladoria, analista, auditoria e diretoria)

**Given** o relatório
**Then** é só leitura, isolado por empresa, e cada consulta ou exportação grava `visualizar`/`exportar` na trilha com os parâmetros

**Given** a tela "Conferência" em Administrativo
**Then** tem competência, duas origens, filial, tolerância, resumo, tabela de diferenças, só-em-um-lado, resumo por linha do DRE e o botão de exportar CSV

### Story 2.13: Importação automática diária (opcional, AD-15)

As a analista,
I want que a importação rode sozinha todo dia,
So that eu não precise lembrar.

**Acceptance Criteria:**

**Given** a hora em `config_empresa` (padrão 06:30, `America/Sao_Paulo`; a tabela é criada aqui se ainda não existir)
**When** chega a hora
**Then** dispara as importações 2.3 e 2.6

**Given** `pg_try_advisory_lock` em conexão dedicada, com o unlock na mesma conexão (descartada se o unlock falhar)
**When** outra execução está em andamento
**Then** a segunda não roda e registra

**Given** uma execução agendada
**When** termina
**Then** a trilha usa `user_id` nulo e origem agendador
**And** a falha fica visível em `execucoes`

## Epic 3: DRE por departamento, com orçado e ano anterior

Qualquer usuário autorizado lê o DRE do grupo, por filial e por departamento, mensal e acumulado, comparando Realizado, Orçado e Ano anterior.

**Nota:** o perfil `gestor` não vê dado de departamento até o épico 4. O fim deste épico é a primeira demonstração ao cliente.

### Story 3.1: Estrutura do DRE editável (FR-5)

As a analista,
I want criar, editar, reordenar e excluir as linhas do DRE,
So that a estrutura acompanhe o DRE do cliente sem novo deploy.

**Acceptance Criteria:**

**Given** a rota de linhas (hoje só leitura)
**When** o analista cria, edita, reordena ou exclui
**Then** funciona na tela, com descrição, ordem, nível, tipo (conta ou subtotal) e as linhas somadas

**Given** um subtotal que referencia linha inexistente ou cria ciclo
**When** salva
**Then** recusa com mensagem

**Given** uma linha usada no Mapa
**When** tenta excluir
**Then** `409` listando as contas

**Given** competência fechada
**When** a estrutura muda
**Then** o mês fechado não muda (lê do snapshot, 1.8)

**Given** um perfil sem `controladoria`
**When** tenta alterar
**Then** `403`
**And** cada alteração vai à trilha

**Given** a migration
**Then** `dre_linhas` ganha `papel` (opcional, único por empresa quando preenchido)
**And** o analista pode marcar a linha com um papel: faturamento líquido, margem composta, impostos, comercial, logística, administrativo, margem líquida, outras receitas e despesas, resultado antes do IR ou resultado após o IR (AD-20)

### Story 3.2: Carregar estrutura e Mapa por arquivo (FR-5, FR-6)

As a analista,
I want carregar a estrutura e o Mapa de contas por arquivo,
So that eu não digite centenas de contas uma a uma.

**Acceptance Criteria:**

**Given** um arquivo de estrutura (código, descrição, nível, ordem, tipo, soma_de)
**When** carrega
**Then** cria ou atualiza pelo código e valida as referências
**And** erro bloqueante não grava nada

**Given** um arquivo de Mapa (conta_padrão, linha_código, sinal)
**When** carrega
**Then** cria ou atualiza pela conta
**And** linha inexistente rejeita a linha e sinal só vale `1` ou `-1`

**Given** contas com prefixo `%`
**When** resolvidas
**Then** a mais específica vence, como hoje

**Given** reenvio do arquivo
**When** carrega
**Then** atualiza sem duplicar
**And** a trilha registra a carga com a quantidade de linhas

### Story 3.3: Estrutura e Mapa iniciais derivados da planilha (tarefa de dados)

As a dono do produto,
I want gerar a estrutura e o Mapa iniciais a partir do DRE da planilha e validá-los com a área de negócio,
So that o primeiro fechamento não trave em centenas de contas sem mapeamento.

**Acceptance Criteria:**

**Given** as abas de DRE da planilha
**When** o arquivo de estrutura é gerado
**Then** reproduz as linhas: Faturamento Líquido; CMV, Deduções e Verbas; Margem Composta; Impostos sobre Faturamento Líquido; Margem de Contribuição; os departamentos Comercial, Logística e Administrativo e o Total de Despesas; Margem Líquida; Outras Receitas e Despesas; Resultado antes da tributação; IRPJ

**Given** as contas das cargas de validação (CSV e `ERP.*`)
**When** o Mapa é gerado
**Then** cada conta com lançamento tem linha, ou aparece numa lista "sem mapeamento"

**Given** os sinais das contas `ERP.*`
**When** o Mapa é gerado
**Then** ficam marcados como provisórios, confirmados pela paridade (3.7) ou pela resposta do cliente

**Given** os arquivos gerados
**When** o repositório é verificado
**Then** eles não estão nele
**And** a validação registra quem e quando, e eles são carregados em Teste pela 3.2

### Story 3.4: DRE com percentual sobre o Faturamento Líquido (FR-7)

As a usuário autorizado,
I want ver o DRE mensal e acumulado, do grupo ou de uma filial, com o percentual de cada linha sobre o Faturamento Líquido,
So that eu compare a estrutura de custos entre meses e filiais.

**Acceptance Criteria:**

**Given** a tela do DRE
**When** aberta
**Then** cada linha mostra também o percentual sobre o Faturamento Líquido

**Given** Faturamento Líquido zero
**When** o DRE é calculado
**Then** o percentual aparece como "não calculável"

**Given** nenhuma linha com o papel faturamento líquido
**When** o DRE é aberto
**Then** a tela avisa em vez de calcular o percentual

**Given** qualquer subtotal
**When** o DRE é calculado
**Then** é exatamente a soma das linhas que o compõem (teste com centavos)

**Given** lançamentos com várias UFs na mesma filial
**When** o DRE é calculado
**Then** a filial soma todas elas

**Given** competência fechada
**When** o DRE é consultado
**Then** usa o snapshot (1.8), inclusive os percentuais

### Story 3.5: DRE por departamento (FR-6, FR-7)

As a Controller,
I want ver o DRE por departamento, com as contas de cada um,
So that eu veja onde cada área gastou.

**Acceptance Criteria:**

**Given** contas ligadas a departamentos (2.10)
**When** abro a visão por departamento
**Then** lista cada departamento (agrupado em Comercial, Logística e Administrativo) com as contas, o total e o percentual sobre o Faturamento Líquido

**Given** contas sem departamento
**When** abro a visão
**Then** aparecem em "Sem departamento", visível só a perfis sem restrição

**Given** o perfil `gestor` (sem escopo até o épico 4)
**When** abre essa visão ou chama a API
**Then** não recebe dado de departamento

**Given** Fechar
**When** grava o snapshot
**Then** `dre_fechamento_contas` ganha a coluna `departamento` e o snapshot grava o departamento de cada conta

### Story 3.6: Orçado e ano anterior, "sem orçado" e detalhe até a filial (FR-8)

As a usuário autorizado,
I want comparar o Realizado com o Orçado e o Ano anterior por linha e conta, descendo até a filial,
So that eu entenda de onde vem cada variação.

**Acceptance Criteria:**

**Given** o DRE
**When** aberto
**Then** cada linha e conta mostra Realizado, Orçado, Ano anterior e a variação em valor e em percentual

**Given** Orçado zero
**When** a variação é calculada
**Then** o percentual aparece como "não calculável"

**Given** conta ou filial sem orçado
**When** o DRE é consultado
**Then** mostra o Realizado com "sem orçado" e variação não calculável, nunca zero

**Given** uma conta
**When** abro o detalhe
**Then** vejo por filial os valores e a origem de cada um (Oracle ou arquivo)

**Given** um Ano anterior sem dado (as views começam em 2021)
**When** o DRE é consultado
**Then** aparece vazio, não zero

### Story 3.7: Paridade do DRE com a planilha

As a dono do produto,
I want comparar o DRE do sistema com o da planilha para um mês de referência,
So that eu saiba que os números batem antes de mostrar ao cliente.

**Nota:** a conferência por linha de subtotal usa as fórmulas apuradas em 29/10/2026 (soma simples dos componentes, valores já com sinal; ver a nota do Épico 3 e `respostas-cliente-2026-10-28.md`). A base do percentual segue a 3.15.

**Acceptance Criteria:**

**Given** um mês de referência com Oracle consolidado, CSV de contas e orçado carregados em Teste, e um arquivo com os valores da planilha por linha do DRE e filial (fora do repositório)
**When** peço a paridade
**Then** lista por linha e filial o valor do sistema, o da planilha, a diferença e se passou da tolerância (0,01)

**Given** diferença numa linha de subtotal
**When** a paridade termina
**Then** aponta a primeira linha componente que diverge

**Given** valores iguais com sinal oposto
**When** a paridade termina
**Then** marca "sinal invertido", para ajustar o Mapa

**Given** todas as linhas dentro da tolerância
**When** a paridade termina
**Then** o relatório conclui "bate" e grava a execução em `execucoes`
**And** é só leitura e reaproveita o mecanismo da conferência 2.12

### Story 3.8: Primeira demonstração ao cliente (marco)

As a dono do produto,
I want mostrar o DRE por departamento ao cliente com um mês de referência,
So that o cliente valide o rumo antes de os gerentes entrarem.

**Acceptance Criteria:**

**Given** o épico 3 em Teste com um mês de referência que bate (3.7)
**When** a demonstração é feita com os gestores do cliente
**Then** o que foi aprovado e o que precisa mudar ficam registrados

**Given** as perguntas abertas do PRD (rateio, limite de desvio, lista de aviso, folha)
**When** a demonstração termina
**Then** cada uma ganha resposta ou data de resposta

**Given** a demonstração
**When** acontece
**Then** nenhum dado real do cliente sai do ambiente de Teste (sem capturas de tela no repositório)

## Epic 4: Acesso seguro por perfil e departamento

Cada pessoa vê e faz só o que cabe ao seu papel, o Gerente enxerga apenas o seu departamento, e o cadastro público deixa de existir.

**Nota:** a história 4.1 não depende de nenhuma outra e sai antes de todas as demais, inclusive do épico 1. Não há expiração de usuário por tempo: o acesso termina por bloqueio (4.6). O perfil de TI fica para depois do A1.

### Story 4.1: Fechar o cadastro público (FR-19, AR-14), a primeira a sair

As a dono do produto,
I want que ninguém consiga criar conta pela internet,
So that só quem eu autorizo entre no sistema.

**Acceptance Criteria:**

**Given** `ALLOW_SELF_REGISTER` ausente ou `false`
**When** alguém chama `POST /api/auth/register`
**Then** responde `404` e nenhuma empresa ou usuário é criado

**Given** `ALLOW_SELF_REGISTER=true`
**When** alguém chama o cadastro
**Then** o comportamento é o de hoje (uso em desenvolvimento)

**Given** o frontend
**When** a rota `/register` é aberta
**Then** redireciona para o login

**Given** as rotas públicas
**When** um teste as lista
**Then** confirma que nenhuma outra cria usuário ou empresa
**And** o padrão `false` consta em `.env.example` e no compose
**And** a história pode ser entregue sozinha, antes de qualquer outra

### Story 4.2: Perfil `analista` e matriz de capacidades (AD-11)

As a Controller,
I want perfis com capacidades claras,
So that o analista tenha o que precisa sem poder fechar o mês.

**Acceptance Criteria:**

**Given** a migration
**Then** `users.perfil` aceita `analista` além dos atuais

**Given** `perfis.go`
**Then** define as capacidades (ler, ler no escopo, carregar, ajustar, editar Mapa, estrutura e dicionário, configurar rateio, fechar, reabrir, aplicar rateio, definir limite, justificar, gerir usuários) e a matriz: analista, controladoria (Controller), gestor (Gerente), diretoria e auditoria (só leitura)

**Given** o middleware
**When** protege uma rota
**Then** aceita uma capacidade e, até a 4.3, ainda aceita a lista de perfis atual

**Given** a matriz
**When** um teste a percorre
**Then** confirma que o analista carrega e edita Mapa e estrutura mas não fecha nem reabre, e que diretoria e auditoria não têm nenhuma capacidade de escrita

**Given** o admin de plataforma
**When** acessa qualquer capacidade
**Then** segue com acesso total

### Story 4.3: Migrar as rotas para capacidades (AD-11)

As a Controller,
I want que todas as rotas usem a matriz,
So that não haja listas de perfil soltas e divergentes.

**Acceptance Criteria:**

**Given** as rotas atuais
**When** são migradas
**Then** cada uma declara uma capacidade e os perfis existentes mantêm o comportamento de hoje (testes da 1.1)

**Given** diretoria e auditoria
**When** chamam qualquer rota de escrita
**Then** são recusadas (teste que percorre as rotas)

**Given** um analista
**When** carrega, edita Mapa ou estrutura
**Then** funciona
**And** recebe `403` ao fechar ou reabrir

**Given** a última rota migrada
**When** a migração termina
**Then** a assinatura com lista de perfis é removida

**Given** uma rota criada depois desta história
**When** o teste de rotas roda
**Then** falha se ela usar lista de perfis em vez de capacidade

### Story 4.4: Ids de área novos e menu (AD-12)

As a usuário,
I want o menu com as áreas do cliente (Administrativo, Configurações, Comercial, Operação),
So that eu encontre cada coisa onde o contrato diz.

**Acceptance Criteria:**

**Given** a migration
**Then** `users.areas` é transformada por um único `UPDATE` com `CASE` (`estrategico`→`empresa`, `administrativo`→`configuracoes`, `operacional`→`operacao`) e o `DEFAULT` é atualizado

**Given** usuários de cada combinação de áreas (inclusive vazia e as três)
**When** a migration roda
**Then** cada um fica com o equivalente
**And** rodar de novo não altera nada

**Given** o backend
**When** valida áreas
**Then** as constantes e a validação usam os ids novos e recusam os antigos

**Given** o `navigation.ts`
**When** o menu é montado
**Then** mostra Administrativo (DRE, fechamentos, Mapa), Configurações (carga, usuários, trilha), Comercial e Operação ("em breve") e o módulo de administração da plataforma passa a se chamar "Plataforma"
**And** backend, rotas e frontend saem no mesmo release

### Story 4.5: Usuários com perfil e departamentos (FR-18, AD-10)

As a Controller,
I want definir o perfil de cada usuário e os departamentos de cada Gerente,
So that o escopo de cada um fique claro.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `usuario_departamentos (user_id, departamento_id)`, mantida por `limpar_dados_teste` (é configuração)

**Given** a tela de usuários
**When** o Controller escolhe `analista` ou `gestor`
**Then** salva, e para `gestor` pode ligar um ou mais departamentos

**Given** um perfil que não é `gestor`
**When** se tenta ligar departamentos
**Then** a ligação é recusada

**Given** qualquer alteração de perfil ou departamento
**When** é salva
**Then** vai à trilha
**And** só a capacidade "gerir usuários" acessa a tela

### Story 4.6: Bloqueio de usuário no lugar da expiração (FR-22, AD-19)

As a Controller,
I want bloquear e desbloquear um usuário com motivo,
So that o acesso termine quando eu decidir, e não por prazo.

**Acceptance Criteria:**

**Given** a migration
**Then** `users` ganha `bloqueado_em`, `bloqueado_por` e `motivo_bloqueio`

**Given** um usuário bloqueado
**When** tenta entrar ou renovar a sessão
**Then** é recusado com mensagem clara

**Given** uma sessão aberta
**When** o usuário é bloqueado
**Then** a requisição seguinte devolve `403`

**Given** bloquear ou desbloquear sem motivo
**When** envia
**Then** recusa; com motivo, grava na trilha

**Given** o próprio usuário
**When** tenta se bloquear
**Then** recusa
**And** o último administrador de plataforma ativo não pode ser bloqueado

**Given** a verificação por `trial_ends_at`
**When** o login e o middleware rodam
**Then** ela foi removida e um usuário comum com a coluna vazia entra normalmente (teste que reproduz o caso do login)
**And** a tela de usuários troca a coluna "Trial" por "Bloqueado" e o menu deixa de mostrar "Licença vence"

### Story 4.7: Escopo por departamento que nega por padrão (FR-18, AD-10)

As a Controller,
I want que o Gerente nunca receba dado de outro departamento,
So that a confidencialidade não dependa da tela.

**Acceptance Criteria:**

**Given** o escopo como `struct {Irrestrito bool; Deptos []string}` com valor zero sem acesso
**When** um Gerente com os departamentos A e B consulta qualquer rota que devolve valor por conta
**Then** recebe só as contas de A e B

**Given** um Gerente sem departamento
**When** consulta
**Then** recebe resposta vazia, nunca tudo

**Given** o Gerente pedindo outro departamento direto na API
**When** chama
**Then** `403`

**Given** a rota do DRE do grupo (faturamento, margens)
**When** o Gerente chama
**Then** `403`, sem receber a base

**Given** perfis sem restrição
**When** consultam
**Then** veem tudo

**Given** o casamento de conta por prefixo
**When** é feito
**Then** acontece em Go, nunca com `LIKE`
**And** mês fechado aplica o filtro sobre o snapshot por contas

### Story 4.8: "Meu departamento" para o Gerente (FR-18, FR-8)

As a Gerente,
I want ver as contas do meu departamento com Realizado, Orçado, Ano anterior e variação,
So that eu entenda o meu resultado.

**Acceptance Criteria:**

**Given** um Gerente com departamentos ligados
**When** abre "Meu departamento"
**Then** vê as contas deles com os quatro valores, os totais e a variação

**Given** o percentual sobre o Faturamento Líquido
**When** é exibido
**Then** é calculado no servidor, sem devolver a base

**Given** mês fechado
**When** o Gerente consulta
**Then** lê do snapshot

**Given** o menu
**When** é montado
**Then** a aba aparece só para o `gestor`
**And** nenhum outro departamento é visível, nem por URL direta

### Story 4.9: Criar usuário da empresa (FR-19)

As a administrador de plataforma,
I want criar o usuário da empresa com perfil e departamentos,
So that o gerente entre sem cadastro público.

**Acceptance Criteria:**

**Given** e-mail, nome, perfil e áreas
**When** o administrador cria
**Then** o usuário é vinculado à empresa e recebe um link para definir a própria senha; sem SMTP configurado, a tela mostra o link para o administrador repassar

**Given** o usuário criado
**When** é gravado
**Then** não recebe nenhum prazo (o `CreateUserHandler` deixa de gravar os 14 dias)

**Given** e-mail já existente
**When** cria
**Then** `409`

**Given** a criação
**When** termina
**Then** vai à trilha
**And** só o administrador de plataforma cria; o perfil de TI, que poderá criar usuários, fica para depois

### Story 4.10: Acompanhamento do projeto

As a gestor do cliente,
I want ver o andamento das histórias do projeto num quadro Kanban dentro do sistema,
So that eu acompanhe o que está pronto, em andamento ou aguardando resposta, sem precisar de conta em outra ferramenta.

**Nota:** o quadro é somente leitura e vem de `epics.md` e `sprint-status.yaml`, embutidos no binário (`backend/acompanhamento/dados.json`, regenerado por `go run ./cmd/gerar-acompanhamento`); o dono do produto continua usando o quadro do GitHub em paralelo. A área nova `acompanhamento` não entra no padrão de `users.areas`.

**Acceptance Criteria:**

**Given** um usuário só com a área `acompanhamento`
**When** entra no sistema
**Then** cai em `/acompanhamento`, com cinco colunas (Backlog, Pronto, Em andamento, Em revisão, Feito), contador por coluna, filtro por épico e busca por texto
**And** o cartão mostra id, título, épico e o selo "Aguardando o cliente" quando a história depende de resposta ou decisão do cliente

**Given** um cartão
**When** o usuário clica
**Then** vê o resumo e os critérios de aceite, sem nenhuma ação de escrita

**Given** a rota `GET /api/aud/acompanhamento`
**When** chamada por usuário com a área `acompanhamento`
**Then** devolve o JSON com cabeçalho de cache curto; qualquer método diferente de GET recebe `405`
**And** esse usuário recebe `403` em todas as outras rotas `/api/aud/*`, e quem não tem a área recebe `403` nesta

**Given** uma alteração em `epics.md` ou `sprint-status.yaml`
**When** o `dados.json` não foi regenerado
**Then** um teste Go falha dizendo como regenerar

## Epic 5: Gerentes justificam ajustes e desvios

Os ajustes vêm do ERP, os gerentes justificam (com log), e cada gerente recebe por e-mail o DRE do seu departamento, toda sexta (acumulado) e no fechamento.

**Nota:** o limite de desvio e a Lista de aviso começam sem valor definido pelo cliente; até lá, nenhum desvio exige justificativa e a tela avisa.

### Story 5.1: Limite de desvio e Lista de aviso (FR-13)

> **REVISÃO 28/10/2026 (resposta do cliente):** não há limite de desvio: todo desvio tem a linha de justificativa, **não obrigatória**. A parte de limite fica sem efeito (nenhum desvio fica "pendente" por regra); a Lista de aviso continua configurável.

As a Controller,
I want definir o limite de desvio e a Lista de aviso,
So that o sistema saiba o que exige justificativa e quem avisar.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `auditoria.config_empresa (empresa_id, chave, valor JSONB)` (criada aqui se a 2.13 não existiu), mantida por `limpar_dados_teste` (é configuração)
**And** `dre_fechamentos` ganha `limite_desvio`

**Given** a tela de Configurações
**When** o Controller define o limite em percentual e/ou valor
**Then** salva e grava na trilha
**And** um analista recebe `403`

**Given** a Lista de aviso
**When** o Controller informa os e-mails
**Then** salva em `lista_aviso`
**And** e-mail com formato inválido é recusado

**Given** nenhum limite definido
**When** a tela é aberta
**Then** nenhum desvio exige justificativa e a tela mostra "defina o limite"

**Given** Fechar
**When** grava o fechamento
**Then** guarda o `limite_desvio` vigente
**And** mudar o limite depois não altera a classificação de um mês fechado

### Story 5.2: Ver os desvios que exigem justificativa (FR-12)

> **REVISÃO 28/10/2026 (resposta do cliente):** a lista mostra **todos** os desvios (não só os acima de um limite), e a justificativa é opcional.

As a Gerente,
I want ver as contas do meu departamento cujo desvio passou do limite,
So that eu saiba o que preciso explicar.

**Acceptance Criteria:**

**Given** o limite definido
**When** o Gerente abre "Desvios"
**Then** lista só as contas do seu escopo cujo desvio contra o Orçado ou contra o Ano anterior passa do limite, com os valores e a base

**Given** um desvio
**When** é identificado
**Then** a chave é `desvio:<AAAA-MM>:<filial>:<conta>:<base>`, gerada só por `dre.ChaveDesvio`
**And** a tela, a API e o e-mail usam a mesma função

**Given** nenhuma justificativa existente
**When** a lista é aberta
**Then** todos aparecem como "pendente"

**Given** conta sem orçado
**When** o desvio é calculado
**Then** não entra na base Orçado ("sem orçado" não é desvio)

**Given** mês fechado
**When** a lista é aberta
**Then** usa o snapshot e o limite do fechamento

**Given** o Controller
**When** abre "Desvios"
**Then** vê os desvios de todos os departamentos
**And** a lista é paginada

### Story 5.3: Registrar a justificativa de um desvio (FR-12, AD-7)

As a Gerente,
I want escrever a justificativa de um desvio do meu departamento,
So that a controladoria saiba por que a área fugiu do orçado.

**Acceptance Criteria:**

**Given** a migration
**Then** `auditoria.justificativas (alvo_tipo, alvo_chave, texto, autor, valor_desvio, criado_em)` é append-only por gatilho que respeita `imutavel_salvo_em_teste`
**And** ela entra em `limpar_dados_teste`

**Given** um Gerente com escopo
**When** justifica um desvio do seu departamento
**Then** grava o texto (não vazio, até 2.000 caracteres) com autor, momento e o valor do desvio, e o status vira "justificada"

**Given** um desvio de outro departamento (API direta)
**When** o Gerente justifica
**Then** `403`

**Given** edição de uma justificativa
**When** o Gerente salva
**Then** cria nova versão, a anterior permanece e a vigente é a de maior `id`

**Given** o valor do desvio mudou depois, em competência aberta
**When** a lista é aberta
**Then** o status vira "a revisar" (derivado, nunca gravado)

**Given** a justificativa
**When** é gravada
**Then** vai à trilha
**And** a diretoria e a controladoria só leem

### Story 5.4: Ajustes vindos do ERP no realizado (FR-4, AD-6)

As a Controller,
I want que os ajustes feitos no ERP apareçam no DRE separados do valor carregado,
So that o gerente veja o que foi ajustado sem que o sistema aceite ajuste manual.

**Nota:** decisão do cliente (28/10/2026): o ajuste vem do ERP e chega pelo banco, junto da ingestão; o sistema não tem lançamento manual de ajuste. O formato do ajuste na view de origem (coluna própria ou lançamento marcado) ainda depende do dono dos dados; esta história só é executável quando ele responder.

**Acceptance Criteria:**

**Given** a migration
**Then** `lancamentos` ganha `tipo` (`lancamento` ou `ajuste`, padrão `lancamento`)
**And** o Ingestor é o único que grava `tipo`, a partir do que a fonte informa

**Given** uma carga com linhas de ajuste
**When** a importação termina
**Then** as linhas de ajuste substituem as do mesmo escopo, como qualquer lançamento (nunca duplicam)

**Given** `dre.Realizado`
**When** calcula o realizado
**Then** soma lançamentos e ajustes uma só vez, com o sinal do Mapa
**And** o detalhe mostra o valor carregado e os ajustes separados

**Given** Fechar
**When** grava o snapshot
**Then** `dre_fechamento_contas` ganha a coluna `ajustes` e o snapshot a preenche

**Given** qualquer perfil
**When** tenta criar, editar ou apagar um ajuste pelo sistema
**Then** não existe rota para isso

### Story 5.5: Justificar os ajustes e registrar o log (FR-4, FR-12)

> **REVISÃO 28/10/2026 (resposta do cliente):** **descartada.** O gerente justifica só o desvio (5.3), nunca o ajuste. Ajuste e alteração de conta são feitos só pela controladoria; se houver lançamento em conta incorreta, o gerente pede à controladoria que realoque. Não implementar.

As a Gerente,
I want ver os ajustes das contas do meu departamento e justificá-los,
So that nenhum ajuste fique sem explicação e fique registrado quem explicou.

**Nota:** decisão do cliente (28/10/2026): o gerente só justifica; cada justificativa gera um registro (log) na trilha. O ajuste não tem id próprio: a chave é `ajuste:<AAAA-MM>:<filial gerencial>:<conta>`.

**Acceptance Criteria:**

**Given** a migration
**Then** `justificativas` aceita a chave canônica de ajuste (AD-7), append-only como as demais

**Given** ajustes em contas do seu escopo
**When** o Gerente abre "Ajustes"
**Then** vê cada um com valor e status "pendente" ou "justificado"

**Given** um ajuste
**When** o Gerente justifica
**Then** o status vira "justificado" e a trilha registra autor, chave e texto
**And** uma justificativa vigente basta, mesmo com mais de um Gerente no departamento

**Given** ajuste de outro departamento
**When** o Gerente tenta justificar
**Then** `403`
**And** o Controller vê os ajustes pendentes de todos os departamentos

**Given** o valor do ajuste mudar numa nova carga
**When** a chave é a mesma
**Then** a justificativa anterior continua no histórico, mas o ajuste volta a "pendente"

### Story 5.6: Fila de e-mail e envio (AD-13)

As a dono do produto,
I want uma fila de e-mails com envio confiável,
So that nenhum e-mail se perca nem saia duplicado.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `auditoria.outbox_email (empresa_id, tipo, chave_dedup UNIQUE, destinatarios, assunto, corpo_html, corpo_texto, status, tentativas, erro, criado_em, enviado_em)`, incluída em `limpar_dados_teste`

**Given** uma função nova em `services/email.go` que envia HTML e texto com `Message-ID` estável (reaproveita `sendMailSSL`)
**When** o worker drena a fila com `FOR UPDATE SKIP LOCKED`
**Then** envia e marca como enviado (entrega pelo menos uma vez); um teste enfileira um item sintético por `dre.Envio.Enfileirar` e confere os estados

**Given** um item com a mesma `chave_dedup`
**When** é enfileirado de novo
**Then** não duplica

**Given** SMTP não configurado ou erro
**When** o worker tenta enviar
**Then** o status é `falha` com a mensagem e as tentativas contam (até 3, com intervalo)

**Given** o ambiente em Teste
**When** um item é enfileirado
**Then** o destinatário é só o administrador de plataforma da empresa, nunca os destinatários originais

**Given** o enfileirar e o resultado do envio
**When** acontecem
**Then** vão à trilha

### Story 5.7: Aviso dos ajustes pendentes por e-mail (FR-4)

> **REVISÃO 28/10/2026 (resposta do cliente):** **descartada**, pelo mesmo motivo da 5.5: o gerente não justifica ajuste, então não há aviso de ajuste a ele. Não implementar.

As a Controller,
I want que os gerentes sejam avisados quando chegam ajustes a justificar,
So that a justificativa não dependa de alguém abrir a tela.

**Nota:** depende da resposta do cliente sobre quem recebe o aviso (o gerente do departamento, a Lista de aviso ou ambos). Até lá, não executar.

**Acceptance Criteria:**

**Given** uma carga que traga ajuste novo ou alterado (5.4)
**When** a transação termina
**Then** enfileira `aviso_ajuste` (5.6) com chave `ajuste:<AAAA-MM>:<filial gerencial>:<conta>:<valor>` para os destinatários definidos

**Given** o e-mail do aviso
**When** é gerado
**Then** traz conta, departamento, competência e valor, sem dado de outros departamentos

**Given** o mesmo ajuste na mesma carga reenviada
**When** a importação repete
**Then** não enfileira de novo (chave de deduplicação)

**Given** o ambiente em Teste
**When** chega um ajuste
**Then** o aviso vai só ao administrador (regra da 5.6)

### Story 5.8: E-mail do DRE por departamento no fechamento (FR-14)

As a Gerente,
I want receber por e-mail o DRE do meu departamento quando o mês fecha,
So that eu saiba o que justificar sem abrir o sistema à procura.

**Acceptance Criteria:**

**Given** Fechar
**When** grava o fechamento
**Then** na mesma transação enfileira um `dre_departamento` por Gerente (chave `fech:<id>:user:<uid>`), com o corpo renderizado do snapshot: todos os departamentos do destinatário, Realizado, Orçado, variação, a quantidade de desvios pendentes e o link para a tela "Desvios" (5.2), onde ele justifica

**Given** o corpo do e-mail
**When** é gerado
**Then** não contém dado de outros departamentos nem o DRE do grupo (teste com um Gerente de dois departamentos)

**Given** um Gerente sem departamento ou bloqueado
**When** o fechamento acontece
**Then** não recebe, e o motivo é registrado

**Given** o ambiente em Teste
**When** o fechamento acontece
**Then** o administrador recebe um e-mail por Gerente, com o assunto indicando o destinatário original

**Given** reabertura e novo fechamento
**When** Fechar roda de novo
**Then** a nova versão gera novo envio
**And** o enfileirar e o resultado vão à trilha

### Story 5.9: Painel de envios (FR-14)

As a Controller,
I want ver o estado de cada e-mail e reenviar os que falharam,
So that nenhum gerente fique sem o DRE sem eu saber.

**Acceptance Criteria:**

**Given** a tela de envios
**When** o Controller escolhe uma competência
**Then** lista tipo, destinatário, status, tentativas e erro de cada e-mail, e um resumo por status

**Given** um e-mail com falha
**When** o Controller clica em "Reenviar"
**Then** nova tentativa com a mesma chave e o mesmo `Message-ID`

**Given** um perfil sem a capacidade
**When** abre a tela ou chama a API
**Then** `403`
**And** o reenvio vai à trilha

## Epic 6: Dashboards de desvios e tendência

A diretoria vê os maiores desvios por departamento, gerente e filial, a tendência da despesa sobre o Faturamento Líquido, a ponte do resultado e a projeção do ano.

**Nota:** a história 6.6 é opcional e não vem de nenhum FR: nasceu dos botões de copiar imagem do painel que o cliente já usa. O Gerente vê só o seu departamento e não acessa a ponte.

### Story 6.1: Ranking dos maiores desvios (FR-16)

As a diretoria,
I want ver as contas com os maiores desvios do período,
So that eu saiba onde olhar primeiro.

**Acceptance Criteria:**

**Given** os desvios derivados por `dre.ChaveDesvio` (5.2)
**When** abro o ranking
**Then** ordena do maior para o menor valor absoluto, com conta, filial, departamento, Gerente, base (Orçado ou Ano anterior), valores, variação e o status da justificativa

**Given** os filtros
**When** filtro por ano, competência, filial, departamento, Gerente ou base
**Then** o ranking e os totais respeitam o filtro

**Given** o limite da 5.1
**When** o ranking é aberto
**Then** os desvios acima dele aparecem marcados, e o ranking mostra todos

**Given** um Gerente
**When** abre o ranking
**Then** vê só o ranking do seu escopo, nem por API direta

**Given** mês fechado
**When** o ranking é aberto
**Then** usa o snapshot e o limite do fechamento
**And** a lista é paginada e clicar numa linha abre o detalhe da conta (3.6)

### Story 6.2: Desvios por departamento, Gerente, filial e ano (FR-16)

As a diretoria,
I want agrupar os desvios por departamento, Gerente, filial ou ano,
So that eu compare as áreas.

**Acceptance Criteria:**

**Given** o dashboard
**When** escolho o agrupamento
**Then** mostra para cada grupo Realizado e Orçado (gráfico e tabela), a variação e a cobertura de justificativas (justificadas entre as acima do limite)

**Given** contas sem departamento
**When** o dashboard é aberto
**Then** aparecem em "Sem departamento", só para perfis sem restrição

**Given** qualquer total
**When** é exibido
**Then** é exatamente a soma dos itens

**Given** um Gerente
**When** abre o dashboard
**Then** vê só o seu departamento
**And** diretoria e Controller veem todos, em leitura

### Story 6.3: Despesa sobre o Faturamento Líquido e "Está subindo?" (FR-17)

As a diretoria,
I want ver quanto a despesa pesa sobre o Faturamento Líquido a cada ano e se está subindo,
So that eu perceba cedo a perda de eficiência.

**Acceptance Criteria:**

**Given** as linhas com os papéis faturamento líquido, comercial, logística e administrativo (AD-20)
**When** abro "Tendência"
**Then** mostra por ano a despesa total sobre o Faturamento Líquido e a de cada grupo

**Given** o acumulado do ano atual
**When** é comparado
**Then** com o mesmo período do ano anterior, em pontos percentuais, indicando "subindo", "estável" ou "caindo" (tolerância de 0,1 ponto) para cada grupo

**Given** Faturamento Líquido zero ou ano sem dado
**When** a tendência é calculada
**Then** aparece "não calculável" ou vazio, nunca zero

**Given** um papel não cadastrado
**When** a tela é aberta
**Then** avisa

**Given** um Gerente
**When** abre a tendência
**Then** vê só o seu departamento, com o percentual calculado no servidor e sem a base

### Story 6.4: Ponte do resultado (FR-17)

As a diretoria,
I want ver por que a margem mudou de um ano para o outro,
So that eu entenda o que a moveu.

**Acceptance Criteria:**

**Given** dois anos no mesmo período e o alvo (Margem líquida ou Resultado após o IR, pelos papéis)
**When** abro a ponte
**Then** mostra o alvo do ano anterior, as contribuições de Margem composta, Impostos, Comercial, Logística e Administrativo (e, para o resultado após o IR, Outras receitas e despesas e IRPJ), e o alvo do ano atual

**Given** a unidade em pontos percentuais do Faturamento Líquido
**When** a ponte é calculada
**Then** a soma das contribuições é igual à diferença total

**Given** a unidade em valor
**When** a ponte é calculada
**Then** inclui "Volume de vendas" e a soma fecha a diferença exata (teste com centavos)

**Given** o resultado
**When** é exibido
**Then** um texto resume a maior contribuição positiva e a maior negativa

**Given** Faturamento Líquido zero em algum ano
**When** a ponte é calculada
**Then** aparece "não calculável"

**Given** um Gerente
**When** abre a ponte
**Then** `403` (a ponte envolve o resultado do grupo)

### Story 6.5: Projeção de fechamento do ano (FR-17)

As a diretoria,
I want ver para onde o ano caminha,
So that eu possa agir antes de dezembro.

**Acceptance Criteria:**

**Given** o ano corrente com realizado até o mês M
**When** abro "Projeção"
**Then** mostra o realizado de janeiro a M e o estimado de M+1 a dezembro, marcando o que é estimado e o método

**Given** o método Orçado
**When** é escolhido
**Then** o estimado é o orçado dos meses restantes

**Given** o método Sazonal
**When** é escolhido
**Then** o estimado é o restante do ano anterior multiplicado pela razão entre o acumulado atual e o do ano anterior; razão fora do intervalo de 0 a 3 vale 1

**Given** que não há orçado
**When** a projeção é aberta
**Then** usa o Sazonal e o método Orçado fica desabilitado

**Given** M igual a 12
**When** a projeção é aberta
**Then** a projeção é o realizado

**Given** os indicadores
**When** são exibidos
**Then** mostram Faturamento Líquido, despesas sobre o Faturamento Líquido, margem líquida e resultado após o IR, comparados ao ano anterior e ao orçado do ano

**Given** um Gerente
**When** abre a projeção
**Then** vê só a despesa projetada do seu departamento, sem o Faturamento Líquido
**And** o filtro de filial funciona

### Story 6.6: Copiar o gráfico como imagem (opcional)

As a diretoria,
I want copiar ou baixar um gráfico como imagem,
So that eu cole na apresentação.

**Acceptance Criteria:**

**Given** qualquer gráfico dos dashboards
**When** clico em "Copiar imagem" ou "PNG"
**Then** recebo a imagem do que está na tela, com título, período e filtros

**Given** um Gerente
**When** copia um gráfico
**Then** a imagem só contém o que ele já vê

**Given** a imagem
**When** é gerada
**Then** é gerada no navegador, e não há exportação para PDF ou Excel (fora de escopo no PRD)

## Epic 7: Rateio entre filiais

O DRE por filial mostra o custo real, com as despesas dos departamentos centralizados rateadas entre as unidades.

**Nota:** este épico só começa depois de o cliente responder quais departamentos são centralizados e se o critério é fixo ou mensal (perguntas 4 e 5 do PRD). Cada regra vale a partir de uma competência até surgir a seguinte, o que cobre os dois casos.

### Story 7.1: Regras de rateio (FR-15)

> **REVISÃO 28/10/2026 (resposta do cliente):** o rateio é gravado **mês a mês**, tanto na visão por faturamento quanto por percentual manual; mudar o percentual de um mês não altera os outros meses.

As a analista,
I want definir para cada departamento centralizado o critério de rateio,
So that o sistema saiba como distribuir o custo.

**Acceptance Criteria:**

**Given** a migration
**Then** `departamentos` ganha `centralizado` e existem `rateio_regras (empresa_id, departamento_id, criterio, vigente_de)` e `rateio_percentuais (regra_id, filial, percentual)`, mantidas por `limpar_dados_teste` (são configuração)

**Given** um departamento centralizado
**When** o analista cria uma regra pelo critério **uso**
**Then** informa o percentual de cada filial, e a regra é recusada se a soma não for 100%

**Given** o critério **faturamento**
**When** a regra é criada
**Then** não pede percentuais: a distribuição será proporcional ao Faturamento Líquido de cada filial na competência (linha com o papel faturamento líquido, AD-20)

**Given** várias regras do mesmo departamento
**When** uma competência é consultada
**Then** vale a de maior `vigente_de` até essa competência

**Given** um perfil sem a capacidade "configurar rateio"
**When** tenta criar ou alterar uma regra
**Then** `403`
**And** cada alteração vai à trilha

### Story 7.2: Aplicar o rateio na competência (FR-15, AD-14, AD-8)

As a Controller,
I want aplicar o rateio de uma competência,
So that o custo dos departamentos centralizados seja distribuído entre as filiais.

**Acceptance Criteria:**

**Given** a migration
**Then** existe `rateio_aplicado (empresa_id, competencia, departamento_id, filial, valor, regra_id, aplicado_por, aplicado_em, vigente)`, escrita só pelo módulo `dre/rateio` e incluída em `limpar_dados_teste`

**Given** uma competência aberta com regras vigentes
**When** o Controller aplica
**Then** distribui o custo realizado de cada departamento centralizado entre as filiais pela regra, e a soma rateada é exatamente o valor do departamento

**Given** que a divisão deixa centavos
**When** o rateio é aplicado
**Then** o resto vai à filial de maior percentual (empate: menor código)

**Given** o rateio já aplicado numa competência aberta
**When** o Controller aplica de novo
**Then** substitui o anterior, sem duplicar

**Given** competência fechada
**When** o Controller tenta aplicar
**Then** `409` pela guarda de período (1.6)

**Given** um perfil sem "aplicar rateio"
**When** tenta aplicar
**Then** `403`
**And** a aplicação vai à trilha

### Story 7.3: Aba "Rateio" (FR-15)

As a analista,
I want ver o resultado do rateio por departamento e filial,
So that eu confira antes de fechar.

**Acceptance Criteria:**

**Given** um rateio aplicado
**When** abro a aba "Rateio"
**Then** lista, por departamento centralizado e filial, o percentual, o critério, o valor rateado e o total do departamento

**Given** a soma por departamento
**When** é exibida
**Then** é igual ao valor do departamento (conferência visível na tela)

**Given** nenhum rateio aplicado
**When** abro a aba
**Then** mostra "não aplicado" e as regras vigentes

**Given** um Gerente
**When** tenta abrir a aba
**Then** não acessa

### Story 7.4: O DRE por filial mostra o rateio (FR-15)

As a diretoria,
I want ver o DRE de cada filial com o rateio numa linha separada,
So that eu veja o custo real de cada unidade sem perder o resultado direto.

**Acceptance Criteria:**

**Given** um rateio aplicado
**When** abro o DRE de uma filial
**Then** mostra uma linha "Rateio" separada do realizado direto

**Given** o DRE do grupo
**When** é aberto com rateio aplicado
**Then** o total não muda: a soma da linha "Rateio" de todas as filiais é zero

**Given** nenhum rateio aplicado
**When** o DRE é aberto
**Then** a linha não aparece

**Given** competência fechada
**When** o DRE é aberto
**Then** lê o rateio do snapshot
**And** o Gerente não vê a linha de rateio de outros departamentos

### Story 7.5: Rateio no fechamento e na reabertura (AD-14, AD-17)

As a Controller,
I want que o rateio entre no fechamento e saia na reabertura,
So that um mês fechado nunca carregue um rateio defasado.

**Acceptance Criteria:**

**Given** regras vigentes e rateio não aplicado
**When** o Controller fecha
**Then** `409` listando os departamentos sem rateio aplicado

**Given** o rateio aplicado
**When** o Controller fecha
**Then** o snapshot por contas inclui o rateio

**Given** a reabertura (1.7)
**When** o Controller reabre
**Then** o rateio aplicado da competência fica invalidado (`vigente = false`) e precisa ser reaplicado antes de fechar de novo

**Given** nenhuma regra vigente
**When** o Controller fecha
**Then** o fechamento não pede rateio

## Epic 8: Go-live com dado real (trilha paralela)

A equipe do cliente passa a usar o sistema com dado real sabendo que há backup restaurável, que o Oracle cobre as contas do DRE e que não restou dívida de segurança.

**Nota:** este épico corre em paralelo aos demais e termina antes da virada para Produção. A história 8.2 deve começar já, porque depende de uma resposta do cliente. A 8.1 já tem parte feita no servidor em 27/10 (Auditoria na lista do script e restauração testada); falta confirmar a primeira execução automática. A 8.7 é opcional.

### Story 8.1: Backup diário da Auditoria, com restauração testada (NFR6, AR-15)

As a dono do produto,
I want que o banco da Auditoria entre no backup diário e que eu já tenha restaurado uma cópia,
So that um backup só valha se alguém provou que ele volta.

**Acceptance Criteria:**

**Given** o cron das 23:00 (UTC)
**When** a primeira execução automática com a Auditoria termina
**Then** `/var/log/pg-backup.log` mostra "Dump de fb_auditoria OK" e "Upload de fb_auditoria OK", e a pasta do dia no OneDrive tem o arquivo

**Given** o script do servidor
**When** é versionado
**Then** uma cópia sem e-mails nem senhas fica em `ops/` no repositório

**Given** um runbook `docs/runbook-backup.md`
**When** é lido
**Then** descreve como localizar o contêiner, gerar um dump, restaurar num banco temporário e conferir contagens

**Given** a restauração
**When** é repetida
**Then** acontece antes da virada para Produção e a cada trimestre, com data e resultado registrados

### Story 8.2: Autorização do cliente para o destino do backup (NFR6), começar já

As a dono do produto,
I want a decisão do cliente sobre onde o backup dos dados dele pode ficar,
So that o contrato seja respeitado.

**Acceptance Criteria:**

**Given** que o dump contém dados financeiros do cliente e hoje vai sem criptografia para o OneDrive do dono do produto
**When** a pergunta é enviada ao cliente
**Then** a data e o texto ficam registrados

**Given** a resposta do cliente
**When** chega
**Then** fica registrada uma de três decisões: aceita o destino atual; exige conta da empresa; exige criptografia

**Given** a exigência de criptografia
**When** é aplicada
**Then** o backup da Auditoria passa a usar `rclone crypt` só para este banco, com a chave guardada fora do servidor de backup, e a restauração é testada de novo

**Given** nenhuma decisão registrada
**When** a virada é tentada
**Then** não segue (item bloqueante da 8.4)

### Story 8.3: Cobertura da fonte: o que ainda depende de arquivo (P11)

As a dono do produto,
I want ver quais contas o Oracle já cobre e quais ainda dependem de arquivo,
So that eu saiba o que falta antes de virar para Produção.

**Acceptance Criteria:**

**Given** as contas com lançamento
**When** abro "Cobertura"
**Then** lista cada conta com a origem dona, a última competência carregada e se há lançamento do Oracle para ela

**Given** o resumo
**When** é exibido
**Then** mostra quantas contas e que parte do valor do realizado ainda depende de arquivo

**Given** a lista
**When** é comparada com o aviso da virada (1.3)
**Then** é a mesma
**And** é só leitura, para perfis sem restrição

### Story 8.4: Checklist antes de virar para Produção (NFR6, NFR7)

As a administrador de plataforma,
I want um checklist na tela de virar para Produção,
So that eu não vire por esquecimento.

**Acceptance Criteria:**

**Given** a tela de virada (1.3)
**When** o administrador a abre
**Then** vê cada item com o estado (verde, vermelho ou "confirmar") e quem confirmou e quando

**Given** os itens bloqueantes
**When** o administrador tenta virar
**Then** a virada só segue com todos verdes: cadastro público fechado (verificado pela configuração); SMTP com envio de teste concluído; paridade do mês de referência registrada como "bate" (3.7); decisão sobre o destino do backup registrada (8.2); backup confirmado e restauração testada nos últimos 30 dias (confirmação manual com autor e data)

**Given** a cobertura (8.3)
**When** o checklist é exibido
**Then** aparece como aviso, que não bloqueia

**Given** cada confirmação manual
**When** é feita
**Then** vai à trilha
**And** a virada continua exigindo a senha de novo e a frase digitada (1.3)

### Story 8.5: Imagens e versões sem validade vencida (AR-16)

As a dono do produto,
I want que as imagens do deploy tenham versão fixa e suportada,
So that um deploy não mude de comportamento sozinho nem use base sem correção de segurança.

**Acceptance Criteria:**

**Given** `frontend/Dockerfile`
**When** é revisado
**Then** o `node:18-alpine` (sem suporte desde 2025-04-30) é trocado por uma versão LTS suportada, e o build do frontend passa (`npm run typecheck`, `npm run lint`, `vite build`)

**Given** `alpine:latest` e `cloudflare/cloudflared:latest`
**When** são revisados
**Then** passam a usar tags fixas

**Given** `go.mod`
**When** `go mod tidy` roda
**Then** deixa o `go-ora` como dependência direta
**And** o deploy no Coolify é testado depois da troca, com o túnel de volta a "Healthy"

### Story 8.6: Apagar usuário sem quebrar a trilha (AR-16)

As a administrador de plataforma,
I want que apagar um usuário não falhe nem apague a história dele,
So that a trilha continue imutável.

**Acceptance Criteria:**

**Given** o `DeleteUserHandler`, que faz `DELETE FROM users`, e a chave `ON DELETE SET NULL` de `audit_log.user_id`
**When** o administrador apaga um usuário que tem registros na trilha
**Then** o comportamento atual é registrado em teste (o gatilho de imutabilidade bloqueia o `UPDATE` da chave)

**Given** a decisão de que usuário é desativado e nunca apagado (AD-19)
**When** a rota é chamada para quem tem registro
**Then** passa a recusar e oferece o bloqueio

**Given** um usuário sem nenhum registro
**When** o administrador o apaga
**Then** pode ser apagado
**And** a mensagem de recusa orienta para o Bloqueio de usuário

### Story 8.7: Alerta ao administrador quando a importação ou o envio falha (opcional)

As a administrador de plataforma,
I want ser avisado por e-mail quando uma importação ou um envio falha,
So that eu não descubra só quando alguém reclamar.

**Acceptance Criteria:**

**Given** uma execução com status de falha em `execucoes`
**When** termina
**Then** o administrador recebe um e-mail com a causa

**Given** um e-mail da outbox que esgotou as tentativas
**When** o worker desiste
**Then** o administrador é avisado, uma vez por item

**Given** o SMTP indisponível
**When** o alerta é tentado
**Then** não tenta de novo em laço, e a falha segue visível na tela
**And** é opcional e não bloqueia a virada

