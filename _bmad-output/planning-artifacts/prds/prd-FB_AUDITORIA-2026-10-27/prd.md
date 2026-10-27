---
title: "FB_AUDITORIA — Release A1: DRE gerencial"
status: final
created: 2026-10-27
updated: 2026-10-27
---

# PRD: FB_AUDITORIA — Release A1: DRE gerencial
*Título provisório — confirmar.*

## 0. Document Purpose

Este PRD define a Release A1 do FB_AUDITORIA: o DRE gerencial automatizado de um grupo de distribuição (uma empresa, várias filiais), que substitui o controle em Excel. Serve ao dono do produto, aos gestores do cliente que validam a entrega e aos fluxos seguintes do BMAD (UX, arquitetura, épicos). O vocabulário está ancorado no Glossário (§3); os requisitos funcionais (FR) ficam aninhados nas funcionalidades (§4), com numeração global; suposições aparecem marcadas inline como `[ASSUMPTION]` e indexadas em §9.

O repositório é público. Este documento descreve estrutura e regras, e **nunca** contrato, valores comerciais, CNPJ ou números reais do cliente.

A Fase 1 do produto já existe em código (DRE por filial com orçado e ano anterior, fechamento versionado, mapa conta→linha, trilha imutável, carga por CSV). Esta release a leva ao uso real: ingestão do banco do cliente, visão por departamento, justificativas, envio por e-mail, rateio e dashboards. Ficam de fora a gestão de verbas (conta-corrente por fornecedor e filial), o comercial e a operação (releases seguintes); as contas de verba continuam fazendo parte do DRE.

## 1. Vision

Hoje o DRE gerencial do cliente é uma planilha Excel pesada, alimentada por macro, com ajustes manuais e uma aba onde cada gerente anota por que seu departamento fugiu do orçado. O resultado depende de quem monta a planilha e de quando ela é enviada.

O FB_AUDITORIA entrega o mesmo DRE como sistema: os números vêm do ERP do cliente, a estrutura do DRE e o mapa de contas são dados editáveis pela controladoria, o mês é fechado e congelado, e cada gerente recebe o resultado do seu departamento e justifica os desvios dentro da ferramenta. A ideia é a mesma do Farol: o sistema vai atrás do gestor, e não o contrário.

O que importa para quem decide é a confiança: qualquer número tem origem rastreável, qualquer ajuste manual deixa registro e exige justificativa, e um mês fechado não muda em silêncio.

## 2. Target User

### 2.1 Jobs To Be Done
- **Analista de controladoria:** fechar o mês sem refazer planilha; carregar os dados, conferir e lançar ajustes com registro de quem, quando e por quê.
- **Controller:** aprovar e fechar o mês com a certeza de que nenhuma conta ficou sem mapeamento e de que o fechamento não será alterado depois.
- **Gerente de departamento:** entender rapidamente onde seu departamento fugiu do orçado e registrar a justificativa sem depender de planilha por e-mail.
- **Diretoria:** ver o resultado do grupo, por filial e por departamento, e onde estão os desvios que merecem atenção.

### 2.2 Non-Users (v1)
- Colaboradores sem função de gestão (a visão colaborador pertence às frentes de operação, em releases futuras).
- Auditoria externa e contabilidade externa.

### 2.3 Key User Journeys

- **UJ-1. Rafael carrega o mês e lança um ajuste.**
  - **Persona + context:** Rafael, analista de controladoria, prepara o fechamento de setembro.
  - **Entry state:** autenticado com perfil analista; o mês de setembro está aberto.
  - **Path:** confere a carga do realizado vinda do banco do cliente; carrega o arquivo de contas gerenciais; vê que uma conta ficou sem mapeamento e a mapeia; lança um ajuste numa conta de despesa, com motivo.
  - **Climax:** o ajuste aparece no DRE do departamento afetado e o sistema avisa por e-mail a **Lista de aviso**.
  - **Resolution:** o ajuste fica pendente de justificativa do gerente; Rafael acompanha o status.
  - **Edge case:** se o mês já estiver fechado, o sistema recusa a carga e o ajuste e informa que é preciso reabrir o fechamento.

