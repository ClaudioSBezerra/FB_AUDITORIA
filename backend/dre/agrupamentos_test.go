package dre_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func salvarA(t *testing.T, s *dre.Agrupamentos, f testdb.Fixture, id, cod string, membros ...string) (dre.Agrupamento, error) {
	t.Helper()
	return s.Salvar(context.Background(), dre.SalvarAgrupamentoIn{EmpresaID: f.EmpresaID, UserID: f.UserID, ID: id, Codigo: cod, Nome: "Agr " + cod, Filiais: membros})
}

func TestAgrupamentos_CriarResolverMultiplosECodigos(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	fs := &dre.Filiais{DB: db}
	as := &dre.Agrupamentos{DB: db}
	for _, g := range []struct {
		cod string
		erp []string
	}{{"G1", []string{"100", "101"}}, {"G2", []string{"200"}}, {"G3", nil}} {
		if _, err := salvarG(t, fs, f, "", g.cod, g.erp...); err != nil {
			t.Fatal(err)
		}
	}

	a1, err := salvarA(t, as, f, "", "A1", "G1", "G2")
	if err != nil {
		t.Fatal(err)
	}
	// G2 em dois agrupamentos.
	if _, err := salvarA(t, as, f, "", "A2", "G2", "G3"); err != nil {
		t.Fatal(err)
	}

	alvo, err := dre.FiliaisDoAgrupamento(ctx, db, f.EmpresaID, "A1")
	if err != nil || !reflect.DeepEqual(alvo.Gerenciais, []string{"G1", "G2"}) || !reflect.DeepEqual(alvo.ERP, []string{"100", "101", "200"}) {
		t.Fatalf("A1: %v %+v", err, alvo)
	}
	alvo, err = dre.FiliaisDoAgrupamento(ctx, db, f.EmpresaID, "A2")
	if err != nil || !reflect.DeepEqual(alvo.ERP, []string{"200"}) {
		t.Fatalf("A2: %v %+v", err, alvo)
	}
	var nf *dre.ErrAgrupamentoNaoEncontrado
	if _, err := dre.FiliaisDoAgrupamento(ctx, db, f.EmpresaID, "X9"); !errors.As(err, &nf) {
		t.Errorf("inexistente: %v", err)
	}
	// Agrupamento sem membros existe e resolve vazio.
	if _, err := salvarA(t, as, f, "", "A3"); err != nil {
		t.Fatal(err)
	}
	if alvo, err = dre.FiliaisDoAgrupamento(ctx, db, f.EmpresaID, "A3"); err != nil || len(alvo.ERP) != 0 {
		t.Errorf("A3: %v %+v", err, alvo)
	}

	// Editar substitui os membros.
	if _, err := salvarA(t, as, f, a1.ID, "A1", "G1"); err != nil {
		t.Fatal(err)
	}
	if alvo, _ = dre.FiliaisDoAgrupamento(ctx, db, f.EmpresaID, "A1"); !reflect.DeepEqual(alvo.ERP, []string{"100", "101"}) {
		t.Errorf("A1 editado: %+v", alvo)
	}

	// Códigos: repetido, igual a filial gerencial, reservado (e o inverso).
	var em *dre.ErrAgrupamentoCodigoEmUso
	if _, err := salvarA(t, as, f, "", "A1"); !errors.As(err, &em) {
		t.Errorf("repetido: %v", err)
	}
	var col *dre.ErrCodigoColide
	for _, cod := range []string{"G1", "GRUPO", "grupo"} {
		if _, err := salvarA(t, as, f, "", cod); !errors.As(err, &col) {
			t.Errorf("%s: %v", cod, err)
		}
	}
	if _, err := salvarG(t, fs, f, "", "A1"); !errors.As(err, &col) {
		t.Errorf("filial gerencial com código de agrupamento: %v", err)
	}
	if _, err := salvarG(t, fs, f, "", "Grupo"); !errors.As(err, &col) {
		t.Errorf("filial gerencial com palavra reservada: %v", err)
	}
	// Membro que não é filial gerencial; entrada inválida.
	var mi *dre.ErrMembroInexistente
	if _, err := salvarA(t, as, f, "", "A4", "ZZ"); !errors.As(err, &mi) {
		t.Errorf("membro inexistente: %v", err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.agrupamentos_filiais WHERE empresa_id=$1::uuid AND codigo='A4'`, f.EmpresaID); n != 0 {
		t.Errorf("A4 não deveria existir (atômico): %d", n)
	}
	var ee *dre.ErroEntrada
	if _, err := salvarA(t, as, f, "", "A5", "G1", "G1"); !errors.As(err, &ee) {
		t.Errorf("membro repetido: %v", err)
	}
	if _, err := salvarA(t, as, f, "", " "); !errors.As(err, &ee) {
		t.Errorf("vazio: %v", err)
	}

	// Filial gerencial que é membro não pode ser excluída (409), nem some em silêncio.
	l, _ := fs.Listar(ctx, f.EmpresaID)
	var idG1 string
	for _, g := range l {
		if g.Codigo == "G1" {
			idG1 = g.ID
		}
	}
	var ea *dre.ErrFilialEmAgrupamento
	if err := fs.Excluir(ctx, f.EmpresaID, f.UserID, idG1); !errors.As(err, &ea) || ea.Agrupamento != "A1" || !strings.Contains(err.Error(), "A1") {
		t.Errorf("excluir membro: %v", err)
	}

	// Isolamento por empresa.
	outra := testdb.NovaEmpresa(t, db)
	if _, err := dre.FiliaisDoAgrupamento(ctx, db, outra.EmpresaID, "A1"); !errors.As(err, &nf) {
		t.Errorf("outra empresa vê A1: %v", err)
	}
	if lo, _ := as.Listar(ctx, outra.EmpresaID); len(lo) != 0 {
		t.Errorf("outra empresa lista: %+v", lo)
	}
	// Membro de outra empresa não vale (código inexistente lá).
	if _, err := salvarA(t, as, outra, "", "B1", "G1"); !errors.As(err, &mi) {
		t.Errorf("membro de outra empresa: %v", err)
	}

	// Excluir agrupamento: membros caem, filiais gerenciais ficam; trilha.
	if err := as.Excluir(ctx, f.EmpresaID, f.UserID, a1.ID); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.agrupamentos_filiais_membros WHERE empresa_id=$1::uuid AND agrupamento_id=$2::uuid`, f.EmpresaID, a1.ID); n != 0 {
		t.Errorf("membros restantes: %d", n)
	}
	if lg, _ := fs.Listar(ctx, f.EmpresaID); len(lg) != 3 {
		t.Errorf("filiais gerenciais: %d", len(lg))
	}
	if err := as.Excluir(ctx, f.EmpresaID, f.UserID, a1.ID); !errors.As(err, &nf) {
		t.Errorf("excluir de novo: %v", err)
	}
	// criar A1, A2, A3, editar A1, excluir A1 = 5; os recusados não gravam.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='agrupamento_filiais'`, f.EmpresaID); n != 5 {
		t.Errorf("trilha: %d", n)
	}
	// Agora G1 pode sair (já não é membro).
	if err := fs.Excluir(ctx, f.EmpresaID, f.UserID, idG1); err != nil {
		t.Errorf("excluir G1 livre: %v", err)
	}
}

func TestAgrupamentos_ErrosSemPalavrasDoPostgres(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	as := &dre.Agrupamentos{DB: db}
	if _, err := salvarA(t, as, f, "", "A1"); err != nil {
		t.Fatal(err)
	}
	_, err := salvarA(t, as, f, "", "A1")
	if err == nil || strings.Contains(err.Error(), "already exists") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Errorf("mensagem: %v", err)
	}
}
