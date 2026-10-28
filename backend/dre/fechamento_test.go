package dre_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func TestResolverConta(t *testing.T) {
	mapa := []dre.MapaItem{
		{ContaPadrao: "3%", LinhaCodigo: "A", Sinal: 1},
		{ContaPadrao: "3.1%", LinhaCodigo: "B", Sinal: 1},
		{ContaPadrao: "3.1.1", LinhaCodigo: "C", Sinal: -1},
	}
	casos := []struct {
		conta, linha string
		ok           bool
	}{
		{"3.1.1", "C", true}, // exato vence
		{"3.1.2", "B", true}, // prefixo mais longo
		{"3.9", "A", true},   // prefixo curto
		{"4.1", "", false},   // sem casamento
	}
	for _, c := range casos {
		m, ok := dre.ResolverConta(mapa, c.conta)
		if ok != c.ok || m.LinhaCodigo != c.linha {
			t.Errorf("%s: veio %q/%v, esperado %q/%v", c.conta, m.LinhaCodigo, ok, c.linha, c.ok)
		}
	}
}

func semear(t *testing.T) (testdb.Fixture, []dre.MapaItem) {
	t.Helper()
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for _, m := range [][]any{{"3.1", "RB"}, {"4.1", "CMV"}} {
		if _, err := db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,$2,$3,1)`, f.EmpresaID, m[0], m[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, l := range [][]any{{"3.1", 1000.0}, {"4.1", 300.0}, {"9.9", 5.0}} {
		if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2026-09-01','01',$2,$3)`, f.EmpresaID, l[0], l[1]); err != nil {
			t.Fatal(err)
		}
	}
	mapa, err := dre.CarregarMapa(context.Background(), db, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	return f, mapa
}

func TestRealizadoIgualEmTxEEmDB(t *testing.T) {
	db := testdb.Open(t)
	f, mapa := semear(t)
	p := dre.Periodo{Ini: mustData("2026-09-01"), Fim: mustData("2026-09-01")}
	ctx := context.Background()
	porDB, semDB, err := dre.Realizado(ctx, db, f.EmpresaID, "", p, mapa)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	porTx, semTx, err := dre.Realizado(ctx, tx, f.EmpresaID, "", p, mapa)
	if err != nil {
		t.Fatal(err)
	}
	if !igual(porDB, porTx) || !igual(semDB, semTx) {
		t.Errorf("divergem: %v/%v vs %v/%v", porDB, semDB, porTx, semTx)
	}
	if porDB["RB"] != 1000 || porDB["CMV"] != 300 || semDB["9.9"] != 5 {
		t.Errorf("valores inesperados: %v %v", porDB, semDB)
	}
}

func TestFecharContextoCanceladoNaoGravaNada(t *testing.T) {
	db := testdb.Open(t)
	f, _ := semear(t)
	// remove a conta sem mapa para que só o contexto impeça o fechamento
	if _, err := db.Exec(`DELETE FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND conta='9.9'`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&dre.Fechamento{DB: db}).Fechar(ctx, dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9})
	if err == nil {
		t.Fatal("esperava erro")
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("fechamentos: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_fechamento'`, f.EmpresaID); n != 0 {
		t.Errorf("trilha: %d", n)
	}
}

func TestFecharContaSemMapaListaContas(t *testing.T) {
	db := testdb.Open(t)
	f, _ := semear(t)
	_, err := (&dre.Fechamento{DB: db}).Fechar(context.Background(), dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9})
	var e *dre.ErrContasSemMapa
	if !errors.As(err, &e) || e.Total != 1 || e.Contas[0] != "9.9" {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func igual(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func mustData(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func TestFecharFalhaTardiaDesfazTudo(t *testing.T) {
	db := testdb.Open(t)
	f, _ := semear(t)
	if _, err := db.Exec(`DELETE FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND conta='9.9'`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de)
		VALUES ($1::uuid,'RB','Receita',1,10,'conta','{}')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE OR REPLACE FUNCTION auditoria.tmp_falha_fechar() RETURNS trigger AS $$
		BEGIN IF NEW.entidade = 'dre_fechamento' AND NEW.empresa_id = '` + f.EmpresaID + `'::uuid THEN RAISE EXCEPTION 'falha de teste'; END IF; RETURN NEW; END $$ LANGUAGE plpgsql;
		CREATE TRIGGER tmp_falha_fechar BEFORE INSERT ON auditoria.audit_log FOR EACH ROW EXECUTE FUNCTION auditoria.tmp_falha_fechar()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DROP TRIGGER IF EXISTS tmp_falha_fechar ON auditoria.audit_log`)
		db.Exec(`DROP FUNCTION IF EXISTS auditoria.tmp_falha_fechar()`)
	})
	if _, err := (&dre.Fechamento{DB: db}).Fechar(context.Background(), dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9}); err == nil {
		t.Fatal("esperava erro")
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`,
		`SELECT COUNT(*) FROM auditoria.dre_fechamento_linhas l JOIN auditoria.dre_fechamentos f ON f.id=l.fechamento_id WHERE f.empresa_id=$1::uuid`,
		`SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_fechamento'`,
	} {
		if n := contar(t, db, q, f.EmpresaID); n != 0 {
			t.Errorf("%s: %d", q, n)
		}
	}
}

func TestFecharCompetenciaInvalida(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for _, c := range [][2]int{{2026, 0}, {2026, 13}, {1999, 5}, {2101, 5}} {
		_, err := (&dre.Fechamento{DB: db}).Fechar(context.Background(), dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: c[0], Mes: c[1]})
		var e *dre.ErroEntrada
		if !errors.As(err, &e) {
			t.Errorf("%v: esperava ErroEntrada, veio %v", c, err)
		}
	}
}