- **UJ-2. Helena fecha o mês.**
  - **Persona + context:** Helena, Controller, responsável pelo fechamento mensal.
  - **Entry state:** autenticada com perfil Controller; carga e ajustes de setembro concluídos.
  - **Path:** abre o DRE do mês; confere totais por linha e por filial contra o orçado; vê se há contas sem mapeamento; fecha o mês.
  - **Climax:** o fechamento congela o realizado numa versão numerada e dispara o envio do DRE aos gerentes.
  - **Resolution:** os números apresentados não mudam mais; uma reabertura cria nova versão e preserva a anterior.
  - **Edge case:** com conta sem mapeamento, o sistema recusa o fechamento e lista as contas.

- **UJ-3. Paulo justifica o desvio do seu departamento.**
  - **Persona + context:** Paulo, gerente de Logística, recebe o DRE do departamento por e-mail.
  - **Entry state:** autenticado com perfil gerente; escopo restrito ao departamento de Logística.
  - **Path:** abre o link do e-mail; vê as contas do departamento com realizado, orçado, ano anterior e variação; seleciona as contas com desvio acima do limite; escreve a justificativa de cada uma.
  - **Climax:** as contas deixam de constar como pendentes e a justificativa fica visível para a controladoria.
  - **Resolution:** Helena e a diretoria enxergam o desvio já com a explicação do gerente.
  - **Edge case:** se Paulo tentar abrir outro departamento, o sistema nega o acesso.

- **UJ-4. Sônia acompanha os desvios do grupo.**
  - **Persona + context:** Sônia, diretoria, quer saber onde o resultado fugiu do orçado.
  - **Entry state:** autenticada com perfil diretoria (somente leitura).
  - **Path:** abre o dashboard; filtra por ano e filial; vê os maiores desvios por departamento e por gerente; desce até as contas.
  - **Climax:** identifica um desvio relevante e vê a justificativa já registrada, ou vê que ainda está pendente.
  - **Resolution:** decide se cobra o gerente; nada é editável nesta visão.

## 3. Glossary

*Os termos abaixo valem em todo o documento; sem sinônimos.*

- **Empresa** — o grupo cliente, único inquilino (`empresa_id`) desta release. Tem várias **Filiais**.
- **Filial** — unidade da Empresa (código `CODFILIAL`). Atributo do **Lançamento**, nunca um inquilino separado.
- **Competência** — mês de referência de um **Lançamento** (primeiro dia do mês).
- **Conta gerencial** — código de conta do plano gerencial do cliente (numérico), ao qual um **Lançamento** pertence.
- **Lançamento** — valor de uma **Conta gerencial**, numa **Filial** e numa **Competência**.
- **Realizado** — valor efetivamente apurado; vem do banco do cliente ou de carga por arquivo.
- **Orçado** — valor planejado para a **Conta gerencial**, **Filial** e **Competência**.
- **Ano anterior** — o **Realizado** da mesma **Competência** um ano antes.
- **Linha do DRE** — linha da estrutura do DRE, que pode somar **Contas gerenciais** (tipo conta) ou outras linhas (tipo subtotal).
- **Mapa de contas** — associação configurável de **Conta gerencial** a **Linha do DRE** (com sinal) e a **Departamento**.
- **Departamento** — área de gestão que responde por um conjunto de **Contas gerenciais** (por exemplo, Compras, Contabilidade) e pertence a um grupo (Comercial, Logística ou Administrativo).
- **Gerente** — usuário responsável por um **Departamento**.
- **Ajuste** — correção manual ao **Realizado** de uma **Conta gerencial**, sempre com motivo.
- **Desvio** — diferença entre o **Realizado** e o **Orçado** (e também contra o **Ano anterior**) de uma **Conta gerencial** num **Departamento**.
- **Justificativa** — texto do **Gerente** que explica um **Desvio** ou um **Ajuste**.
- **Fechamento** — congelamento versionado do **Realizado** de uma **Competência**.
- **Rateio** — distribuição das despesas de **Departamentos** centralizados entre as **Filiais**, por critério definido.
- **Trilha de auditoria** — registro imutável de toda escrita relevante (quem, quando, o quê).
- **Perfil** — papel do usuário: analista, Controller, **Gerente** ou diretoria.
- **Lista de aviso** — grupo de e-mail que recebe o aviso de cada **Ajuste**.
- **Ambiente** — marcador da **Empresa**: Teste (os dados podem ser apagados) ou Produção (nada pode ser apagado); a virada de Teste para Produção é de mão única.
- **Bloqueio de usuário** — impede um usuário de entrar e de usar o sistema sem apagá-lo; é feito e desfeito por quem gere usuários, sempre com motivo.

## 4. Features

### 4.1 Ingestão do Realizado e do Orçado

