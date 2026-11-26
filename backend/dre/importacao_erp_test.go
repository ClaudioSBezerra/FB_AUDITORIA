package dre_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/dre/fontes/oracle"
	"fb_auditoria/internal/testdb"
)

// erpFalso faz o papel do banco do cliente (dados fictícios).
type erpFalso struct {
	cons map[string][]oracle.LinhaConsolidado
	real map[string][]oracle.LinhaRealizado
	err  error
}

func (e *erpFalso) Consolidado(_ context.Context, c time.Time) ([]oracle.LinhaConsolidado, error) {
	return e.cons[c.Format("2006-01")], e.err
}
func (e *erpFalso) Realizado(_ context.Context, c time.Time) ([]oracle.LinhaRealizado, error) {
	return e.real[c.Format("2006-01")], e.err
}

func linhaReal(filial, conta string, v float64) oracle.LinhaRealizado {
	return oracle.LinhaRealizado{Filial: filial, Conta: conta, Descricao: "Conta " + conta, Data: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), Valor: v}
}

func linhaCons(filial, mes string, fat float64) oracle.LinhaConsolidado {
	return oracle.LinhaConsolidado{UF: "XX", Filial: filial, MesAno: mes,
		Valores: map[string]float64{"VLFATBRUTO": fat, "VLTOTAL_DEV": 5, "VLCMVDEVOL": 3}}
}

func novaImportacao(t *testing.T, f testdb.Fixture, erp *erpFalso) *dre.ImportacaoERP {
	t.Helper()
	db := testdb.Open(t)
	contas, err := oracle.CarregarContas("")
	if err != nil {
		t.Fatal(err)
	}
	return &dre.ImportacaoERP{DB: db, Ingestor: &dre.Ingestor{DB: db}, EmpresaID: f.EmpresaID,
		Fontes: func(c time.Time) []dre.Fonte {
			return []dre.Fonte{oracle.NovaConsolidado(erp, c, contas), oracle.NovaRealizado(erp, c)}
		}}
}

