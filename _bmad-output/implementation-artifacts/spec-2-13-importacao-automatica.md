---
title: 'Importação automática do banco do cliente (história 2.13)'
type: 'feature'
status: 'in-review'
---

## Intenção

O dono do produto quer rodar a importação manualmente quando a base do cliente for atualizada e depois ligar o agendamento (AD-15).

## O que foi feito

- `auditoria.agendamento_erp` (migração 019): configuração por empresa, desligada por padrão.
- Frequência diária, semanal ou mensal, hora de `America/Sao_Paulo`. Mensal relê o mês anterior; diária e semanal, o anterior e o corrente.
- Laço de um minuto no processo da API, só quando o banco do cliente está configurado. Um horário dispara uma vez; ligar não dispara horário que já passou; servidor parado no horário roda ao voltar.
- Mesmo advisory lock da importação manual; com outra em andamento, o horário fica pendente.
- Execução em `execucoes` e na trilha com `user_id` nulo e gatilho `agendador`.
- `GET`/`PUT /api/aud/importacao-erp/agendamento` (capacidade `carregar`) e bloco na tela de carga.

## Fora do escopo

- Orçado (2.6 não existe). Aviso por e-mail em falha. Nova tentativa automática após falha.

## Atenção

- A limpeza de dados de teste não desliga o agendamento.

## Ajuste (pedido do cliente)

- Opção "também no primeiro dia útil do mês" (migração 020): segunda a sexta, sem feriados; nesse disparo relê só o mês anterior.
