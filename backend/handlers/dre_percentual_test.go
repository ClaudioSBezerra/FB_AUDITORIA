package handlers

import (
	"math"
	"net/http"
	"testing"

	"fb_auditoria/internal/testdb"
)

// estruturaPct: RB, DED, FL (= RB - DED), CMV e LB (= FL - CMV); mapa 3.1->RB, 3.2->DED, 4.1->CMV.
func estruturaPct(t *testing.T, f testdb.Fixture, comPapel bool) {
	t.Helper()
	for _, l := range [][]any{
		{"RB", "Receita", 1, 10, "conta", "{}"}, {"DED", "Deduções", 1, 20, "conta", "{}"},
		{"FL", "= Faturamento Líquido", 0, 30, "subtotal", "{RB,-DED}"}, {"CMV", "Custo", 1, 40, "conta", "{}"},
		{"LB", "= Lucro Bruto", 0, 50, "subtotal", "{FL,-CMV}"},
	} {
		exec(t, `INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de) VALUES ($1::uuid,$2,$3,$4,$5,$6,$7)`,
			f.EmpresaID, l[0], l[1], l[2], l[3], l[4], l[5])
	}
	if comPapel {
		exec(t, `UPDATE auditoria.dre_linhas SET papel='faturamento_liquido' WHERE empresa_id=$1::uuid AND codigo='FL'`, f.EmpresaID)
	}
	for _, m := range [][]any{{"3.1", "RB"}, {"3.2", "DED"}, {"4.1", "CMV"}} {
		exec(t, `INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,$2,$3,1)`, f.EmpresaID, m[0], m[1])
	}
}

func lancPct(t *testing.T, f testdb.Fixture, comp, filial, conta string, valor float64) {
	t.Helper()
	exec(t, `INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,$2,$3,$4,$5)`,
		f.EmpresaID, comp, filial, conta, valor)
}

func linhasPct(t *testing.T, o map[string]any) map[string]map[string]any {
	t.Helper()
	out := map[string]map[string]any{}
	for _, x := range o["linhas"].([]any) {
		m := x.(map[string]any)
		out[m["codigo"].(string)] = m
	}
	return out
}