**Description:** O **Realizado** consolidado (faturamento, devoluções, CMV de devolução, ST, desconto de custo) vem de uma view do banco do cliente, apenas por leitura. O **Orçado** vem de outra view, por **Conta gerencial** e **Filial**, com uma coluna por mês, que o sistema transforma em um **Lançamento** por **Competência**. O **Realizado** por **Conta gerencial** (verbas, despesas, receitas, folha, impostos) entra, nesta release, por arquivo CSV; a troca para o banco do cliente acontece quando a fonte for confirmada. Realiza UJ-1.

**Functional Requirements:**

#### FR-1: Importar o Realizado consolidado do banco do cliente

O sistema importa, por **Competência** e **Filial**, os valores da view de realizado consolidado, sem escrever no banco do cliente. Realiza UJ-1.

**Consequences (testable):**
- Reimportar a mesma **Competência** substitui os valores anteriores em vez de duplicá-los.
- A importação é recusada para **Competência** com **Fechamento** ativo.
- O campo de mês da view (texto `MM/AAAA`, formato confirmado na leitura da view) é convertido para **Competência**; valor fora do formato é rejeitado com mensagem que identifica a linha.
- A view entrega o **Realizado** por **Filial** e UF; o **Realizado** de uma **Filial** é a soma das suas UFs. `[ASSUMPTION: a UF é a do destino da venda e não é dimensão do DRE nesta release]`
- O volume da view é pequeno (centenas de linhas), então a importação pode reler o histórico inteiro, preservando as **Competências** com **Fechamento** ativo.
- Cada importação grava na **Trilha de auditoria**: quem iniciou, intervalo importado e quantidade de linhas.

#### FR-2: Importar o Orçado do banco do cliente

O sistema importa o **Orçado** por ano, **Filial** e **Conta gerencial**, convertendo as 12 colunas de mês em um **Lançamento** de **Orçado** por **Competência**.

**Consequences (testable):**
- Uma linha de ano, filial e conta com 12 valores vira 12 registros de **Orçado**, um por mês.
- Valores nulos num mês são tratados como ausentes, e não como zero: o DRE mostra "sem orçado" e não calcula a variação.
- Reimportar o ano substitui o **Orçado** anterior daquele ano, exceto em **Competência** com **Fechamento** ativo.

#### FR-3: Carregar Realizado por Conta gerencial por arquivo CSV (provisório)

O analista carrega um arquivo CSV com o **Realizado** por **Conta gerencial**, **Filial** e **Competência**, enquanto a fonte no banco do cliente não está confirmada. Realiza UJ-1.

**Consequences (testable):**
- O arquivo é validado antes da gravação; linhas inválidas são listadas e nada é gravado se houver erro bloqueante.
- A carga é recusada para **Competência** com **Fechamento** ativo.
- Cada carga grava na **Trilha de auditoria** e fica identificada como de origem arquivo.

**Out of Scope:** automatizar a leitura da planilha do cliente; o formato do CSV é o já aceito pelo importador existente, ampliado conforme necessário.

#### FR-4: Lançar Ajuste com motivo, aviso e Justificativa

O analista lança um **Ajuste** em uma **Conta gerencial** de uma **Competência** aberta, informando valor e motivo. O sistema avisa por e-mail a **Lista de aviso** e exige a **Justificativa** dos **Gerentes** dos **Departamentos** afetados. Realiza UJ-1.

**Consequences (testable):**
- Sem motivo, o **Ajuste** não é gravado.
- O **Ajuste** é registrado na **Trilha de auditoria** com valor anterior, valor novo, autor e momento, e não pode ser editado nem apagado depois (correção é um novo **Ajuste**).
- O **Realizado** exibido no DRE é o valor carregado mais os **Ajustes**, e a tela de detalhe mostra os dois.
- O **Ajuste** consta como pendente até o **Gerente** registrar a **Justificativa**.
- O **Ajuste** é recusado em **Competência** com **Fechamento** ativo.

**Notes:** `[NOTE FOR PM]` confirmar quem pode lançar **Ajuste** além do analista, e se o aviso por e-mail vai para os próprios **Gerentes** afetados ou para uma lista fixa.

### 4.2 Estrutura do DRE e Mapa de contas

