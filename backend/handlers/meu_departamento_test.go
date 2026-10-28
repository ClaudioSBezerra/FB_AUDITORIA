package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"
)

// História 4.8: "Meu departamento". Cenário fictício de cenarioDep (set/2026):
// D1 = 3.1 (1200) + 4.1 (100); D2 = 4.2 (-30). Faturamento Líquido = 1000.
// Acrescenta o ano anterior (set/2025): 3.1 = 1000 e 4.1 = 50; 4.2 sem movimento.

type contaMeu struct {
	Conta        string   `json:"conta"`
	Realizado    float64  `json:"realizado"`
	Orcado       *float64 `json:"orcado"`
	AnoAnterior  *float64 `json:"ano_anterior"`
	VarOrcado    *float64 `json:"var_orcado_valor"`
	VarOrcadoPct *float64 `json:"var_orcado_pct"`
	VarAA        *float64 `json:"var_ano_anterior_valor"`
	VarAAPct     *float64 `json:"var_ano_anterior_pct"`
	Pct          *float64 `json:"pct_faturamento"`
}

type respMeu struct {
	PctStatus     string   `json:"pct_status"`
	Avisos        []string `json:"avisos"`
	Departamentos []struct {
		Codigo string     `json:"codigo"`
		Totais contaMeu   `json:"totais"`
		Contas []contaMeu `json:"contas"`
	} `json:"departamentos"`
	Total contaMeu `json:"total"`
}

func cenarioMeu(t *testing.T, f testdb.Fixture) {
	t.Helper()
	cenarioDep(t, f)
	lancPct(t, f, "2025-09-01", "01", "3.1", 1000)
	lancPct(t, f, "2025-09-01", "01", "4.1", 50)
}

func meuDe(t *testing.T, db interface{}, f testdb.Fixture, gid, url string) (int, respMeu, string) {
	t.Helper()
	w := chamar(MeuDepartamentoHandler(testdb.Open(t)), reqDe(http.MethodGet, url, f.EmpresaID, gid, "gestor"))
	var v respMeu
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, v, w.Body.String()
}

func contaDe(v respMeu, depto, conta string) (contaMeu, bool) {
	for _, d := range v.Departamentos {
		if d.Codigo != depto {
			continue
		}
		for _, c := range d.Contas {
			if c.Conta == conta {
				return c, true
			}
		}
	}
	return contaMeu{}, false
}

func codigosMeu(v respMeu) string {
	var cs []string
	for _, d := range v.Departamentos {
		cs = append(cs, d.Codigo)
	}
	return strings.Join(cs, ",")
}

func casas(p *float64) float64 {
	if p == nil {
		return -999999
	}
	return float64(int64(*p*100+0.5)) / 100
}

