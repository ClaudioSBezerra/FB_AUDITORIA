package dre_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func guardarComp(t *testing.T, db *sql.DB, empresa, comp string) error {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	return dre.Periodo{}.Guardar(context.Background(), tx, empresa, mustData(comp))
}

func TestReabrirLiberaEscritaEFechaMaisUm(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	fc := &dre.Fechamento{DB: db}
	in := dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9}
	if _, err := fc.Fechar(ctx, in); err != nil {
		t.Fatal(err)
	}
	var pf *dre.ErrPeriodoFechado
	if err := guardarComp(t, db, f.EmpresaID, "2026-09-01"); !errors.As(err, &pf) {
		t.Fatalf("fechada: %v", err)
	}
	var jf *dre.ErrJaFechado
	if _, err := fc.Fechar(ctx, in); !errors.As(err, &jf) {
		t.Fatalf("esperava ErrJaFechado: %v", err)
	}
	if _, err := fc.Reabrir(ctx, dre.ReabrirIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9, Motivo: " \t "}); err == nil {
		t.Fatal("motivo em branco deveria ser recusado")
	}
	if _, err := fc.Reabrir(ctx, dre.ReabrirIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9, Motivo: "ajuste"}); err != nil {
		t.Fatal(err)
	}
	if err := guardarComp(t, db, f.EmpresaID, "2026-09-01"); err != nil {
		t.Fatalf("reaberta deveria aceitar escrita: %v", err)
	}
	var nf *dre.ErrNaoFechado
	if _, err := fc.Reabrir(ctx, dre.ReabrirIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9, Motivo: "x"}); !errors.As(err, &nf) {
		t.Fatalf("esperava ErrNaoFechado: %v", err)
	}
	out, err := fc.Fechar(ctx, in)
	if err != nil || out.Versao != 2 {
		t.Fatalf("refechar: %v %+v", err, out)
	}
	if err := guardarComp(t, db, f.EmpresaID, "2026-09-01"); !errors.As(err, &pf) {
		t.Fatalf("refechada: %v", err)
	}
	in.Filial = "01"
	var ee *dre.ErroEntrada
	if _, err := fc.Fechar(ctx, in); !errors.As(err, &ee) {
		t.Fatalf("filial deveria ser recusada: %v", err)
	}
}

func TestGatilhoFechamentoImutavel(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	fc := &dre.Fechamento{DB: db}
	if _, err := fc.Fechar(ctx, dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE auditoria.dre_fechamentos SET observacao='x' WHERE empresa_id=$1::uuid`,
		`UPDATE auditoria.dre_fechamentos SET reaberto_em=now() WHERE empresa_id=$1::uuid`, // sem motivo
		`UPDATE auditoria.dre_fechamentos SET reaberto_em=now(), motivo_reabertura='m', observacao='x' WHERE empresa_id=$1::uuid`,
		`DELETE FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`,
	} {
		if _, err := db.Exec(q, f.EmpresaID); err == nil {
			t.Errorf("deveria recusar: %s", q)
		}
	}
	if _, err := fc.Reabrir(ctx, dre.ReabrirIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9, Motivo: "m"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`UPDATE auditoria.dre_fechamentos SET motivo_reabertura='outro' WHERE empresa_id=$1::uuid`,
		`UPDATE auditoria.dre_fechamentos SET reaberto_em=now() WHERE empresa_id=$1::uuid`,
		`DELETE FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`,
	} {
		if _, err := db.Exec(q, f.EmpresaID); err == nil {
			t.Errorf("segunda mudança deveria ser recusada: %s", q)
		}
	}
	// em teste, com a sessão de limpeza, o DELETE passa
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT auditoria.limpar_dados_teste($1::uuid)`, f.EmpresaID); err != nil {
		t.Fatalf("limpeza em teste: %v", err)
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`, f.EmpresaID).Scan(&n); err != nil || n != 0 {
		t.Errorf("limpeza deveria apagar o fechamento: %d %v", n, err)
	}
}

func TestIndiceUnicoParcialUmAtivo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ins := `INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial) VALUES ($1::uuid,'2026-09-01',$2,$3)`
	if _, err := db.Exec(ins, f.EmpresaID, 1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ins, f.EmpresaID, 2, "01"); err == nil {
		t.Fatal("segundo ativo (por filial) deveria colidir com o índice")
	}
}

// reaplicarMigration roda a 006 dentro de uma transação revertida, depois de
// preparar os dados com `preparar` (que roda sem o índice único parcial).
func reaplicarMigration(t *testing.T, db *sql.DB, preparar func(tx *sql.Tx), conferir func(tx *sql.Tx, errMig error)) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DROP INDEX auditoria.ux_dre_fechamentos_ativo`); err != nil {
		t.Fatal(err)
	}
	preparar(tx)
	_, errMig := tx.Exec(testdb.SQLMigration(t, "006_fechamento_reabertura.sql"))
	if errMig != nil {
		// a transação ficou abortada: confere por savepoint não é possível; reabre outra
		tx.Rollback()
		tx2, err := db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx2.Rollback()
		conferir(tx2, errMig)
		return
	}
	conferir(tx, nil)
}

