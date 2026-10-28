package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"
)

// Teste de caracterização do fechamento (DREFechamentoHandler): registra o
// comportamento de hoje, incluindo o que muda nas histórias 1.5 a 1.7.

func fechar(t *testing.T, f testdb.Fixture, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	db := testdb.Open(t)
	r := httptest.NewRequest(http.MethodPost, "/api/aud/dre/fechamento", strings.NewReader(corpo))
	return chamar(DREFechamentoHandler(db), reqComCtx(r, f))
}

func semearMapaELancamentos(t *testing.T, f testdb.Fixture, comSemMapa bool) {
	t.Helper()
	db := testdb.Open(t)
	for _, m := range [][]any{{"3.1", "RB"}, {"3.2", "DED"}, {"4.1", "CMV"}} {
		if _, err := db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,$2,$3,1)`,
			f.EmpresaID, m[0], m[1]); err != nil {
			t.Fatal(err)
		}
	}
	lancs := [][]any{{"3.1", 1000.00}, {"3.2", 200.00}, {"4.1", 300.00}}
	if comSemMapa {
		lancs = append(lancs, []any{"9.9", 50.00})
	}
	for _, l := range lancs {
		if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor) VALUES ($1::uuid,'2026-09-01',$2,$3)`,
			f.EmpresaID, l[0], l[1]); err != nil {
			t.Fatal(err)
		}
	}
}

const corpoSetembro = `{"ano":2026,"mes":9}`

func TestFechamento_MesMapeado_Versao1(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)

	w := fechar(t, f, corpoSetembro)
	if w.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if versaoDaResposta(t, w.Body.Bytes()) != 1 {
		t.Errorf("esperava versao 1: %s", w.Body.String())
	}

	// RB=1000, DED=200, RL=800, CMV=300, LB=500; sem despesas, LL=500.
	esperado := map[string]string{"RB": "1000.00", "DED": "200.00", "RL": "800.00", "CMV": "300.00", "LB": "500.00",
		"DV": "0.00", "DP": "0.00", "DA": "0.00", "OP": "0.00", "RO": "500.00", "RF": "0.00", "LAIR": "500.00", "IR": "0.00", "LL": "500.00"}
	rows, err := db.Query(`SELECT l.linha_codigo, l.valor::text FROM auditoria.dre_fechamento_linhas l
		JOIN auditoria.dre_fechamentos f ON f.id = l.fechamento_id WHERE f.empresa_id = $1::uuid`, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var c, v string
		if err := rows.Scan(&c, &v); err != nil {
			t.Fatal(err)
		}
		got[c] = v
	}
	if len(got) != len(esperado) {
		t.Errorf("esperava %d linhas congeladas, vieram %d: %v", len(esperado), len(got), got)
	}
	for c, v := range esperado {
		if got[c] != v {
			t.Errorf("linha %s: esperado %s, veio %q", c, v, got[c])
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_fechamento' AND acao='fechar'`, f.EmpresaID); n != 1 {
		t.Errorf("trilha dre_fechamento/fechar: esperado 1, veio %d", n)
	}
}

func TestFechamento_ContaSemMapa_409_NadaGravado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, true)

	w := fechar(t, f, corpoSetembro)
	if w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "9.9") {
		t.Errorf("corpo deveria listar a conta 9.9: %s", w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("dre_fechamentos: esperado 0, veio %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamento_linhas l JOIN auditoria.dre_fechamentos f ON f.id=l.fechamento_id WHERE f.empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("dre_fechamento_linhas: esperado 0, veio %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_fechamento'`, f.EmpresaID); n != 0 {
		t.Errorf("trilha: esperado 0, veio %d", n)
	}
}

// Fechar de novo com fechamento ativo é recusado (409) e nada é gravado (1.7).
func TestFechamento_FecharDeNovo_409(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)

	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("1º fechamento: %d %s", w.Code, w.Body.String())
	}
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusConflict {
		t.Fatalf("2º fechamento: esperava 409, veio %d: %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid AND competencia='2026-09-01'`, f.EmpresaID); n != 1 {
		t.Errorf("esperava 1 fechamento, vieram %d", n)
	}
}

func versaoDaResposta(t *testing.T, corpo []byte) int {
	t.Helper()
	var r struct {
		Versao int `json:"versao"`
	}
	if err := json.Unmarshal(corpo, &r); err != nil {
		t.Fatalf("resposta não é JSON: %v: %s", err, corpo)
	}
	return r.Versao
}

