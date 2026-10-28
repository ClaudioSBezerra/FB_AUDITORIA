package dre_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"fb_auditoria/dre"
	csvfonte "fb_auditoria/dre/fontes/csv"
	"fb_auditoria/internal/testdb"
)

func importarErr(db *sql.DB, f testdb.Fixture, conteudo string) (dre.Resultado, error) {
	return (&dre.Ingestor{DB: db}).Importar(context.Background(), f.EmpresaID, f.UserID, csvfonte.Nova(strings.NewReader(conteudo), "t.csv"))
}

func fechar(t *testing.T, f testdb.Fixture, mes int) (dre.FecharOut, error) {
	t.Helper()
	return (&dre.Fechamento{DB: testdb.Open(t)}).Fechar(context.Background(), dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: mes})
}

// empresaMapeada cria empresa com linha RB, mapa 3.1->RB e um lançamento em 2026-09.
func empresaMapeada(t *testing.T) testdb.Fixture {
	t.Helper()
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for _, q := range []string{
		`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de) VALUES ($1::uuid,'RB','Receita',1,10,'conta','{}')`,
		`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'3.1','RB',1)`,
		`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, origem) VALUES ($1::uuid,'2026-09-01','01','3.1',10,'csv')`,
	} {
		if _, err := db.Exec(q, f.EmpresaID); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestGuardaAbertaEFechada(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	comp := mustData("2026-09-01")
	guardar := func() error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		return dre.Periodo{}.Guardar(ctx, tx, f.EmpresaID, comp)
	}
	if err := guardar(); err != nil {
		t.Fatalf("aberta: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por) VALUES ($1::uuid,'2026-09-01',1,'01',$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	var pf *dre.ErrPeriodoFechado
	if err := guardar(); !errors.As(err, &pf) || len(pf.Escopos) != 1 || pf.Escopos[0].Filial != "01" || pf.Escopos[0].Competencia != "2026-09" {
		t.Fatalf("fechada: %v", err)
	}
}

func TestChaveLockEstavel(t *testing.T) {
	a := dre.ChaveLock("e1", mustData("2026-09-01"))
	if a != dre.ChaveLock("e1", mustData("2026-09-15")) {
		t.Error("mesma competência deve dar a mesma chave")
	}
	if a == dre.ChaveLock("e2", mustData("2026-09-01")) || a == dre.ChaveLock("e1", mustData("2026-10-01")) {
		t.Error("chaves colidem")
	}
	if a != dre.ChaveLock("e1", mustData("2026-09-01")) {
		t.Error("instável")
	}
}

func TestArquivoMistoRecusadoInteiro(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por) VALUES ($1::uuid,'2026-09-01',1,'',$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	_, err := importar(t, db, f, "competencia;filial;conta;valor\n2026-08;01;3.1;1,00\n2026-09;01;3.1;2,00\n")
	var pf *dre.ErrPeriodoFechado
	if !errors.As(err, &pf) || len(pf.Escopos) != 1 || pf.Escopos[0].Competencia != "2026-09" {
		t.Fatalf("esperava ErrPeriodoFechado de 2026-09: %v", err)
	}
	if n := contar(t, db, total, f.EmpresaID); n != 0 {
		t.Errorf("gravou %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.cargas WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("cargas: %d", n)
	}
	// competência aberta importa
	if _, err := importar(t, db, f, "competencia;filial;conta;valor\n2026-08;01;3.1;1,00\n"); err != nil {
		t.Fatal(err)
	}
}

func TestFecharEsperaOLockDaGuarda(t *testing.T) {
	db := testdb.Open(t)
	f := empresaMapeada(t)
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := (dre.Periodo{}).Guardar(ctx, tx, f.EmpresaID, mustData("2026-09-01")); err != nil {
		t.Fatal(err)
	}
	feito := make(chan error, 1)
	go func() {
		_, err := (&dre.Fechamento{DB: db}).Fechar(ctx, dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9})
		feito <- err
	}()
	select {
	case err := <-feito:
		t.Fatalf("Fechar terminou antes do Commit: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-feito:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Fechar não terminou após o Commit")
	}
}

// Invariante: o snapshot de RB é igual ao somatório dos lançamentos de RB;
// ou a carga entrou antes do snapshot, ou foi recusada depois.
func TestCorridaFecharEImportar(t *testing.T) {
	db := testdb.Open(t)
	for i := 0; i < 10; i++ {
		f := empresaMapeada(t)
		var wg sync.WaitGroup
		var errF, errI error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, errF = (&dre.Fechamento{DB: db}).Fechar(context.Background(), dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9})
		}()
		go func() {
			defer wg.Done()
			_, errI = importarErr(db, f, "competencia;filial;conta;valor\n2026-09;01;3.1;90,00\n")
		}()
		wg.Wait()
		if errF != nil {
			t.Fatalf("iter %d: Fechar: %v", i, errF)
		}
		var pf *dre.ErrPeriodoFechado
		if errI != nil && !errors.As(errI, &pf) {
			t.Fatalf("iter %d: Importar: %v", i, errI)
		}
		var snap, lanc float64
		if err := db.QueryRow(`SELECT l.valor FROM auditoria.dre_fechamento_linhas l JOIN auditoria.dre_fechamentos c ON c.id=l.fechamento_id
			WHERE c.empresa_id=$1::uuid AND l.linha_codigo='RB'`, f.EmpresaID).Scan(&snap); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT COALESCE(SUM(valor),0) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND conta='3.1'`, f.EmpresaID).Scan(&lanc); err != nil {
			t.Fatal(err)
		}
		if snap != lanc {
			t.Fatalf("iter %d: snapshot %v != lançamentos %v (importar: %v)", i, snap, lanc, errI)
		}
		if errI == nil && lanc != 90 { // a carga substitui o escopo 01 de csv
			t.Fatalf("iter %d: carga aceita mas lançamentos=%v", i, lanc)
		}
		if errI != nil && lanc != 10 {
			t.Fatalf("iter %d: carga recusada mas lançamentos=%v", i, lanc)
		}
	}
}

func TestGuardaNormalizaDiaENullFilial(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por) VALUES ($1::uuid,'2026-09-01',1,'',$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	guardar := func(c string) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		return (dre.Periodo{}).Guardar(ctx, tx, f.EmpresaID, mustData(c))
	}
	var pf *dre.ErrPeriodoFechado
	if err := guardar("2026-09-15"); !errors.As(err, &pf) {
		t.Fatalf("dia 15 deveria enxergar o mês fechado: %v", err)
	}
	// filial NULL (se a coluna permitir) não pode quebrar o Scan
	if _, err := db.Exec(`ALTER TABLE auditoria.dre_fechamentos ALTER COLUMN filial DROP NOT NULL`); err != nil {
		t.Skipf("coluna filial não aceita NULL: %v", err)
	}
	// o gatilho de 1.7 recusa UPDATE; em teste só passa com a sessão de limpeza aberta
	liberado := func(q string, args ...any) error {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`SELECT set_config('auditoria.limpando_teste', $1, true)`, f.EmpresaID); err != nil {
			return err
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return err
		}
		return tx.Commit()
	}
	t.Cleanup(func() {
		liberado(`UPDATE auditoria.dre_fechamentos SET filial='' WHERE filial IS NULL`)
		db.Exec(`ALTER TABLE auditoria.dre_fechamentos ALTER COLUMN filial SET NOT NULL`)
	})
	if err := liberado(`UPDATE auditoria.dre_fechamentos SET filial=NULL WHERE empresa_id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if err := guardar("2026-09-01"); !errors.As(err, &pf) || pf.Escopos[0].Filial != "" {
		t.Fatalf("filial NULL: %v", err)
	}
}

func TestCompetenciaAusenteRecusada(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	_, err := (&dre.Ingestor{DB: db}).Importar(context.Background(), f.EmpresaID, f.UserID, fonteFixa{})
	var el *dre.ErroLinhas
	if !errors.As(err, &el) || !strings.Contains(err.Error(), "competência ausente") {
		t.Fatalf("esperava erro de competência ausente: %v", err)
	}
}

type fonteFixa struct{}

func (fonteFixa) Nome() string { return "fixa" }
func (fonteFixa) Ler(context.Context, dre.Escopo) ([]dre.Lancamento, error) {
	return []dre.Lancamento{{Linha: 2, Filial: "01", Conta: "3.1", Valor: 1}}, nil
}

func TestOrdemDosLocksSemDeadlock(t *testing.T) {
	db := testdb.Open(t)
	a := "competencia;filial;conta;valor\n2026-08;01;3.1;1,00\n2026-09;01;3.1;1,00\n"
	b := "competencia;filial;conta;valor\n2026-09;02;3.1;1,00\n2026-08;02;3.1;1,00\n"
	for i := 0; i < 20; i++ {
		f := testdb.NovaEmpresa(t, db)
		var wg sync.WaitGroup
		var ea, eb error
		wg.Add(2)
		go func() { defer wg.Done(); _, ea = importarErr(db, f, a) }()
		go func() { defer wg.Done(); _, eb = importarErr(db, f, b) }()
		wg.Wait()
		if ea != nil || eb != nil {
			t.Fatalf("iter %d: %v / %v", i, ea, eb)
		}
	}
}