**Description:** A estrutura do DRE e o **Mapa de contas** são dados editáveis pela controladoria, sem publicar nova versão do sistema. A estrutura inicial reproduz o DRE do cliente: Faturamento Líquido; CMV, Deduções e Verbas; Margem Composta; Impostos sobre Faturamento Líquido; Margem de Contribuição; departamentos (Comercial, Logística, Administrativo) e Total de Despesas; Margem Líquida; Outras Receitas e Despesas; Resultado antes da tributação; IRPJ. Realiza UJ-1.

**Functional Requirements:**

#### FR-5: Manter a estrutura de Linhas do DRE

O analista edita as **Linhas do DRE** (descrição, ordem, nível, tipo conta ou subtotal e as linhas somadas). Realiza UJ-1.

**Consequences (testable):**
- Uma linha subtotal soma exatamente as linhas listadas; referência a linha inexistente é recusada.
- Mudanças na estrutura não alteram **Competências** com **Fechamento** ativo.
- Toda alteração grava na **Trilha de auditoria**.

#### FR-6: Manter o Mapa de contas

O analista associa **Conta gerencial** a **Linha do DRE** (com sinal) e a **Departamento**. A associação mais específica prevalece sobre uma por prefixo.

**Consequences (testable):**
- **Conta gerencial** com **Lançamento** e sem **Linha do DRE** é listada como "sem mapeamento".
- **Conta gerencial** de um **Departamento** só é visível ao **Gerente** daquele **Departamento**.
- Toda alteração grava na **Trilha de auditoria**.

**Notes:** `[NOTE FOR PM]` o dicionário de contas (descrição, grupo, departamento) ainda não tem fonte confirmada; até lá, é cadastrado por arquivo.

#### FR-20: Carregar o dicionário de contas por arquivo

O analista carrega um arquivo com **Conta gerencial**, descrição, grupo e **Departamento**, enquanto o dicionário não tem fonte no banco do cliente. Realiza UJ-1.

**Consequences (testable):**
- Linhas com **Conta gerencial** repetida ou **Departamento** inexistente são listadas, e nada é gravado se houver erro bloqueante.
- Reenviar o arquivo atualiza descrição, grupo e **Departamento** sem apagar a associação a **Linha do DRE** do **Mapa de contas**.
- A carga grava na **Trilha de auditoria**.

### 4.3 Visões do DRE

**Description:** O DRE é apresentado por **Competência** (mensal) e acumulado do ano, por **Filial**, consolidado do grupo e por **Departamento**, comparando **Realizado**, **Orçado** e **Ano anterior**, com variação em valor e em percentual e percentual sobre o Faturamento Líquido. O detalhe desce da **Linha do DRE** até a **Conta gerencial** e a **Filial**. O detalhe até o documento de origem não existe com as fontes atuais, que chegam agregadas por **Conta gerencial**, **Filial** e mês; passaria a existir se o cliente expusesse uma fonte no nível do **Lançamento** (pergunta aberta 11). Realiza UJ-2, UJ-3, UJ-4.

**Functional Requirements:**

#### FR-7: Exibir o DRE mensal e acumulado

O usuário autorizado vê o DRE de uma **Competência** ou do acumulado do ano, para uma **Filial** ou para o grupo.

**Consequences (testable):**
- Cada subtotal é igual à soma das linhas que o compõem.
- Para uma **Competência** com **Fechamento** ativo, o DRE usa os valores congelados.
- O percentual sobre Faturamento Líquido usa a linha de Faturamento Líquido como base.

#### FR-8: Comparar com Orçado e Ano anterior e detalhar

O usuário vê, por **Linha do DRE** e **Conta gerencial**, **Realizado**, **Orçado**, **Ano anterior** e a variação, e desce até a **Filial**.

**Consequences (testable):**
- A variação é Realizado menos Orçado, em valor e em percentual; com Orçado zero, o percentual aparece como não calculável.
- **Filial** ou **Conta gerencial** sem **Orçado** mostra o **Realizado** com a indicação "sem orçado" e variação não calculável.
- O detalhe mostra a origem de cada valor (banco do cliente, arquivo ou **Ajuste**).

### 4.4 Fechamento mensal

**Description:** O Controller fecha uma **Competência**. O fechamento congela o **Realizado** numa versão numerada. Reabrir cria uma nova versão e preserva a anterior. Realiza UJ-2.

**Functional Requirements:**

#### FR-9: Fechar a Competência

O Controller fecha a **Competência** do grupo inteiro (fechamento sempre consolidado).

**Consequences (testable):**
- O fechamento é recusado se houver **Conta gerencial** com **Lançamento** sem **Linha do DRE**, e o sistema lista as contas.
- O fechamento é recusado se o usuário não for Controller.
- O fechamento grava as linhas do DRE congeladas e a versão, e registra na **Trilha de auditoria**.

