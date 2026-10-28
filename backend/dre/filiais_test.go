package dre_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func salvarG(t *testing.T, s *dre.Filiais, f testdb.Fixture, id, cod string, erp ...string) (dre.FilialGerencial, error) {
	t.Helper()
	return s.Salvar(context.Background(), dre.SalvarFilialIn{EmpresaID: f.EmpresaID, UserID: f.UserID, ID: id, Codigo: cod, Nome: "Grupo " + cod, Filiais: erp})
}

func TestFiliais_CriarResolverSubstituirConflito(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	s := &dre.Filiais{DB: db}

	g1, err := salvarG(t, s, f, "", "G1", "100", "101")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := dre.FilialGerencialDe(ctx, db, f.EmpresaID, "101"); got != "G1" {
		t.Errorf("filial em grupo: %q", got)
	}
	if got, _ := dre.FilialGerencialDe(ctx, db, f.EmpresaID, "999"); got != "999" {
		t.Errorf("filial solta: %q", got)
	}

	// Filial já ligada: 409 citando G1 e nada muda.
	var fl *dre.ErrFilialLigada
	_, err = salvarG(t, s, f, "", "G2", "101", "102")
	if !errors.As(err, &fl) || fl.Dono != "G1" || !strings.Contains(err.Error(), "G1") {
		t.Fatalf("esperava ErrFilialLigada G1: %v", err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.filiais_gerenciais WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 1 {
		t.Errorf("G2 não deveria existir: %d grupos", n)
	}
	if got, _ := dre.FilialGerencialDe(ctx, db, f.EmpresaID, "102"); got != "102" {
		t.Errorf("102 não deveria ter sido ligada: %q", got)
	}

	// Substituir a lista: 101 livre, 102 ligada.
	if _, err := salvarG(t, s, f, g1.ID, "G1", " 100 ", "102"); err != nil {
		t.Fatal(err)
	}
	if got, _ := dre.FilialGerencialDe(ctx, db, f.EmpresaID, "101"); got != "101" {
		t.Errorf("101 deveria estar livre: %q", got)
	}
	if got, _ := dre.FilialGerencialDe(ctx, db, f.EmpresaID, "102"); got != "G1" {
		t.Errorf("102: %q", got)
	}
	l, err := s.Listar(ctx, f.EmpresaID)
	if err != nil || len(l) != 1 || !reflect.DeepEqual(l[0].Filiais, []string{"100", "102"}) {
		t.Fatalf("listar: %v %+v", err, l)
	}

	// Código repetido na empresa.
	var ce *dre.ErrCodigoEmUso
	if _, err := salvarG(t, s, f, "", "G1"); !errors.As(err, &ce) {
		t.Errorf("esperava ErrCodigoEmUso: %v", err)
	}
	// Entrada inválida.
	var ee *dre.ErroEntrada
	for _, c := range []struct {
		cod, nome string
		erp       []string
	}{{"", "n", nil}, {"G3", " ", nil}, {"G3", "n", []string{"1", "1"}}, {"G3", "n", []string{"1", " "}}} {
		_, err := s.Salvar(ctx, dre.SalvarFilialIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Codigo: c.cod, Nome: c.nome, Filiais: c.erp})
		if !errors.As(err, &ee) {
			t.Errorf("%+v: esperava ErroEntrada: %v", c, err)
		}
	}
	// Editar id inexistente.
	var nf *dre.ErrFilialNaoEncontrada
	if _, err := salvarG(t, s, f, "00000000-0000-0000-0000-000000000000", "GX"); !errors.As(err, &nf) {
		t.Errorf("esperava não encontrada: %v", err)
	}

	// Trilha.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='filial_gerencial'`, f.EmpresaID); n != 2 {
		t.Errorf("trilha: %d (esperava criar + editar)", n)
	}
}

func TestFiliais_IsolamentoPorEmpresa(t *testing.T) {
	db := testdb.Open(t)
	a, b := testdb.NovaEmpresa(t, db), testdb.NovaEmpresa(t, db)
	s := &dre.Filiais{DB: db}
	if _, err := salvarG(t, s, a, "", "G1", "100"); err != nil {
		t.Fatal(err)
	}
	if _, err := salvarG(t, s, b, "", "G1", "100"); err != nil {
		t.Fatalf("mesma filial e código em outra empresa não conflitam: %v", err)
	}
	l, _ := s.Listar(context.Background(), b.EmpresaID)
	if len(l) != 1 {
		t.Errorf("empresa B vê %d", len(l))
	}
	if err := s.Excluir(context.Background(), b.EmpresaID, b.UserID, mustID(t, s, a)); err == nil {
		t.Error("excluir grupo de outra empresa deveria falhar")
	}
	if l, _ := s.Listar(context.Background(), a.EmpresaID); len(l) != 1 {
		t.Error("grupo da empresa A foi afetado")
	}
}

