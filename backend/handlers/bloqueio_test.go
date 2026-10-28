package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fb_auditoria/internal/testdb"
)

const segredoTeste = "segredo-de-teste-com-mais-de-32-bytes-ok"

// fixtureComAmbiente liga o usuário da fixture ao ambiente da empresa (o login acha a empresa por aí).
func fixtureComAmbiente(t *testing.T, db *sql.DB) testdb.Fixture {
	t.Helper()
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO user_environments (user_id, environment_id)
		SELECT $1::uuid, eg.environment_id FROM companies c JOIN enterprise_groups eg ON eg.id=c.group_id WHERE c.id=$2::uuid`,
		f.UserID, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	return f
}

func emailDe(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var e string
	if err := db.QueryRow(`SELECT email FROM users WHERE id=$1::uuid`, id).Scan(&e); err != nil {
		t.Fatal(err)
	}
	return e
}

func entrar(t *testing.T, db *sql.DB, email string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"`+email+`","password":"senha-de-teste"}`))
	r.RemoteAddr = "192.0.2.10:1234"
	return chamar(LoginHandler(db), r)
}

func bloquearSQL(t *testing.T, db *sql.DB, id, motivo string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE users SET bloqueado_em=now(), motivo_bloqueio=$2 WHERE id=$1::uuid`, id, motivo); err != nil {
		t.Fatal(err)
	}
}

// O caso que derrubava o login: coluna vazia virava "vencido agora".
func TestBloqueio_LoginComTrialVazioOuVencidoEntra(t *testing.T) {
	t.Setenv("JWT_SECRET", segredoTeste)
	db := testdb.Open(t)
	vazio := fixtureComAmbiente(t, db)
	vencido := fixtureComAmbiente(t, db)
	if _, err := db.Exec(`UPDATE users SET trial_ends_at = now() - interval '30 days' WHERE id=$1::uuid`, vencido.UserID); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE id=$1::uuid AND trial_ends_at IS NULL`, vazio.UserID); n != 1 {
		t.Fatal("o teste precisa do usuário com trial_ends_at vazio")
	}
	for nome, f := range map[string]testdb.Fixture{"vazio": vazio, "vencido": vencido} {
		w := entrar(t, db, emailDe(t, db, f.UserID))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: login %d %s", nome, w.Code, w.Body.String())
		}
		var resp struct{ Token string }
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Token == "" {
			t.Fatalf("%s: sem token (%v)", nome, err)
		}
	}
}

