package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"

	"github.com/lib/pq"
)

func TestPerfis_Matriz(t *testing.T) {
	escrita := map[Capacidade]bool{}
	for _, c := range CapacidadesDeEscrita {
		escrita[c] = true
	}
	// Toda capacidade de escrita consta em TodasCapacidades.
	todas := map[Capacidade]bool{}
	for _, c := range TodasCapacidades {
		todas[c] = true
	}
	for c := range escrita {
		if !todas[c] {
			t.Errorf("%s fora de TodasCapacidades", c)
		}
	}

	// Analista: carrega e edita Mapa/estrutura/dicionário; não fecha, não reabre.
	for _, c := range []Capacidade{CapLer, CapCarregar, CapEditarMapa, CapEditarEstrutura, CapEditarDicionario} {
		if !PerfilTem("analista", c) {
			t.Errorf("analista sem %s", c)
		}
	}
	for _, c := range []Capacidade{CapFechar, CapReabrir, CapGerirUsuarios, CapLerTrilha} {
		if PerfilTem("analista", c) {
			t.Errorf("analista não deveria ter %s", c)
		}
	}

	// Diretoria e auditoria: nenhuma capacidade de escrita.
	for _, p := range []string{"diretoria", "auditoria"} {
		for c := range escrita {
			if PerfilTem(p, c) {
				t.Errorf("%s com capacidade de escrita %s", p, c)
			}
		}
		if !PerfilTem(p, CapLer) {
			t.Errorf("%s deveria ler", p)
		}
	}

	// Controller: tudo.
	for _, c := range TodasCapacidades {
		if !PerfilTem("controladoria", c) {
			t.Errorf("controladoria sem %s", c)
		}
	}

	// Gerente: só o escopo e justificar; sem leitura irrestrita.
	for _, c := range TodasCapacidades {
		want := c == CapLerNoEscopo || c == CapJustificar
		if PerfilTem("gestor", c) != want {
			t.Errorf("gestor %s: %v, esperado %v", c, !want, want)
		}
	}

	// Perfil desconhecido não tem nada; admin de plataforma tem tudo, mesmo sem perfil.
	adm := &AudContext{Admin: true}
	desc := &AudContext{Perfil: "inexistente"}
	for _, c := range TodasCapacidades {
		if PerfilTem("inexistente", c) || desc.Pode(c) {
			t.Errorf("perfil desconhecido com %s", c)
		}
		if !adm.Pode(c) {
			t.Errorf("admin de plataforma sem %s", c)
		}
	}
}

// A migration 014 aceita analista na coluna e continua recusando perfil inventado.
func TestPerfis_MigracaoAceitaAnalista(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for _, p := range []string{"analista", "auditoria", "controladoria", "diretoria", "gestor"} {
		if _, err := db.Exec(`UPDATE users SET perfil=$2 WHERE id=$1::uuid`, f.UserID, p); err != nil {
			t.Errorf("perfil %s recusado: %v", p, err)
		}
	}
	if _, err := db.Exec(`UPDATE users SET perfil='inventado' WHERE id=$1::uuid`, f.UserID); err == nil {
		t.Error("perfil inventado aceito")
	}
}

// O middleware aceita capacidade por método (ou só a área, sem capacidade).
func TestPerfis_MiddlewareCapacidade(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste-com-mais-de-32-bytes-ok")
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO user_environments (user_id, environment_id)
		SELECT $1::uuid, eg.environment_id FROM companies c JOIN enterprise_groups eg ON eg.id=c.group_id WHERE c.id=$2::uuid`,
		f.UserID, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	tok, err := GenerateToken(f.UserID, "user")
	if err != nil {
		t.Fatal(err)
	}
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	chama := func(h http.HandlerFunc, metodo string) int {
		r := httptest.NewRequest(metodo, "/x", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		h(w, r)
		return w.Code
	}
	mapa := AuditoriaAuthMiddleware(db, ok, AreaEmpresa, CapLerNoEscopo, CapEditarMapa)
	fechar := AuditoriaAuthMiddleware(db, ok, AreaEmpresa, CapFechar)
	soArea := AuditoriaAuthMiddleware(db, ok, AreaEmpresa)

	casos := []struct {
		perfil            string
		leMapa, editaMapa int
		fecha, soArea     int
	}{
		{"analista", 200, 200, 403, 200},
		{"controladoria", 200, 200, 200, 200},
		{"diretoria", 200, 403, 403, 200},
		{"auditoria", 200, 403, 403, 200},
		{"gestor", 200, 403, 403, 200},
	}
	for _, c := range casos {
		if _, err := db.Exec(`UPDATE users SET perfil=$2, areas=$3 WHERE id=$1::uuid`, f.UserID, c.perfil, pq.Array([]string{AreaEmpresa})); err != nil {
			t.Fatal(err)
		}
		if got := chama(mapa, http.MethodGet); got != c.leMapa {
			t.Errorf("%s lê mapa: %d", c.perfil, got)
		}
		if got := chama(mapa, http.MethodPut); got != c.editaMapa {
			t.Errorf("%s edita mapa: %d", c.perfil, got)
		}
		if got := chama(fechar, http.MethodPost); got != c.fecha {
			t.Errorf("%s fecha: %d", c.perfil, got)
		}
		if got := chama(soArea, http.MethodGet); got != c.soArea {
			t.Errorf("%s só pela área: %d", c.perfil, got)
		}
	}
}

// Defesa em profundidade: os handlers também checam a capacidade, não só o middleware.
func TestPerfis_HandlersChecamCapacidade(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	chamaCom := func(h http.Handler, metodo, perfil string) int {
		r := httptest.NewRequest(metodo, "/x", strings.NewReader(`{}`))
		c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: perfil}
		r = r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
		return chamar(h, r).Code
	}
	for _, p := range []string{"analista", "diretoria", "auditoria", "gestor"} {
		if c := chamaCom(DREFechamentoHandler(db), http.MethodPost, p); c != http.StatusForbidden {
			t.Errorf("%s fecha: %d", p, c)
		}
		if c := chamaCom(DREReabrirHandler(db), http.MethodPost, p); c != http.StatusForbidden {
			t.Errorf("%s reabre: %d", p, c)
		}
	}
	for _, p := range []string{"diretoria", "auditoria", "gestor"} {
		if c := chamaCom(DREMapaHandler(db), http.MethodPut, p); c != http.StatusForbidden {
			t.Errorf("%s edita mapa: %d", p, c)
		}
	}
	// O analista passa da checagem de capacidade (o corpo vazio dá 400, não 403).
	if c := chamaCom(DREMapaHandler(db), http.MethodPut, "analista"); c == http.StatusForbidden {
		t.Error("analista não edita mapa")
	}
	if c := chamaCom(DepartamentosHandler(db), http.MethodPost, "analista"); c == http.StatusForbidden {
		t.Error("analista não edita departamentos")
	}
}