func TestFechamento_409_TruncaEm20Contas(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for i := 0; i < 25; i++ {
		if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor) VALUES ($1::uuid,'2026-09-01',$2,1)`,
			f.EmpresaID, fmt.Sprintf("8.%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	w := fechar(t, f, corpoSetembro)
	if w.Code != http.StatusConflict {
		t.Fatalf("esperava 409, veio %d: %s", w.Code, w.Body.String())
	}
	corpo := w.Body.String()
	if !strings.Contains(corpo, "(25)") || !strings.Contains(corpo, "e mais 5") {
		t.Errorf("total/truncamento ausentes: %s", corpo)
	}
	if !strings.Contains(corpo, "8.19") || strings.Contains(corpo, "8.20") {
		t.Errorf("lista deveria ter exatamente 20 contas: %s", corpo)
	}
}

func TestDRE_ExibidoIgualAoCongelado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)

	r := httptest.NewRequest(http.MethodGet, "/api/aud/dre?ano=2026&mes=9&visao=mensal", nil)
	w := chamar(DREHandler(db), reqComCtx(r, f))
	if w.Code != http.StatusOK {
		t.Fatalf("DRE: %d %s", w.Code, w.Body.String())
	}
	var exib struct {
		Congelado bool `json:"congelado"`
		Linhas    []struct {
			Codigo    string  `json:"codigo"`
			Realizado float64 `json:"realizado"`
		} `json:"linhas"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &exib); err != nil || exib.Congelado {
		t.Fatalf("resposta inesperada: %v %s", err, w.Body.String())
	}
	if wf := fechar(t, f, corpoSetembro); wf.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", wf.Code, wf.Body.String())
	}
	rows, err := db.Query(`SELECT l.linha_codigo, l.valor FROM auditoria.dre_fechamento_linhas l
		JOIN auditoria.dre_fechamentos f ON f.id = l.fechamento_id WHERE f.empresa_id = $1::uuid`, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	congelado := map[string]float64{}
	for rows.Next() {
		var c string
		var v float64
		if err := rows.Scan(&c, &v); err != nil {
			t.Fatal(err)
		}
		congelado[c] = v
	}
	if len(exib.Linhas) == 0 || len(exib.Linhas) != len(congelado) {
		t.Fatalf("linhas: exibidas %d, congeladas %d", len(exib.Linhas), len(congelado))
	}
	for _, l := range exib.Linhas {
		if l.Realizado != congelado[l.Codigo] {
			t.Errorf("%s: exibido %v, congelado %v", l.Codigo, l.Realizado, congelado[l.Codigo])
		}
	}
}

func reabrir(t *testing.T, f testdb.Fixture, perfil, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	db := testdb.Open(t)
	r := httptest.NewRequest(http.MethodPost, "/api/aud/dre/fechamentos/reabrir", strings.NewReader(corpo))
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: perfil}
	r = r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
	return chamar(DREReabrirHandler(db), r)
}

