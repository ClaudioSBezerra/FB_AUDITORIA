package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

type triResp struct {
	Colunas []struct {
		Rotulo        string   `json:"rotulo"`
		Vazio         bool     `json:"vazio"`
		Parcial       bool     `json:"parcial"`
		MesesFechados []string `json:"meses_fechados"`
		PctStatus     string   `json:"pct_status"`
	} `json:"colunas"`
	Linhas []struct {
		Codigo  string `json:"codigo"`
		Tipo    string `json:"tipo"`
		Valores []struct {
			Rotulo      string   `json:"rotulo"`
			Realizado   float64  `json:"realizado"`
			Orcado      float64  `json:"orcado"`
			AnoAnterior float64  `json:"ano_anterior"`
			PctFat      *float64 `json:"pct_faturamento"`
		} `json:"valores"`
	} `json:"linhas"`
}

func chamarTri(t *testing.T, f testdb.Fixture, perfil, url string) (int, triResp) {
	t.Helper()
	w := chamar(DRETrimestresHandler(testdb.Open(t)), reqFilial(http.MethodGet, url, "", f, perfil))
	var v triResp
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, v
}

// val devolve o realizado da linha na coluna (T1..T4, Ano).
func (v triResp) val(linha, rotulo string) float64 {
	for _, l := range v.Linhas {
		if l.Codigo != linha {
			continue
		}
		for _, x := range l.Valores {
			if x.Rotulo == rotulo {
				return x.Realizado
			}
		}
	}
	return -999999
}

func (v triResp) pct(linha, rotulo string) *float64 {
	for _, l := range v.Linhas {
		if l.Codigo == linha {
			for _, x := range l.Valores {
				if x.Rotulo == rotulo {
					return x.PctFat
				}
			}
		}
	}
	return nil
}

func (v triResp) col(rotulo string) int {
	for i, c := range v.Colunas {
		if c.Rotulo == rotulo {
			return i
		}
	}
	return -1
}

// Cenário (fictício) de 2025: RB (3.1), DED (3.2), CMV (4.1); FL = RB - DED.
func cenarioTri(t *testing.T, f testdb.Fixture) {
	t.Helper()
	estruturaPct(t, f, true)
	for _, l := range []struct {
		comp, fil, conta string
		v                float64
	}{
		{"2025-01-01", "01", "3.1", 100}, {"2025-02-01", "01", "3.1", 200}, {"2025-03-01", "02", "3.1", 300},
		{"2025-01-01", "01", "3.2", 10}, {"2025-03-01", "02", "3.2", 30},
		{"2025-02-01", "01", "4.1", 60},
		{"2025-04-01", "01", "3.1", 1000}, {"2025-05-01", "01", "4.1", 100},
		{"2024-02-01", "01", "3.1", 50}, // ano anterior de fev/2025
	} {
		lancPct(t, f, l.comp, l.fil, l.conta, l.v)
	}
	exec(t, `INSERT INTO auditoria.dre_orcado (empresa_id, competencia, filial, linha_codigo, valor) VALUES
		($1::uuid,'2025-01-01','01','RB',500),($1::uuid,'2025-03-01','01','RB',700)`, f.EmpresaID)
}

func TestTrimestres_SomaDeTresMesesSubtotaisEPercentual(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioTri(t, f)
	code, v := chamarTri(t, f, "controladoria", "/x?ano=2025")
	if code != 200 || len(v.Colunas) != 5 {
		t.Fatalf("%d %+v", code, v.Colunas)
	}
	// T1: RB 600, DED 40, FL 560, CMV 60, LB 500.
	for linha, want := range map[string]float64{"RB": 600, "DED": 40, "FL": 560, "CMV": 60, "LB": 500} {
		if g := v.val(linha, "T1"); g != want {
			t.Errorf("T1 %s = %v (queria %v)", linha, g, want)
		}
	}
	if v.val("RB", "T2") != 1000 || v.val("FL", "T2") != 1000 || v.val("CMV", "T2") != 100 || v.val("RB", "T3") != 0 {
		t.Errorf("T2/T3: %v %v %v %v", v.val("RB", "T2"), v.val("FL", "T2"), v.val("CMV", "T2"), v.val("RB", "T3"))
	}
	// Ano = soma dos trimestres, subtotal = soma das linhas, em toda coluna.
	for _, l := range v.Linhas {
		var s float64
		for _, r := range []string{"T1", "T2", "T3", "T4"} {
			s += v.val(l.Codigo, r)
		}
		if g := v.val(l.Codigo, "Ano"); g != s {
			t.Errorf("Ano %s = %v, soma dos trimestres %v", l.Codigo, g, s)
		}
	}
	for _, r := range []string{"T1", "T2", "T3", "T4", "Ano"} {
		if v.val("FL", r) != v.val("RB", r)-v.val("DED", r) || v.val("LB", r) != v.val("FL", r)-v.val("CMV", r) {
			t.Errorf("subtotais em %s", r)
		}
	}
	// Percentual sobre o FL do próprio trimestre: T1 CMV 60/560, T2 CMV 100/1000.
	if p := v.pct("CMV", "T1"); p == nil || !proximo(*p, 60.0/560*100) {
		t.Errorf("pct T1 = %v", p)
	}
	if p := v.pct("CMV", "T2"); p == nil || *p != 10 {
		t.Errorf("pct T2 = %v", p)
	}
	if p := v.pct("FL", "T1"); p == nil || *p != 100 {
		t.Errorf("FL%% T1 = %v", p)
	}
	if p := v.pct("CMV", "T3"); p != nil {
		t.Errorf("T3 sem faturamento deveria ser nulo: %v", *p)
	}
	// Orçado e ano anterior.
	for _, l := range v.Linhas {
		if l.Codigo == "RB" {
			if l.Valores[0].Orcado != 1200 || l.Valores[0].AnoAnterior != 50 {
				t.Errorf("RB T1 orçado/AA: %+v", l.Valores[0])
			}
		}
	}
	// Todo mês aberto: parcial, sem meses fechados.
	if c := v.Colunas[v.col("T1")]; !c.Parcial || len(c.MesesFechados) != 0 {
		t.Errorf("T1 aberto: %+v", c)
	}
	// Filial 02 só tem março.
	_, v2 := chamarTri(t, f, "diretoria", "/x?ano=2025&filial=02")
	if v2.val("RB", "T1") != 300 || v2.val("RB", "T2") != 0 {
		t.Errorf("filial 02: %v %v", v2.val("RB", "T1"), v2.val("RB", "T2"))
	}
}

