package main

// FB_AUDITORIA — Controladoria e Auditoria (DRE, verbas, pagamentos a terceiros)
import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"fb_auditoria/handlers"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

const (
	BackendVersion = "0.1.0"
	FeatureSet     = "Auditoria — DRE"
)

type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	Service   string `json:"service"`
	Version   string `json:"version"`
	Features  string `json:"features"`
	Database  string `json:"database"`
	DBError   string `json:"db_error,omitempty"`
}

var (
	db      *sql.DB
	dbMutex sync.RWMutex
	dbErr   error
)

func getDB() *sql.DB {
	dbMutex.RLock()
	defer dbMutex.RUnlock()
	return db
}

// ensureDatabaseExists conecta no banco 'postgres' (default) e cria a database
// alvo se ela não existir. Necessário quando o volume do PostgreSQL persiste
// entre redeploys mas o POSTGRES_DB foi mudado (o postgres só cria a DB
// na primeira inicialização do volume).
func ensureDatabaseExists(connStr string) error {
	dbName := ""
	prefixEnd := strings.Index(connStr, "://")
	if prefixEnd < 0 {
		return nil
	}
	rest := connStr[prefixEnd+3:]
	slashIdx := strings.Index(rest, "/")
	if slashIdx < 0 {
		return nil
	}
	afterSlash := rest[slashIdx+1:]
	queryIdx := strings.Index(afterSlash, "?")
	if queryIdx < 0 {
		dbName = afterSlash
	} else {
		dbName = afterSlash[:queryIdx]
	}
	if dbName == "" || dbName == "postgres" {
		return nil
	}

	adminConnStr := connStr[:prefixEnd+3] + rest[:slashIdx+1] + "postgres"
	if queryIdx >= 0 {
		adminConnStr += "?" + afterSlash[queryIdx+1:]
	}

	adminConn, err := sql.Open("postgres", adminConnStr)
	if err != nil {
		return err
	}
	defer adminConn.Close()

	if err := adminConn.Ping(); err != nil {
		return err
	}

	var exists bool
	err = adminConn.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	fmt.Printf("Database %q does not exist. Creating...\n", dbName)
	_, err = adminConn.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, strings.ReplaceAll(dbName, `"`, ``)))
	if err != nil {
		return err
	}
	fmt.Printf("Database %q created successfully\n", dbName)
	return nil
}

func initDBAsync() {
	go func() {
		var conn *sql.DB
		var err error
		connStr := os.Getenv("DATABASE_URL")
		if connStr == "" {
			connStr = "postgres://postgres:postgres@localhost:5432/fb_auditoria?sslmode=disable"
			fmt.Println("DATABASE_URL not set, using default local connection:", connStr)
		}

		attempt := 0
		for {
			attempt++

			if attempt == 1 || attempt%6 == 0 {
				if ensureErr := ensureDatabaseExists(connStr); ensureErr != nil {
					fmt.Printf("ensureDatabaseExists falhou (attempt %d): %v\n", attempt, ensureErr)
				}
			}

			conn, err = sql.Open("postgres", connStr)
			if err == nil {
				err = conn.Ping()
				if err == nil {
					conn.SetMaxOpenConns(50)
					conn.SetMaxIdleConns(15)
					conn.SetConnMaxLifetime(15 * time.Minute)

					dbMutex.Lock()
					db = conn
					dbErr = nil
					dbMutex.Unlock()

					fmt.Println("Successfully connected to the database!")
					onDBConnected()
					return
				}
			}

			dbMutex.Lock()
			dbErr = fmt.Errorf("attempt %d: %v", attempt, err)
			dbMutex.Unlock()

			fmt.Printf("Failed to connect to database (attempt %d): %v. Retrying in 5s...\n", attempt, err)
			time.Sleep(5 * time.Second)
		}
	}()
}

