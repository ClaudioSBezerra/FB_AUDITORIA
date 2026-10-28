package dre_test

import (
	"context"
	"database/sql"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

// empresaSnapshot: linhas RB, CMV e LB (= RB - CMV); mapa 3.1->RB e 4.1->CMV;
// 2026-09 nas filiais 01 e 02, 2025-09 na 01 e orçado de 2026-09.
func empresaSnapshot(t *testing.T) (*sql.DB, testdb.Fixture) {
	t.Helper()
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for _, q := range []string{
		`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de) VALUES ($1::uuid,'RB','Receita',1,10,'conta','{}')`,
		`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de) VALUES ($1::uuid,'CMV','Custo',1,20,'conta','{}')`,
		`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de) VALUES ($1::uuid,'LB','Lucro bruto',0,30,'subtotal','{RB,-CMV}')`,
		`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'3.1','RB',1)`,
		`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'4.1','CMV',1)`,
		`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2026-09-01','01','3.1',1000)`,
		`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2026-09-01','01','4.1',300)`,
		`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2026-09-01','02','3.1',500)`,
		`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2025-09-01','01','3.1',800)`,
		`INSERT INTO auditoria.dre_orcado (empresa_id, competencia, filial, linha_codigo, valor) VALUES ($1::uuid,'2026-09-01','01','RB',700)`,
		`INSERT INTO auditoria.dre_orcado (empresa_id, competencia, filial, linha_codigo, valor) VALUES ($1::uuid,'2026-09-01','02','RB',500)`,
	} {
		if _, err := db.Exec(q, f.EmpresaID); err != nil {
			t.Fatal(err)
		}
	}
	return db, f
}

func ler(t *testing.T, db *sql.DB, f testdb.Fixture, filial, ini, fim string) dre.Leitura {
	t.Helper()
	ctx := context.Background()
	linhas, err := dre.CarregarLinhas(ctx, db, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	mapa, err := dre.CarregarMapa(ctx, db, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := (dre.Leitor{Q: db}).Ler(ctx, f.EmpresaID, filial, dre.Periodo{Ini: mustData(ini), Fim: mustData(fim)}, linhas, mapa)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func esperar(t *testing.T, rotulo string, got map[string]float64, want map[string]float64) {
	t.Helper()
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %s = %v, esperado %v", rotulo, k, got[k], v)
		}
	}
}

func fecharSet(t *testing.T, db *sql.DB, f testdb.Fixture) {
	t.Helper()
	if _, err := fechar(t, f, 9); err != nil {
		t.Fatal(err)
	}
}

func TestMesFechadoLidoDoSnapshotMesmoMudandoMapaELancamentos(t *testing.T) {
	db, f := empresaSnapshot(t)
	fecharSet(t, db, f)
	antes := ler(t, db, f, "", "2026-09-01", "2026-09-01")
	esperar(t, "consolidado real", antes.Real, map[string]float64{"RB": 1500, "CMV": 300, "LB": 1200})
	esperar(t, "consolidado orcado", antes.Orc, map[string]float64{"RB": 1200, "LB": 1200})
	esperar(t, "consolidado aa", antes.AA, map[string]float64{"RB": 800, "LB": 800})
	f01 := ler(t, db, f, "01", "2026-09-01", "2026-09-01")
	esperar(t, "filial 01 real", f01.Real, map[string]float64{"RB": 1000, "CMV": 300, "LB": 700})
	esperar(t, "filial 01 orcado", f01.Orc, map[string]float64{"RB": 700})
	esperar(t, "filial 01 aa", f01.AA, map[string]float64{"RB": 800})

	// muda mapa, estrutura, lançamentos e orçado: o mês fechado não muda
	for _, q := range []string{
		`UPDATE auditoria.dre_mapa SET linha_codigo='CMV' WHERE empresa_id=$1::uuid AND conta_padrao='3.1'`,
		`DELETE FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND codigo='LB'`,
		`UPDATE auditoria.lancamentos SET valor=1 WHERE empresa_id=$1::uuid`,
		`UPDATE auditoria.dre_orcado SET valor=1 WHERE empresa_id=$1::uuid`,
	} {
		if _, err := db.Exec(q, f.EmpresaID); err != nil {
			t.Fatal(err)
		}
	}
	depois := ler(t, db, f, "", "2026-09-01", "2026-09-01")
	esperar(t, "depois real", depois.Real, map[string]float64{"RB": 1500, "CMV": 300, "LB": 1200})
	esperar(t, "depois orcado", depois.Orc, map[string]float64{"RB": 1200})
	esperar(t, "depois aa", depois.AA, map[string]float64{"RB": 800})
	if len(depois.Sem) != 0 {
		t.Errorf("mês fechado não deveria consultar lançamentos: sem=%v", depois.Sem)
	}
	d01 := ler(t, db, f, "01", "2026-09-01", "2026-09-01")
	esperar(t, "filial 01 depois", d01.Real, map[string]float64{"RB": 1000, "CMV": 300})

	// detalhe por conta vem do snapshot
	l := dre.Leitor{Q: db}
	contas, sem, err := l.ContasCongeladas(context.Background(), "", depois.Ativos, map[string]bool{"RB": true, "CMV": true})
	if err != nil || len(sem) != 0 {
		t.Fatalf("contas: %v %v", err, sem)
	}
	got := map[string]float64{}
	for _, c := range contas {
		got[c.Conta+"/"+c.Linha] = c.Valor
	}
	if len(got) != 2 || got["3.1/RB"] != 1500 || got["4.1/CMV"] != 300 {
		t.Errorf("contas congeladas: %v", got)
	}
}

func TestAcumuladoMistoSomaSnapshotEVivo(t *testing.T) {
	db, f := empresaSnapshot(t)
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2026-08-01','01','3.1',100)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	fecharSet(t, db, f)
	r := ler(t, db, f, "", "2026-01-01", "2026-09-01")
	esperar(t, "misto", r.Real, map[string]float64{"RB": 1600, "CMV": 300, "LB": 1300})
	// o mapa muda: agosto (aberto) acompanha, setembro (fechado) não
	if _, err := db.Exec(`UPDATE auditoria.dre_mapa SET linha_codigo='CMV' WHERE empresa_id=$1::uuid AND conta_padrao='3.1'`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	r = ler(t, db, f, "", "2026-01-01", "2026-09-01")
	esperar(t, "misto após mapa", r.Real, map[string]float64{"RB": 1500, "CMV": 400})
}

func TestReabertoVoltaAoVivo(t *testing.T) {
	db, f := empresaSnapshot(t)
	fecharSet(t, db, f)
	if _, err := (&dre.Fechamento{DB: db}).Reabrir(context.Background(), dre.ReabrirIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9, Motivo: "ajuste"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2026-09-01','01','3.1',10)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	r := ler(t, db, f, "", "2026-09-01", "2026-09-01")
	esperar(t, "reaberto", r.Real, map[string]float64{"RB": 1510})
	if len(r.Ativos) != 0 {
		t.Errorf("sem ativo esperado: %v", r.Ativos)
	}
}

func TestSnapshotImutavelELimpezaApaga(t *testing.T) {
	db, f := empresaSnapshot(t)
	fecharSet(t, db, f)
	for _, q := range []string{
		`UPDATE auditoria.dre_fechamento_linhas SET valor=0 WHERE fechamento_id IN (SELECT id FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid)`,
		`DELETE FROM auditoria.dre_fechamento_linhas WHERE fechamento_id IN (SELECT id FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid)`,
		`UPDATE auditoria.dre_fechamento_contas SET realizado=0 WHERE empresa_id=$1::uuid`,
		`DELETE FROM auditoria.dre_fechamento_contas WHERE empresa_id=$1::uuid`,
	} {
		if _, err := db.Exec(q, f.EmpresaID); err == nil {
			t.Errorf("deveria recusar: %s", q)
		}
	}
	if contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamento_contas WHERE empresa_id=$1::uuid`, f.EmpresaID) == 0 {
		t.Fatal("fechar deveria gravar contas")
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SELECT auditoria.limpar_dados_teste($1::uuid)`, f.EmpresaID); err != nil {
		t.Fatalf("limpeza: %v", err)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM auditoria.dre_fechamento_contas WHERE empresa_id=$1::uuid`,
		`SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`,
		`SELECT COUNT(*) FROM auditoria.dre_fechamento_linhas WHERE fechamento_id IN (SELECT id FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid)`,
	} {
		var n int
		if err := tx.QueryRow(q, f.EmpresaID).Scan(&n); err != nil || n != 0 {
			t.Errorf("%s: %d %v", q, n, err)
		}
	}
}

func TestFechamentoAntigoSoComLinhas(t *testing.T) {
	db, f := empresaSnapshot(t)
	var id int64
	if err := db.QueryRow(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial) VALUES ($1::uuid,'2026-09-01',1,'') RETURNING id`, f.EmpresaID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamento_linhas (fechamento_id, linha_codigo, descricao, nivel, ordem, valor) VALUES ($1,'RB','Receita',1,10,42)`, id); err != nil {
		t.Fatal(err)
	}
	r := ler(t, db, f, "", "2026-09-01", "2026-09-01")
	esperar(t, "antigo", r.Real, map[string]float64{"RB": 42})
	esperar(t, "antigo orcado ao vivo", r.Orc, map[string]float64{"RB": 1200})
	_, sem, err := (dre.Leitor{Q: db}).ContasCongeladas(context.Background(), "", r.Ativos, map[string]bool{"RB": true})
	if err != nil || len(sem) != 1 || sem[0] != "2026-09" {
		t.Errorf("fechamento antigo sem contas: %v %v", sem, err)
	}
}

// Filial só com lançamento no ano anterior e filial só com orçado no mês entram
// no snapshot: depois de mudar os dados vivos, a leitura devolve o congelado.
func TestFiliaisSoAnoAnteriorESoOrcadoNoSnapshot(t *testing.T) {
	db, f := empresaSnapshot(t)
	for _, q := range []string{
		`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2025-09-01','03','3.1',250)`,
		`INSERT INTO auditoria.dre_orcado (empresa_id, competencia, filial, linha_codigo, valor) VALUES ($1::uuid,'2026-09-01','04','RB',333)`,
	} {
		if _, err := db.Exec(q, f.EmpresaID); err != nil {
			t.Fatal(err)
		}
	}
	fecharSet(t, db, f)
	for _, q := range []string{
		`UPDATE auditoria.lancamentos SET valor=1 WHERE empresa_id=$1::uuid`,
		`UPDATE auditoria.dre_orcado SET valor=1 WHERE empresa_id=$1::uuid`,
	} {
		if _, err := db.Exec(q, f.EmpresaID); err != nil {
			t.Fatal(err)
		}
	}
	r3 := ler(t, db, f, "03", "2026-09-01", "2026-09-01")
	esperar(t, "filial 03 aa", r3.AA, map[string]float64{"RB": 250})
	r4 := ler(t, db, f, "04", "2026-09-01", "2026-09-01")
	esperar(t, "filial 04 orcado", r4.Orc, map[string]float64{"RB": 333})
}