func TestTrimestres_FechadoVemDoSnapshotMesAMes(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioTri(t, f)
	for _, m := range []string{"1", "2", "3"} {
		if w := fechar(t, f, `{"ano":2025,"mes":`+m+`}`); w.Code != http.StatusCreated {
			t.Fatalf("fechar %s: %d %s", m, w.Code, w.Body.String())
		}
	}
	_, antes := chamarTri(t, f, "controladoria", "/x?ano=2025")
	if c := antes.Colunas[antes.col("T1")]; c.Parcial || len(c.MesesFechados) != 3 {
		t.Fatalf("T1 fechado: %+v", c)
	}
	if c := antes.Colunas[antes.col("T2")]; !c.Parcial || len(c.MesesFechados) != 0 {
		t.Fatalf("T2: %+v", c)
	}
	// Mapa e estrutura mudam depois; lançamento novo por fora da guarda.
	exec(t, `UPDATE auditoria.dre_mapa SET linha_codigo='CMV' WHERE empresa_id=$1::uuid AND conta_padrao='3.1'`, f.EmpresaID)
	exec(t, `UPDATE auditoria.dre_linhas SET soma_de='{RB}' WHERE empresa_id=$1::uuid AND codigo='FL'`, f.EmpresaID)
	lancPct(t, f, "2025-01-01", "01", "3.1", 9999)
	_, depois := chamarTri(t, f, "controladoria", "/x?ano=2025")
	for _, l := range antes.Linhas {
		for _, r := range []string{"T1"} {
			if a, d := antes.val(l.Codigo, r), depois.val(l.Codigo, r); a != d {
				t.Errorf("%s %s mudou: %v -> %v", l.Codigo, r, a, d)
			}
		}
	}
	if p, q := antes.pct("CMV", "T1"), depois.pct("CMV", "T1"); p == nil || q == nil || *p != *q {
		t.Errorf("pct T1 mudou: %v %v", p, q)
	}
	// Agrupamento e Grupo seguem as mesmas regras de exclusão do DRE.
	if code, _ := chamarTri(t, f, "controladoria", "/x?ano=2025&filial=01&grupo=1"); code != http.StatusBadRequest {
		t.Errorf("filial+grupo: %d", code)
	}
	if code, _ := chamarTri(t, f, "controladoria", "/x?ano=2025&agrupamento=ZZ"); code != http.StatusNotFound {
		t.Errorf("agrupamento inexistente: %d", code)
	}
	if code, _ := chamarTri(t, f, "controladoria", "/x?ano=1"); code != http.StatusBadRequest {
		t.Errorf("ano inválido: %d", code)
	}
}

func TestTrimestres_FuturoVazioEGestor403(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioTri(t, f)
	code, v := chamarTri(t, f, "controladoria", "/x?ano=2099")
	if code != 200 || len(v.Colunas) != 5 {
		t.Fatalf("futuro: %d", code)
	}
	for _, c := range v.Colunas {
		if !c.Vazio || c.Parcial {
			t.Errorf("coluna %s: %+v", c.Rotulo, c)
		}
	}
	for _, perfil := range []string{"gestor", "", "qualquer"} {
		if code, _ := chamarTri(t, f, perfil, "/x?ano=2025"); code != http.StatusForbidden {
			t.Errorf("perfil %q: %d", perfil, code)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_trimestres' AND acao='negado'`, f.EmpresaID); n != 3 {
		t.Errorf("negativas = %d", n)
	}
}

// Ano em curso: o trimestre que ainda não começou vem vazio (hoje controlado).
func TestTrimestres_AnoEmCursoTrimestreFuturoVazio(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioTri(t, f)
	linhas, err := dre.CarregarLinhas(context.Background(), db, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	mapa, err := dre.CarregarMapa(context.Background(), db, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	cols, err := (dre.Leitor{Q: db}).LerTrimestres(context.Background(), f.EmpresaID, []string{""}, 2025, time.Date(2025, 5, 15, 0, 0, 0, 0, time.UTC), linhas, mapa)
	if err != nil {
		t.Fatal(err)
	}
	vazios := map[string]bool{}
	for _, c := range cols {
		vazios[c.Rotulo] = c.Vazio
	}
	if vazios["T1"] || vazios["T2"] || !vazios["T3"] || !vazios["T4"] || vazios["Ano"] {
		t.Errorf("vazios: %v", vazios)
	}
	if cols[1].Real["RB"] != 1000 || cols[4].Real["RB"] != 1600 {
		t.Errorf("T2/Ano RB: %v %v", cols[1].Real["RB"], cols[4].Real["RB"])
	}
}