func TestFechamento_ReabrirEFecharDeNovo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	// sem motivo / só espaços: 400, nada muda
	for _, corpo := range []string{`{"ano":2026,"mes":9}`, `{"ano":2026,"mes":9,"motivo":"   "}`} {
		if w := reabrir(t, f, "controladoria", corpo); w.Code != http.StatusBadRequest {
			t.Fatalf("sem motivo: esperava 400, veio %d", w.Code)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid AND reaberto_em IS NULL`, f.EmpresaID); n != 1 {
		t.Fatalf("ativos: %d", n)
	}
	// perfil sem permissão: 403, nada muda
	if w := reabrir(t, f, "diretoria", `{"ano":2026,"mes":9,"motivo":"x"}`); w.Code != http.StatusForbidden {
		t.Fatalf("diretoria: esperava 403, veio %d", w.Code)
	}
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusConflict {
		t.Fatalf("ainda ativo: %d", w.Code)
	}
	if w := reabrir(t, f, "controladoria", `{"ano":2026,"mes":9,"motivo":"  correção de rateio "}`); w.Code != http.StatusOK {
		t.Fatalf("reabrir: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid AND reaberto_em IS NOT NULL AND reaberto_por=$2::uuid AND motivo_reabertura='correção de rateio'`, f.EmpresaID, f.UserID); n != 1 {
		t.Errorf("reabertura não gravada: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_fechamento' AND acao='reabrir'`, f.EmpresaID); n != 1 {
		t.Errorf("trilha reabrir: %d", n)
	}
	// reabrir o que não está fechado: 409
	if w := reabrir(t, f, "controladoria", `{"ano":2026,"mes":9,"motivo":"de novo"}`); w.Code != http.StatusConflict {
		t.Fatalf("sem ativo: esperava 409, veio %d", w.Code)
	}
	// fechar de novo: versão 2, a 1 continua legível
	w := fechar(t, f, corpoSetembro)
	if w.Code != http.StatusCreated || versaoDaResposta(t, w.Body.Bytes()) != 2 {
		t.Fatalf("refechar: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamento_linhas l JOIN auditoria.dre_fechamentos c ON c.id=l.fechamento_id WHERE c.empresa_id=$1::uuid AND c.versao=1`, f.EmpresaID); n == 0 {
		t.Error("versão 1 deveria continuar legível")
	}
	// listagem mostra situação
	rl := httptest.NewRequest(http.MethodGet, "/api/aud/dre/fechamentos", nil)
	wl := chamar(DREFechamentoHandler(db), reqComCtx(rl, f))
	var lista []struct {
		Versao int    `json:"versao"`
		Ativo  bool   `json:"ativo"`
		Motivo string `json:"motivo_reabertura"`
	}
	if err := json.Unmarshal(wl.Body.Bytes(), &lista); err != nil || len(lista) != 2 {
		t.Fatalf("listagem: %v %s", err, wl.Body.String())
	}
	if lista[0].Versao != 2 || !lista[0].Ativo || lista[1].Versao != 1 || lista[1].Ativo || lista[1].Motivo != "correção de rateio" {
		t.Errorf("situação inesperada: %+v", lista)
	}
}

func TestFechamento_FilialRecusada400(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	if w := fechar(t, f, `{"ano":2026,"mes":9,"filial":"01"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400, veio %d: %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("gravou %d", n)
	}
}

func TestFechamento_DiretoriaNaoFecha403(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	r := httptest.NewRequest(http.MethodPost, "/api/aud/dre/fechamentos", strings.NewReader(corpoSetembro))
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: "diretoria"}
	r = r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
	if w := chamar(DREFechamentoHandler(db), r); w.Code != http.StatusForbidden {
		t.Fatalf("esperava 403, veio %d", w.Code)
	}
}

func estadoDRE(t *testing.T, f testdb.Fixture, query string) (congelado, ativo bool, versao, fechVersao int, rb float64) {
	t.Helper()
	db := testdb.Open(t)
	r := httptest.NewRequest(http.MethodGet, "/api/aud/dre?visao=mensal&ano=2026&mes=9"+query, nil)
	w := chamar(DREHandler(db), reqComCtx(r, f))
	if w.Code != http.StatusOK {
		t.Fatalf("DRE: %d %s", w.Code, w.Body.String())
	}
	var o struct {
		Congelado  bool `json:"congelado"`
		Ativo      bool `json:"fechamento_ativo"`
		Versao     int  `json:"versao"`
		FechVersao int  `json:"fechamento_versao"`
		Linhas     []struct {
			Codigo    string  `json:"codigo"`
			Realizado float64 `json:"realizado"`
		} `json:"linhas"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	for _, l := range o.Linhas {
		if l.Codigo == "RB" {
			rb = l.Realizado
		}
	}
	return o.Congelado, o.Ativo, o.Versao, o.FechVersao, rb
}

func TestDRE_EstadosFechadoReabertoRefechado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)

	if c, a, _, _, _ := estadoDRE(t, f, ""); c || a {
		t.Fatalf("aberto: congelado=%v ativo=%v", c, a)
	}
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	if c, a, v, fv, _ := estadoDRE(t, f, ""); !c || !a || v != 1 || fv != 1 {
		t.Fatalf("fechado: %v %v %d %d", c, a, v, fv)
	}
	// filtro de filial sobre mês fechado: o fechamento ativo continua visível
	if _, a, _, fv, _ := estadoDRE(t, f, "&filial=01"); !a || fv != 1 {
		t.Errorf("filtro de filial: ativo=%v versao=%d", a, fv)
	}
	if w := reabrir(t, f, "controladoria", `{"ano":2026,"mes":9,"motivo":"ajuste"}`); w.Code != http.StatusOK {
		t.Fatalf("reabrir: %d", w.Code)
	}
	// lançamento novo aparece ao vivo
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor) VALUES ($1::uuid,'2026-09-01','3.1',500)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if c, a, _, _, rb := estadoDRE(t, f, ""); c || a || rb != 1500 {
		t.Fatalf("reaberto: congelado=%v ativo=%v RB=%v", c, a, rb)
	}
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("refechar: %d %s", w.Code, w.Body.String())
	}
	if c, a, v, fv, rb := estadoDRE(t, f, ""); !c || !a || v != 2 || fv != 2 || rb != 1500 {
		t.Fatalf("refechado: %v %v %d %d %v", c, a, v, fv, rb)
	}
}

