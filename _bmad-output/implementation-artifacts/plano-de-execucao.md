# Plano de execução da Release A1

Complementa o `sprint-status.yaml`, que não tem estado "bloqueado". Atualizado em 28/10/2026.

## Limites de segurança (valem para qualquer execução sem acompanhamento)

- **Nunca fazer push.** Cada push no `main` dispara um deploy em produção. Só commit local; quem decide o push é o dono do produto.
- Nada de acesso ao servidor, ao Coolify, ao Cloudflare, ao Oracle do cliente nem ao quadro do GitHub.
- Banco de dados só o local de teste (`fb_auditoria_test`), nunca o de produção.
- Nenhum segredo em arquivo versionado; nenhum dado real do cliente em nenhum arquivo.
- Parar na primeira dúvida de intenção, falha que não se resolve ou teste que não passa. Registrar o motivo e não seguir para a história seguinte.
- Uma história por commit, para o dono do produto poder revisar e reverter uma a uma.

## Ordem de execução entre épicos

4.1 (feita, em produção) → Épico 1 (1.1 a 1.9; a 1.9 pode sair a qualquer hora) → Épico 2 → Épico 3 → Épico 4 (4.2 a 4.9) → Épico 5 → Épico 6 → Épico 8 (o que sobrar). O Épico 7 só depois das respostas do cliente.

## Histórias que um agente pode fazer sozinho (código com critérios claros)

1.1 a 1.9; 2.1 a 2.10, 2.12; 3.1, 3.2, 3.4 a 3.7; 4.2 a 4.9; 5.1 a 5.9; 6.1 a 6.5; 8.3 a 8.6.
Opcionais (só se sobrar tempo e nada mais estiver pendente): 2.13, 6.6, 8.7.

## Histórias que dependem de decisão humana ou de ação fora do repositório (não executar)

- **2.11** Dicionário inicial validado com o cliente (precisa da área de negócio dele).
- **3.3** Estrutura e Mapa iniciais derivados da planilha e validados.
- **3.8** Primeira demonstração ao cliente.
- **8.1** Confirmar a primeira execução do backup (log do servidor).
- **8.2** Decisão do cliente sobre o destino do backup.
- **Épico 7 inteiro** (rateio): depende das perguntas 4 e 5 do PRD.

## Pendências de ambiente que travam a entrega, não o código

- O Coolify só consegue fazer deploy enquanto o repositório estiver **público**. Antes de privá-lo de novo, o Coolify precisa de credencial (chave de deploy ou GitHub App), sem recriar a aplicação (o volume do banco e o backup dependem dela).
- Revisão do que sai do Git (endereço do banco do cliente, nome do schema e das views) está combinada para depois da conclusão.
- `AGENTS.md` está desatualizado: o `npm run lint` não roda (sem configuração do ESLint), o `typecheck` tem um erro anterior em `ResetPassword.tsx`, e já existem testes Go. Ver `deferred-work.md`.
