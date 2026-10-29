package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/handlers"
	"fb_auditoria/internal/testdb"

	"github.com/lib/pq"
)

// História 4.3: toda rota /api/aud/ declara capacidade; nenhuma usa lista de perfis.
// Os testes leem main.go como o de acompanhamento e passam cada rota pelo
// AuditoriaAuthMiddleware real.

func ehEscrita(c handlers.Capacidade) bool {
	for _, e := range handlers.CapacidadesDeEscrita {
		if e == c {
			return true
		}
	}
	return false
}

// Rotas sem capacidade: só área (acompanhamento) ou protegidas no handler.
func semCapacidadePermitida(rt rotaAud) bool {
	return rotasSemArea[rt.caminho] || rt.caminho == "/api/aud/acompanhamento"
}

func TestRotas_DeclaramCapacidadeSemListaDePerfis(t *testing.T) {
	for _, rt := range rotasAud(t) {
		if len(rt.invalidos) > 0 {
			t.Errorf("%s declara %v: use uma capacidade (handlers.Cap*), não lista de perfis", rt.caminho, rt.invalidos)
		}
		if len(rt.caps) == 0 && !semCapacidadePermitida(rt) {
			t.Errorf("%s sem capacidade", rt.caminho)
		}
		if len(rt.caps) > 2 {
			t.Errorf("%s com mais de duas capacidades (leitura, escrita)", rt.caminho)
		}
	}
	// A assinatura com lista de perfis não existe mais.
	src := lerFonte(t, "main.go")
	if strings.Contains(src, "perfis ...string") || strings.Contains(src, "perfis...") {
		t.Error("main.go ainda tem withAud com lista de perfis")
	}
	if strings.Contains(lerFonte(t, "handlers/auditoria_auth.go"), "perfis ...string") {
		t.Error("AuditoriaAuthMiddleware ainda aceita lista de perfis")
	}
}

type ambienteRotas struct {
	t     *testing.T
	set   func(perfil string)
	chama func(rt rotaAud, metodo string) int
}

func novoAmbienteRotas(t *testing.T) ambienteRotas {
	t.Helper()
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
	return ambienteRotas{
		t: t,
		set: func(perfil string) {
			if _, err := db.Exec(`UPDATE users SET perfil=$2, areas=$3 WHERE id=$1::uuid`, f.UserID, perfil,
				pq.Array([]string{handlers.AreaEmpresa, handlers.AreaConfiguracoes, handlers.AreaComercial, handlers.AreaOperacao, handlers.AreaAcompanhamento})); err != nil {
				t.Fatal(err)
			}
		},
		chama: func(rt rotaAud, metodo string) int {
			r := httptest.NewRequest(metodo, rt.caminho, nil)
			r.Header.Set("Authorization", "Bearer "+tok)
			w := httptest.NewRecorder()
			handlers.AuditoriaAuthMiddleware(db, ok, rt.area, rt.caps...)(w, r)
			return w.Code
		},
	}
}

// Toda rota de escrita recusa diretoria e auditoria (e o gestor, como hoje).
func TestRotas_EscritaRecusaDiretoriaAuditoriaEGestor(t *testing.T) {
	a := novoAmbienteRotas(t)
	escritas := 0
	for _, rt := range rotasAud(t) {
		var metodo string
		switch {
		case len(rt.caps) == 2 && ehEscrita(rt.caps[1]):
			metodo = http.MethodPost
		case len(rt.caps) == 1 && ehEscrita(rt.caps[0]):
			metodo = http.MethodPost
		default:
			continue
		}
		escritas++
		for _, p := range []string{"diretoria", "auditoria", "gestor"} {
			a.set(p)
			if c := a.chama(rt, metodo); c != http.StatusForbidden {
				t.Errorf("%s %s por %s: %d, esperado 403", metodo, rt.caminho, p, c)
			}
		}
		a.set("controladoria")
		if c := a.chama(rt, metodo); c != http.StatusOK {
			t.Errorf("%s %s por controladoria: %d", metodo, rt.caminho, c)
		}
	}
	if escritas < 12 {
		t.Errorf("só %d rotas de escrita reconhecidas; o teste perdeu rotas", escritas)
	}
}

// Comportamento efetivo de hoje para os quatro perfis que já existiam: quem
// passa na leitura (GET) e na escrita (POST) de cada rota. Fonte: main.go e os
// handlers antes da 4.3. O analista é acrescentado nas linhas em que a matriz o inclui.
var todos4 = []string{"controladoria", "gestor", "diretoria", "auditoria"}
var soCtrl = []string{"controladoria"}
var leitoresSemGestor = []string{"controladoria", "auditoria", "diretoria"}

type esperado struct{ leitura, escrita []string }

