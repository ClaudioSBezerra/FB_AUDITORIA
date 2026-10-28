package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"fb_auditoria/internal/testdb"
)

// História 4.9: o administrador de plataforma cria o usuário da empresa.
// Dados fictícios; SMTP trocado por funções de teste.

func adminDePlataforma(t *testing.T, db *sql.DB, f testdb.Fixture) string {
	t.Helper()
	t.Setenv("JWT_SECRET", segredoTeste)
	if _, err := db.Exec(`UPDATE users SET role='admin' WHERE id=$1::uuid`, f.UserID); err != nil {
		t.Fatal(err)
	}
	tok, err := GenerateToken(f.UserID, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func smtpDeTeste(t *testing.T, configurado bool, enviar func(email, nome, link string, horas int) error) {
	t.Helper()
	ant1, ant2 := smtpConfigurado, enviarConvite
	smtpConfigurado = func() bool { return configurado }
	enviarConvite = enviar
	t.Cleanup(func() { smtpConfigurado, enviarConvite = ant1, ant2 })
}

func postCriar(db *sql.DB, tok, corpo string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/admin/users/empresa", strings.NewReader(corpo))
	r.Header.Set("Authorization", "Bearer "+tok)
	return chamar(AuthMiddleware(db, AdminUsuarioEmpresaHandler(db), "admin"), r)
}

type respCriar struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Link          string `json:"link"`
	Caminho       string `json:"caminho"`
	EmailEnviado  bool   `json:"email_enviado"`
	ValidadeHoras int    `json:"validade_horas"`
	Departamentos []struct{ Codigo string }
}

func corpoCriar(empresa, email, perfil string, deptos ...string) string {
	b, _ := json.Marshal(map[string]any{
		"email": email, "full_name": "Gerente Ficticio", "perfil": perfil,
		"areas": []string{"empresa"}, "departamentos": deptos, "company_id": empresa,
	})
	return string(b)
}

func tokenDoCaminho(c string) string { return strings.TrimPrefix(c, "/reset-senha?token=") }

func TestCriarUsuarioEmpresa_CriaVinculadoELinkPermiteDefinirSenhaELogar(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	tok := adminDePlataforma(t, db, f)
	smtpDeTeste(t, false, nil)
	d1, d2 := novoDepto(t, db, f.EmpresaID, "D1"), novoDepto(t, db, f.EmpresaID, "D2")
	_ = novoDepto(t, db, f.EmpresaID, "D3")

	w := postCriar(db, tok, corpoCriar(f.EmpresaID, "  Novo.Gerente@Teste.Local ", "gestor", d1, d2))
	if w.Code != http.StatusCreated {
		t.Fatalf("criar: %d %s", w.Code, w.Body.String())
	}
	var resp respCriar
	json.Unmarshal(w.Body.Bytes(), &resp)
	// Sem SMTP o link vai na resposta (sem APP_URL só o caminho).
	if resp.EmailEnviado || resp.Caminho == "" || resp.ValidadeHoras != 72 {
		t.Fatalf("resposta: %+v", resp)
	}
	email := "novo.gerente@teste.local"
	if resp.Email != email {
		t.Errorf("e-mail normalizado: %q", resp.Email)
	}

	// Vinculado à empresa certa, com perfil, áreas, departamentos e sem prazo.
	var perfil, role string
	var verif bool
	var trial sql.NullTime
	if err := db.QueryRow(`SELECT perfil, role, is_verified, trial_ends_at FROM users WHERE id=$1::uuid`, resp.ID).Scan(&perfil, &role, &verif, &trial); err != nil {
		t.Fatal(err)
	}
	if perfil != "gestor" || role != "user" || !verif || trial.Valid {
		t.Errorf("usuário: perfil=%s role=%s verif=%v trial=%v", perfil, role, verif, trial)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM user_environments ue JOIN enterprise_groups eg ON eg.environment_id=ue.environment_id
		JOIN companies c ON c.group_id=eg.id WHERE ue.user_id=$1::uuid AND c.id=$2::uuid AND ue.preferred_company_id=$2::uuid`, resp.ID, f.EmpresaID); n != 1 {
		t.Errorf("vínculo à empresa = %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM user_environments WHERE user_id=$1::uuid`, resp.ID); n != 1 {
		t.Errorf("vínculos a ambientes = %d", n)
	}
	if got := strings.Join(codigosVinculados(t, db, resp.ID), ","); got != "D1,D2" {
		t.Errorf("departamentos = %s", got)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE id=$1::uuid AND areas = ARRAY['empresa']`, resp.ID); n != 1 {
		t.Error("áreas não gravadas")
	}

	// Aparece na empresa certa e não aparece na outra.
	lista := func(emp testdb.Fixture) string {
		return chamar(AudUsuariosHandler(db), reqComCtx(httptest.NewRequest(http.MethodGet, "/x", nil), emp)).Body.String()
	}
	if !strings.Contains(lista(f), email) || strings.Contains(lista(outra), email) {
		t.Error("isolamento entre empresas")
	}

	// Antes de definir a senha ninguém entra.
	for _, senha := range []string{"", "senha-de-teste", "12345678", email} {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"`+email+`","password":"`+senha+`"}`))
		r.RemoteAddr = "192.0.2.50:1"
		if w := chamar(LoginHandler(db), r); w.Code == http.StatusOK {
			t.Fatalf("logou sem definir a senha (%q)", senha)
		}
	}
	// Validade do link: 72 horas.
	var exp time.Time
	var tipo string
	if err := db.QueryRow(`SELECT expires_at, type FROM verification_tokens WHERE user_id=$1::uuid`, resp.ID).Scan(&exp, &tipo); err != nil {
		t.Fatal(err)
	}
	if d := time.Until(exp); tipo != "password_reset" || d < 71*time.Hour || d > 73*time.Hour {
		t.Errorf("token: %s expira em %v", tipo, d)
	}

	// O link define a senha (mecanismo de redefinição existente) e depois o login funciona na empresa criada.
	token := tokenDoCaminho(resp.Caminho)
	def := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/auth/reset-password",
			strings.NewReader(`{"token":"`+token+`","password":"minha-senha-nova","confirm_password":"minha-senha-nova"}`))
		return chamar(ResetPasswordHandler(db), r)
	}
	if w := def(); w.Code != http.StatusOK {
		t.Fatalf("definir senha: %d %s", w.Code, w.Body.String())
	}
	if w := def(); w.Code == http.StatusOK {
		t.Error("o link serviu duas vezes")
	}
	r := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"email":"`+email+`","password":"minha-senha-nova"}`))
	r.RemoteAddr = "192.0.2.51:1"
	wl := chamar(LoginHandler(db), r)
	if wl.Code != http.StatusOK || !strings.Contains(wl.Body.String(), f.EmpresaID) {
		t.Fatalf("login depois de definir a senha: %d %s", wl.Code, wl.Body.String())
	}
}

func TestCriarUsuarioEmpresa_TrilhaSemSegredo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	tok := adminDePlataforma(t, db, f)
	smtpDeTeste(t, false, nil)
	d1 := novoDepto(t, db, f.EmpresaID, "D1")
	w := postCriar(db, tok, corpoCriar(f.EmpresaID, "trilha@teste.local", "gestor", d1))
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var resp respCriar
	json.Unmarshal(w.Body.Bytes(), &resp)
	token := tokenDoCaminho(resp.Caminho)
	var hash string
	db.QueryRow(`SELECT password_hash FROM users WHERE id=$1::uuid`, resp.ID).Scan(&hash)

	rows, err := db.Query(`SELECT entidade, entidade_id, acao, COALESCE(payload::text,'') FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='usuario'`, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	criou := false
	for rows.Next() {
		var ent, id, acao, payload string
		rows.Scan(&ent, &id, &acao, &payload)
		for _, proibido := range []string{token, hash, "password", "senha", "token", "reset-senha"} {
			if strings.Contains(strings.ToLower(payload), strings.ToLower(proibido)) {
				t.Errorf("trilha (%s) contém %q: %s", acao, proibido, payload)
			}
		}
		if acao == "criar" && id == resp.ID {
			criou = true
			for _, quer := range []string{"trilha@teste.local", "gestor", "D1", "empresa"} {
				if !strings.Contains(payload, quer) {
					t.Errorf("trilha sem %q: %s", quer, payload)
				}
			}
		}
	}
	if !criou {
		t.Error("criação não foi à trilha")
	}
	// O autor é o administrador.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='criar' AND user_id=$2::uuid`, f.EmpresaID, f.UserID); n != 1 {
		t.Errorf("autor = %d", n)
	}
}