#### FR-10: Reabrir o Fechamento

O Controller reabre uma **Competência** fechada informando o motivo; o sistema cria nova versão e mantém as anteriores legíveis.

**Consequences (testable):**
- Sem motivo, a reabertura é recusada.
- A versão anterior continua acessível e inalterada.
- Enquanto reaberta, carga e **Ajuste** voltam a ser permitidos para aquela **Competência**.

#### FR-11: Bloquear escrita em Competência fechada

Carga, importação e **Ajuste** são recusados em **Competência** com **Fechamento** ativo, de modo que o drill-down não mostre dados posteriores ao fechamento.

**Consequences (testable):**
- Toda escrita de realizado em **Competência** fechada retorna recusa com a mensagem de que é preciso reabrir o **Fechamento**.

### 4.5 Justificativa de Desvios

**Description:** O **Gerente** vê as contas do seu **Departamento** e registra a **Justificativa** dos **Desvios** acima de um limite definido pela controladoria. A controladoria e a diretoria enxergam o status de cada uma (pendente ou justificada). Realiza UJ-3, UJ-4.

**Functional Requirements:**

#### FR-12: Registrar Justificativa

O **Gerente** registra a **Justificativa** de um **Desvio** ou de um **Ajuste** do seu **Departamento**.

**Consequences (testable):**
- O **Gerente** só vê e só justifica contas do seu **Departamento**.
- Cada **Justificativa** guarda autor, momento e texto, e é registrada na **Trilha de auditoria**; editar cria novo registro e preserva o anterior.
- O dashboard mostra, por **Departamento**, quantas contas estão pendentes e quantas justificadas.

#### FR-13: Definir o limite de Desvio

O Controller define o limite a partir do qual um **Desvio** exige **Justificativa** (em percentual e/ou valor). Realiza UJ-2 e UJ-3. `[ASSUMPTION: limite global, ajustável por Controller; sem limite por departamento nesta release]`

**Consequences (testable):**
- Alterar o limite não reclassifica retroativamente **Competências** com **Fechamento** ativo.

### 4.6 Envio do DRE aos Gerentes

**Description:** O sistema envia a cada **Gerente**, por e-mail, o DRE do seu **Departamento**, com link para justificar. Realiza UJ-3.

**Functional Requirements:**

#### FR-14: Enviar o DRE do Departamento por e-mail

O sistema envia o e-mail a cada **Gerente** com o resumo do **Departamento** e o link da tela de **Justificativa**.

**Consequences (testable):**
- Cada **Gerente** recebe apenas o seu **Departamento**.
- O envio é disparado pelo **Fechamento** da **Competência** (ver pergunta aberta 4).
- Em **Ambiente** de Teste o e-mail não vai aos **Gerentes** (ver FR-21).
- O envio é registrado na **Trilha de auditoria** (destinatário, momento, **Competência**) e falhas de envio são visíveis ao Controller.
- O e-mail não contém o DRE completo do grupo.

**Notes:** `[NOTE FOR PM]` ainda a confirmar com o cliente: a data de fechamento habitual e se o envio é automático na data ou só depois que o Controller fecha. Até lá, o envio acontece após o **Fechamento**. `[ASSUMPTION]`

### 4.7 Rateio de despesas centralizadas

**Description:** As despesas de **Departamentos** centralizados são distribuídas entre as **Filiais** por um critério (uso ou faturamento), para que o DRE por **Filial** reflita o custo real de cada unidade.

**Functional Requirements:**

#### FR-15: Configurar e aplicar o Rateio

O analista define, para cada **Departamento** centralizado, o critério de **Rateio** (percentual por uso informado, ou proporcional ao faturamento da **Filial**). O Controller o aplica na **Competência**. Realiza UJ-2.

**Consequences (testable):**
- A soma dos valores rateados é igual ao valor do **Departamento** centralizado; o sistema recusa percentuais que não somam 100%.
- O DRE por **Filial** mostra a linha de **Rateio** separada do **Realizado** direto.
- O **Rateio** respeita o **Fechamento** (não muda em **Competência** fechada).

**Notes:** `[NOTE FOR PM]` ainda a confirmar com o cliente: quais **Departamentos** são centralizados e se o critério é fixo ou muda por mês.

### 4.8 Dashboards de desvios