var comportamentoDeHoje = map[string]esperado{
	"/api/aud/usuarios":                   {soCtrl, soCtrl},
	"/api/aud/usuarios/bloquear":          {soCtrl, soCtrl},
	"/api/aud/usuarios/desbloquear":       {soCtrl, soCtrl},
	"/api/aud/filiais-gerenciais":         {todos4, soCtrl},
	"/api/aud/agrupamentos-filiais":       {todos4, soCtrl},
	"/api/aud/departamentos":              {todos4, soCtrl},
	"/api/aud/departamentos/carga":        {soCtrl, soCtrl},
	"/api/aud/filiais-gerenciais/carga":   {soCtrl, soCtrl},
	"/api/aud/agrupamentos-filiais/carga": {soCtrl, soCtrl},
	"/api/aud/dicionario":                 {todos4, todos4}, // só GET; outros métodos dão 405 no handler
	"/api/aud/dicionario/carga":           {soCtrl, soCtrl},
	"/api/aud/dicionario/conta":           {soCtrl, soCtrl},
	"/api/aud/audit-log":                  {[]string{"controladoria", "auditoria"}, []string{"controladoria", "auditoria"}},
	"/api/aud/dre":                        {leitoresSemGestor, leitoresSemGestor}, // 4.7: DRE do grupo, 403 ao Gerente
	"/api/aud/dre/filiais":                {todos4, todos4},
	"/api/aud/dre/linhas":                 {todos4, todos4},
	"/api/aud/dre/linhas/papel":           {soCtrl, soCtrl},
	"/api/aud/dre/departamentos":          {todos4, todos4},                       // 4.7: o Gerente entra com escopo
	"/api/aud/dre/lancamentos":            {leitoresSemGestor, leitoresSemGestor}, // 4.7: rota antiga fechada ao Gerente
	"/api/aud/dre/trimestres":             {leitoresSemGestor, leitoresSemGestor},
	"/api/aud/meu-departamento":           {todos4, todos4}, // 4.8: passa no portão; o handler só aceita o gestor
	"/api/aud/dre/contas":                 {todos4, todos4}, // 4.7: com escopo
	"/api/aud/dre/contas/lancamentos":     {todos4, todos4}, // 4.7: com escopo
	"/api/aud/dre/fechamentos":            {todos4, soCtrl},
	"/api/aud/dre/fechamentos/reabrir":    {soCtrl, soCtrl},
	"/api/aud/dre/mapa":                   {todos4, soCtrl},
	"/api/aud/dre/estrutura/carga":        {soCtrl, soCtrl},
	"/api/aud/dre/mapa/carga":             {soCtrl, soCtrl},
	"/api/aud/dre/mapa/sem-mapa":          {leitoresSemGestor, leitoresSemGestor}, // 3.2: mostra valor
	"/api/aud/dre/orcado":                 {soCtrl, soCtrl},
	"/api/aud/dados/atualizar":            {soCtrl, soCtrl},
	"/api/aud/dre/lancamentos/importar":   {soCtrl, soCtrl},
	"/api/aud/conferencia":                {leitoresSemGestor, leitoresSemGestor}, // 2.12: leitura irrestrita
	"/api/aud/conferencia/origens":        {leitoresSemGestor, leitoresSemGestor},
	"/api/aud/comparativo":                {leitoresSemGestor, leitoresSemGestor}, // 2.18: leitura irrestrita
	"/api/aud/comparativo/periodos":       {leitoresSemGestor, leitoresSemGestor},
	"/api/aud/comparativo/carga":          {soCtrl, soCtrl},
}

// Onde o analista entra (GET: analistaLeSim ou analistaEscreve; POST: analistaEscreve) — decisão da 4.3, ver spec.
var analistaEscreve = map[string]bool{
	"/api/aud/filiais-gerenciais": true, "/api/aud/agrupamentos-filiais": true, "/api/aud/departamentos": true,
	"/api/aud/departamentos/carga": true, "/api/aud/filiais-gerenciais/carga": true, "/api/aud/agrupamentos-filiais/carga": true, "/api/aud/dicionario/carga": true, "/api/aud/dicionario/conta": true, "/api/aud/dre/linhas/papel": true,
	"/api/aud/dre/mapa": true, "/api/aud/dre/orcado": true, "/api/aud/dados/atualizar": true,
	"/api/aud/dre/lancamentos/importar": true, "/api/aud/dre/estrutura/carga": true, "/api/aud/dre/mapa/carga": true,
	"/api/aud/dre/mapa/sem-mapa": true,
	// rotas só de leitura em que escrita == leitura
	"/api/aud/dicionario": true, "/api/aud/dre": true, "/api/aud/dre/filiais": true, "/api/aud/dre/linhas": true,
	"/api/aud/dre/departamentos": true, "/api/aud/dre/lancamentos": true, "/api/aud/dre/trimestres": true,
	"/api/aud/dre/contas": true, "/api/aud/dre/contas/lancamentos": true, "/api/aud/meu-departamento": true,
	"/api/aud/conferencia": true, "/api/aud/conferencia/origens": true,
	"/api/aud/comparativo": true, "/api/aud/comparativo/periodos": true, "/api/aud/comparativo/carga": true,
}
var analistaLeSim = map[string]bool{
	"/api/aud/filiais-gerenciais": true, "/api/aud/agrupamentos-filiais": true, "/api/aud/departamentos": true,
	"/api/aud/dicionario": true, "/api/aud/dre": true, "/api/aud/dre/filiais": true, "/api/aud/dre/linhas": true,
	"/api/aud/dre/departamentos": true, "/api/aud/dre/lancamentos": true, "/api/aud/dre/trimestres": true,
	"/api/aud/dre/contas": true, "/api/aud/dre/contas/lancamentos": true, "/api/aud/dre/fechamentos": true, "/api/aud/meu-departamento": true,
	"/api/aud/dre/mapa": true,
}

