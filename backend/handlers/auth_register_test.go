package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Os testes do cadastro público não usam banco: com a rota fechada o handler
// responde antes de tocar nele, e com a rota aberta um corpo inválido é
// recusado antes de qualquer consulta.

func chamarRegister(t *testing.T, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(corpo))
	rec := httptest.NewRecorder()
	RegisterHandler(nil)(rec, req)
	return rec
}

func TestRegisterFechadoPorPadrao(t *testing.T) {
	t.Setenv("ALLOW_SELF_REGISTER", "") // ausente
	corpo := `{"email":"x@x.com","password":"123456","full_name":"X","company_name":"Y"}`
	if rec := chamarRegister(t, corpo); rec.Code != http.StatusNotFound {
		t.Fatalf("variável ausente: esperado 404, veio %d", rec.Code)
	}
}

func TestRegisterFechadoComFalseEValoresDiferentesDeTrue(t *testing.T) {
	for _, v := range []string{"false", "0", "no", "sim", "truee", " "} {
		t.Setenv("ALLOW_SELF_REGISTER", v)
		if rec := chamarRegister(t, `{}`); rec.Code != http.StatusNotFound {
			t.Fatalf("ALLOW_SELF_REGISTER=%q: esperado 404, veio %d", v, rec.Code)
		}
	}
}

func TestRegisterAbertoComTrue(t *testing.T) {
	for _, v := range []string{"true", "TRUE", " True "} {
		t.Setenv("ALLOW_SELF_REGISTER", v)
		// Corpo inválido: o handler passa pela guarda e recusa com 400, sem tocar o banco.
		if rec := chamarRegister(t, `isto não é json`); rec.Code != http.StatusBadRequest {
			t.Fatalf("ALLOW_SELF_REGISTER=%q: esperado 400 (guarda aberta), veio %d", v, rec.Code)
		}
	}
}
