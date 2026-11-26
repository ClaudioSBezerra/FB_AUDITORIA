package dre_test

import (
	"context"
	"errors"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func TestRetirar_MesELiberaAContaParaOutraFonte(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	ing := &dre.Ingestor{DB: db}
	for _, comp := range []string{"2026-09-01", "2026-10-01"} {
		if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, origem)
			VALUES ($1::uuid,$2,'11','4010001',10,'csv.contas')`, f.EmpresaID, comp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO auditoria.conta_fonte (empresa_id, conta_padrao, origem) VALUES ($1::uuid,'4010001','csv.contas')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	n := func(q string) int { return contar(t, db, q, f.EmpresaID) }
	const lanc = `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`
	const donas = `SELECT COUNT(*) FROM auditoria.conta_fonte WHERE empresa_id=$1::uuid`

	p, err := ing.PreverRetirada(ctx, f.EmpresaID, "2026-09")
	if err != nil || p.Linhas != 1 || p.Confirmacao != "2026-09" || n(lanc) != 2 {
		t.Fatalf("prévia: %+v %v", p, err)
	}
	var entrada *dre.ErroEntrada
	if _, err := ing.Retirar(ctx, f.EmpresaID, f.UserID, "2026-09", "sim"); !errors.As(err, &entrada) || n(lanc) != 2 {
		t.Fatalf("confirmação errada deveria recusar: %v", err)
	}
	r, err := ing.Retirar(ctx, f.EmpresaID, f.UserID, "2026-09", "2026-09")
	if err != nil || r.Linhas != 1 || n(lanc) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	// ainda há lançamento da conta em outubro: a posse fica
	if r.ContasLiberadas != 0 || n(donas) != 1 {
		t.Errorf("posse liberada cedo demais: %+v", r)
	}
	r, err = ing.Retirar(ctx, f.EmpresaID, f.UserID, "", dre.ConfirmacaoRetirarTudo)
	if err != nil || n(lanc) != 0 || n(donas) != 0 || r.ContasLiberadas != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	// em produção, "tudo" é recusado (a empresa não volta para teste)
	if _, err := db.Exec(`UPDATE companies SET ambiente='producao' WHERE id=$1::uuid`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := ing.Retirar(ctx, f.EmpresaID, f.UserID, "", dre.ConfirmacaoRetirarTudo); !errors.As(err, &entrada) {
		t.Fatalf("tudo em produção deveria recusar: %v", err)
	}
}
