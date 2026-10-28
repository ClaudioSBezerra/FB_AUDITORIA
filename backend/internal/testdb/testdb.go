// Package testdb prepara um PostgreSQL local de teste para os testes de
// integração do backend.
//
// Regras de segurança: o nome do banco termina em "_test" e nunca é
// "fb_auditoria"; DATABASE_URL do ambiente nunca é usada. O banco é recriado
// limpo uma vez por execução de `go test` (por processo de teste).
//
// Variáveis de ambiente (todas opcionais):
//
//	TEST_DB_HOST      (padrão localhost; só aceita localhost, 127.0.0.1 ou ::1)
//	TEST_DB_PORT      (padrão 5432)
//	TEST_DB_USER      (padrão postgres)
//	TEST_DB_PASSWORD  (padrão postgres)
//	TEST_DB_NAME      (padrão fb_auditoria_test)
//
// `go test ./...` roda pacotes em paralelo sobre o mesmo banco; use
// `go test -p 1 ./...` quando mais de um pacote usar este helper.
package testdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

const nomePadrao = "fb_auditoria_test"

var nomeValido = regexp.MustCompile(`^[a-z0-9_]+_test$`)

// ValidarNome recusa qualquer nome que não seja um banco de teste.
func ValidarNome(nome string) error {
	if nome == "fb_auditoria" {
		return fmt.Errorf("testdb: recusado: %q é o banco real; o banco de teste deve terminar em _test", nome)
	}
	if !nomeValido.MatchString(nome) {
		return fmt.Errorf("testdb: nome de banco inválido %q: deve terminar em _test (apenas a-z, 0-9 e _)", nome)
	}
	return nil
}

func env(chave, padrao string) string {
	if v := os.Getenv(chave); v != "" {
		return v
	}
	return padrao
}

func dsn(banco string) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		env("TEST_DB_HOST", "localhost"), env("TEST_DB_PORT", "5432"),
		env("TEST_DB_USER", "postgres"), env("TEST_DB_PASSWORD", "postgres"), banco)
}

var (
	once    sync.Once
	onceErr error
	onceDB  *sql.DB
)

// Abrir valida o nome ANTES de qualquer conexão, recria o banco (uma vez por
// processo) e aplica as migrations. Devolve o mesmo *sql.DB nas chamadas
// seguintes.
func Abrir(nome string) (*sql.DB, error) {
	if err := ValidarNome(nome); err != nil {
		return nil, err
	}
	if h := env("TEST_DB_HOST", "localhost"); h != "localhost" && h != "127.0.0.1" && h != "::1" {
		return nil, fmt.Errorf("testdb: recusado: TEST_DB_HOST=%q não é local; o banco de teste é recriado com DROP DATABASE", h)
	}
	once.Do(func() { onceDB, onceErr = preparar(nome) })
	return onceDB, onceErr
}

func preparar(nome string) (*sql.DB, error) {
	admin, err := sql.Open("postgres", dsn("postgres"))
	if err != nil {
		return nil, err
	}
	defer admin.Close()
	if err := admin.Ping(); err != nil {
		return nil, fmt.Errorf("testdb: PostgreSQL inacessível em %s:%s (usuário %s): %w — suba o Postgres local ou ajuste TEST_DB_*",
			env("TEST_DB_HOST", "localhost"), env("TEST_DB_PORT", "5432"), env("TEST_DB_USER", "postgres"), err)
	}
	// nome já validado por ValidarNome (só a-z, 0-9, _), seguro como identificador.
	if _, err := admin.Exec(`DROP DATABASE IF EXISTS "` + nome + `" WITH (FORCE)`); err != nil {
		return nil, fmt.Errorf("testdb: DROP DATABASE: %w", err)
	}
	if _, err := admin.Exec(`CREATE DATABASE "` + nome + `"`); err != nil {
		return nil, fmt.Errorf("testdb: CREATE DATABASE: %w", err)
	}
	db, err := sql.Open("postgres", dsn(nome))
	if err != nil {
		return nil, err
	}
	if err := aplicarMigrations(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func dirMigrations() string {
	_, arq, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(arq), "..", "..", "migrations")
}

// aplicarMigrations repete a regra do runner do main: ordem de nome, uma Exec por arquivo.
func aplicarMigrations(db *sql.DB) error {
	arqs, err := filepath.Glob(filepath.Join(dirMigrations(), "*.sql"))
	if err != nil || len(arqs) == 0 {
		return fmt.Errorf("testdb: migrations não encontradas em %s: %v", dirMigrations(), err)
	}
	sort.Strings(arqs)
	for _, a := range arqs {
		b, err := os.ReadFile(a)
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(b)); err != nil {
			return fmt.Errorf("testdb: migration %s: %w", filepath.Base(a), err)
		}
	}
	return nil
}

// Open devolve o banco de teste pronto ou falha o teste com mensagem clara.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	db, err := Abrir(env("TEST_DB_NAME", nomePadrao))
	if err != nil {
		t.Fatalf("%v", err)
	}
	return db
}

// Fixture identifica a empresa e o usuário criados para um teste.
type Fixture struct {
	EmpresaID string
	UserID    string
}

// NovaEmpresa cria ambiente, grupo, empresa e um usuário `controladoria`
// próprios do teste (nada depende de ordem entre testes).
func NovaEmpresa(t testing.TB, db *sql.DB) Fixture {
	t.Helper()
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("rand: %v", err)
	}
	sufixo := hex.EncodeToString(buf)
	hash, err := bcrypt.GenerateFromPassword([]byte("senha-de-teste"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	var f Fixture
	var envID, grpID string
	passos := []struct {
		q    string
		args []any
		dst  *string
	}{
		{`INSERT INTO environments (name) VALUES ($1) RETURNING id`, []any{"env-" + sufixo}, &envID},
		{`INSERT INTO enterprise_groups (environment_id, name) VALUES ($1::uuid,$2) RETURNING id`, []any{&envID, "grp-" + sufixo}, &grpID},
		{`INSERT INTO companies (group_id, name) VALUES ($1::uuid,$2) RETURNING id`, []any{&grpID, "emp-" + sufixo}, &f.EmpresaID},
		{`INSERT INTO users (email, password_hash, full_name, perfil) VALUES ($1,$2,$3,'controladoria') RETURNING id`,
			[]any{sufixo + "@teste.local", string(hash), "Controller " + sufixo}, &f.UserID},
	}
	for _, p := range passos {
		args := make([]any, len(p.args))
		for i, a := range p.args {
			if s, ok := a.(*string); ok {
				args[i] = *s
			} else {
				args[i] = a
			}
		}
		if err := db.QueryRow(p.q, args...).Scan(p.dst); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	return f
}
