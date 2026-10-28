package dre_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"

	"fb_auditoria/dre"
	csvfonte "fb_auditoria/dre/fontes/csv"
	"fb_auditoria/internal/testdb"
)

const base = "competencia;filial;conta;valor\n2026-09;01;3.1;10,00\n2026-09;02;3.2;20,00\n"

func importar(t *testing.T, db *sql.DB, f testdb.Fixture, conteudo string) (dre.Resultado, error) {
	t.Helper()
	ing := &dre.Ingestor{DB: db}
	return ing.Importar(context.Background(), f.EmpresaID, f.UserID, csvfonte.Nova(strings.NewReader(conteudo), "t.csv"))
}

func contar(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

const total = `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`

func TestPrimeiraCargaEReenvio(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	r, err := importar(t, db, f, base)
	if err != nil || r.Linhas != 2 || r.CargaID == 0 {
		t.Fatalf("primeira carga: %+v %v", r, err)
	}
	if _, err := importar(t, db, f, base); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, db, total, f.EmpresaID); n != 2 {
		t.Errorf("reenvio duplicou: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='lancamentos' AND acao='importar'`, f.EmpresaID); n != 2 {
		t.Errorf("trilha: %d", n)
	}
}

func TestReenvioSoSubstituiEscoposDoArquivoEPreservaOutraOrigem(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := importar(t, db, f, base); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, origem)
		VALUES ($1::uuid,'2026-09-01','01','9.9',5,'winthor')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	// só a filial 01 volta, com outra linha
	if _, err := importar(t, db, f, "competencia;filial;conta;valor\n2026-09;01;3.1;99,00\n"); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, db, total+` AND origem='winthor'`, f.EmpresaID); n != 1 {
		t.Errorf("outra origem foi apagada: %d", n)
	}
	if n := contar(t, db, total+` AND origem='csv' AND filial='02'`, f.EmpresaID); n != 1 {
		t.Errorf("escopo ausente do arquivo foi apagado: %d", n)
	}
	if n := contar(t, db, total+` AND origem='csv' AND filial='01' AND valor=99`, f.EmpresaID); n != 1 {
		t.Errorf("escopo 01 não substituído")
	}
}

func TestRejeitaListandoLinhasENadaGrava(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := importar(t, db, f, base); err != nil {
		t.Fatal(err)
	}
	ruim := "competencia;filial;conta;valor\n2026-09;01;3.1;77,00\n2026-09;;3.2;1,00\n2026-09;02;3.3;xx\n"
	_, err := importar(t, db, f, ruim)
	var el *dre.ErroLinhas
	if !errors.As(err, &el) || len(el.Problemas) != 2 || el.Problemas[0].Linha != 3 || el.Problemas[1].Linha != 4 {
		t.Fatalf("esperava 2 problemas (linhas 3 e 4): %v", err)
	}
	if n := contar(t, db, total+` AND valor=77`, f.EmpresaID); n != 0 {
		t.Error("gravou algo apesar do erro")
	}
	if n := contar(t, db, total, f.EmpresaID); n != 2 {
		t.Errorf("escopos anteriores alterados: %d", n)
	}
}

func TestListaLimitadaA20(t *testing.T) {
	var b strings.Builder
	b.WriteString("competencia;filial;conta;valor\n")
	for i := 0; i < 30; i++ {
		b.WriteString("2026-09;;3.1;1,00\n")
	}
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	_, err := importar(t, db, f, b.String())
	if err == nil || !strings.Contains(err.Error(), "30 linha(s)") || !strings.Contains(err.Error(), "e mais 10") {
		t.Fatalf("mensagem: %v", err)
	}
}

func TestArquivoSoComCabecalhoNaoApaga(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := importar(t, db, f, base); err != nil {
		t.Fatal(err)
	}
	_, err := importar(t, db, f, "competencia;filial;conta;valor\n")
	var ee *dre.ErroEntrada
	if !errors.As(err, &ee) {
		t.Fatalf("esperava ErroEntrada: %v", err)
	}
	if n := contar(t, db, total, f.EmpresaID); n != 2 {
		t.Errorf("apagou com arquivo vazio: %d", n)
	}
}

func TestImportacoesConcorrentesNaoDuplicam(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	var wg sync.WaitGroup
	erros := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := importar(t, db, f, base)
			erros <- err
		}()
	}
	wg.Wait()
	close(erros)
	for err := range erros {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := contar(t, db, total, f.EmpresaID); n != 2 {
		t.Errorf("concorrência duplicou: %d (esperado 2)", n)
	}
}
