package handlers

import (
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

// Hoje o segundo fechamento do mesmo mês é aceito e vira versão 2 (muda na 1.7:
// um só fechamento ativo por mês, reabertura com motivo).
func TestFechamento_FecharDeNovo_Versao2_MudaNa1_7(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	semearMapaELancamentos(t, f, false)

	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("1º fechamento: %d %s", w.Code, w.Body.String())
	}
	w := fechar(t, f, corpoSetembro)
	if w.Code != http.StatusCreated {
		t.Fatalf("2º fechamento: esperava 201 (hoje permite), veio %d: %s", w.Code, w.Body.String())
	}
	if versaoDaResposta(t, w.Body.Bytes()) != 2 {
		t.Errorf("esperava versao 2: %s", w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid AND competencia='2026-09-01'`, f.EmpresaID); n != 2 {
		t.Errorf("esperava 2 fechamentos, vieram %d", n)
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