func contem(l []string, p string) bool {
	for _, x := range l {
		if x == p {
			return true
		}
	}
	return false
}

func TestRotas_PerfisExistentesMantemComportamento(t *testing.T) {
	a := novoAmbienteRotas(t)
	vistas := []string{}
	for _, rt := range rotasAud(t) {
		e, ok := comportamentoDeHoje[rt.caminho]
		if !ok {
			if semCapacidadePermitida(rt) {
				continue
			}
			t.Errorf("%s sem linha em comportamentoDeHoje: defina o que cada perfil deve poder", rt.caminho)
			continue
		}
		vistas = append(vistas, rt.caminho)
		for _, p := range todos4 {
			a.set(p)
			if c := a.chama(rt, http.MethodGet); (c == http.StatusOK) != contem(e.leitura, p) {
				t.Errorf("GET %s por %s: %d", rt.caminho, p, c)
			}
			if c := a.chama(rt, http.MethodPost); (c == http.StatusOK) != contem(e.escrita, p) {
				t.Errorf("POST %s por %s: %d", rt.caminho, p, c)
			}
		}
		// Analista: só onde a matriz o inclui.
		a.set("analista")
		if c := a.chama(rt, http.MethodGet); (c == http.StatusOK) != (analistaLeSim[rt.caminho] || analistaEscreve[rt.caminho]) {
			t.Errorf("GET %s por analista: %d", rt.caminho, c)
		}
		if c := a.chama(rt, http.MethodPost); (c == http.StatusOK) != analistaEscreve[rt.caminho] {
			t.Errorf("POST %s por analista: %d", rt.caminho, c)
		}
	}
	for c := range comportamentoDeHoje {
		if !contem(vistas, c) {
			t.Errorf("%s da tabela não existe em main.go", c)
		}
	}
}

// O analista carrega e edita Mapa e estrutura; fechar e reabrir dão 403.
func TestRotas_AnalistaCarregaEditaMasNaoFechaNemReabre(t *testing.T) {
	a := novoAmbienteRotas(t)
	a.set("analista")
	rotas := map[string]rotaAud{}
	for _, rt := range rotasAud(t) {
		rotas[rt.caminho] = rt
	}
	casos := []struct {
		caminho, metodo string
		want            int
	}{
		{"/api/aud/dre/lancamentos/importar", http.MethodPost, 200},
		{"/api/aud/dados/atualizar", http.MethodPost, 200},
		{"/api/aud/dre/orcado", http.MethodPut, 200},
		{"/api/aud/dre/mapa", http.MethodPut, 200},
		{"/api/aud/dre/estrutura/carga", http.MethodPost, 200},
		{"/api/aud/dre/mapa/carga", http.MethodPost, 200},
		{"/api/aud/dre/linhas/papel", http.MethodPut, 200},
		{"/api/aud/departamentos", http.MethodPost, 200},
		{"/api/aud/departamentos/carga", http.MethodPost, 200},
		{"/api/aud/filiais-gerenciais/carga", http.MethodPost, 200},
		{"/api/aud/agrupamentos-filiais/carga", http.MethodPost, 200},
		{"/api/aud/filiais-gerenciais", http.MethodPut, 200},
		{"/api/aud/agrupamentos-filiais", http.MethodDelete, 200},
		{"/api/aud/dicionario/carga", http.MethodPost, 200},
		{"/api/aud/dicionario/conta", http.MethodPut, 200},
		{"/api/aud/dre/fechamentos", http.MethodGet, 200},
		{"/api/aud/dre/fechamentos", http.MethodPost, 403},
		{"/api/aud/dre/fechamentos/reabrir", http.MethodPost, 403},
		{"/api/aud/usuarios", http.MethodGet, 403},
		{"/api/aud/usuarios", http.MethodPut, 403},
		{"/api/aud/audit-log", http.MethodGet, 403},
	}
	for _, c := range casos {
		rt, ok := rotas[c.caminho]
		if !ok {
			t.Fatalf("rota %s ausente", c.caminho)
		}
		if got := a.chama(rt, c.metodo); got != c.want {
			t.Errorf("analista %s %s: %d, esperado %d", c.metodo, c.caminho, got, c.want)
		}
	}
}
