package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"
)

// usuarioNaEmpresa cria um usuário (dados fictícios) ligado ao ambiente da empresa.
func usuarioNaEmpresa(t *testing.T, db *sql.DB, f testdb.Fixture, perfil string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`INSERT INTO users (email, password_hash, full_name, perfil)
		VALUES (gen_random_uuid()::text || '@teste.local', 'x', 'Usuario Ficticio', $1) RETURNING id`, perfil).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO user_environments (user_id, environment_id)
		SELECT $1::uuid, eg.environment_id FROM companies c JOIN enterprise_groups eg ON eg.id=c.group_id WHERE c.id=$2::uuid
		ON CONFLICT DO NOTHING`, id, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	return id
}

func novoDepto(t *testing.T, db *sql.DB, empresaID, codigo string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`INSERT INTO auditoria.departamentos (empresa_id, codigo, nome, grupo)
		VALUES ($1::uuid, $2, $3, 'Comercial') RETURNING id`, empresaID, codigo, "Depto "+codigo).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func putUsuario(t *testing.T, db *sql.DB, f testdb.Fixture, corpo string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodPut, "/api/aud/usuarios", strings.NewReader(corpo))
	return chamar(AudUsuariosHandler(db), reqComCtx(r, f))
}

func codigosVinculados(t *testing.T, db *sql.DB, userID string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT d.codigo FROM auditoria.usuario_departamentos ud
		JOIN auditoria.departamentos d ON d.id=ud.departamento_id WHERE ud.user_id=$1::uuid ORDER BY 1`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		rows.Scan(&c)
		out = append(out, c)
	}
	return out
}