func importarERP(t *testing.T, i *dre.ImportacaoERP, f testdb.Fixture, de, ate string) dre.Execucao {
	t.Helper()
	meses, err := dre.MesesDoPeriodo(de, ate, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	p, err := i.Iniciar(context.Background(), dre.EscopoAtualizacao{EmpresaID: f.EmpresaID, UserID: f.UserID}, "manual", meses)
	if err != nil {
		t.Fatal(err)
	}
	ex, err := i.Executar(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func TestImportacaoERP_GravaReimportaEPulaMesFechado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	erp := &erpFalso{
		cons: map[string][]oracle.LinhaConsolidado{"2026-09": {linhaCons("01", "09/2026", 100)}, "2026-10": {linhaCons("01", "10/2026", 200)}},
		real: map[string][]oracle.LinhaRealizado{"2026-09": {linhaReal("11", "4010001", -10), linhaReal("13", "4010001", -4)}, "2026-10": {linhaReal("11", "4010001", -20)}},
	}
	i := novaImportacao(t, f, erp)
	ex := importarERP(t, i, f, "2026-09", "2026-10")
	if ex.Status != dre.ExecucaoConcluida || len(ex.Itens) != 4 {
		t.Fatalf("%+v", ex)
	}
	q := func(sql string, args ...any) int {
		return contar(t, db, sql, append([]any{f.EmpresaID}, args...)...)
	}
	// Valor como a fonte entrega, UF gravada, filial sem zero à esquerda, coluna sem conta fora.
	if n := q(`SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND origem='oracle.consolidado' AND filial='1' AND uf='XX' AND conta='2' AND valor=5`); n != 2 {
		t.Errorf("devoluções: %d", n)
	}
	if n := q(`SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND origem='oracle.consolidado' AND conta NOT IN ('1','2')`); n != 0 {
		t.Errorf("CMV de devolução não deveria entrar: %d", n)
	}
	if n := q(`SELECT COUNT(*) FROM auditoria.conta_fonte WHERE empresa_id=$1::uuid`); n != 3 {
		t.Errorf("donas: %d", n)
	}
	// Reimportar não duplica; filial que sumiu no ERP sai.
	erp.real["2026-09"] = []oracle.LinhaRealizado{linhaReal("11", "4010001", -12)}
	importarERP(t, i, f, "2026-09", "2026-09")
	if n := q(`SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND origem='oracle.realizado' AND competencia='2026-09-01'`); n != 1 {
		t.Errorf("reimportação: %d", n)
	}
	// Leitura vazia nunca apaga.
	erp.real["2026-09"] = nil
	ex = importarERP(t, i, f, "2026-09", "2026-09")
	if ex.Status != dre.ExecucaoComRessalvas || ex.Itens[1].Status != dre.ItemSemDados {
		t.Fatalf("vazio: %+v", ex)
	}
	if n := q(`SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND origem='oracle.realizado' AND competencia='2026-09-01'`); n != 1 {
		t.Errorf("vazio apagou: %d", n)
	}
	// Mês fechado é pulado sem falhar os demais.
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, filial, versao, fechado_por)
		VALUES ($1::uuid,'2026-09-01','',1,$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	ex = importarERP(t, i, f, "2026-09", "2026-10")
	if ex.Itens[0].Status != dre.ItemFechado || ex.Itens[2].Status != dre.ItemAplicado {
		t.Fatalf("fechado: %+v", ex.Itens)
	}
	lista, err := i.Execucoes(context.Background(), f.EmpresaID, 10)
	if err != nil || len(lista) != 4 || lista[0].Status != dre.ExecucaoComRessalvas || lista[0].Fim == nil || len(lista[0].Itens) != 4 {
		t.Fatalf("execuções: %v %+v", err, lista)
	}
	if n := q(`SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='importacao_erp' AND user_id=$2::uuid`, f.UserID); n != 8 {
		t.Errorf("trilha: %d", n)
	}
}

// Um arquivo com o consolidado numa filial e o ERP por filial não se somam.
func TestImportacaoERP_NaoSomaComArquivoNemEmOutraFilial(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	// Carga anterior à posse das contas (como a que já existe): só em lancamentos.
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, origem)
		VALUES ($1::uuid,'2026-09-01','11','4010001',-100,'csv.contas')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	erp := &erpFalso{real: map[string][]oracle.LinhaRealizado{
		"2026-09": {linhaReal("13", "4010001", -4)}, "2026-10": {linhaReal("13", "4010001", -7)}}}
	i := novaImportacao(t, f, erp)
	ex := importarERP(t, i, f, "2026-09", "2026-10")
	if ex.Itens[1].Status != dre.ItemRejeitado || !strings.Contains(ex.Itens[1].Mensagem, "somaria em dobro") || ex.Itens[3].Status != dre.ItemAplicado {
		t.Fatalf("%+v", ex.Itens)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND competencia='2026-09-01'`, f.EmpresaID); n != 1 {
		t.Errorf("setembro mudou: %d", n)
	}
	// A conta passou a ser do ERP: o arquivo é recusado, citando a dona.
	_, err := (&dre.Ingestor{DB: db}).Importar(context.Background(), f.EmpresaID, f.UserID,
		&fonteFake{nome: "csv.contas", lancs: []dre.Lancamento{lanc("2026-10", "11", "4010001", 1)}})
	var dona *dre.ErrContaDeOutraFonte
	if !errors.As(err, &dona) || !strings.Contains(err.Error(), "oracle.realizado") {
		t.Fatalf("%v", err)
	}
	if _, err := (&dre.Ingestor{DB: db}).Simular(context.Background(), f.EmpresaID,
		&fonteFake{nome: "csv", lancs: []dre.Lancamento{lanc("2026-10", "11", "4010001", 1)}}); !errors.As(err, &dona) {
		t.Fatalf("simulação: %v", err)
	}
}

func TestImportacaoERP_FalhaMantemDadosEExclusaoMutua(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	erp := &erpFalso{real: map[string][]oracle.LinhaRealizado{"2026-09": {linhaReal("11", "4010001", -1)}},
		cons: map[string][]oracle.LinhaConsolidado{"2026-09": {linhaCons("11", "09/2026", 10)}}}
	i := novaImportacao(t, f, erp)
	importarERP(t, i, f, "2026-09", "2026-09")
	erp.err = &dre.ErroFonte{Msg: "não foi possível alcançar o banco do cliente"}
	ex := importarERP(t, i, f, "2026-09", "2026-10")
	if ex.Status != dre.ExecucaoFalha || len(ex.Itens) != 1 || !strings.Contains(ex.Mensagem, "alcançar") {
		t.Fatalf("%+v", ex)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 3 {
		t.Errorf("dados: %d", n)
	}
	esc := dre.EscopoAtualizacao{EmpresaID: f.EmpresaID, UserID: f.UserID}
	meses := []time.Time{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	p, err := i.Iniciar(context.Background(), esc, "manual", meses)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := i.Iniciar(context.Background(), esc, "manual", meses); !errors.Is(err, dre.ErrImportacaoEmAndamento) {
		t.Fatalf("segunda: %v", err)
	}
	if _, err := i.Executar(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if _, err := i.Atualizar(context.Background(), dre.EscopoAtualizacao{EmpresaID: testdb.NovaEmpresa(t, db).EmpresaID}); err == nil {
		t.Error("outra empresa não usa o banco do cliente")
	}
}

func TestMesesDoPeriodo(t *testing.T) {
	hoje := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	if m, err := dre.MesesDoPeriodo("2025-12", "2026-11", hoje); err != nil || len(m) != 12 {
		t.Fatalf("%d %v", len(m), err)
	}
	for _, c := range [][2]string{{"2025-11", "2026-11"}, {"2026-10", "2026-09"}, {"2026-12", "2026-12"}, {"x", "2026-01"}} {
		if _, err := dre.MesesDoPeriodo(c[0], c[1], hoje); err == nil {
			t.Errorf("%v deveria ser recusado", c)
		}
	}
}
