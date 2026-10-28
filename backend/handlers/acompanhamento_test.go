package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fb_auditoria/acompanhamento"
)

func TestAcompanhamento_SomenteLeitura(t *testing.T) {
	h := AcompanhamentoHandler(nil)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		w := httptest.NewRecorder()
		h(w, httptest.NewRequest(m, "/api/aud/acompanhamento", nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: code=%d, esperado 405", m, w.Code)
		}
	}
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, "/api/aud/acompanhamento", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET: code=%d", w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, max-age=60" {
		t.Errorf("Cache-Control=%q", cc)
	}
	var d acompanhamento.Dados
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil || len(d.Historias) == 0 {
		t.Fatalf("corpo inválido: %v", err)
	}
}