func ultimaTrilha(t *testing.T, db *sql.DB, empresaID, entidade, id string) map[string]any {
	t.Helper()
	var p []byte
	if err := db.QueryRow(`SELECT payload FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade=$2 AND entidade_id=$3
		ORDER BY id DESC LIMIT 1`, empresaID, entidade, id).Scan(&p); err != nil {
		t.Fatalf("trilha %s/%s: %v", entidade, id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(p, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestUsuarios_GestorComDoisDepartamentos(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	uid := usuarioNaEmpresa(t, db, f, "analista")
	a, b := novoDepto(t, db, f.EmpresaID, "A1"), novoDepto(t, db, f.EmpresaID, "B1")
	novoDepto(t, db, f.EmpresaID, "C1")

	w := putUsuario(t, db, f, `{"user_id":"`+uid+`","perfil":"gestor","areas":["empresa"],"departamentos":["`+a+`","`+b+`","`+a+`"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d %s", w.Code, w.Body.String())
	}
	if got := strings.Join(codigosVinculados(t, db, uid), ","); got != "A1,B1" {
		t.Fatalf("vínculos %s", got)
	}
	// trilha na mesma operação, com antes e depois
	tr := ultimaTrilha(t, db, f.EmpresaID, "usuario", uid)
	antes, depois := tr["antes"].(map[string]any), tr["depois"].(map[string]any)
	if antes["perfil"] != "analista" || depois["perfil"] != "gestor" || len(depois["departamentos"].([]any)) != 2 || len(antes["departamentos"].([]any)) != 0 {
		t.Fatalf("trilha: %v", tr)
	}
	// GET traz os departamentos de cada usuário
	r := httptest.NewRequest(http.MethodGet, "/api/aud/usuarios", nil)
	wg := chamar(AudUsuariosHandler(db), reqComCtx(r, f))
	var lista []struct {
		ID            string     `json:"id"`
		Departamentos []DeptoRef `json:"departamentos"`
	}
	if err := json.Unmarshal(wg.Body.Bytes(), &lista); err != nil {
		t.Fatal(err)
	}
	achou := false
	for _, u := range lista {
		if u.ID == uid {
			achou = true
			if len(u.Departamentos) != 2 || u.Departamentos[0].Codigo != "A1" {
				t.Fatalf("GET: %+v", u.Departamentos)
			}
		} else if u.Departamentos == nil {
			t.Fatal("departamentos deve ser lista vazia, não null")
		}
	}
	if !achou {
		t.Fatal("usuário ausente da lista")
	}

	// Sem o campo, mantém; com lista menor, troca; com [], limpa.
	putUsuario(t, db, f, `{"user_id":"`+uid+`","perfil":"gestor","areas":["empresa"]}`)
	if len(codigosVinculados(t, db, uid)) != 2 {
		t.Fatal("PUT sem departamentos deveria manter")
	}
	putUsuario(t, db, f, `{"user_id":"`+uid+`","perfil":"gestor","areas":["empresa"],"departamentos":["`+b+`"]}`)
	if got := strings.Join(codigosVinculados(t, db, uid), ","); got != "B1" {
		t.Fatalf("troca: %s", got)
	}
	putUsuario(t, db, f, `{"user_id":"`+uid+`","perfil":"gestor","areas":["empresa"],"departamentos":[]}`)
	if len(codigosVinculados(t, db, uid)) != 0 {
		t.Fatal("[] deveria limpar")
	}
}

func TestUsuarios_DepartamentoEmPerfilQueNaoEGestorRecusa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	uid := usuarioNaEmpresa(t, db, f, "analista")
	a := novoDepto(t, db, f.EmpresaID, "A1")
	for _, perfil := range []string{"analista", "controladoria", "diretoria", "auditoria"} {
		w := putUsuario(t, db, f, `{"user_id":"`+uid+`","perfil":"`+perfil+`","areas":["empresa"],"departamentos":["`+a+`"]}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: code=%d", perfil, w.Code)
		}
	}
	if len(codigosVinculados(t, db, uid)) != 0 {
		t.Fatal("nada deveria ser ligado")
	}
	var perfil string
	db.QueryRow(`SELECT perfil FROM users WHERE id=$1::uuid`, uid).Scan(&perfil)
	if perfil != "analista" {
		t.Fatalf("perfil mudou para %s apesar do 400", perfil)
	}
}

func TestUsuarios_DepartamentoInvalidoOuDeOutraEmpresaRecusa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	uid := usuarioNaEmpresa(t, db, f, "gestor")
	alheio := novoDepto(t, db, outra.EmpresaID, "X1")
	for _, id := range []string{alheio, "nao-e-uuid", "00000000-0000-0000-0000-000000000000"} {
		w := putUsuario(t, db, f, `{"user_id":"`+uid+`","perfil":"gestor","areas":["empresa"],"departamentos":["`+id+`"]}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: code=%d", id, w.Code)
		}
	}
	if len(codigosVinculados(t, db, uid)) != 0 {
		t.Fatal("nada deveria ser ligado")
	}
	// usuário de outra empresa: 404
	estranho := usuarioNaEmpresa(t, db, outra, "gestor")
	if w := putUsuario(t, db, f, `{"user_id":"`+estranho+`","perfil":"gestor","areas":["empresa"]}`); w.Code != http.StatusNotFound {
		t.Errorf("usuário de outra empresa: %d", w.Code)
	}
}

func TestUsuarios_MudarDeGestorLimpaVinculosETrilha(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	uid := usuarioNaEmpresa(t, db, f, "gestor")
	a := novoDepto(t, db, f.EmpresaID, "A1")
	if w := putUsuario(t, db, f, `{"user_id":"`+uid+`","perfil":"gestor","areas":["empresa"],"departamentos":["`+a+`"]}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w := putUsuario(t, db, f, `{"user_id":"`+uid+`","perfil":"analista","areas":["empresa"]}`); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if len(codigosVinculados(t, db, uid)) != 0 {
		t.Fatal("vínculos deveriam ter sido limpos")
	}
	tr := ultimaTrilha(t, db, f.EmpresaID, "usuario", uid)
	antes, depois := tr["antes"].(map[string]any), tr["depois"].(map[string]any)
	if len(antes["departamentos"].([]any)) != 1 || len(depois["departamentos"].([]any)) != 0 || depois["perfil"] != "analista" {
		t.Fatalf("trilha: %v", tr)
	}
}

func TestUsuarios_SoGerirUsuariosAcessa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for _, perfil := range []string{"analista", "gestor", "diretoria", "auditoria"} {
		for _, metodo := range []string{http.MethodGet, http.MethodPut} {
			r := httptest.NewRequest(metodo, "/api/aud/usuarios", strings.NewReader(`{}`))
			c := &AudContext{UserID: f.UserID, EmpresaID: f.EmpresaID, Perfil: perfil}
			r = r.WithContext(context.WithValue(r.Context(), AudContextKey, c))
			if w := chamar(AudUsuariosHandler(db), r); w.Code != http.StatusForbidden {
				t.Errorf("%s %s: %d", perfil, metodo, w.Code)
			}
		}
	}
}

// Configuração, não dado de teste: a limpeza não a apaga diretamente, mas ela
// sai junto com o departamento (cascata) e a limpeza segue funcionando.
func TestUsuarios_LimpezaDeTesteContinuaFuncionando(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	uid := usuarioNaEmpresa(t, db, f, "gestor")
	a := novoDepto(t, db, f.EmpresaID, "A1")
	if _, err := db.Exec(`INSERT INTO auditoria.usuario_departamentos (user_id, departamento_id, empresa_id) VALUES ($1::uuid,$2::uuid,$3::uuid)`,
		uid, a, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`SELECT auditoria.limpar_dados_teste($1::uuid)`, f.EmpresaID); err != nil {
		t.Fatalf("limpeza: %v", err)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.usuario_departamentos WHERE user_id=$1::uuid`, uid); n != 0 {
		t.Errorf("vínculos restantes: %d", n)
	}
	// o usuário continua existindo
	if n := contar(t, db, `SELECT COUNT(*) FROM users WHERE id=$1::uuid`, uid); n != 1 {
		t.Error("usuário não deveria ser apagado")
	}
}

// O vínculo é sempre de departamento da mesma empresa (FK composta).
func TestUsuarios_VinculoExigeMesmaEmpresa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	uid := usuarioNaEmpresa(t, db, f, "gestor")
	alheio := novoDepto(t, db, outra.EmpresaID, "X1")
	if _, err := db.Exec(`INSERT INTO auditoria.usuario_departamentos (user_id, departamento_id, empresa_id) VALUES ($1::uuid,$2::uuid,$3::uuid)`,
		uid, alheio, f.EmpresaID); err == nil {
		t.Fatal("vínculo com departamento de outra empresa aceito")
	}
}