**Description:** Os dashboards mostram os principais **Desvios** por ano, **Departamento**, **Gerente** e **Filial**, comparando **Orçado** e **Realizado**, com o status das **Justificativas**. Realiza UJ-4.

**Functional Requirements:**

#### FR-16: Exibir os principais Desvios

O usuário autorizado vê o ranking dos maiores **Desvios** e filtra por ano, **Filial**, **Departamento** e **Gerente**.

**Consequences (testable):**
- O **Gerente** vê apenas o seu **Departamento**; a diretoria e a controladoria veem todos.
- Cada **Desvio** mostra o status da **Justificativa**.

#### FR-17: Exibir tendência, ponte do resultado e projeção do ano

O usuário autorizado vê a despesa sobre o Faturamento Líquido por ano e se está subindo, a ponte que explica a diferença entre **Orçado** (ou **Ano anterior**) e **Realizado**, e a projeção de fechamento do ano. Referência visual: o protótipo de painel que o cliente já usa. Realiza UJ-4.

**Consequences (testable):**
- A ponte do resultado fecha: a soma das contribuições é igual à diferença total entre os dois pontos comparados.
- A projeção do ano indica quais meses são **Realizado** e quais são estimados, e o método de estimativa. `[ASSUMPTION: estimativa = realizado acumulado mais orçado dos meses restantes]`
- O **Gerente** vê esses indicadores apenas para o seu **Departamento**.

### 4.9 Perfis e acesso

**Description:** Os perfis são: analista (carrega, ajusta, edita mapa), Controller (aprova e fecha), **Gerente** (justifica o seu **Departamento**, podendo responder por mais de um `[ASSUMPTION]`) e diretoria (somente leitura). O backend valida perfil e escopo em toda rota; a interface só reflete.

**Functional Requirements:**

#### FR-18: Controlar acesso por perfil e por Departamento

Cada rota valida o perfil e, para o **Gerente**, o **Departamento**.

**Consequences (testable):**
- Acesso a recurso fora do perfil ou do escopo retorna negação, mesmo chamando a API diretamente.
- Diretoria não consegue escrever em nenhuma rota.

#### FR-19: Restringir o cadastro de usuários

Usuários são criados apenas por administrador; o cadastro público é desativado. Sem jornada própria: requisito de segurança.

**Consequences (testable):**
- A rota pública de cadastro responde recusa quando o cadastro aberto está desligado (padrão).
- A criação de usuário e a mudança de perfil gravam na **Trilha de auditoria**.

#### FR-22: Bloquear e desbloquear usuário

Quem gere usuários bloqueia e desbloqueia um usuário, sempre com motivo. O sistema não expira usuários por tempo: o acesso só termina por **Bloqueio de usuário**. Sem jornada própria: requisito de segurança.

**Consequences (testable):**
- Usuário bloqueado não consegue entrar nem renovar a sessão, e uma sessão aberta deixa de funcionar na requisição seguinte.
- Bloquear e desbloquear exigem motivo e gravam na **Trilha de auditoria**.
- Ninguém bloqueia a si mesmo, e o último administrador de plataforma ativo não pode ser bloqueado.
- Nenhum usuário perde acesso por passagem de tempo: a expiração de teste por usuário deixa de existir.

**Feature-specific NFRs:**
- Contas de folha de pagamento são sensíveis: `[NOTE FOR PM]` definir se o **Gerente** vê o detalhe por conta ou só o total do seu **Departamento**.

### 4.10 Ambiente de Teste e Produção

**Description:** Enquanto o cliente valida o sistema com arquivos e planilhas, o **Ambiente** da **Empresa** é Teste e os dados podem ser apagados. Quando o sistema passa a usar o banco do cliente de verdade, o **Ambiente** vira Produção e nada mais pode ser apagado. A virada é de mão única.

**Functional Requirements:**

#### FR-21: Marcar o Ambiente como Teste ou Produção

O administrador de plataforma define o **Ambiente** da **Empresa**. Em Teste ele pode apagar os dados de domínio; em Produção nenhum dado de domínio pode ser apagado. Sem jornada própria: requisito de segurança operacional.

