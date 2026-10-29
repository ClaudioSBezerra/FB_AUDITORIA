package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

// Dados fictícios. A: origem "csv"; B: origem "csv.contas".
func prepararConferencia(t *testing.T, db *sql.DB, f testdb.Fixture) {
	t.Helper()
	ins := func(origem, filial, conta, desc string, valor float64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, conta_desc, valor, origem)
			VALUES ($1::uuid,'2026-09-01',$2,$3,$4,$5,$6)`, f.EmpresaID, filial, conta, desc, valor, origem); err != nil {
			t.Fatal(err)
		}
	}
	// filial normalizada: "01" e "1" são a mesma; 100 + 50 contra 150
	ins("csv", "01", "4010001", "Conta A", 100)
	ins("csv", "1", "4010001", "Conta A", 50)
	ins("csv.contas", "1", "4010001", "Conta A", 150)
	// dentro da tolerância (0,01) e fora dela
	ins("csv", "2", "4010002", "Conta B", 10)
	ins("csv.contas", "2", "4010002", "Conta B", 10.01)
	ins("csv", "2", "4010004", "Conta D", 20)
	ins("csv.contas", "2", "4010004", "Conta D", 22.5)
	// só de um lado (uma conta que começa com "=" para o CSV)
	ins("csv", "3", "4010003", "só em A", 5)
	ins("csv.contas", "4", "=4010005", "=HYPERLINK(\"x\")", 7)
	if _, err := db.Exec(`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, ordem) VALUES ($1::uuid,'RB','Receita',1)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'40100%','RB',-1)`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
}

func chamarConf(t *testing.T, db *sql.DB, f testdb.Fixture, perfil string, params url.Values) *httpResp {
	t.Helper()
	w := chamar(ConferenciaHandler(db), reqFilial(http.MethodGet, "/api/aud/conferencia?"+params.Encode(), "", f, perfil))
	return &httpResp{w.Code, w.Body.String(), w.Header().Get("Content-Type")}
}

type httpResp struct {
	Code int
	Body string
	Tipo string
}

func paramsConf(extra ...string) url.Values {
	v := url.Values{"competencia": {"2026-09"}, "origem_a": {"csv"}, "origem_b": {"csv.contas"}}
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	return v
}

func TestConferencia_DiferencasToleranciaESoEmUmLado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	prepararConferencia(t, db, f)
	// dado de outra empresa não aparece
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, origem)
		VALUES ($1::uuid,'2026-09-01','9','999',1000,'csv')`, outra.EmpresaID); err != nil {
		t.Fatal(err)
	}
	r := chamarConf(t, db, f, "controladoria", paramsConf())
	if r.Code != http.StatusOK {
		t.Fatalf("status %d: %s", r.Code, r.Body)
	}
	var c dre.Conferencia
	if err := json.Unmarshal([]byte(r.Body), &c); err != nil {
		t.Fatal(err)
	}
	if c.A.Total != 185 || c.B.Total != 189.51 || c.A.Linhas != 5 || c.B.Linhas != 4 || c.A.Contas != 4 || c.B.Contas != 4 {
		t.Fatalf("totais: %+v %+v", c.A, c.B)
	}
	// 3 pares nos dois lados; só um difere além da tolerância
	if c.ParesComparados != 3 || c.Iguais != 2 || c.TotalDiferencas != 1 || len(c.Diferencas) != 1 {
		t.Fatalf("pares=%d iguais=%d dif=%+v", c.ParesComparados, c.Iguais, c.Diferencas)
	}
	d := c.Diferencas[0]
	if d.Filial != "2" || d.Conta != "4010004" || d.ValorA != 20 || d.ValorB != 22.5 || d.Diferenca != -2.5 || d.Descricao != "Conta D" {
		t.Fatalf("diferença: %+v", d)
	}
	if c.TotalSoEmA != 1 || c.SoEmA[0].Filial != "3" || c.TotalSoEmB != 1 || c.SoEmB[0].Filial != "4" || c.SoEmB[0].Valor != 7 {
		t.Fatalf("só em um lado: %+v %+v", c.SoEmA, c.SoEmB)
	}
	if c.Fechada || !c.SomaNoDRE || !strings.Contains(c.Aviso, "soma") {
		t.Fatalf("fechada=%v soma=%v aviso=%q", c.Fechada, c.SomaNoDRE, c.Aviso)
	}
	// resumo por linha do DRE pelo Mapa (valor bruto, sem o sinal -1 do Mapa)
	if len(c.ResumoLinhas) != 2 || c.ResumoLinhas[0].Codigo != "RB" || c.ResumoLinhas[0].ValorA != 185 || c.ResumoLinhas[0].ValorB != 182.51 || c.ResumoLinhas[1].Codigo != "" {
		t.Fatalf("resumo: %+v", c.ResumoLinhas)
	}
	// tolerância maior engole a diferença; menor traz a de 0,01
	var c2 dre.Conferencia
	r = chamarConf(t, db, f, "analista", paramsConf("tolerancia", "3,00"))
	json.Unmarshal([]byte(r.Body), &c2)
	if r.Code != 200 || c2.TotalDiferencas != 0 || c2.Iguais != 3 {
		t.Fatalf("tolerância 3: %d %s", r.Code, r.Body)
	}
	var c3 dre.Conferencia
	r = chamarConf(t, db, f, "auditoria", paramsConf("tolerancia", "0"))
	json.Unmarshal([]byte(r.Body), &c3)
	if r.Code != 200 || c3.TotalDiferencas != 2 {
		t.Fatalf("tolerância 0: %d %s", r.Code, r.Body)
	}
	// filtro por filial (com zero à esquerda) e paginação
	var c4 dre.Conferencia
	r = chamarConf(t, db, f, "diretoria", paramsConf("filial", "02", "tolerancia", "0"))
	json.Unmarshal([]byte(r.Body), &c4)
	if c4.Filial != "2" || c4.ParesComparados != 2 || c4.TotalDiferencas != 2 || c4.TotalSoEmA != 0 {
		t.Fatalf("filial 02: %s", r.Body)
	}
	var c5 dre.Conferencia
	r = chamarConf(t, db, f, "controladoria", paramsConf("tolerancia", "0", "limite", "1", "offset", "1"))
	json.Unmarshal([]byte(r.Body), &c5)
	if c5.TotalDiferencas != 2 || len(c5.Diferencas) != 1 || c5.Diferencas[0].Conta != "4010002" {
		t.Fatalf("paginação: %s", r.Body)
	}
}