func TestReabrir_MotivoLongoRecusado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	if w := reabrir(t, f, "controladoria", `{"ano":2026,"mes":9,"motivo":"`+strings.Repeat("é", 501)+`"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("501 runas: esperava 400, veio %d", w.Code)
	}
	if w := reabrir(t, f, "controladoria", `{"ano":2026,"mes":9,"motivo":"`+strings.Repeat("é", 500)+`"}`); w.Code != http.StatusOK {
		t.Fatalf("500 runas: esperava 200, veio %d", w.Code)
	}
}

func detalhe(t *testing.T, f testdb.Fixture, query string) (lancs []struct {
	ID     int64   `json:"id"`
	Conta  string  `json:"conta"`
	Valor  float64 `json:"valor"`
	Linha  string  `json:"linha"`
	Origem string  `json:"origem"`
}) {
	t.Helper()
	db := testdb.Open(t)
	r := httptest.NewRequest(http.MethodGet, "/api/aud/dre/lancamentos?"+query, nil)
	w := chamar(DRELancamentosHandler(db), reqComCtx(r, f))
	if w.Code != http.StatusOK {
		t.Fatalf("detalhe: %d %s", w.Code, w.Body.String())
	}
	var o struct {
		Lancamentos []struct {
			ID     int64   `json:"id"`
			Conta  string  `json:"conta"`
			Valor  float64 `json:"valor"`
			Linha  string  `json:"linha"`
			Origem string  `json:"origem"`
		} `json:"lancamentos"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	return o.Lancamentos
}

// Mês fechado: o detalhe por conta vem do snapshot; mudar o Mapa não o altera.
func TestDetalhe_MesFechadoVemDoSnapshot(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	if _, err := db.Exec(`UPDATE auditoria.dre_mapa SET linha_codigo='CMV' WHERE empresa_id=$1::uuid AND conta_padrao='3.1'`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	l := detalhe(t, f, "ano=2026&mes=9&visao=mensal&linha=RB")
	if len(l) != 1 || l[0].Conta != "3.1" || l[0].Valor != 1000 || l[0].Origem != "snapshot" || l[0].ID != 0 {
		t.Fatalf("detalhe congelado: %+v", l)
	}
	if c, _, _, _, rb := estadoDRE(t, f, ""); !c || rb != 1000 {
		t.Errorf("DRE congelado: %v %v", c, rb)
	}
}

func jsonDe(t *testing.T, h http.Handler, f testdb.Fixture, url string) map[string]any {
	t.Helper()
	w := chamar(h, reqComCtx(httptest.NewRequest(http.MethodGet, url, nil), f))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", url, w.Code, w.Body.String())
	}
	var o map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	return o
}

type linhaDet struct {
	Competencia string
	Conta       string
	Valor       float64
	Origem      string
}

func detalheLista(o map[string]any) []linhaDet {
	var out []linhaDet
	for _, x := range o["lancamentos"].([]any) {
		m := x.(map[string]any)
		out = append(out, linhaDet{m["competencia"].(string), m["conta"].(string), m["valor"].(float64), m["origem"].(string)})
	}
	return out
}

func exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := testdb.Open(t).Exec(q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// (a) lançamento novo na mesma conta e linha não aparece no detalhe de mês fechado.
func TestDetalhe_MesFechadoIgnoraLancamentoNovo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	exec(t, `INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor) VALUES ($1::uuid,'2026-09-01','3.1',77)`, f.EmpresaID)
	l := detalheLista(jsonDe(t, DRELancamentosHandler(db), f, "/x?ano=2026&mes=9&visao=mensal&linha=RB"))
	if len(l) != 1 || l[0].Valor != 1000 || l[0].Origem != "snapshot" {
		t.Fatalf("esperava só o snapshot: %+v", l)
	}
}

// (b) fechamento legado: sem contas, com aviso citando o mês.
func TestDetalhe_FechamentoLegadoTemAviso(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	exec(t, `INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial) VALUES ($1::uuid,'2026-09-01',1,'')`, f.EmpresaID)
	o := jsonDe(t, DRELancamentosHandler(db), f, "/x?ano=2026&mes=9&visao=mensal&linha=RB")
	if a, _ := o["aviso"].(string); !strings.Contains(a, "2026-09") {
		t.Errorf("aviso ausente: %v", o["aviso"])
	}
	if l := detalheLista(o); len(l) != 0 {
		t.Errorf("legado sem contas: %+v", l)
	}
}