func onDBConnected() {
	database := getDB()

	migrationDir := "migrations"
	if _, err := os.Stat(migrationDir); os.IsNotExist(err) {
		if _, err := os.Stat("backend/migrations"); err == nil {
			migrationDir = "backend/migrations"
		}
	}

	fmt.Printf("Looking for migrations in: %s\n", migrationDir)
	files, err := filepath.Glob(filepath.Join(migrationDir, "*.sql"))
	if err != nil {
		log.Printf("Error finding migration files: %v", err)
	} else {
		var tableExists bool
		_ = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='schema_migrations')`).Scan(&tableExists)

		if !tableExists {
			_, err = database.Exec(`CREATE TABLE schema_migrations (
				filename VARCHAR(255) PRIMARY KEY,
				executed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
			)`)
			if err != nil {
				log.Printf("Warning: Failed to create schema_migrations table: %v", err)
			}
		} else {
			var hasFilename bool
			_ = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='schema_migrations' AND column_name='filename')`).Scan(&hasFilename)
			if !hasFilename {
				var oldCol string
				_ = database.QueryRow(`SELECT column_name FROM information_schema.columns WHERE table_name='schema_migrations' ORDER BY ordinal_position LIMIT 1`).Scan(&oldCol)
				if oldCol != "" {
					log.Printf("Renaming schema_migrations column '%s' → 'filename'", oldCol)
					_, renameErr := database.Exec(fmt.Sprintf(`ALTER TABLE schema_migrations RENAME COLUMN %s TO filename`, oldCol))
					if renameErr != nil {
						log.Printf("ERROR: Failed to rename column: %v.", renameErr)
					}
				}
			}

			var colType string
			_ = database.QueryRow(`SELECT data_type FROM information_schema.columns WHERE table_name='schema_migrations' AND column_name='filename'`).Scan(&colType)
			if colType != "" && colType != "character varying" && colType != "text" {
				_, _ = database.Exec(`DROP TABLE schema_migrations`)
				_, _ = database.Exec(`CREATE TABLE schema_migrations (
					filename VARCHAR(255) PRIMARY KEY,
					executed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
				)`)
				log.Println("schema_migrations table recreated with correct schema")
			} else {
				var hasExec bool
				_ = database.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='schema_migrations' AND column_name='executed_at')`).Scan(&hasExec)
				if !hasExec {
					_, _ = database.Exec(`ALTER TABLE schema_migrations ADD COLUMN executed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP`)
				}
			}
		}

		if len(files) == 0 {
			log.Println("Warning: No migration files found!")
		}
		for _, file := range files {
			baseName := filepath.Base(file)
			var alreadyExecuted bool
			errCheck := database.QueryRow("SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename=$1)", baseName).Scan(&alreadyExecuted)
			if errCheck != nil {
				log.Printf("Warning: Could not check migration status for %s: %v", baseName, errCheck)
				continue
			}
			if alreadyExecuted {
				continue
			}

			log.Printf("Executing migration: %s", file)
			migration, err := os.ReadFile(file)
			if err != nil {
				log.Fatalf("ABORTING: Could not read migration file %s: %v", file, err)
			}
			_, err = database.Exec(string(migration))
			if err != nil {
				if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
					log.Printf("Migration %s skipped (already exists): %v", file, err)
					_, _ = database.Exec("INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING", baseName)
				} else {
					log.Fatalf("ABORTING: Migration %s FAILED: %v\n"+
						"Backend will NOT start with pending migrations to avoid running with an inconsistent schema.\n"+
						"Fix the migration manually (psql -f) and restart.", file, err)
				}
			} else {
				log.Printf("Migration %s executed successfully.", file)
				_, insertErr := database.Exec("INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING", baseName)
				if insertErr != nil {
					log.Fatalf("ABORTING: Migration %s ran but could not be recorded: %v", file, insertErr)
				}
			}
		}
	}
}

func main() {
	_ = godotenv.Load()
	fmt.Printf("FB_AUDITORIA Backend v%s\n", BackendVersion)

	handlers.ValidateJWTSecret()
	estadoCadastro := "FECHADO"
	if handlers.SelfRegisterEnabled() {
		estadoCadastro = "ABERTO (ALLOW_SELF_REGISTER=true, só para desenvolvimento)"
	}
	log.Printf("[config] cadastro público: %s", estadoCadastro)
	initDBAsync()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8088"
	}

	http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		dbStatus := "connecting..."
		var lastErr string
		if database := getDB(); database != nil {
			if err := database.Ping(); err != nil {
				dbStatus = "error: " + err.Error()
			} else {
				dbStatus = "connected"
			}
		} else {
			dbMutex.RLock()
			if dbErr != nil {
				dbStatus = "error"
				lastErr = dbErr.Error()
			}
			dbMutex.RUnlock()
		}
		json.NewEncoder(w).Encode(HealthResponse{
			Status:    "running",
			Timestamp: time.Now().Format(time.RFC3339),
			Service:   "FB_AUDITORIA",
			Version:   BackendVersion,
			Features:  FeatureSet,
			Database:  dbStatus,
			DBError:   lastErr,
		})
	})

	withDB := func(f func(*sql.DB) http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				http.Error(w, "Database initializing, please wait...", http.StatusServiceUnavailable)
				return
			}
			f(database)(w, r)
		}
	}
	withAuth := func(f func(*sql.DB) http.HandlerFunc, role string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				http.Error(w, "Database initializing...", http.StatusServiceUnavailable)
				return
			}
			handlers.AuthMiddleware(database, f(database), role)(w, r)
		}
	}
	// withAud: JWT + empresa ativa + perfil + área do menu (ver auditoria_auth.go).
	// caps: vazio = só a área; caps[0] leitura (todos os métodos se for a única),
	// caps[1] escrita. Não aceita lista de perfis (AD-11).
	withAud := func(f func(*sql.DB) http.HandlerFunc, area string, caps ...handlers.Capacidade) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			database := getDB()
			if database == nil {
				http.Error(w, "Database initializing...", http.StatusServiceUnavailable)
				return
			}
			handlers.AuditoriaAuthMiddleware(database, f(database), area, caps...)(w, r)
		}
	}
	gz := handlers.GzipMiddleware

	// ── Auth ──────────────────────────────────────────────────────────────────
	http.HandleFunc("/api/auth/register", withDB(handlers.RegisterHandler))
	http.HandleFunc("/api/auth/login", withDB(handlers.LoginHandler))
	http.HandleFunc("/api/auth/me", withAuth(handlers.GetMeHandler, ""))
	http.HandleFunc("/api/auth/forgot-password", withDB(handlers.ForgotPasswordHandler))
	http.HandleFunc("/api/auth/reset-password", withDB(handlers.ResetPasswordHandler))
	http.HandleFunc("/api/auth/change-password", withAuth(handlers.ChangePasswordHandler, ""))
	http.HandleFunc("/api/auth/refresh", withDB(handlers.RefreshHandler))
	http.HandleFunc("/api/auth/logout", withDB(handlers.LogoutHandler))

	// ── Hierarquia de tenant ──────────────────────────────────────────────────
	http.HandleFunc("/api/user/hierarchy", withAuth(handlers.GetUserHierarchyHandler, ""))
	http.HandleFunc("/api/user/companies", withAuth(handlers.GetUserCompaniesHandler, ""))
	http.HandleFunc("/api/user/preferred-company", withAuth(handlers.UpdatePreferredCompanyHandler, ""))

	// ── Admin de plataforma — usuários ───────────────────────────────────────
	http.HandleFunc("/api/admin/users", withAuth(handlers.ListUsersHandler, "admin"))
	http.HandleFunc("/api/admin/users/create", withAuth(handlers.CreateUserHandler, "admin"))
	http.HandleFunc("/api/admin/users/empresa", withAuth(handlers.AdminUsuarioEmpresaHandler, "admin"))
	http.HandleFunc("/api/admin/users/promote", withAuth(handlers.PromoteUserHandler, "admin"))
	http.HandleFunc("/api/admin/users/delete", withAuth(handlers.DeleteUserHandler, "admin"))
	http.HandleFunc("/api/admin/users/reassign", withAuth(handlers.ReassignUserHandler, "admin"))

	// ── Config — ambientes / grupos / empresas / logo ────────────────────────
	http.HandleFunc("/api/config/environments", withAuth(func(db *sql.DB) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handlers.GetEnvironmentsHandler(db)(w, r)
			case http.MethodPost:
				handlers.CreateEnvironmentHandler(db)(w, r)
			case http.MethodPut:
				handlers.UpdateEnvironmentHandler(db)(w, r)
			case http.MethodDelete:
				handlers.DeleteEnvironmentHandler(db)(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}
	}, ""))
	http.HandleFunc("/api/config/groups", withAuth(func(db *sql.DB) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handlers.GetGroupsHandler(db)(w, r)
			case http.MethodPost:
				handlers.CreateGroupHandler(db)(w, r)
			case http.MethodDelete:
				handlers.DeleteGroupHandler(db)(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}
	}, ""))
	http.HandleFunc("/api/config/companies", withAuth(func(db *sql.DB) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handlers.GetCompaniesHandler(db)(w, r)
			case http.MethodPost:
				handlers.CreateCompanyHandler(db)(w, r)
			case http.MethodPut:
				handlers.UpdateCompanyHandler(db)(w, r)
			case http.MethodDelete:
				handlers.DeleteCompanyHandler(db)(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}
	}, ""))
	http.HandleFunc("/api/config/empresa/logo", func(w http.ResponseWriter, r *http.Request) {
		database := getDB()
		if database == nil {
			http.Error(w, "Database initializing...", http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodPost {
			handlers.AuthMiddleware(database, handlers.UploadEmpresaLogoHandler(database), "admin")(w, r)
		} else {
			handlers.AuthMiddleware(database, handlers.ServeEmpresaLogoHandler(database), "")(w, r)
		}
	})

	// ── Auditoria — acesso, usuários da empresa, trilha ──────────────────────
	http.HandleFunc("/api/aud/me", withAud(handlers.AudMeHandler, ""))
	http.HandleFunc("/api/aud/ambiente/limpar", withAud(handlers.AmbienteLimparHandler, ""))
	http.HandleFunc("/api/aud/ambiente/virar", withAud(handlers.AmbienteVirarHandler, ""))
	http.HandleFunc("/api/aud/acompanhamento", withAud(handlers.AcompanhamentoHandler, handlers.AreaAcompanhamento))
	http.HandleFunc("/api/aud/usuarios", withAud(handlers.AudUsuariosHandler, handlers.AreaConfiguracoes, handlers.CapGerirUsuarios))
	http.HandleFunc("/api/aud/usuarios/bloquear", withAud(handlers.AudUsuarioBloquearHandler, handlers.AreaConfiguracoes, handlers.CapGerirUsuarios))
	http.HandleFunc("/api/aud/usuarios/desbloquear", withAud(handlers.AudUsuarioDesbloquearHandler, handlers.AreaConfiguracoes, handlers.CapGerirUsuarios))
	http.HandleFunc("/api/aud/filiais-gerenciais", withAud(handlers.FiliaisGerenciaisHandler, handlers.AreaConfiguracoes, handlers.CapLerNoEscopo, handlers.CapEditarEstrutura))
	http.HandleFunc("/api/aud/agrupamentos-filiais", withAud(handlers.AgrupamentosFiliaisHandler, handlers.AreaConfiguracoes, handlers.CapLerNoEscopo, handlers.CapEditarEstrutura))
	http.HandleFunc("/api/aud/departamentos", withAud(handlers.DepartamentosHandler, handlers.AreaConfiguracoes, handlers.CapLerNoEscopo, handlers.CapEditarEstrutura))
	http.HandleFunc("/api/aud/dicionario", withAud(handlers.DicionarioHandler, handlers.AreaConfiguracoes, handlers.CapLerNoEscopo))
	http.HandleFunc("/api/aud/dicionario/carga", withAud(handlers.DicionarioCargaHandler, handlers.AreaConfiguracoes, handlers.CapEditarDicionario))
	http.HandleFunc("/api/aud/dicionario/conta", withAud(handlers.DicionarioContaHandler, handlers.AreaConfiguracoes, handlers.CapEditarDicionario))
	http.HandleFunc("/api/aud/audit-log", withAud(handlers.AuditLogHandler, handlers.AreaConfiguracoes, handlers.CapLerTrilha))

	// ── Administrativo (área empresa) — DRE ────────────────────────────────────────────────────
	// Escopo por departamento (4.7): rotas com valor por conta aceitam ler_no_escopo e aplicam
	// o escopo calculado no servidor (handlers/escopo.go); as do grupo exigem ler.
	est := handlers.AreaEmpresa
	http.HandleFunc("/api/aud/dre", gz(withAud(handlers.DREHandler, est, handlers.CapLer)))
	http.HandleFunc("/api/aud/dre/filiais", withAud(handlers.DREFiliaisHandler, est, handlers.CapLerNoEscopo))
	http.HandleFunc("/api/aud/dre/linhas", withAud(handlers.DRELinhasHandler, est, handlers.CapLerNoEscopo))
	http.HandleFunc("/api/aud/dre/linhas/papel", withAud(handlers.DRELinhaPapelHandler, est, handlers.CapEditarEstrutura))
	http.HandleFunc("/api/aud/dre/departamentos", gz(withAud(handlers.DREDepartamentosHandler, est, handlers.CapLerNoEscopo)))
	// 4.8: visão pessoal do Gerente; o handler recusa quem não é gestor (403).
	http.HandleFunc("/api/aud/meu-departamento", gz(withAud(handlers.MeuDepartamentoHandler, est, handlers.CapLerNoEscopo)))
	http.HandleFunc("/api/aud/dre/lancamentos", gz(withAud(handlers.DRELancamentosHandler, est, handlers.CapLer)))
	http.HandleFunc("/api/aud/dre/trimestres", gz(withAud(handlers.DRETrimestresHandler, est, handlers.CapLer)))
	http.HandleFunc("/api/aud/dre/contas", gz(withAud(handlers.DREContasHandler, est, handlers.CapLerNoEscopo)))
	http.HandleFunc("/api/aud/dre/contas/lancamentos", gz(withAud(handlers.DREContaLancamentosHandler, est, handlers.CapLerNoEscopo)))
	http.HandleFunc("/api/aud/dre/fechamentos", withAud(handlers.DREFechamentoHandler, est, handlers.CapLerNoEscopo, handlers.CapFechar))
	http.HandleFunc("/api/aud/dre/fechamentos/reabrir", withAud(handlers.DREReabrirHandler, est, handlers.CapReabrir))
	// Escrita do DRE: cada rota declara a capacidade (perfis.go); diretoria e auditoria só leem.
	http.HandleFunc("/api/aud/dre/mapa", withAud(handlers.DREMapaHandler, est, handlers.CapLerNoEscopo, handlers.CapEditarMapa))
	http.HandleFunc("/api/aud/dre/orcado", withAud(handlers.DREOrcadoHandler, est, handlers.CapCarregar))
	http.HandleFunc("/api/aud/dados/atualizar", withAud(handlers.DadosAtualizarHandler, est, handlers.CapCarregar))
	http.HandleFunc("/api/aud/dre/lancamentos/importar", withAud(handlers.DREImportarHandler, est, handlers.CapCarregar))
	// 2.12: conferência entre origens de lançamentos; leitura irrestrita (o gestor recebe 403).
	http.HandleFunc("/api/aud/conferencia", gz(withAud(handlers.ConferenciaHandler, est, handlers.CapLer)))
	http.HandleFunc("/api/aud/conferencia/origens", withAud(handlers.ConferenciaOrigensHandler, est, handlers.CapLer))

	// ── Frontend estático (SPA React) ─────────────────────────────────────────
	staticDir := "./static"
	if _, err := os.Stat(staticDir); err == nil {
		fs := http.FileServer(http.Dir(staticDir))
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			filePath := filepath.Join(staticDir, filepath.Clean(r.URL.Path))
			if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
				if ext := filepath.Ext(r.URL.Path); ext != "" && ext != ".html" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
				http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
				return
			}
			if r.URL.Path == "/" || r.URL.Path == "/index.html" {
				w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			}
			fs.ServeHTTP(w, r)
		})
		fmt.Println("Serving frontend from ./static")
	}

	fmt.Printf("FB_AUDITORIA starting on port %s...\n", port)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      handlers.SlowRequestLog(handlers.SecurityMiddleware(http.DefaultServeMux, handlers.GetAllowedOrigins())),
		ReadTimeout:  10 * time.Minute,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigChan
		log.Printf("Received signal %v, shutting down gracefully...", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
		if database := getDB(); database != nil {
			database.Close()
		}
		os.Exit(0)
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