// Usuário comum com trial vencido não é expulso numa sessão já aberta.
func TestBloqueio_SessaoComTrialVencidoNaoEExpulsa(t *testing.T) {
	t.Setenv("JWT_SECRET", segredoTeste)
	db := testdb.Open(t)
	f := fixtureComAmbiente(t, db)
	if _, err := db.Exec(`UPDATE users SET trial_ends_at = now() - interval '30 days', perfil='controladoria' WHERE id=$1::uuid`, f.UserID); err != nil {
		t.Fatal(err)
	}
	tok, _ := GenerateToken(f.UserID, "user")
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	AuditoriaAuthMiddleware(db, ok, AreaEmpresa)(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("aud: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	AuthMiddleware(db, ok, "")(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("geral: %d", w.Code)
	}
}

func TestBloqueio_LoginRecusadoComMotivoEVoltaAoDesbloquear(t *testing.T) {
	t.Setenv("JWT_SECRET", segredoTeste)
	db := testdb.Open(t)
	f := fixtureComAmbiente(t, db)
	email := emailDe(t, db, f.UserID)
	bloquearSQL(t, db, f.UserID, "ex-funcionario")
	w := entrar(t, db, email)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "bloqueado") || !strings.Contains(w.Body.String(), "ex-funcionario") {
		t.Fatalf("login bloqueado: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "token") {
		t.Fatal("não deveria vir token")
	}
	// senha errada não revela o bloqueio
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"`+email+`","password":"errada"}`))
	r.RemoteAddr = "192.0.2.11:1234"
	if w := chamar(LoginHandler(db), r); w.Code != http.StatusUnauthorized || strings.Contains(w.Body.String(), "bloqueado") {
		t.Fatalf("senha errada: %d %s", w.Code, w.Body.String())
	}
	if _, err := db.Exec(`UPDATE users SET bloqueado_em=NULL, motivo_bloqueio=NULL WHERE id=$1::uuid`, f.UserID); err != nil {
		t.Fatal(err)
	}
	if w := entrar(t, db, email); w.Code != http.StatusOK {
		t.Fatalf("depois de desbloquear: %d", w.Code)
	}
}

func TestBloqueio_SessaoAbertaRecebe403NaRequisicaoSeguinte(t *testing.T) {
	t.Setenv("JWT_SECRET", segredoTeste)
	db := testdb.Open(t)
	f := fixtureComAmbiente(t, db)
	tok, _ := GenerateToken(f.UserID, "user")
	ok := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }
	chama := func(h http.HandlerFunc) int {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		h(w, r)
		return w.Code
	}
	aud := AuditoriaAuthMiddleware(db, ok, AreaEmpresa)
	geral := AuthMiddleware(db, ok, "")
	if chama(aud) != 200 || chama(geral) != 200 {
		t.Fatal("antes de bloquear deveria passar")
	}
	bloquearSQL(t, db, f.UserID, "teste")
	if c := chama(aud); c != http.StatusForbidden {
		t.Errorf("rota /api/aud: %d", c)
	}
	if c := chama(geral); c != http.StatusForbidden {
		t.Errorf("rota geral: %d", c)
	}
	// admin de plataforma bloqueado também é barrado
	if _, err := db.Exec(`UPDATE users SET role='admin' WHERE id=$1::uuid`, f.UserID); err != nil {
		t.Fatal(err)
	}
	tok, _ = GenerateToken(f.UserID, "admin")
	if chama(aud) != 403 || chama(geral) != 403 {
		t.Error("admin bloqueado deveria receber 403")
	}
	// usuário apagado não passa
	if _, err := db.Exec(`UPDATE users SET bloqueado_em=NULL WHERE id=$1::uuid`, f.UserID); err != nil {
		t.Fatal(err)
	}
	if chama(geral) != 200 {
		t.Fatal("desbloqueado deveria passar")
	}
}

func TestBloqueio_RenovacaoDeSessaoRecusada(t *testing.T) {
	t.Setenv("JWT_SECRET", segredoTeste)
	db := testdb.Open(t)
	f := fixtureComAmbiente(t, db)
	renovar := func(token string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
		r.AddCookie(&http.Cookie{Name: "refresh_token", Value: token})
		return chamar(RefreshHandler(db), r)
	}
	guardar := func(token string) {
		refreshTokenStore.Store(token, refreshTokenData{UserID: f.UserID, Role: "user", ExpiresAt: time.Now().Add(time.Hour)})
	}
	guardar("refresh-ok-" + f.UserID)
	if w := renovar("refresh-ok-" + f.UserID); w.Code != http.StatusOK {
		t.Fatalf("renovação normal: %d %s", w.Code, w.Body.String())
	}
	bloquearSQL(t, db, f.UserID, "fim do contrato")
	guardar("refresh-bloq-" + f.UserID)
	w := renovar("refresh-bloq-" + f.UserID)
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "fim do contrato") {
		t.Fatalf("renovação bloqueada: %d %s", w.Code, w.Body.String())
	}
	if _, ok := refreshTokenStore.Load("refresh-bloq-" + f.UserID); ok {
		t.Error("refresh token deveria ser descartado")
	}
}

func reqComo(r *http.Request, f testdb.Fixture, perfil string, admin bool) *http.Request {
	c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: perfil, Areas: []string{AreaConfiguracoes}, Admin: admin}
	return r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
}

func postBloqueio(db *sql.DB, bloquear bool, f testdb.Fixture, perfil string, admin bool, corpo string) *httptest.ResponseRecorder {
	h := AudUsuarioDesbloquearHandler(db)
	if bloquear {
		h = AudUsuarioBloquearHandler(db)
	}
	return chamar(h, reqComo(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(corpo)), f, perfil, admin))
}

func TestBloqueio_EndpointsExigemMotivoETrilha(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	alvo := usuarioNaEmpresa(t, db, f, "analista")
	for _, c := range []string{`{"user_id":"` + alvo + `"}`, `{"user_id":"` + alvo + `","motivo":"   "}`} {
		if w := postBloqueio(db, true, f, "controladoria", false, c); w.Code != http.StatusBadRequest {
			t.Errorf("bloquear sem motivo %q: %d", c, w.Code)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE id=$1::uuid AND bloqueado_em IS NOT NULL`, alvo); n != 0 {
		t.Fatal("não deveria bloquear sem motivo")
	}
	if w := postBloqueio(db, true, f, "controladoria", false, `{"user_id":"`+alvo+`","motivo":"acesso indevido"}`); w.Code != http.StatusOK {
		t.Fatalf("bloquear: %d %s", w.Code, w.Body.String())
	}
	var por, motivo sql.NullString
	db.QueryRow(`SELECT bloqueado_por::text, motivo_bloqueio FROM users WHERE id=$1::uuid`, alvo).Scan(&por, &motivo)
	if por.String != f.UserID || motivo.String != "acesso indevido" {
		t.Fatalf("gravou por=%v motivo=%v", por, motivo)
	}
	tr := ultimaTrilha(t, db, f.EmpresaID, "usuario", alvo)
	if tr["motivo"] != "acesso indevido" || tr["depois"].(map[string]any)["bloqueado"] != true {
		t.Fatalf("trilha: %v", tr)
	}
	if w := postBloqueio(db, true, f, "controladoria", false, `{"user_id":"`+alvo+`","motivo":"de novo"}`); w.Code != http.StatusConflict {
		t.Errorf("bloquear já bloqueado: %d", w.Code)
	}
	// lista mostra o bloqueio
	wl := chamar(AudUsuariosHandler(db), reqComCtx(httptest.NewRequest(http.MethodGet, "/x", nil), f))
	var lista []struct {
		ID, Motivo string
		Bloqueado  bool
	}
	json.Unmarshal([]byte(strings.NewReplacer(`"motivo_bloqueio"`, `"Motivo"`, `"bloqueado"`, `"Bloqueado"`, `"id"`, `"ID"`).Replace(wl.Body.String())), &lista)
	achou := false
	for _, u := range lista {
		if u.ID == alvo {
			achou = true
			if !u.Bloqueado || u.Motivo != "acesso indevido" {
				t.Errorf("lista: %+v", u)
			}
		}
	}
	if !achou {
		t.Error("alvo fora da lista")
	}

	// desbloqueio também exige motivo
	if w := postBloqueio(db, false, f, "controladoria", false, `{"user_id":"`+alvo+`"}`); w.Code != http.StatusBadRequest {
		t.Errorf("desbloquear sem motivo: %d", w.Code)
	}
	if w := postBloqueio(db, false, f, "controladoria", false, `{"user_id":"`+alvo+`","motivo":"liberado pela diretoria"}`); w.Code != http.StatusOK {
		t.Fatalf("desbloquear: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE id=$1::uuid AND bloqueado_em IS NULL AND bloqueado_por IS NULL AND motivo_bloqueio IS NULL`, alvo); n != 1 {
		t.Fatal("deveria estar livre")
	}
	tr = ultimaTrilha(t, db, f.EmpresaID, "usuario", alvo)
	if tr["motivo"] != "liberado pela diretoria" {
		t.Fatalf("trilha: %v", tr)
	}
	if w := postBloqueio(db, false, f, "controladoria", false, `{"user_id":"`+alvo+`","motivo":"x"}`); w.Code != http.StatusConflict {
		t.Errorf("desbloquear quem não está bloqueado: %d", w.Code)
	}
}