func TestCriarUsuarioEmpresa_ValidacoesNaoGravamNada(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	tok := adminDePlataforma(t, db, f)
	smtpDeTeste(t, false, nil)
	dAlheio := novoDepto(t, db, outra.EmpresaID, "DX")
	d1 := novoDepto(t, db, f.EmpresaID, "D1")
	antes := contar(t, db, `SELECT COUNT(*) FROM users`)
	casos := []struct {
		nome, corpo string
		want        int
	}{
		{"perfil inválido", corpoCriar(f.EmpresaID, "a1@teste.local", "chefe"), 400},
		{"sem perfil", corpoCriar(f.EmpresaID, "a2@teste.local", ""), 400},
		{"e-mail inválido", corpoCriar(f.EmpresaID, "nao-e-email", "gestor"), 400},
		{"e-mail vazio", corpoCriar(f.EmpresaID, "", "gestor"), 400},
		{"sem áreas", `{"email":"a3@teste.local","full_name":"X","perfil":"gestor","areas":[],"company_id":"` + f.EmpresaID + `"}`, 400},
		{"área inválida", `{"email":"a4@teste.local","full_name":"X","perfil":"gestor","areas":["xyz"],"company_id":"` + f.EmpresaID + `"}`, 400},
		{"sem nome", `{"email":"a5@teste.local","full_name":" ","perfil":"gestor","areas":["empresa"],"company_id":"` + f.EmpresaID + `"}`, 400},
		{"departamento em perfil que não é gestor", corpoCriar(f.EmpresaID, "a6@teste.local", "diretoria", d1), 400},
		{"departamento de outra empresa", corpoCriar(f.EmpresaID, "a7@teste.local", "gestor", dAlheio), 400},
		{"departamento inválido", corpoCriar(f.EmpresaID, "a8@teste.local", "gestor", "xx"), 400},
		{"empresa inválida", corpoCriar("nao-uuid", "a9@teste.local", "gestor"), 400},
		{"empresa inexistente", corpoCriar("00000000-0000-0000-0000-000000000000", "b1@teste.local", "gestor"), 404},
		{"corpo inválido", `{`, 400},
	}
	for _, c := range casos {
		if w := postCriar(db, tok, c.corpo); w.Code != c.want {
			t.Errorf("%s: %d %s", c.nome, w.Code, w.Body.String())
		}
	}
	if depois := contar(t, db, `SELECT COUNT(*) FROM users`); depois != antes {
		t.Errorf("usuários gravados em erro: %d -> %d", antes, depois)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM verification_tokens WHERE type='password_reset' AND created_at > now() - interval '1 minute' AND user_id NOT IN (SELECT id FROM users)`); n != 0 {
		t.Error("token órfão")
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='criar'`, f.EmpresaID); n != 0 {
		t.Errorf("trilha de criação em erro: %d", n)
	}
}