func proximo(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestPct_LinhasSubtotaisECentavos(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	// Várias linhas na mesma filial e conta (várias UFs) somam; centavos que estourariam em float.
	lancPct(t, f, "2026-09-01", "01", "3.1", 0.10)
	lancPct(t, f, "2026-09-01", "01", "3.1", 0.20)
	lancPct(t, f, "2026-09-01", "01", "3.2", 0.07)
	lancPct(t, f, "2026-09-01", "01", "4.1", 0.06)
	lancPct(t, f, "2026-09-01", "02", "3.1", 100)
	o := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal&filial=01")
	if o["pct_status"] != "ok" || o["faturamento_liquido_linha"] != "FL" {
		t.Fatalf("status: %v %v", o["pct_status"], o["faturamento_liquido_linha"])
	}
	l := linhasPct(t, o)
	if l["RB"]["realizado"].(float64) != 0.30 {
		t.Errorf("RB = %v, esperado 0.30 exato", l["RB"]["realizado"])
	}
	if l["FL"]["realizado"].(float64) != 0.23 || l["LB"]["realizado"].(float64) != 0.17 {
		t.Errorf("subtotais: FL=%v LB=%v", l["FL"]["realizado"], l["LB"]["realizado"])
	}
	if l["FL"]["pct_faturamento"].(float64) != 100 {
		t.Errorf("FL%% = %v", l["FL"]["pct_faturamento"])
	}
	if !proximo(l["CMV"]["pct_faturamento"].(float64), 0.06/0.23*100) || !proximo(l["RB"]["pct_faturamento"].(float64), 0.30/0.23*100) {
		t.Errorf("percentuais: CMV=%v RB=%v", l["CMV"]["pct_faturamento"], l["RB"]["pct_faturamento"])
	}
	// Consolidado soma as filiais.
	c := linhasPct(t, jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal"))
	if c["RB"]["realizado"].(float64) != 100.30 {
		t.Errorf("consolidado RB = %v", c["RB"]["realizado"])
	}
}

func TestPct_Acumulado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	lancPct(t, f, "2026-08-01", "01", "3.1", 100)
	lancPct(t, f, "2026-09-01", "01", "3.1", 100)
	lancPct(t, f, "2026-09-01", "01", "4.1", 50)
	l := linhasPct(t, jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=acumulada"))
	if l["CMV"]["pct_faturamento"].(float64) != 25 {
		t.Errorf("CMV%% acumulado = %v", l["CMV"]["pct_faturamento"])
	}
}

func TestPct_FaturamentoZeroNaoCalculavel(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	lancPct(t, f, "2026-09-01", "01", "4.1", 50)
	o := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal")
	if o["pct_status"] != "zero" {
		t.Fatalf("status = %v", o["pct_status"])
	}
	for c, m := range linhasPct(t, o) {
		if m["pct_faturamento"] != nil {
			t.Errorf("%s: pct = %v, esperado null", c, m["pct_faturamento"])
		}
	}
}

func TestPct_SemPapelAvisaENaoCalcula(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, false)
	lancPct(t, f, "2026-09-01", "01", "3.1", 100)
	o := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal")
	if o["pct_status"] != "sem_papel" || o["aviso_faturamento"] == "" {
		t.Fatalf("status = %v aviso = %v", o["pct_status"], o["aviso_faturamento"])
	}
	for c, m := range linhasPct(t, o) {
		if m["pct_faturamento"] != nil {
			t.Errorf("%s: pct = %v, esperado null", c, m["pct_faturamento"])
		}
	}
}

func TestPct_MesFechadoUsaSnapshotInclusivePercentuais(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	lancPct(t, f, "2026-09-01", "01", "3.1", 200)
	lancPct(t, f, "2026-09-01", "01", "4.1", 50)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamento_linhas l JOIN auditoria.dre_fechamentos x ON x.id=l.fechamento_id
		WHERE x.empresa_id=$1::uuid AND l.papel='faturamento_liquido'`, f.EmpresaID); n == 0 {
		t.Fatal("papel não foi congelado no snapshot")
	}
	antes := linhasPct(t, jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal"))
	// Mexe em tudo que é vivo: lançamento, papel e estrutura.
	lancPct(t, f, "2026-09-01", "01", "3.1", 800)
	exec(t, `UPDATE auditoria.dre_linhas SET papel=NULL WHERE empresa_id=$1::uuid`, f.EmpresaID)
	exec(t, `UPDATE auditoria.dre_linhas SET papel='faturamento_liquido' WHERE empresa_id=$1::uuid AND codigo='RB'`, f.EmpresaID)
	o := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal")
	depois := linhasPct(t, o)
	if o["faturamento_liquido_linha"] != "FL" {
		t.Errorf("linha-base mudou: %v", o["faturamento_liquido_linha"])
	}
	for c, m := range antes {
		if depois[c]["pct_faturamento"] != m["pct_faturamento"] || depois[c]["realizado"] != m["realizado"] {
			t.Errorf("%s mudou depois de fechado: %v -> %v", c, m, depois[c])
		}
	}
	if antes["CMV"]["pct_faturamento"].(float64) != 25 {
		t.Errorf("CMV%% = %v", antes["CMV"]["pct_faturamento"])
	}
}

func TestPct_FechamentoAnteriorSemPapelUsaPapelVivo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	lancPct(t, f, "2026-09-01", "01", "3.1", 200)
	lancPct(t, f, "2026-09-01", "01", "4.1", 50)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	// Simula fechamento gravado antes da 3.4: sem papel no snapshot.
	exec(t, `SELECT set_config('auditoria.limpando_teste', $1, false)`, f.EmpresaID)
	exec(t, `UPDATE auditoria.dre_fechamento_linhas SET papel=NULL WHERE fechamento_id IN (SELECT id FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid)`, f.EmpresaID)
	o := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal")
	if o["pct_status"] != "ok" || linhasPct(t, o)["CMV"]["pct_faturamento"].(float64) != 25 {
		t.Errorf("fallback falhou: %v", o["pct_status"])
	}
}

func reqPapel(f testdb.Fixture, perfil, corpo string) *http.Request {
	return reqFilial(http.MethodPut, "/api/aud/dre/linhas/papel", corpo, f, perfil)
}

func TestPct_DefinirPapel_UnicoPorEmpresaETrilha(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, false)
	estruturaPct(t, outra, true)
	h := DRELinhaPapelHandler(db)

	if w := chamar(h, reqPapel(f, "diretoria", `{"codigo":"FL","papel":"faturamento_liquido"}`)); w.Code != http.StatusForbidden {
		t.Errorf("diretoria: %d", w.Code)
	}
	if w := chamar(h, reqPapel(f, "controladoria", `{"codigo":"FL","papel":"outro"}`)); w.Code != http.StatusBadRequest {
		t.Errorf("papel inválido: %d", w.Code)
	}
	if w := chamar(h, reqPapel(f, "controladoria", `{"codigo":"XX","papel":"faturamento_liquido"}`)); w.Code != http.StatusNotFound {
		t.Errorf("linha inexistente: %d", w.Code)
	}
	if w := chamar(h, reqPapel(f, "controladoria", `{"codigo":"FL","papel":"faturamento_liquido"}`)); w.Code != http.StatusOK {
		t.Fatalf("definir: %d %s", w.Code, w.Body.String())
	}
	// Trocar de linha tira a marca anterior; só uma por empresa.
	if w := chamar(h, reqPapel(f, "controladoria", `{"codigo":"RB","papel":"faturamento_liquido"}`)); w.Code != http.StatusOK {
		t.Fatalf("trocar: %d", w.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND papel IS NOT NULL`, f.EmpresaID); n != 1 {
		t.Errorf("linhas com papel = %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND codigo='RB' AND papel='faturamento_liquido'`, f.EmpresaID); n != 1 {
		t.Error("RB deveria ter o papel")
	}
	// A outra empresa não foi afetada.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND codigo='FL' AND papel='faturamento_liquido'`, outra.EmpresaID); n != 1 {
		t.Error("outra empresa perdeu o papel")
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_linha' AND acao='definir_papel'`, f.EmpresaID); n != 2 {
		t.Errorf("trilha = %d", n)
	}
	// Desmarcar.
	if w := chamar(h, reqPapel(f, "controladoria", `{"codigo":"RB","papel":""}`)); w.Code != http.StatusOK {
		t.Fatalf("desmarcar: %d", w.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND papel IS NOT NULL`, f.EmpresaID); n != 0 {
		t.Errorf("sobrou papel: %d", n)
	}
	// Banco recusa duas marcas na mesma empresa.
	if _, err := db.Exec(`UPDATE auditoria.dre_linhas SET papel='faturamento_liquido' WHERE empresa_id=$1::uuid AND codigo IN ('RB','FL')`, f.EmpresaID); err == nil {
		t.Error("índice único parcial não recusou")
	}
}