func TestBloqueio_ProprioUsuarioOutraEmpresaECapacidade(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	if w := postBloqueio(db, true, f, "controladoria", false, `{"user_id":"`+f.UserID+`","motivo":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("auto-bloqueio: %d", w.Code)
	}
	estranho := usuarioNaEmpresa(t, db, outra, "analista")
	if w := postBloqueio(db, true, f, "controladoria", false, `{"user_id":"`+estranho+`","motivo":"x"}`); w.Code != http.StatusNotFound {
		t.Errorf("outra empresa: %d", w.Code)
	}
	alvo := usuarioNaEmpresa(t, db, f, "analista")
	for _, p := range []string{"analista", "gestor", "diretoria", "auditoria"} {
		for _, bloquear := range []bool{true, false} {
			if w := postBloqueio(db, bloquear, f, p, false, `{"user_id":"`+alvo+`","motivo":"x"}`); w.Code != http.StatusForbidden {
				t.Errorf("%s bloquear=%v: %d", p, bloquear, w.Code)
			}
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE id=$1::uuid AND bloqueado_em IS NOT NULL`, alvo); n != 0 {
		t.Fatal("não deveria bloquear")
	}
}

func TestBloqueio_UltimoAdministradorNaoPodeSerBloqueado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	alvo := usuarioNaEmpresa(t, db, f, "controladoria")
	// isola: o alvo é o único administrador ativo (os demais voltam no fim)
	rows, err := db.Query(`UPDATE users SET role='user' WHERE role='admin' RETURNING id`)
	if err != nil {
		t.Fatal(err)
	}
	var antigos []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		antigos = append(antigos, id)
	}
	rows.Close()
	t.Cleanup(func() {
		for _, id := range antigos {
			db.Exec(`UPDATE users SET role='admin' WHERE id=$1::uuid`, id)
		}
	})
	if _, err := db.Exec(`UPDATE users SET role='admin' WHERE id=$1::uuid`, alvo); err != nil {
		t.Fatal(err)
	}
	// Controller comum não bloqueia administrador de plataforma.
	if w := postBloqueio(db, true, f, "controladoria", false, `{"user_id":"`+alvo+`","motivo":"x"}`); w.Code != http.StatusForbidden {
		t.Errorf("controller bloqueando admin: %d", w.Code)
	}
	// Admin de plataforma (outro usuário, que não é o admin ativo do banco) tenta bloquear o último.
	if w := postBloqueio(db, true, f, "controladoria", true, `{"user_id":"`+alvo+`","motivo":"x"}`); w.Code != http.StatusConflict {
		t.Fatalf("último admin: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE id=$1::uuid AND bloqueado_em IS NOT NULL`, alvo); n != 0 {
		t.Fatal("o último admin foi bloqueado")
	}
	// Com outro administrador ativo, pode.
	outro := usuarioNaEmpresa(t, db, f, "controladoria")
	db.Exec(`UPDATE users SET role='admin' WHERE id=$1::uuid`, outro)
	if w := postBloqueio(db, true, f, "controladoria", true, `{"user_id":"`+alvo+`","motivo":"x"}`); w.Code != http.StatusOK {
		t.Fatalf("com outro admin: %d %s", w.Code, w.Body.String())
	}
	// e agora o que restou é o último.
	if w := postBloqueio(db, true, f, "controladoria", true, `{"user_id":"`+outro+`","motivo":"x"}`); w.Code != http.StatusConflict {
		t.Fatalf("restante: %d", w.Code)
	}
}

