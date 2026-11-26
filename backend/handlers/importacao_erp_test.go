package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

func TestImportacaoERP_Handler(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	chamar := func(erp *dre.ImportacaoERP, perfil, metodo, corpo string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(metodo, "/api/aud/importacao-erp", strings.NewReader(corpo))
		w := httptest.NewRecorder()
		c := &AudContext{EmpresaID: f.EmpresaID, UserID: f.UserID, Perfil: perfil}
		importacaoERP(db, erp)(w, r.WithContext(context.WithValue(r.Context(), AudContextKey, c)))
		return w
	}
	if w := chamar(nil, "controladoria", http.MethodGet, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"configurado":false`) {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := chamar(nil, "controladoria", http.MethodPost, `{"de":"2026-01","ate":"2026-01"}`); w.Code != 503 {
		t.Fatal(w.Code)
	}
	erp := &dre.ImportacaoERP{DB: db, Ingestor: &dre.Ingestor{DB: db}, EmpresaID: f.EmpresaID,
		Fontes: func(time.Time) []dre.Fonte { return nil }}
	for _, p := range []string{"gestor", "diretoria", "auditoria"} {
		if w := chamar(erp, p, http.MethodPost, `{"de":"2026-01","ate":"2026-01"}`); w.Code != 403 {
			t.Errorf("%s: %d", p, w.Code)
		}
	}
	if w := chamar(erp, "analista", http.MethodPost, `{"de":"2026-02","ate":"2026-01"}`); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := chamar(erp, "analista", http.MethodPost, `{"de":"2026-01","ate":"2026-01"}`); w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	for i := 0; i < 100; i++ {
		w := chamar(erp, "analista", http.MethodGet, "")
		if strings.Contains(w.Body.String(), `"status":"concluida"`) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a execução não terminou")
}