func mustID(t *testing.T, s *dre.Filiais, f testdb.Fixture) string {
	t.Helper()
	l, err := s.Listar(context.Background(), f.EmpresaID)
	if err != nil || len(l) == 0 {
		t.Fatal(err)
	}
	return l[0].ID
}

func TestFiliais_Excluir(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	s := &dre.Filiais{DB: db}
	g, _ := salvarG(t, s, f, "", "G1", "100", "101")

	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor, filial) VALUES ($1::uuid,'2026-09-01','3.1',10,'100')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	var eu *dre.ErrFilialEmUso
	if err := s.Excluir(ctx, f.EmpresaID, f.UserID, g.ID); !errors.As(err, &eu) {
		t.Fatalf("esperava ErrFilialEmUso: %v", err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.filiais_gerenciais_erp WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 2 {
		t.Errorf("nada deveria mudar: %d ligações", n)
	}
	// Uso em orçado também recusa.
	if _, err := db.Exec(`DELETE FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.dre_orcado (empresa_id, competencia, linha_codigo, valor, filial) VALUES ($1::uuid,'2026-09-01','RB',10,'101')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if err := s.Excluir(ctx, f.EmpresaID, f.UserID, g.ID); !errors.As(err, &eu) {
		t.Fatalf("orçado: esperava ErrFilialEmUso: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM auditoria.dre_orcado WHERE empresa_id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if err := s.Excluir(ctx, f.EmpresaID, f.UserID, g.ID); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.filiais_gerenciais_erp WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("ligações deveriam sair: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='filial_gerencial' AND acao='excluir'`, f.EmpresaID); n != 1 {
		t.Errorf("trilha de exclusão: %d", n)
	}
}

// semImutabilidade roda as instruções com a sessão de limpeza de teste aberta
// (os gatilhos de imutabilidade do fechamento a liberam só em empresa de teste).
func semImutabilidade(t *testing.T, db *sql.DB, f testdb.Fixture, stmts ...string) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT set_config('auditoria.limpando_teste', $1, true)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q, f.EmpresaID); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestFiliais_ExcluirComFechamento(t *testing.T) {
	const (
		delLinhas = `DELETE FROM auditoria.dre_fechamento_linhas WHERE filial='100' AND fechamento_id IN (SELECT id FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid)`
		delContas = `DELETE FROM auditoria.dre_fechamento_contas WHERE filial='100' AND empresa_id=$1::uuid`
		cabecalho = `UPDATE auditoria.dre_fechamentos SET filial='100' WHERE empresa_id=$1::uuid`
		semFilial = `UPDATE auditoria.dre_fechamentos SET filial='' WHERE empresa_id=$1::uuid`
	)
	casos := []struct {
		nome  string
		stmts []string
	}{
		{"todas", nil},
		{"só linhas", []string{delContas}},
		{"só contas", []string{delLinhas}},
		{"só cabeçalho", []string{delLinhas, delContas, cabecalho}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			db := testdb.Open(t)
			f := testdb.NovaEmpresa(t, db)
			ctx := context.Background()
			s := &dre.Filiais{DB: db}
			g, _ := salvarG(t, s, f, "", "G1", "100")
			db.Exec(`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de) VALUES ($1::uuid,'RB','Receita',1,10,'conta','{}')`, f.EmpresaID)
			db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'3.1','RB',1)`, f.EmpresaID)
			if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor, filial) VALUES ($1::uuid,'2026-09-01','3.1',10,'100')`, f.EmpresaID); err != nil {
				t.Fatal(err)
			}
			if _, err := (&dre.Fechamento{DB: db}).Fechar(ctx, dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`DELETE FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); err != nil {
				t.Fatal(err)
			}
			semImutabilidade(t, db, f, c.stmts...)
			var eu *dre.ErrFilialEmUso
			if err := s.Excluir(ctx, f.EmpresaID, f.UserID, g.ID); !errors.As(err, &eu) || eu.Onde != "fechamentos" {
				t.Fatalf("esperava ErrFilialEmUso(fechamentos): %v", err)
			}
			if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='filial_gerencial' AND acao='excluir'`, f.EmpresaID); n != 0 {
				t.Errorf("exclusão recusada não deve gravar trilha: %d", n)
			}
			semImutabilidade(t, db, f, delLinhas, delContas, semFilial)
			if err := s.Excluir(ctx, f.EmpresaID, f.UserID, g.ID); err != nil {
				t.Fatalf("sem nenhum uso deveria excluir: %v", err)
			}
		})
	}
}