func TestBloqueio_BloqueadoPorSomeSemQuebrarABloqueado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	quem := usuarioNaEmpresa(t, db, f, "controladoria")
	alvo := usuarioNaEmpresa(t, db, f, "analista")
	if _, err := db.Exec(`UPDATE users SET bloqueado_em=now(), bloqueado_por=$2::uuid, motivo_bloqueio='x' WHERE id=$1::uuid`, alvo, quem); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id=$1::uuid`, quem); err != nil {
		t.Fatalf("apagar quem bloqueou: %v", err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE id=$1::uuid AND bloqueado_em IS NOT NULL AND bloqueado_por IS NULL`, alvo); n != 1 {
		t.Fatal("bloqueio deveria permanecer com bloqueado_por nulo")
	}
}

func TestBloqueio_UsuarioApagadoNaoPassa(t *testing.T) {
	t.Setenv("JWT_SECRET", segredoTeste)
	db := testdb.Open(t)
	f := fixtureComAmbiente(t, db)
	tok, _ := GenerateToken(f.UserID, "user")
	if _, err := db.Exec(`DELETE FROM user_environments WHERE user_id=$1::uuid`, f.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM users WHERE id=$1::uuid`, f.UserID); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	AuthMiddleware(db, func(http.ResponseWriter, *http.Request) {}, "")(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("%d", w.Code)
	}
}
