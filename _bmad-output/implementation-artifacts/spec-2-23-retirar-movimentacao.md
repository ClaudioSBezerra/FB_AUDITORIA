---
title: 'Retirar a movimentação carregada (história 2.23)'
type: 'feature'
status: 'in-review'
---

## Intenção

Receber a carga completa do banco do cliente sem a movimentação antiga carregada por arquivo, mantendo os cadastros.

## O que foi feito

- `dre.Ingestor.PreverRetirada` e `Retirar`: um mês, ou tudo (só em ambiente de teste), com `Periodo.Guardar`.
- Libera `conta_fonte` das contas sem lançamento daquela origem.
- Mesmo lock da importação do banco do cliente; trilha na mesma transação.
- `GET`/`POST /api/aud/movimentacao/retirada` (capacidade `carregar`) e bloco na tela de carga.

## Fora do escopo

- Orçado, planilha do comparativo, registro de cargas e de execuções, fechamentos e cadastros.

## Atenção

- Com o agendamento ligado, a próxima execução pode trazer meses de volta.