func TestFiliais_RenomearParaCodigoExistenteEIgualAFilial(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	s := &dre.Filiais{DB: db}
	salvarG(t, s, f, "", "G1", "100")
	g2, _ := salvarG(t, s, f, "", "G2", "200")
	var ce *dre.ErrCodigoEmUso
	if _, err := salvarG(t, s, f, g2.ID, "G1", "200"); !errors.As(err, &ce) {
		t.Errorf("renomear para código existente: %v", err)
	}
	var ci *dre.ErrCodigoIgualFilial
	if _, err := salvarG(t, s, f, "", "100"); !errors.As(err, &ci) {
		t.Errorf("código igual a filial de outro grupo (criar): %v", err)
	}
	if _, err := salvarG(t, s, f, g2.ID, "100", "200"); !errors.As(err, &ci) {
		t.Errorf("código igual a filial de outro grupo (editar): %v", err)
	}
	// Igual à própria filial é permitido.
	if _, err := salvarG(t, s, f, g2.ID, "200", "200"); err != nil {
		t.Errorf("código igual à própria filial: %v", err)
	}
}

func TestFiliais_Limites(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	s := &dre.Filiais{DB: db}
	var ee *dre.ErroEntrada
	muitas := make([]string, 501)
	for i := range muitas {
		muitas[i] = strconv.Itoa(i)
	}
	if _, err := salvarG(t, s, f, "", strings.Repeat("c", 41)); !errors.As(err, &ee) {
		t.Errorf("código longo: %v", err)
	}
	if _, err := salvarG(t, s, f, "", "G", strings.Repeat("9", 21)); !errors.As(err, &ee) {
		t.Errorf("filial longa: %v", err)
	}
	if _, err := salvarG(t, s, f, "", "G", muitas...); !errors.As(err, &ee) {
		t.Errorf("muitas filiais: %v", err)
	}
	var nf *dre.ErrFilialNaoEncontrada
	if _, err := salvarG(t, s, f, "nao-e-uuid", "G"); !errors.As(err, &nf) {
		t.Errorf("id inválido: %v", err)
	}
	if err := s.Excluir(context.Background(), f.EmpresaID, f.UserID, "nao-e-uuid"); !errors.As(err, &nf) {
		t.Errorf("excluir id inválido: %v", err)
	}
}

func TestFiliais_TrilhaEditarGuardaAntes(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	s := &dre.Filiais{DB: db}
	g, _ := salvarG(t, s, f, "", "G1", "100")
	if _, err := salvarG(t, s, f, g.ID, "G1", "101"); err != nil {
		t.Fatal(err)
	}
	var p string
	if err := db.QueryRow(`SELECT payload::text FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='filial_gerencial' AND acao='editar'`, f.EmpresaID).Scan(&p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, `"antes"`) || !strings.Contains(p, `"100"`) {
		t.Errorf("trilha sem estado anterior: %s", p)
	}
	if _, err := salvarG(t, s, f, g.ID, "G1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Excluir(context.Background(), f.EmpresaID, f.UserID, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT payload::text FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='excluir'`, f.EmpresaID).Scan(&p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p, `"filiais": []`) || !strings.Contains(p, `"nome"`) {
		t.Errorf("trilha de excluir: %s", p)
	}
}

func TestFiliais_LimpezaDeTesteApagaMapa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	s := &dre.Filiais{DB: db}
	if _, err := salvarG(t, s, f, "", "G1", "100", "101"); err != nil {
		t.Fatal(err)
	}
	var out string
	if err := db.QueryRow(`SELECT auditoria.limpar_dados_teste($1::uuid)::text`, f.EmpresaID).Scan(&out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"filiais_gerenciais_erp": 2`) || !strings.Contains(out, `"filiais_gerenciais": 1`) {
		t.Errorf("contagens: %s", out)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.filiais_gerenciais WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("sobraram %d grupos", n)
	}
}