func TestMeuDepartamento_GerenteComAeBVeSoAeBComVariacao(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioMeu(t, f)
	gid := gestorCom(t, db, f, "D1", "D2")
	code, v, corpo := meuDe(t, db, f, gid, "/x?ano=2026&mes=9&visao=mensal")
	if code != http.StatusOK || codigosMeu(v) != "D1,D2" {
		t.Fatalf("%d %s", code, codigosMeu(v))
	}
	// Conta com ano anterior: 3.1 real 1200, AA 1000 -> +200, 20%; % do FL = 120.
	c31, ok := contaDe(v, "D1", "3.1")
	if !ok || c31.Realizado != 1200 || c31.AnoAnterior == nil || *c31.AnoAnterior != 1000 || c31.VarAA == nil || *c31.VarAA != 200 || casas(c31.VarAAPct) != 20 || casas(c31.Pct) != 120 {
		t.Errorf("3.1: %+v", c31)
	}
	// 4.1: 100 contra 50.
	if c, _ := contaDe(v, "D1", "4.1"); c.AnoAnterior == nil || *c.AnoAnterior != 50 || casas(c.VarAAPct) != 100 {
		t.Errorf("4.1: %+v", c)
	}
	// 4.2 (D2) não teve ano anterior: null, nunca zero; variação null.
	c42, ok := contaDe(v, "D2", "4.2")
	if !ok || c42.Realizado != -30 || c42.AnoAnterior != nil || c42.VarAA != nil || c42.VarAAPct != nil {
		t.Errorf("4.2: %+v", c42)
	}
	// Orçado por conta não existe: null em toda parte, com variação null.
	for _, d := range v.Departamentos {
		for _, c := range append(d.Contas, d.Totais) {
			if c.Orcado != nil || c.VarOrcado != nil || c.VarOrcadoPct != nil {
				t.Errorf("orçado inventado em %s: %+v", d.Codigo, c)
			}
		}
	}
	// Totais por departamento e geral.
	d1 := v.Departamentos[0].Totais
	if d1.Realizado != 1300 || d1.AnoAnterior == nil || *d1.AnoAnterior != 1050 || *d1.VarAA != 250 || casas(d1.Pct) != 130 {
		t.Errorf("total D1: %+v", d1)
	}
	if d2 := v.Departamentos[1].Totais; d2.Realizado != -30 || d2.AnoAnterior != nil {
		t.Errorf("total D2: %+v", d2)
	}
	if v.Total.Realizado != 1270 || v.Total.AnoAnterior == nil || *v.Total.AnoAnterior != 1050 || casas(v.Total.Pct) != 127 {
		t.Errorf("total geral: %+v", v.Total)
	}
	// A base do faturamento nunca vai no corpo.
	var m map[string]any
	json.Unmarshal([]byte(corpo), &m)
	for k := range m {
		if strings.Contains(k, "faturamento_liquido") {
			t.Errorf("campo %q no corpo", k)
		}
	}
	if strings.Contains(corpo, "faturamento_liquido") || strings.Contains(corpo, `"base"`) {
		t.Errorf("base vazou: %s", corpo)
	}
	// Só A: B some.
	g1 := gestorCom(t, db, f, "D1")
	if _, v1, _ := meuDe(t, db, f, g1, "/x?ano=2026&mes=9&visao=mensal"); codigosMeu(v1) != "D1" || v1.Total.Realizado != 1300 {
		t.Errorf("só D1: %s %v", codigosMeu(v1), v1.Total.Realizado)
	}
	// Contas sem departamento (3.2, 5.1) nunca aparecem.
	if strings.Contains(corpo, `"3.2"`) || strings.Contains(corpo, `"5.1"`) || strings.Contains(corpo, "Sem departamento") {
		t.Errorf("conta sem departamento vazou")
	}
}

func TestMeuDepartamento_MesFechadoLeDoSnapshot(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioMeu(t, f)
	gid := gestorCom(t, db, f, "D1", "D2")
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	// Depois do fechamento: lançamento novo, ano anterior novo e a conta 3.1 muda de departamento.
	lancPct(t, f, "2026-09-01", "01", "3.1", 77)
	lancPct(t, f, "2025-09-01", "01", "3.1", 500)
	exec(t, `UPDATE auditoria.conta_departamento SET departamento_id=(SELECT id FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND codigo='D2')
		WHERE empresa_id=$1::uuid AND conta_padrao='3.1'`, f.EmpresaID)
	code, v, _ := meuDe(t, db, f, gid, "/x?ano=2026&mes=9&visao=mensal")
	if code != http.StatusOK {
		t.Fatal(code)
	}
	c, ok := contaDe(v, "D1", "3.1")
	if !ok || c.Realizado != 1200 || c.AnoAnterior == nil || *c.AnoAnterior != 1000 {
		t.Errorf("snapshot 3.1: %+v (dept congelado D1)", c)
	}
	if _, ok := contaDe(v, "D2", "3.1"); ok {
		t.Error("3.1 apareceu em D2 (departamento não está congelado)")
	}
	if v.PctStatus != "ok" || casas(c.Pct) != 120 {
		t.Errorf("pct do snapshot: %s %v", v.PctStatus, c.Pct)
	}
	// Mês aberto em seguida (acumulada com ago aberto): soma o snapshot de set com o vivo de ago.
	lancPct(t, f, "2026-08-01", "01", "4.1", 10)
	_, va, _ := meuDe(t, db, f, gid, "/x?ano=2026&mes=9&visao=acumulada")
	if c, _ := contaDe(va, "D1", "4.1"); c.Realizado != 110 {
		t.Errorf("acumulada 4.1 = %v", c.Realizado)
	}
}