func TestConferencia_FechadaGestor403EValidacao(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	prepararConferencia(t, db, f)
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por)
		VALUES ($1::uuid,'2026-09-01',1,'',$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	var c dre.Conferencia
	r := chamarConf(t, db, f, "controladoria", paramsConf())
	json.Unmarshal([]byte(r.Body), &c)
	if r.Code != 200 || !c.Fechada {
		t.Fatalf("competência fechada devia ser indicada: %s", r.Body)
	}
	r = chamarConf(t, db, f, "gestor", paramsConf())
	if r.Code != http.StatusForbidden {
		t.Fatalf("gestor: %d, esperado 403", r.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='conferencia' AND acao='negado'`, f.EmpresaID); n != 1 {
		t.Errorf("negado na trilha: %d", n)
	}
	for nome, p := range map[string]url.Values{
		"sem_competencia": {"origem_a": {"csv"}, "origem_b": {"x"}},
		"iguais":          paramsConf("origem_b", "csv"),
		"sem_origem":      paramsConf("origem_a", ""),
		"tolerancia":      paramsConf("tolerancia", "abc"),
	} {
		if r := chamarConf(t, db, f, "controladoria", p); r.Code != http.StatusBadRequest {
			t.Errorf("%s: %d, esperado 400 (%s)", nome, r.Code, r.Body)
		}
	}
	// origem inexistente: relatório vazio, sem erro e sem aviso de soma
	var v dre.Conferencia
	r = chamarConf(t, db, f, "controladoria", paramsConf("origem_b", "nao.existe"))
	json.Unmarshal([]byte(r.Body), &v)
	if r.Code != 200 || v.SomaNoDRE || v.B.Linhas != 0 || v.TotalSoEmA != 4 {
		t.Fatalf("origem vazia: %d %s", r.Code, r.Body)
	}
}

func TestConferencia_CSVSemInjecaoETrilha(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	prepararConferencia(t, db, f)
	r := chamarConf(t, db, f, "controladoria", paramsConf("formato", "csv", "tolerancia", "0,01"))
	if r.Code != 200 || !strings.HasPrefix(r.Tipo, "text/csv") || !strings.HasPrefix(r.Body, "\xef\xbb\xbftipo;filial;conta;descricao;") {
		t.Fatalf("csv: %d %q %.80q", r.Code, r.Tipo, r.Body)
	}
	if !strings.Contains(r.Body, "so_em_b;4;'=4010005;\"'=HYPERLINK(") {
		t.Fatalf("célula com = devia ganhar apóstrofo: %s", r.Body)
	}
	if strings.Contains(r.Body, ";=4010005") || strings.Contains(r.Body, ";=HYPERLINK") {
		t.Fatalf("fórmula sem proteção: %s", r.Body)
	}
	if !strings.Contains(r.Body, "diferenca;2;4010004;Conta D;20,00;22,50;-2,50") {
		t.Fatalf("linha de diferença esperada com vírgula decimal e número negativo sem apóstrofo: %s", r.Body)
	}
	chamarConf(t, db, f, "controladoria", paramsConf())
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='conferencia' AND acao='exportar' AND payload->>'origem_a'='csv' AND payload->>'competencia'='2026-09'`, f.EmpresaID); n != 1 {
		t.Errorf("trilha exportar: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='conferencia' AND acao='visualizar' AND payload->>'origem_b'='csv.contas'`, f.EmpresaID); n != 1 {
		t.Errorf("trilha visualizar: %d", n)
	}
}

func TestConferenciaOrigens(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	prepararConferencia(t, db, f)
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, origem)
		VALUES ($1::uuid,'2026-09-01','9','999',1,'oracle')`, outra.EmpresaID); err != nil {
		t.Fatal(err)
	}
	get := func(perfil string) (int, []struct {
		Origem string `json:"origem"`
		Linhas int    `json:"linhas"`
		NaComp int    `json:"linhas_na_competencia"`
	}) {
		w := chamar(ConferenciaOrigensHandler(db), reqFilial(http.MethodGet, "/api/aud/conferencia/origens?competencia=2026-09", "", f, perfil))
		var v []struct {
			Origem string `json:"origem"`
			Linhas int    `json:"linhas"`
			NaComp int    `json:"linhas_na_competencia"`
		}
		json.Unmarshal(w.Body.Bytes(), &v)
		return w.Code, v
	}
	code, v := get("analista")
	if code != 200 || len(v) != 2 || v[0].Origem != "csv" || v[0].Linhas != 5 || v[1].Origem != "csv.contas" || v[1].NaComp != 4 {
		t.Fatalf("origens: %d %+v", code, v)
	}
	if code, _ := get("gestor"); code != http.StatusForbidden {
		t.Fatalf("gestor: %d", code)
	}
}
