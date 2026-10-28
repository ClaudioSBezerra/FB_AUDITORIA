# Respostas da área de negócio do cliente (28/10/2026) — resumo

Só estrutura e regras. Os e-mails dos gerentes, a lista de aviso e os percentuais de rateio são dado do cliente: entram pelo sistema (tela de configuração ou carga), nunca em arquivo versionado.

## Decisões

1. **Fechamento mensal:** todo dia 1 ou primeiro dia útil do mês. O disparo do fechamento é iniciado manualmente pela controladoria, depois da revisão geral e da revisão dos e-mails (trocas por férias etc.). Não é automático por data.
2. **Disparo parcial semanal:** toda sexta-feira, com o acumulado do mês até aquele dia (semana 1 de 01 a 07, semana 2 de 01 a 14 etc.), em nível de lançamento. É requisito novo.
3. **Rateio:** nova aba, com duas visões: a normal (padrão do sistema) e a rateada por faturamento. Configuração por caixas: incluir ou tirar departamentos do rateio e editar à mão o percentual por filial. Há uma matriz departamento × filial que diz em quais filiais cada departamento entra, e o percentual padrão por departamento e filial.
4. **Destinatários:** lista de e-mails por gerente, organizada por bloco (matriz consolidada e cada filial com gerente). Cada gerente tem a sua lista, e alguns e-mails se repetem em várias listas (controladoria, TI).
5. **Desvios:** todos ficam disponíveis para comentário e justificativa.
6. **Folha:** só o lançamento consolidado do departamento; a abertura salarial não existe no sistema.
7. **Ajuste manual:** sem ajuste manual nas contas do DRE, "apenas direto no sistema". Controladoria e analista configuram disparos, percentuais de rateio etc. Quatro pessoas são a lista de aviso de ajustes.

## Impacto no planejamento (a decidir)

- **Confirmado pelo cliente (28/10/2026): o ajuste vem do ERP e chega ao sistema pelo banco, junto da ingestão. O sistema não tem lançamento manual de ajuste.** Consequência: a FR-4 como escrita (lançar ajuste no sistema) cai; as histórias 5.4, 5.5 e 5.7 e o AD-6 precisam ser reescritos. Mudança ainda não aplicada no `epics.md`, no PRD, na arquitetura nem no quadro.
- **Épico 5 (antes da confirmação):** o "ajuste manual imutável" (FR-4) provavelmente perde o sentido: precisa ser reinterpretado ("direto no sistema" = lançamento dentro do sistema?). Perguntar ao cliente. Disparo semanal e disparo de fechamento manual entram no outbox de e-mail (AD de e-mail).
- **Épico 7 (rateio):** deixa de depender de resposta do cliente. Já há regra: visão por faturamento, matriz de inclusão, percentual editável por filial.
- **Épico 2/8:** destinatários e percentuais precisam de tela ou carga, e não de seed no repositório.
- **Folha:** consolidada por departamento, sem dado individual (também é questão de LGPD).
