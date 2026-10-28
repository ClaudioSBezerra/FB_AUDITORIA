package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"fb_auditoria/internal/testdb"
)

// reqComCtx monta a requisição com AudContext direto, sem passar pelo JWT.
func reqComCtx(r *http.Request, f testdb.Fixture) *http.Request {
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: "controladoria", Areas: []string{AreaConfiguracoes}}
	return r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
}

func chamar(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func contar(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("contar: %v", err)
	}
	return n
}