func TestCriarUsuarioEmpresa_EmailRepetidoSemDiferenciarMaiuscula409(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	tok := adminDePlataforma(t, db, f)
	smtpDeTeste(t, false, nil)
	if w := postCriar(db, tok, corpoCriar(f.EmpresaID, "repetido@teste.local", "analista")); w.Code != http.StatusCreated {
		t.Fatalf("primeiro: %d %s", w.Code, w.Body.String())
	}
	for _, e := range []string{"repetido@teste.local", "REPETIDO@Teste.Local"} {
		if w := postCriar(db, tok, corpoCriar(f.EmpresaID, e, "analista")); w.Code != http.StatusConflict {
			t.Errorf("%s: %d", e, w.Code)
		}
		// mesmo e-mail em outra empresa: o e-mail é único na plataforma
		if w := postCriar(db, tok, corpoCriar(outra.EmpresaID, e, "analista")); w.Code != http.StatusConflict {
			t.Errorf("%s em outra empresa: %d", e, w.Code)
		}
	}
	// E-mail de um usuário já existente (gravado com maiúscula).
	var existente string
	db.QueryRow(`SELECT email FROM users WHERE id=$1::uuid`, f.UserID).Scan(&existente)
	if w := postCriar(db, tok, corpoCriar(f.EmpresaID, strings.ToUpper(existente), "analista")); w.Code != http.StatusConflict {
		t.Errorf("existente em maiúsculas: %d", w.Code)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE lower(email)='repetido@teste.local'`); n != 1 {
		t.Errorf("usuários com o e-mail = %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE entidade='usuario' AND acao='criar' AND payload::text LIKE '%repetido@teste.local%'`); n != 1 {
		t.Errorf("trilha de criação duplicada = %d", n)
	}
}

