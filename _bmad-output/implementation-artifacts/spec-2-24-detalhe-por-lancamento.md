---
title: 'Detalhe por lançamento do banco do cliente (história 2.24)'
type: 'feature'
status: 'in-review'
---

## Intenção

O cliente quer ver o lançamento (data, parceiro, nota, valor, filial) sem abrir o ERP.

## O que foi feito

- Migração 021: datas, parceiro e identificador em `lancamentos`.
- `JC_ORACLE_REALIZ_DETALHE=true` lê uma linha por lançamento; desligado, segue somando por filial, conta e dia.
- `JC_ORACLE_REALIZ_HISTORICO` (opcional) quando a view trouxer o histórico.
- Detalhe da conta mostra data, parceiro, nota; o gestor vê o nome só dos tipos em `DRE_GESTOR_TIPOS_PARCEIRO` (padrão F).
- `jc-probe -realizado` usa o objeto e a coluna de data configurados.

## Pendente

- Decisão do cliente sobre exibir nomes ao gestor (dado pessoal).
- Nunca testado contra a view real.
