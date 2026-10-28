package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"
)

func TestAgrupamentosFiliais_Handler(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	hg, ha := FiliaisGerenciaisHandler(db), AgrupamentosFiliaisHandler(db)
	const url = "/api/aud/agrupamentos-filiais"
	for _, c := range []string{`{"codigo":"G1","nome":"G","filiais":["100"]}`, `{"codigo":"G2","nome":"G","filiais":["200"]}`} {
		if w := chamar(hg, reqFilial(http.MethodPost, "/x", c, f, "controladoria")); w.Code != http.StatusCreated {
			t.Fatalf("filial gerencial: %d %s", w.Code, w.Body.String())
		}
	}
	// 403 para quem não é controladoria; nada gravado.
	for _, p := range []string{"auditoria", "diretoria", "gestor"} {
		if w := chamar(ha, reqFilial(http.MethodPost, url, `{"codigo":"A1","nome":"A","filiais":["G1"]}`, f, p)); w.Code != http.StatusForbidden {
			t.Errorf("%s POST: %d", p, w.Code)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.agrupamentos_filiais WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Fatalf("nada deveria ser gravado: %d", n)
	}
	w := chamar(ha, reqFilial(http.MethodPost, url, `{"codigo":"A1","nome":"Agr","filiais":["G1","G2"]}`, f, "controladoria"))
	if w.Code != http.StatusCreated {
		t.Fatalf("criar: %d %s", w.Code, w.Body.String())
	}
	var a struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &a)
	if w = chamar(ha, reqFilial(http.MethodGet, url, "", f, "diretoria")); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"A1"`) || !strings.Contains(w.Body.String(), `"G2"`) {
		t.Errorf("GET: %d %s", w.Code, w.Body.String())
	}
	// Erros: 409 repetido e colisão, 400 membro inexistente e entrada, 404.
	for corpo, want := range map[string]int{
		`{"codigo":"A1","nome":"x","filiais":[]}`:     http.StatusConflict,
		`{"codigo":"G1","nome":"x","filiais":[]}`:     http.StatusConflict,
		`{"codigo":"Grupo","nome":"x","filiais":[]}`:  http.StatusConflict,
		`{"codigo":"A9","nome":"x","filiais":["ZZ"]}`: http.StatusBadRequest,
		`{"codigo":"","nome":"x"}`:                    http.StatusBadRequest,
		`lixo`:                                        http.StatusBadRequest,
	} {
		if w = chamar(ha, reqFilial(http.MethodPost, url, corpo, f, "controladoria")); w.Code != want {
			t.Errorf("%s: %d (queria %d)", corpo, w.Code, want)
		}
	}
	if w = chamar(ha, reqFilial(http.MethodPut, url, `{"id":"`+a.ID+`","codigo":"A1","nome":"Agr","filiais":["G1"]}`, f, "controladoria")); w.Code != http.StatusOK {
		t.Errorf("PUT: %d %s", w.Code, w.Body.String())
	}
	if w = chamar(ha, reqFilial(http.MethodPut, url, `{"id":"`+a.ID+`","codigo":"A1","nome":"Agr"}`, f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("PUT sem filiais: %d", w.Code)
	}
	// Filial gerencial membro: exclusão recusada com 409 citando o agrupamento.
	var gid string
	db.QueryRow(`SELECT id::text FROM auditoria.filiais_gerenciais WHERE empresa_id=$1::uuid AND codigo='G1'`, f.EmpresaID).Scan(&gid)
	if w = chamar(hg, reqFilial(http.MethodDelete, "/x?id="+gid, "", f, "controladoria")); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "A1") {
		t.Errorf("DELETE membro: %d %s", w.Code, w.Body.String())
	}
	if w = chamar(ha, reqFilial(http.MethodDelete, url+"?id="+a.ID, "", f, "diretoria")); w.Code != http.StatusForbidden {
		t.Errorf("DELETE diretoria: %d", w.Code)
	}
	if w = chamar(ha, reqFilial(http.MethodDelete, url+"?id="+a.ID, "", f, "controladoria")); w.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d", w.Code)
	}
	if w = chamar(ha, reqFilial(http.MethodDelete, url+"?id="+a.ID, "", f, "controladoria")); w.Code != http.StatusNotFound {
		t.Errorf("DELETE de novo: %d", w.Code)
	}
	if w = chamar(ha, reqFilial(http.MethodDelete, url, "", f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("DELETE sem id: %d", w.Code)
	}
	if w = chamar(ha, reqFilial(http.MethodPatch, url, "", f, "controladoria")); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PATCH: %d", w.Code)
	}
}

// DRE por agrupamento e por grupo: soma das filiais pedidas, sem mudar a leitura.
func TestDRE_AgrupamentoEGrupo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	hg, ha := FiliaisGerenciaisHandler(db), AgrupamentosFiliaisHandler(db)
	for _, c := range []string{`{"codigo":"G1","nome":"G","filiais":["01","02"]}`, `{"codigo":"G2","nome":"G","filiais":["03"]}`, `{"codigo":"G3","nome":"G","filiais":[]}`} {
		if w := chamar(hg, reqFilial(http.MethodPost, "/x", c, f, "controladoria")); w.Code != http.StatusCreated {
			t.Fatal(w.Body.String())
		}
	}
	for _, c := range []string{`{"codigo":"A1","nome":"A","filiais":["G1","G2"]}`, `{"codigo":"A2","nome":"A","filiais":["G2"]}`, `{"codigo":"A3","nome":"A","filiais":["G3"]}`, `{"codigo":"A4","nome":"A","filiais":[]}`} {
		if w := chamar(ha, reqFilial(http.MethodPost, "/x", c, f, "controladoria")); w.Code != http.StatusCreated {
			t.Fatal(w.Body.String())
		}
	}
	lancPct(t, f, "2026-09-01", "01", "3.1", 100)
	lancPct(t, f, "2026-09-01", "02", "3.1", 10)
	lancPct(t, f, "2026-09-01", "03", "3.1", 1)
	lancPct(t, f, "2026-09-01", "04", "3.1", 1000) // filial solta, fora dos agrupamentos
	rb := func(q string) float64 {
		return valorLinha(jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal"+q), "RB", "realizado")
	}
	if v := rb("&agrupamento=A1"); v != 111 {
		t.Errorf("A1 = %v", v)
	}
	if v := rb("&agrupamento=A2"); v != 1 {
		t.Errorf("A2 = %v", v)
	}
	if v := rb("&grupo=1"); v != 1111 {
		t.Errorf("grupo = %v", v)
	}
	if v := rb("&filial=01"); v != 100 {
		t.Errorf("filial = %v", v)
	}
	if v := rb(""); v != 1111 {
		t.Errorf("sem recorte = %v", v)
	}
	// Erros: inexistente 404, vazio 409, combinação 400.
	for q, want := range map[string]int{
		"&agrupamento=X9": http.StatusNotFound, "&agrupamento=A3": http.StatusConflict, "&agrupamento=A4": http.StatusConflict,
		"&agrupamento=A1&filial=01": http.StatusBadRequest, "&agrupamento=A1&grupo=1": http.StatusBadRequest, "&grupo=2": http.StatusBadRequest,
	} {
		w := chamar(DREHandler(db), reqComCtx(httpGet("/x?ano=2026&mes=9&visao=mensal"+q), f))
		if w.Code != want {
			t.Errorf("%s: %d (queria %d) %s", q, w.Code, want, w.Body.String())
		}
	}
	// Outra empresa não enxerga o agrupamento.
	outra := testdb.NovaEmpresa(t, db)
	if w := chamar(DREHandler(db), reqComCtx(httpGet("/x?ano=2026&mes=9&visao=mensal&agrupamento=A1"), outra)); w.Code != http.StatusNotFound {
		t.Errorf("outra empresa: %d", w.Code)
	}

	// Mês fechado: vem do snapshot por filial do ERP e a soma não muda com lançamento novo.
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	lancPct(t, f, "2026-09-01", "01", "3.1", 5000)
	for q, want := range map[string]float64{"&agrupamento=A1": 111, "&agrupamento=A2": 1, "&grupo=1": 1111, "&filial=02": 10} {
		if v := rb(q); v != want {
			t.Errorf("fechado %s = %v (queria %v)", q, v, want)
		}
	}
	o := jsonDe(t, DREHandler(db), f, "/x?ano=2026&mes=9&visao=mensal&agrupamento=A1")
	if o["congelado"] != true {
		t.Errorf("congelado: %v", o["congelado"])
	}
}

func TestDREDepartamentos_Agrupamento(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f) // D1: filial 01 = 1100 (3.1 e 4.1), filial 02 = 200 (3.1)
	for _, c := range []string{`{"codigo":"G1","nome":"G","filiais":["02"]}`} {
		if w := chamar(FiliaisGerenciaisHandler(db), reqFilial(http.MethodPost, "/x", c, f, "controladoria")); w.Code != http.StatusCreated {
			t.Fatal(w.Body.String())
		}
	}
	if w := chamar(AgrupamentosFiliaisHandler(db), reqFilial(http.MethodPost, "/x", `{"codigo":"A1","nome":"A","filiais":["G1"]}`, f, "controladoria")); w.Code != http.StatusCreated {
		t.Fatal(w.Body.String())
	}
	_, v := chamarDep(t, f, "diretoria", "/x?ano=2026&mes=9&visao=mensal&agrupamento=A1")
	if tot, _, ok := achar(v, "Comercial", "D1"); !ok || tot != 200 {
		t.Errorf("agrupamento D1: %v %v", tot, ok)
	}
	_, vg := chamarDep(t, f, "diretoria", "/x?ano=2026&mes=9&visao=mensal&grupo=1")
	if tot, _, ok := achar(vg, "Comercial", "D1"); !ok || tot != 1300 {
		t.Errorf("grupo D1: %v %v", tot, ok)
	}
	if code, _ := chamarDep(t, f, "diretoria", "/x?ano=2026&mes=9&visao=mensal&agrupamento=X9"); code != http.StatusNotFound {
		t.Errorf("inexistente: %d", code)
	}
}

func httpGet(url string) *http.Request { return httptest.NewRequest(http.MethodGet, url, nil) }