// (c) período misto: mês fechado do snapshot + mês vivo, ordenados por competência e conta.
func TestDetalhe_PeriodoMistoOrdenado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	exec(t, `INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor) VALUES ($1::uuid,'2026-08-01','4.1',9),($1::uuid,'2026-08-01','3.1',5)`, f.EmpresaID)
	l := detalheLista(jsonDe(t, DRELancamentosHandler(db), f, "/x?ano=2026&mes=9&visao=acumulada&linha=LB"))
	if len(l) != 5 {
		t.Fatalf("esperava 5 linhas: %+v", l)
	}
	for i := 1; i < len(l); i++ {
		a, b := l[i-1], l[i]
		if a.Competencia > b.Competencia || (a.Competencia == b.Competencia && a.Conta > b.Conta) {
			t.Errorf("fora de ordem: %+v antes de %+v", a, b)
		}
	}
	if l[0].Origem == "snapshot" || l[4].Origem != "snapshot" {
		t.Errorf("origens: %+v", l)
	}
}

// (d) filial sobre mês fechado: valores da filial, do snapshot.
func TestDetalhe_FilialSobreMesFechado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	exec(t, `INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2026-09-01','01','3.1',40)`, f.EmpresaID)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	exec(t, `UPDATE auditoria.lancamentos SET valor=1 WHERE empresa_id=$1::uuid`, f.EmpresaID)
	l := detalheLista(jsonDe(t, DRELancamentosHandler(db), f, "/x?ano=2026&mes=9&visao=mensal&linha=RB&filial=01"))
	if len(l) != 1 || l[0].Valor != 40 || l[0].Origem != "snapshot" {
		t.Fatalf("filial 01: %+v", l)
	}
}

func valorLinha(o map[string]any, cod, campo string) float64 {
	for _, x := range o["linhas"].([]any) {
		m := x.(map[string]any)
		if m["codigo"] == cod {
			return m[campo].(float64)
		}
	}
	return -1
}

// (e) DRE de mês fechado: orçado e ano anterior congelados; congelado com filtro de filial.
func TestDRE_FechadoTrazOrcadoEAnoAnteriorCongelados(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	exec(t, `INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor) VALUES ($1::uuid,'2026-09-01','01','3.1',40),($1::uuid,'2025-09-01','','3.1',800)`, f.EmpresaID)
	exec(t, `INSERT INTO auditoria.dre_orcado (empresa_id, competencia, filial, linha_codigo, valor) VALUES ($1::uuid,'2026-09-01','','RB',1200)`, f.EmpresaID)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	exec(t, `UPDATE auditoria.dre_orcado SET valor=1 WHERE empresa_id=$1::uuid`, f.EmpresaID)
	exec(t, `UPDATE auditoria.lancamentos SET valor=1 WHERE empresa_id=$1::uuid AND competencia='2025-09-01'`, f.EmpresaID)
	o := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal")
	if valorLinha(o, "RB", "orcado") != 1200 || valorLinha(o, "RB", "ano_anterior") != 800 {
		t.Errorf("orcado/aa: %v %v", valorLinha(o, "RB", "orcado"), valorLinha(o, "RB", "ano_anterior"))
	}
	if o["congelado"] != true {
		t.Error("congelado esperado")
	}
	if o01 := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal&filial=01"); o01["congelado"] != true || valorLinha(o01, "RB", "realizado") != 40 {
		t.Errorf("filial 01: congelado=%v RB=%v", o01["congelado"], valorLinha(o01, "RB", "realizado"))
	}
}

// (f) renomear/reordenar linha depois do fechamento não muda o mês fechado.
func TestDRE_EstruturaCongeladaNoMesFechado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	antes := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal")["linhas"]
	exec(t, `UPDATE auditoria.dre_linhas SET descricao='Outro nome', ordem=999 WHERE empresa_id=$1::uuid AND codigo='RB'`, f.EmpresaID)
	exec(t, `DELETE FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND codigo='OP'`, f.EmpresaID)
	depois := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal")["linhas"]
	a, _ := json.Marshal(antes)
	b, _ := json.Marshal(depois)
	if string(a) != string(b) {
		t.Errorf("estrutura do mês fechado mudou:\n%s\n%s", a, b)
	}
}
