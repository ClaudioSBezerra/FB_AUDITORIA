package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"
)

func TestDepartamentos_Handler(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	h := DepartamentosHandler(db)
	const url = "/api/aud/departamentos"

	w := chamar(h, reqFilial(http.MethodPost, url, `{"codigo":"D1","nome":"Depto","grupo":"Comercial"}`, f, "diretoria"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("diretoria POST: %d", w.Code)
	}
	w = chamar(h, reqFilial(http.MethodPost, url, `{"codigo":"D1","nome":"Depto","grupo":"Comercial"}`, f, "controladoria"))
	if w.Code != http.StatusCreated {
		t.Fatalf("criar: %d %s", w.Code, w.Body.String())
	}
	var d struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &d)

	w = chamar(h, reqFilial(http.MethodGet, url, "", f, "diretoria"))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"D1"`) {
		t.Fatalf("GET: %d %s", w.Code, w.Body.String())
	}
	if w = chamar(h, reqFilial(http.MethodGet, url, "", outra, "controladoria")); strings.Contains(w.Body.String(), "D1") {
		t.Errorf("outra empresa vê o departamento")
	}
	if w = chamar(h, reqFilial(http.MethodPost, url, `{"codigo":"D1","nome":"x","grupo":"Comercial"}`, f, "controladoria")); w.Code != http.StatusConflict {
		t.Errorf("código repetido: %d", w.Code)
	}
	for _, c := range []string{`{"codigo":"","nome":"x","grupo":"Comercial"}`, `{"codigo":"D2","nome":"x","grupo":"Nada"}`, `lixo`} {
		if w = chamar(h, reqFilial(http.MethodPost, url, c, f, "controladoria")); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", c, w.Code)
		}
	}
	if w = chamar(h, reqFilial(http.MethodPut, url, `{"id":"`+d.ID+`","codigo":"D1","nome":"Novo","grupo":"Logística"}`, f, "controladoria")); w.Code != http.StatusOK {
		t.Errorf("PUT: %d %s", w.Code, w.Body.String())
	}
	if w = chamar(h, reqFilial(http.MethodPut, url, `{"codigo":"D1","nome":"x","grupo":"Comercial"}`, f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("PUT sem id: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodPut, url, `{"id":"x","codigo":"D1","nome":"x","grupo":"Comercial"}`, f, "controladoria")); w.Code != http.StatusNotFound {
		t.Errorf("PUT id inválido: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodDelete, url+"?id="+d.ID, "", f, "diretoria")); w.Code != http.StatusForbidden {
		t.Errorf("DELETE diretoria: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodDelete, url, "", f, "controladoria")); w.Code != http.StatusBadRequest {
		t.Errorf("DELETE sem id: %d", w.Code)
	}
	if w = chamar(h, reqFilial(http.MethodDelete, url+"?id="+d.ID, "", f, "controladoria")); w.Code != http.StatusNoContent {
		t.Errorf("DELETE: %d %s", w.Code, w.Body.String())
	}
	if w = chamar(h, reqFilial(http.MethodPatch, url, "", f, "controladoria")); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("PATCH: %d", w.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='departamento'`, f.EmpresaID); n != 3 {
		t.Errorf("trilha: %d (criar, editar, excluir)", n)
	}
}
