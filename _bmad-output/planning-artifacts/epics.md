---
status: draft
stepsCompleted: []
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
FR-4: Lançar Ajuste em Conta gerencial de Competência aberta, com valor e motivo obrigatório; avisar a Lista de aviso por e-mail; exigir Justificativa dos Gerentes afetados; o Ajuste é imutável.
FR-5: Manter a estrutura de Linhas do DRE (descrição, ordem, nível, tipo conta ou subtotal, linhas somadas).
FR-6: Manter o Mapa de contas (Conta gerencial para Linha do DRE com sinal, e para Departamento); a associação mais específica prevalece.
FR-7: Exibir o DRE mensal e acumulado, por Filial ou grupo, com subtotais exatos e percentual sobre o Faturamento Líquido; competência fechada usa os valores congelados.
FR-8: Comparar Realizado com Orçado e Ano anterior, em valor e percentual, por Linha do DRE e Conta gerencial, descendo até a Filial; "sem orçado" é indicado.
FR-9: Fechar a Competência (por Filial ou consolidada), recusando se houver conta sem mapeamento, só pelo Controller, com versão e trilha.
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
NFR10: Disponibilidade, desempenho e retenção da trilha **não estão definidos** no PRD; ficam como pergunta aberta.

### Additional Requirements

Brownfield, sem starter template: a Fase 1 existe em produção; o trabalho é incremental.

- AR-1 (AD-1, AD-2): migrar o fechamento (`dre.go`) e a importação CSV (`dre_import.go`) para o pacote `backend/dre`; nenhum handler escreve em lançamento, orçado, ajuste ou fechamento; um teste falha o build se outro pacote o fizer.
- AR-2 (AD-3, AD-16): `origem` em `lancamentos` e `orcado_conta`; reimportação por escopo; `conta_fonte` (uma conta, uma fonte); leitura vazia nunca apaga; `filial` vazia rejeitada.
- AR-3 (AD-5): `orcado_conta` como única fonte de orçado do DRE; escrita de `dre_orcado` removida.
- AR-4 (AD-6, AD-7): tabelas `ajustes` e `justificativas` append-only por trigger; chave canônica de desvio (`dre.ChaveDesvio`).
- AR-5 (AD-8): `dre.Periodo.Guardar` com `pg_advisory_xact_lock(dre.ChaveLock)` em toda escrita; isolamento READ COMMITTED explícito.
- AR-6 (AD-9, AD-17): `dre_fechamentos` com `reaberto_*`, `limite_desvio` e índice único parcial; Fechar dentro do lock via `dre.Realizado`; snapshot em `dre_fechamento_linhas` e novo `dre_fechamento_contas`, imutáveis.
- AR-7 (AD-10): `departamentos`, `conta_departamento`, `usuario_departamentos`; escopo fail closed na leitura e na escrita; `dre.FiltroEscopo`; prefixo em Go, nunca `LIKE`.
- AR-8 (AD-11): `perfis.go` com capacidades; middleware por capacidade; perfil `analista` no `CHECK`.
- AR-9 (AD-12): migração única de `users.areas` (`estrategico`→`empresa`, `administrativo`→`configuracoes`, `operacional`→`operacao`), novo `DEFAULT`, rotas e `navigation.ts` no mesmo release.
- AR-10 (AD-13): `outbox_email` genérica, worker com `SKIP LOCKED`, `lista_aviso` em `config_empresa`; e-mail do DRE só no fechamento consolidado.
- AR-11 (AD-14): `rateio_regras`, percentuais e `rateio_aplicado`; resto de centavos à maior filial; reabrir invalida.
- AR-12 (AD-15): agendador com `pg_try_advisory_lock` em `*sql.Conn` dedicada, `America/Sao_Paulo`, `auditoria.execucoes`, trilha com `user_id` nulo e origem agendador.
- AR-13: migrations `004+` aditivas e idempotentes, nunca tocam fechamentos; usuários desativados e não apagados.
- AR-14: `ALLOW_SELF_REGISTER=false` por padrão; `JC_EMPRESA_ID` e `JC_ORACLE_*` sem valores padrão no repositório (remover host padrão de compose, `.env.example` e `jc-probe`).
- AR-15: backup do Postgres da Auditoria com restauração testada. Verificado no servidor: o cron diário `pg-backup-gdrive.sh` (23:00 UTC, `pg_dump -Fc`, `rclone` para o OneDrive, retenção de 56 dias, alerta por e-mail) tem lista fixa de stacks e usa `-U postgres`; a Auditoria não está na lista e seu usuário é outro. Trabalho: incluir a Auditoria no script (usuário lido de `POSTGRES_USER`), versionar uma cópia sem segredos no repositório, documentar e executar o teste de restauração; confirmar com o cliente o destino dos dados no OneDrive.
- AR-16 (dívida, não bloqueia): `node:18-alpine`, tags fixas de `cloudflared` e `alpine`, `go mod tidy`; `DeleteUserHandler` versus o trigger de `audit_log`.

### UX Design Requirements

Não há documento de UX. As telas seguem os padrões de componentes e o `navigation.ts` existentes; o protótipo de painel do cliente é referência visual para FR-16 e FR-17.

### FR Coverage Map

{{requirements_coverage_map}}

## Epic List

{{epics_list}}