**Consequences (testable):**
- A **Empresa** nasce em Teste; a virada para Produção é de mão única, e voltar para Teste é recusado.
- Apagar e virar exigem que o administrador digite a senha de novo, mesmo com a sessão aberta, e ambos gravam na **Trilha de auditoria**.
- Apagar remove lançamentos, **Orçado**, **Ajustes**, **Justificativas**, **Fechamentos** e envios, e mantém a estrutura do DRE, o **Mapa de contas**, o dicionário de contas e os usuários; a **Trilha de auditoria** nunca é apagada. `[ASSUMPTION]`
- Em Produção, apagar é recusado pelo banco, inclusive por chamada direta à API.
- Toda tela mostra a faixa "AMBIENTE DE TESTE" enquanto o **Ambiente** for Teste.
- Em Teste, o e-mail do DRE e o aviso de **Ajuste** vão só ao administrador (ou ficam apenas registrados), nunca aos **Gerentes**.
- A virada avisa, sem bloquear, quando alguma **Conta gerencial** com **Lançamento** de origem arquivo ainda não tem fonte no banco do cliente, e lista essas contas.

## 5. Non-Goals (Explicit)

- Não é contabilidade fiscal nem substitui o ERP; o sistema só lê dele.
- Não escreve no banco do cliente.
- Não replica o Excel célula a célula: reproduz o resultado e as visões, não a mecânica da planilha.
- Não cobre a gestão de verbas (conta-corrente por fornecedor e filial), nem comercial, fretes, produtividade ou diárias nesta release; as contas de verba aparecem no DRE como qualquer **Conta gerencial**.
- Não oferece detalhe até o documento de origem enquanto a fonte for agregada por mês.
- Não inclui exportação para PDF/Excel nesta release `[NON-GOAL for MVP]`.
- Não há expiração de senha nem de acesso por tempo: o acesso termina por Bloqueio de usuário.

## 6. MVP Scope

### 6.1 In Scope
- Importação do Realizado consolidado e do Orçado a partir do banco do cliente.
- Carga provisória do Realizado por Conta gerencial por CSV.
- Ajustes manuais com motivo, aviso por e-mail e Justificativa.
- Estrutura do DRE e Mapa de contas editáveis.
- DRE mensal e acumulado, por Filial e por Departamento, com Orçado e Ano anterior.
- Fechamento mensal versionado, com bloqueio de escrita em mês fechado.
- Justificativas dos Gerentes, limite de Desvio e envio do DRE por e-mail.
- Rateio de despesas centralizadas e dashboards de desvios.
- Perfis (analista, Controller, Gerente, diretoria) e cadastro de usuários restrito.
- Ambiente de Teste e Produção, com apagar em Teste e virada de mão única.
- Bloqueio de usuário no lugar de expiração por tempo.

### 6.2 Out of Scope for MVP
- Realizado por Conta gerencial direto do banco do cliente: depende de fonte ainda não confirmada; entra quando houver. `[NOTE FOR PM]`
- Exportação para PDF e Excel.
- Visão do colaborador (pertence às frentes de operação).
- Limites de Desvio por Departamento.
- Perfil de TI para administrar o ambiente da empresa (criar e bloquear usuários), previsto para depois.

### 6.3 Sequência de entrega

- **Núcleo** (não depende de pergunta aberta): FR-1 a FR-3, FR-5 a FR-12, FR-16, FR-18 a FR-22. O FR-19 (cadastro fechado) sai antes de todos, porque o site já é público.
- **Com padrão provisório até o cliente responder:** FR-4 (aviso à **Lista de aviso**), FR-13 (limite configurável, sem valor inicial) e FR-14 (envio só após o **Fechamento**).
- **Só começa com a resposta do cliente:** FR-15 (**Rateio**), que depende das perguntas abertas 4 e 5.
- **Depois do núcleo:** FR-17 (tendência, ponte e projeção).

### 6.4 Situação em relação à Fase 1

*Marcação feita a partir da descrição da Fase 1; a arquitetura confirma contra o código.*

| Funcionalidade | Situação |
|---|---|
| 4.1 Ingestão | Carga por CSV **existe**; importação do banco do cliente e **Ajuste** são **novos** |
| 4.2 Estrutura e **Mapa de contas** | **Existe**; **ampliar** com **Departamento** e carga do dicionário |
| 4.3 Visões do DRE | **Existe** (mensal, acumulada, por **Filial**, **Orçado**, **Ano anterior**); **ampliar** com **Departamento** e percentual sobre **Faturamento Líquido** |
| 4.4 **Fechamento** | **Existe** (versionado, recusa conta sem mapeamento); **ampliar** com bloqueio de escrita e motivo de reabertura; fechamento só consolidado |
| 4.5 a 4.8 | **Novos** |
| 4.9 Perfis e acesso | **Existe** (perfil e áreas validados no backend); **ampliar** com **Gerente** por **Departamento** e cadastro restrito |

