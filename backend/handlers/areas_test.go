package handlers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"

	"github.com/lib/pq"
)

// updateMigracao extrai o UPDATE do arquivo 005 (o teste roda o SQL real).
func updateMigracao(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "migrations", "005_areas_novas.sql"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	i := strings.Index(s, "UPDATE users")
	if i < 0 {
		t.Fatal("UPDATE não encontrado em 005")
	}
	j := strings.Index(s[i:], ";")
	if j < 0 {
		t.Fatal("UPDATE não encontrado em 005")
	}
	return s[i : i+j+1]
}

func areasDe(t *testing.T, db *sql.DB, id string) []string {
	t.Helper()
	var a pq.StringArray
	if err := db.QueryRow(`SELECT areas FROM users WHERE id=$1::uuid`, id).Scan(&a); err != nil {
		t.Fatal(err)
	}
	return []string(a)
}

func TestAreas_MigracaoMatriz(t *testing.T) {
	db := testdb.Open(t)
	upd := updateMigracao(t)
	casos := []struct{ in, want []string }{
		{[]string{"estrategico", "operacional", "administrativo"}, []string{"empresa", "operacao", "configuracoes"}},
		{[]string{"administrativo"}, []string{"configuracoes"}},
		{[]string{}, []string{}},
		{[]string{"empresa", "configuracoes"}, []string{"empresa", "configuracoes"}},
	}
	for _, c := range casos {
		f := testdb.NovaEmpresa(t, db)
		if _, err := db.Exec(`UPDATE users SET areas=$2 WHERE id=$1::uuid`, f.UserID, pq.Array(c.in)); err != nil {
			t.Fatal(err)
		}
		for rodada := 0; rodada < 2; rodada++ {
			if _, err := db.Exec(upd); err != nil {
				t.Fatal(err)
			}
			got := areasDe(t, db, f.UserID)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("in=%v rodada=%d got=%v want=%v", c.in, rodada, got, c.want)
			}
		}
	}
}

func TestAreas_DefaultNovo(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	got := strings.Join(areasDe(t, db, f.UserID), ",")
	if got != "empresa,configuracoes,comercial,operacao" {
		t.Fatalf("default=%s", got)
	}
}

func TestAreas_PutComIdAntigoRecusa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	for _, antigo := range []string{"estrategico", "administrativo", "operacional"} {
		r := httptest.NewRequest(http.MethodPut, "/api/aud/usuarios",
			strings.NewReader(`{"user_id":"`+f.UserID+`","perfil":"controladoria","areas":["`+antigo+`"]}`))
		w := chamar(AudUsuariosHandler(db), reqComCtx(r, f))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: code=%d", antigo, w.Code)
		}
	}
	if got := strings.Join(areasDe(t, db, f.UserID), ","); got != "empresa,configuracoes,comercial,operacao" {
		t.Fatalf("areas alteradas: %s", got)
	}
}

func TestAreas_Rotas403(t *testing.T) {
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
	chama := func(area string) int {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		AuditoriaAuthMiddleware(db, ok, area)(w, r)
		return w.Code
	}
	set := func(areas ...string) {
		if _, err := db.Exec(`UPDATE users SET areas=$2 WHERE id=$1::uuid`, f.UserID, pq.Array(areas)); err != nil {
			t.Fatal(err)
		}
	}
	set(AreaConfiguracoes)
	if c := chama(AreaEmpresa); c != http.StatusForbidden {
		t.Fatalf("DRE só com configuracoes: %d", c)
	}
	if c := chama(AreaConfiguracoes); c != http.StatusOK {
		t.Fatalf("usuarios com configuracoes: %d", c)
	}
	set(AreaEmpresa)
	if c := chama(AreaConfiguracoes); c != http.StatusForbidden {
		t.Fatalf("usuarios só com empresa: %d", c)
	}
	if c := chama(AreaEmpresa); c != http.StatusOK {
		t.Fatalf("DRE com empresa: %d", c)
	}
}

func TestAreas_PutComIdsNovos(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO user_environments (user_id, environment_id)
		SELECT $1::uuid, eg.environment_id FROM companies c JOIN enterprise_groups eg ON eg.id=c.group_id WHERE c.id=$2::uuid`,
		f.UserID, f.EmpresaID); err != nil {
		t.Fatal(err)
	}
	for _, novo := range []string{"empresa", "configuracoes", "comercial", "operacao", "acompanhamento"} {
		r := httptest.NewRequest(http.MethodPut, "/api/aud/usuarios",
			strings.NewReader(`{"user_id":"`+f.UserID+`","perfil":"controladoria","areas":["`+novo+`"]}`))
		w := chamar(AudUsuariosHandler(db), reqComCtx(r, f))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: code=%d body=%s", novo, w.Code, w.Body.String())
		}
		if got := strings.Join(areasDe(t, db, f.UserID), ","); got != novo {
			t.Fatalf("%s: lido %s", novo, got)
		}
	}
}