func inserirFech(t *testing.T, tx *sql.Tx, emp, comp string, versao int, filial string) {
	t.Helper()
	if _, err := tx.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial) VALUES ($1::uuid,$2,$3,$4)`, emp, comp, versao, filial); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationAbortaEmProducaoSemAlterarDado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	// tudo (inclusive a virada para producao, que não volta) fica numa transação revertida
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE companies SET ambiente='producao' WHERE id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`DROP INDEX auditoria.ux_dre_fechamentos_ativo`); err != nil {
		t.Fatal(err)
	}
	inserirFech(t, tx, f.EmpresaID, "2026-09-01", 1, "")
	inserirFech(t, tx, f.EmpresaID, "2026-09-01", 2, "")
	_, errMig := tx.Exec(testdb.SQLMigration(t, "006_fechamento_reabertura.sql"))
	if errMig == nil {
		t.Fatal("migration deveria abortar")
	}
	msg := strings.ToLower(errMig.Error())
	if strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate") {
		t.Errorf("mensagem contém palavra que o runner ignora: %s", msg)
	}
	if !strings.Contains(msg, f.EmpresaID) || !strings.Contains(msg, "2026-09") {
		t.Errorf("mensagem deveria apontar empresa e competência: %s", msg)
	}
}

func TestMigrationConsolidaEmpresaDeTeste(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	reaplicarMigration(t, db, func(tx *sql.Tx) {
		// na reaplicação o gatilho já existe; a sessão de limpeza o libera (na 1ª aplicação ele não existe)
		if _, err := tx.Exec(`SELECT set_config('auditoria.limpando_teste', $1, true)`, f.EmpresaID); err != nil {
			t.Fatal(err)
		}
		inserirFech(t, tx, f.EmpresaID, "2026-09-01", 1, "")
		inserirFech(t, tx, f.EmpresaID, "2026-09-01", 2, "01")
		inserirFech(t, tx, f.EmpresaID, "2026-09-01", 3, "")
		inserirFech(t, tx, f.EmpresaID, "2026-10-01", 1, "02") // sem conflito: segue ativo
		inserirFech(t, tx, f.EmpresaID, "2026-11-01", 1, "01") // só filiais: a de maior versão fica ativa
		inserirFech(t, tx, f.EmpresaID, "2026-11-01", 2, "02")
		inserirFech(t, tx, f.EmpresaID, "2026-11-01", 3, "01")
	}, func(tx *sql.Tx, errMig error) {
		if errMig != nil {
			t.Fatalf("migration: %v", errMig)
		}
		var ativos string
		if err := tx.QueryRow(`SELECT string_agg(competencia::text||'/'||versao, ',' ORDER BY competencia, versao) FROM auditoria.dre_fechamentos
			WHERE empresa_id=$1::uuid AND reaberto_em IS NULL`, f.EmpresaID).Scan(&ativos); err != nil {
			t.Fatal(err)
		}
		if ativos != "2026-09-01/3,2026-10-01/1,2026-11-01/3" {
			t.Errorf("ativos: %s", ativos)
		}
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid AND motivo_reabertura='consolidado na migração, ambiente de teste'`, f.EmpresaID).Scan(&n); err != nil || n != 4 {
			t.Errorf("consolidados: %d %v", n, err)
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`, f.EmpresaID).Scan(&n); err != nil || n != 7 {
			t.Errorf("nenhuma linha pode sumir: %d %v", n, err)
		}
	})
}