## 7. Success Metrics

**Primary**
- **SM-1**: Paridade com a planilha atual — para um mês de referência, o DRE do sistema bate com o da planilha em todas as linhas de subtotal. Validates FR-1, FR-2, FR-3, FR-7.
- **SM-2**: Fechamento sem planilha — o fechamento de um mês é concluído dentro do sistema, sem consolidação em Excel. Validates FR-9, FR-11.
- **SM-3**: Cobertura de justificativas — todos os Desvios acima do limite têm Justificativa dentro do prazo acordado com o cliente `[ASSUMPTION: prazo a definir]`. Validates FR-12, FR-13, FR-14, FR-16.

**Secondary**
- **SM-4**: Rastreabilidade — 100% dos Ajustes têm motivo, autor e Justificativa registrados. Validates FR-4, FR-12.
- **SM-5**: Acesso por perfil — os testes negativos por Perfil e por Departamento retornam negação em 100% dos casos. Validates FR-18, FR-19.
- **SM-6**: Mapa completo — 100% das Contas gerenciais com Lançamento têm Linha do DRE no Fechamento. Validates FR-5, FR-6, FR-9, FR-20.
- **SM-7**: Rateio íntegro — a soma rateada é igual ao valor do Departamento centralizado em 100% das Competências fechadas. Validates FR-15.

**Counter-metrics (do not optimize)**
- **SM-C1**: Quantidade de Justificativas escritas — não otimizar o volume, para não incentivar textos vazios; o que vale é a explicação do Desvio. Counterbalances SM-3.
- **SM-C2**: Quantidade de reaberturas de Fechamento — não deve ser escondida nem evitada à força; uma reabertura legítima precisa ser possível e visível. Counterbalances SM-2.

## 8. Open Questions

1. **Fonte do Realizado por Conta gerencial** (incluindo CMV e Outros Faturamentos): existe view no banco do cliente? Dono: responsável de dados do cliente.
2. **Dicionário de contas** (descrição, grupo, Departamento): de onde vem? Dono: responsável de dados do cliente.
3. **Significado de `AC` e `GRUPO`** nas bases do cliente. Dono: responsável de dados do cliente.
4. **Fechamento:** data habitual e se o envio do DRE é automático na data ou só após o Controller fechar. Dono: o dono do produto, com o cliente.
5. **Rateio:** quais Departamentos são centralizados e se o critério é fixo ou muda por mês. Dono: o dono do produto, com o cliente.
6. **Quem lança Ajuste** além do analista, e se o aviso vai para os Gerentes afetados ou para uma lista fixa. Dono: dono do produto, com a área de negócio do cliente.
7. **Folha de pagamento:** o Gerente vê o detalhe por conta ou só o total do Departamento? Dono: dono do produto, com a área de negócio do cliente.
8. **Limite de Desvio:** valor ou percentual inicial. Dono: dono do produto, com a área de negócio do cliente.
9. **UF na view de realizado:** o mesmo código de **Filial** aparece em várias UFs (a mesma filial em vários estados). A UF é a do destino da venda ou a da filial? Dono: responsável de dados do cliente.
10. **Filiais sem Orçado:** algumas filiais da view de realizado não aparecem na view de orçamento. Elas não têm orçamento, ou ele vem de outra fonte? Dono: responsável de dados do cliente.
11. **Detalhe até o documento:** o ERP do cliente tem os documentos por trás de cada conta (notas, títulos, lançamentos); o cliente consegue expor uma view nesse nível? Sem ela, o detalhe para em **Conta gerencial** e **Filial**. Dono: responsável de dados do cliente.

## 9. Assumptions Index

- §4.1 FR-1 — a UF do realizado consolidado é a do destino da venda e não é dimensão do DRE nesta release.
- §4.5 FR-13 — o limite de Desvio é global, ajustável pelo Controller.
- §4.8 FR-17 — a projeção do ano usa o realizado acumulado mais o orçado dos meses restantes.
- §4.9 FR-18 — um Gerente pode responder por mais de um Departamento.
- §4.10 FR-21 — apagar em Teste mantém estrutura, Mapa, dicionário e usuários e nunca toca a Trilha de auditoria.
- §4.6 FR-14 — até confirmar a data de fechamento, o e-mail só sai após o Fechamento.
- §7 SM-3 — o prazo de justificativa será definido com o cliente.