func TestCriarUsuarioEmpresa_SoOAdministradorDePlataforma(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	t.Setenv("JWT_SECRET", segredoTeste)
	smtpDeTeste(t, false, nil)
	antes := contar(t, db, `SELECT COUNT(*) FROM users`)
	// Controladoria (usuário comum da plataforma, role 'user').
	tokUser, _ := GenerateToken(f.UserID, "user")
	if w := postCriar(db, tokUser, corpoCriar(f.EmpresaID, "x1@teste.local", "gestor")); w.Code != http.StatusForbidden {
		t.Errorf("controladoria: %d", w.Code)
	}
	// Token que diz admin, mas o cadastro diz user (token velho): o handler confere o cadastro.
	tokVelho, _ := GenerateToken(f.UserID, "admin")
	if w := postCriar(db, tokVelho, corpoCriar(f.EmpresaID, "x2@teste.local", "gestor")); w.Code != http.StatusForbidden {
		t.Errorf("token de admin sem ser admin: %d", w.Code)
	}
	// Sem token.
	r := httptest.NewRequest(http.MethodPost, "/api/admin/users/empresa", strings.NewReader(corpoCriar(f.EmpresaID, "x3@teste.local", "gestor")))
	if w := chamar(AuthMiddleware(db, AdminUsuarioEmpresaHandler(db), "admin"), r); w.Code != http.StatusUnauthorized {
		t.Errorf("sem token: %d", w.Code)
	}
	if depois := contar(t, db, `SELECT COUNT(*) FROM users`); depois != antes {
		t.Errorf("usuário criado por quem não é admin")
	}
	// A listagem de departamentos também é do admin.
	g := httptest.NewRequest(http.MethodGet, "/x?company_id="+f.EmpresaID, nil)
	g.Header.Set("Authorization", "Bearer "+tokVelho)
	if w := chamar(AuthMiddleware(db, AdminUsuarioEmpresaHandler(db), "admin"), g); w.Code != http.StatusForbidden {
		t.Errorf("departamentos por não-admin: %d", w.Code)
	}
}

func TestCriarUsuarioEmpresa_EnvioDoLink(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	tok := adminDePlataforma(t, db, f)
	t.Setenv("APP_URL", "https://app.exemplo.test/")

	// SMTP configurado e envio bem-sucedido: o link vai por e-mail e não na resposta.
	var linkEnviado, paraQuem string
	smtpDeTeste(t, true, func(email, nome, link string, horas int) error {
		linkEnviado, paraQuem = link, email
		return nil
	})
	w := postCriar(db, tok, corpoCriar(f.EmpresaID, "enviado@teste.local", "diretoria"))
	var resp respCriar
	json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusCreated || !resp.EmailEnviado || resp.Link != "" || resp.Caminho != "" {
		t.Fatalf("com SMTP: %d %s", w.Code, w.Body.String())
	}
	if paraQuem != "enviado@teste.local" || !strings.HasPrefix(linkEnviado, "https://app.exemplo.test/reset-senha?token=") {
		t.Errorf("e-mail enviado: %s %s", paraQuem, linkEnviado)
	}
	if strings.Contains(w.Body.String(), tokenDoCaminho(strings.TrimPrefix(linkEnviado, "https://app.exemplo.test"))) {
		t.Error("token na resposta apesar de enviado por e-mail")
	}

	// SMTP configurado, mas o envio falha: a tela recebe o link.
	smtpDeTeste(t, true, func(string, string, string, int) error { return http.ErrAbortHandler })
	w = postCriar(db, tok, corpoCriar(f.EmpresaID, "falhou@teste.local", "diretoria"))
	resp = respCriar{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if w.Code != http.StatusCreated || resp.EmailEnviado || !strings.HasPrefix(resp.Link, "https://app.exemplo.test/reset-senha?token=") {
		t.Fatalf("envio falho: %d %s", w.Code, w.Body.String())
	}

	// Sem SMTP: o link usa APP_URL.
	smtpDeTeste(t, false, nil)
	w = postCriar(db, tok, corpoCriar(f.EmpresaID, "semsmtp@teste.local", "diretoria"))
	resp = respCriar{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Link != "https://app.exemplo.test"+resp.Caminho || resp.EmailEnviado {
		t.Errorf("sem SMTP: %+v", resp)
	}
	// A trilha do envio não guarda o link.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='convite' AND payload::text LIKE '%reset-senha%'`, f.EmpresaID); n != 0 {
		t.Error("link na trilha")
	}
}

func TestCriarUsuarioEmpresa_DepartamentosDaEmpresaParaOFormulario(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	tok := adminDePlataforma(t, db, f)
	novoDepto(t, db, f.EmpresaID, "D1")
	novoDepto(t, db, outra.EmpresaID, "DX")
	r := httptest.NewRequest(http.MethodGet, "/x?company_id="+outra.EmpresaID, nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := chamar(AuthMiddleware(db, AdminUsuarioEmpresaHandler(db), "admin"), r)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"DX"`) || strings.Contains(w.Body.String(), `"D1"`) {
		t.Errorf("departamentos da empresa pedida: %d %s", w.Code, w.Body.String())
	}
}
