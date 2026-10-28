-- 012: departamento de cada conta no snapshot do fechamento - história 3.5.
--
-- dre_fechamento_contas passa a gravar o código do departamento da conta no
-- momento do fechamento ('' = sem departamento). Meses fechados leem o
-- departamento daqui, sem reconsultar o dicionário. Fechamentos anteriores
-- ficam com '' (aparecem em "Sem departamento"); nenhuma linha gravada é
-- reescrita. Sem tabela nova: limpar_dados_teste não muda.

ALTER TABLE auditoria.dre_fechamento_contas ADD COLUMN IF NOT EXISTS departamento TEXT NOT NULL DEFAULT '';
