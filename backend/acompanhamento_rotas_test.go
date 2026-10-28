package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"fb_auditoria/handlers"
	"fb_auditoria/internal/testdb"

	"github.com/lib/pq"
)

// Quem tem só a área `acompanhamento` acessa apenas /api/aud/acompanhamento
// (história 4.10). O teste lê as rotas /api/aud/ de main.go e passa cada uma pelo
// AuditoriaAuthMiddleware real, com a área e as capacidades com que foi registrada.

var reRotaAud = regexp.MustCompile(`http\.HandleFunc\("(/api/aud/[^"]+)",\s*(?:gz\()?withAud\(handlers\.\w+,\s*([^,)]+)((?:,\s*[^,)]+)*)\)`)

// Capacidades pelo identificador usado em main.go (história 4.3).
var capsPorNome = map[string]handlers.Capacidade{
	"handlers.CapLer": handlers.CapLer, "handlers.CapLerNoEscopo": handlers.CapLerNoEscopo,
	"handlers.CapLerTrilha": handlers.CapLerTrilha, "handlers.CapCarregar": handlers.CapCarregar,
	"handlers.CapAjustar": handlers.CapAjustar, "handlers.CapEditarMapa": handlers.CapEditarMapa,
	"handlers.CapEditarEstrutura": handlers.CapEditarEstrutura, "handlers.CapEditarDicionario": handlers.CapEditarDicionario,
	"handlers.CapConfigurarRateio": handlers.CapConfigurarRateio, "handlers.CapAplicarRateio": handlers.CapAplicarRateio,
	"handlers.CapDefinirLimite": handlers.CapDefinirLimite, "handlers.CapJustificar": handlers.CapJustificar,
	"handlers.CapFechar": handlers.CapFechar, "handlers.CapReabrir": handlers.CapReabrir,
	"handlers.CapGerirUsuarios": handlers.CapGerirUsuarios,
}

var areasPorNome = map[string]string{
	"est":                         handlers.AreaEmpresa,
	"handlers.AreaEmpresa":        handlers.AreaEmpresa,
	"handlers.AreaConfiguracoes":  handlers.AreaConfiguracoes,
	"handlers.AreaComercial":      handlers.AreaComercial,
	"handlers.AreaOperacao":       handlers.AreaOperacao,
	"handlers.AreaAcompanhamento": handlers.AreaAcompanhamento,
	`""`:                          "",
}

// Rotas sem área, que se protegem dentro do handler (admin de plataforma) ou só
// devolvem o menu do próprio usuário.
var rotasSemArea = map[string]bool{
	"/api/aud/me":              true,
	"/api/aud/ambiente/limpar": true,
	"/api/aud/ambiente/virar":  true,
}

type rotaAud struct {
	caminho string
	area    string
	caps    []handlers.Capacidade
	// invalidos: argumentos que não são capacidade (lista de perfis ou nome
	// desconhecido). Deve ficar vazio.
	invalidos []string
}

func rotasAud(t *testing.T) []rotaAud {
	t.Helper()
	var out []rotaAud
	src := lerFonte(t, "main.go")
	if total := strings.Count(src, `"/api/aud/`); total == 0 {
		t.Fatal("main.go sem rotas /api/aud/")
	}
	for _, l := range strings.Split(src, "\n") {
		if !strings.Contains(l, `HandleFunc("/api/aud/`) {
			continue
		}
		m := reRotaAud.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			t.Fatalf("rota /api/aud/ que o teste não consegue ler: %s", strings.TrimSpace(l))
		}
		area, ok := areasPorNome[strings.TrimSpace(m[2])]
		if !ok {
			t.Fatalf("área %q de %s desconhecida: acrescente a areasPorNome", m[2], m[1])
		}
		rt := rotaAud{caminho: m[1], area: area}
		for _, a := range strings.Split(m[3], ",") {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			if c, ok := capsPorNome[a]; ok {
				rt.caps = append(rt.caps, c)
			} else {
				rt.invalidos = append(rt.invalidos, a)
			}
		}
		out = append(out, rt)
	}
	return out
}

func TestAcompanhamento_SoEstaRota(t *testing.T) {
	t.Setenv("JWT_SECRET", "segredo-de-teste-com-mais-de-32-bytes-ok")
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO user_environments (user_id, environment_id)
		SELECT $1::uuid, eg.environment_id FROM companies c JOIN enterprise_groups eg ON eg.id=c.group_id WHERE c.id=$2::uuid`,
		f.UserID, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	tok, err := handlers.GenerateToken(f.UserID, "user")
	if err != nil {
		t.Fatal(err)
	}
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	chama := func(rt rotaAud) int {
		r := httptest.NewRequest(http.MethodGet, rt.caminho, nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		handlers.AuditoriaAuthMiddleware(db, ok, rt.area, rt.caps...)(w, r)
		return w.Code
	}
	set := func(perfil string, areas ...string) {
		if _, err := db.Exec(`UPDATE users SET perfil=$2, areas=$3 WHERE id=$1::uuid`, f.UserID, perfil, pq.Array(areas)); err != nil {
			t.Fatal(err)
		}
	}

	rotas := rotasAud(t)
	vistas := map[string]bool{}
	for _, rt := range rotas {
		vistas[rt.caminho] = true
	}
	for _, esperada := range []string{
		"/api/aud/acompanhamento", "/api/aud/dre", "/api/aud/dre/departamentos", "/api/aud/dre/lancamentos",
		"/api/aud/dre/fechamentos", "/api/aud/usuarios", "/api/aud/audit-log", "/api/aud/filiais-gerenciais",
		"/api/aud/departamentos", "/api/aud/dicionario", "/api/aud/agrupamentos-filiais", "/api/aud/dados/atualizar",
	} {
		if !vistas[esperada] {
			t.Errorf("rota %s não encontrada em main.go", esperada)
		}
	}

	// Só `acompanhamento`, com o perfil mais forte: nenhuma outra rota abre.
	set("controladoria", handlers.AreaAcompanhamento)
	for _, rt := range rotas {
		c := chama(rt)
		switch {
		case rt.caminho == "/api/aud/acompanhamento":
			if c != http.StatusOK {
				t.Errorf("%s com a área acompanhamento: %d", rt.caminho, c)
			}
		case rotasSemArea[rt.caminho]:
			// protegidas no handler; ver TestAcompanhamento_RotasSemAreaExigemAdmin
		default:
			if rt.area == "" {
				t.Errorf("%s registrada sem área e fora da lista de exceções", rt.caminho)
			}
			if c != http.StatusForbidden {
				t.Errorf("%s com só acompanhamento: %d, esperado 403", rt.caminho, c)
			}
		}
	}

	// Sem a área `acompanhamento` (todas as outras): 403 no quadro.
	set("controladoria", handlers.AreaEmpresa, handlers.AreaConfiguracoes, handlers.AreaComercial, handlers.AreaOperacao)
	for _, rt := range rotas {
		if rt.caminho == "/api/aud/acompanhamento" {
			if c := chama(rt); c != http.StatusForbidden {
				t.Errorf("acompanhamento sem a área: %d, esperado 403", c)
			}
		}
	}
}
