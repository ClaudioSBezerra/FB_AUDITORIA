package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// As rotas sem área (/api/aud/ambiente/*) se protegem no handler: quem só tem a
// área acompanhamento, e não é admin de plataforma, leva 403.
func TestAcompanhamento_RotasSemAreaExigemAdmin(t *testing.T) {
	c := &AudContext{UserID: "u", EmpresaID: "e", Perfil: "controladoria", Areas: []string{AreaAcompanhamento}}
	for nome, h := range map[string]http.HandlerFunc{
		"limpar": AmbienteLimparHandler(nil),
		"virar":  AmbienteVirarHandler(nil),
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/aud/ambiente/"+nome, strings.NewReader(`{}`))
		r = r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
		w := httptest.NewRecorder()
		h(w, r)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s: code=%d, esperado 403", nome, w.Code)
		}
	}
}
