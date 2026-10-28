package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"
)

func reqFilial(metodo, url, corpo string, f testdb.Fixture, perfil string) *http.Request {
	r := httptest.NewRequest(metodo, url, strings.NewReader(corpo))
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: perfil, Areas: []string{AreaConfiguracoes}}
	return r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
}

func TestFiliaisGerenciais_Handler(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	h := FiliaisGerenciaisHandler(db)
	const url = "/api/aud/filiais-gerenciais"

	// Perfil sem escrita: 403 e nada gravado.
	w := chamar(h, reqFilial(http.MethodPost, url, `{"codigo":"G1","nome":"Grupo","filiais":["100"]}`, f, "diretoria"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("diretoria POST: %d", w.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.filiais_gerenciais WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("nada deveria ser gravado: %d", n)
	}

	// Criar.
	w = chamar(h, reqFilial(http.MethodPost, url, `{"codigo":"G1","nome":"Grupo 1","filiais":["100","101"]}`, f, "controladoria"))
	if w.Code != http.StatusCreated {
		t.Fatalf("criar: %d %s", w.Code, w.Body.String())
	}
	var g struct {
		ID      string   `json:"id"`
		Filiais []string `json:"filiais"`
	}
	json.Unmarshal(w.Body.Bytes(), &g)
	if g.ID == "" || len(g.Filiais) != 2 {
		t.Fatalf("resposta: %s", w.Body.String())
	}

	// Leitura por perfil da área (diretoria) e isolamento por empresa.
	w = chamar(h, reqFilial(http.MethodGet, url, "", f, "diretoria"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"G1"`) || !strings.Contains(w.Body.String(), `"101"`) {
		t.Fatalf("GET: %d %s", w.Code, w.Body.String())
	}
	w = chamar(h, reqFilial(http.MethodGet, url, "", outra, "controladoria"))
	if strings.Contains(w.Body.String(), "G1") {
		t.Errorf("outra empresa vê o grupo: %s", w.Body.String())
	}

	// Conflito de filial: 409 citando G1.
	w = chamar(h, reqFilial(http.MethodPost, url, `{"codigo":"G2","nome":"Grupo 2","filiais":["101","102"]}`, f, "controladoria"))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "G1") {
		t.Errorf("conflito: %d %s", w.Code, w.Body.String())
	}
	// Código repetido: 409. Vazio: 400.
	w = chamar(h, reqFilial(http.MethodPost, url, `{"codigo":"G1","nome":"x","filiais":[]}`, f, "controladoria"))
	if w.Code != http.StatusConflict {
		t.Errorf("código repetido: %d", w.Code)
	}
	for _, c := range []string{`{"codigo":"","nome":"x"}`, `{"codigo":"G3","nome":""}`, `{"codigo":"G3","nome":"x","filiais":["1","1"]}`, `lixo`} {
		if w = chamar(h, reqFilial(http.MethodPost, url, c, f, "controladoria")); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", c, w.Code)
		}
	}

	// Editar (PUT) substitui a lista.
	w = chamar(h, reqFilial(http.MethodPut, url, `{"id":"`+g.ID+`","codigo":"G1","nome":"Grupo 1","filiais":["100","102"]}`, f, "controladoria"))
	if w.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", w.Code, w.Body.String())
	}
	// Excluir com uso: 409; sem uso: 204; diretoria: 403.
	if _, err := db.Exec(`INSERT INTO auditoria.lancamentos (empresa_id, competencia, conta, valor, filial) VALUES ($1::uuid,'2026-09-01','3.1',10,'102')`, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if w = chamar(h, reqFilial(http.MethodDelete, url+"?id="+g.ID, "", f, "diretoria")); w.Code != http.StatusForbidden {
		t.Errorf("DELETE diretoria: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodDelete, url+"?id="+g.ID, "", f, "controladoria")); w.Code != http.StatusConflict {
		t.Errorf("DELETE em uso: %d", w.Code)
	}
	db.Exec(`DELETE FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID)
	if w = chamar(h, reqFilial(http.MethodDelete, url+"?id="+g.ID, "", f, "controladoria")); w.Code != http.StatusNoContent {
		t.Errorf("DELETE livre: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='filial_gerencial'`, f.EmpresaID); n != 3 {
		t.Errorf("trilha: %d (criar, editar, excluir)", n)
	}

	// Erros de uso.
	if w = chamar(h, reqFilial(http.MethodPut, url, `{"codigo":"G1","nome":"x","filiais":[]}`, f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("PUT sem id: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodPut, url, `{"id":"`+g.ID+`","codigo":"G1","nome":"x"}`, f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("PUT sem filiais: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodPut, url, `{"id":"x","codigo":"G1","nome":"x","filiais":[]}`, f, "controladoria")); w.Code != http.StatusNotFound {
		t.Errorf("PUT id inválido: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodPatch, url, ``, f, "controladoria")); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PATCH: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodDelete, url, ``, f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("DELETE sem id: %d", w.Code)
	}
}