func TestMeuDepartamento_FechamentoLegadoSemAnoAnterior(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioMeu(t, f)
	gid := gestorCom(t, db, f, "D1")
	exec(t, `INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial) VALUES ($1::uuid,'2026-09-01',1,'')`, f.EmpresaID)
	_, v, _ := meuDe(t, db, f, gid, "/x?ano=2026&mes=9&visao=mensal")
	if len(v.Avisos) == 0 || !strings.Contains(strings.Join(v.Avisos, " "), "2026-09") {
		t.Errorf("aviso do legado: %v", v.Avisos)
	}
	if v.Total.AnoAnterior != nil {
		t.Errorf("ano anterior inventado: %v", *v.Total.AnoAnterior)
	}
}

func TestMeuDepartamento_SemDepartamentoOuSemPapel(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioMeu(t, f)
	g0 := gestorCom(t, db, f)
	code, v, corpo := meuDe(t, db, f, g0, "/x?ano=2026&mes=9&visao=mensal")
	if code != http.StatusOK || len(v.Departamentos) != 0 || v.Total.Realizado != 0 {
		t.Errorf("sem departamento: %d %s", code, corpo)
	}
	// Sem linha marcada como Faturamento Líquido: percentuais nulos e aviso.
	exec(t, `UPDATE auditoria.dre_linhas SET papel=NULL WHERE empresa_id=$1::uuid`, f.EmpresaID)
	g1 := gestorCom(t, db, f, "D1")
	_, v1, _ := meuDe(t, db, f, g1, "/x?ano=2026&mes=9&visao=mensal")
	if v1.PctStatus != "sem_papel" || v1.Total.Pct != nil || len(v1.Avisos) == 0 || v1.Total.Realizado != 1300 {
		t.Errorf("sem papel: %s %+v %v", v1.PctStatus, v1.Total, v1.Avisos)
	}
}

func TestMeuDepartamento_OutroDepartamentoEQuemNaoEGerente(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioMeu(t, f)
	g1 := gestorCom(t, db, f, "D1")
	if code, v, _ := meuDe(t, db, f, g1, "/x?ano=2026&mes=9&departamento=D1"); code != 200 || codigosMeu(v) != "D1" {
		t.Errorf("próprio: %d %s", code, codigosMeu(v))
	}
	for _, d := range []string{"D2", "D%25", "D", "XX"} {
		if code, _, _ := meuDe(t, db, f, g1, "/x?ano=2026&mes=9&departamento="+d); code != http.StatusForbidden {
			t.Errorf("departamento %q: %d", d, code)
		}
	}
	// A visão é pessoal: os demais perfis e o admin de plataforma recebem 403.
	for _, perfil := range []string{"controladoria", "analista", "diretoria", "auditoria"} {
		w := chamar(MeuDepartamentoHandler(db), reqDe(http.MethodGet, "/x?ano=2026&mes=9", f.EmpresaID, f.UserID, perfil))
		if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "realizado") {
			t.Errorf("%s: %d %s", perfil, w.Code, w.Body.String())
		}
	}
	r := reqDe(http.MethodGet, "/x?ano=2026&mes=9", f.EmpresaID, f.UserID, "gestor")
	r.Context().Value(AudContextKey).(*AudContext).Admin = true
	if w := chamar(MeuDepartamentoHandler(db), r); w.Code != http.StatusForbidden {
		t.Errorf("admin de plataforma: %d", w.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='meu_departamento' AND acao='negado'`, f.EmpresaID); n != 9 {
		t.Errorf("negativas registradas = %d", n)
	}
	// Parâmetro inválido.
	if code, _, _ := meuDe(t, db, f, g1, "/x?ano=abc&mes=9"); code != http.StatusBadRequest {
		t.Errorf("ano inválido: %d", code)
	}
}

func TestMeuDepartamento_OutraEmpresaNaoVaza(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	f2 := testdb.NovaEmpresa(t, db)
	cenarioMeu(t, f2)
	cenarioDep(t, f) // mesmo cenário, valores próprios
	lancPct(t, f2, "2026-09-01", "01", "3.1", 5000)
	g := gestorCom(t, db, f, "D1")
	_, v, _ := meuDe(t, db, f, g, "/x?ano=2026&mes=9")
	if c, _ := contaDe(v, "D1", "3.1"); c.Realizado != 1200 {
		t.Errorf("3.1 = %v (somou outra empresa?)", c.Realizado)
	}
}
