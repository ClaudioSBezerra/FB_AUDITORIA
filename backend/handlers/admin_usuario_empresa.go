package handlers

// admin_usuario_empresa.go — o administrador de plataforma cria o usuário DA empresa
// (história 4.9, FR-19). Sem cadastro público e sem senha escolhida pelo administrador:
// o usuário nasce com uma senha descartada (hash de valor aleatório que ninguém conhece)
// e recebe um link, pelo mecanismo de redefinição de senha existente
// (verification_tokens, type 'password_reset', ResetPasswordHandler), para definir a
// própria. Validade do link: 72 horas. Sem SMTP, a resposta traz o link para o
// administrador repassar. Nem a senha nem o token entram na trilha.
//
// Só quem tem users.role = 'admin' (rota /api/admin). O perfil de TI, que poderá criar
// usuários, fica para depois.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"sort"
	"strings"
	"time"

	"fb_auditoria/services"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// Validade do link de primeiro acesso.
const validadeConviteHoras = 72

// Pontos de troca para o teste (sem SMTP de verdade).
var (
	smtpConfigurado = services.SMTPConfigurado
	enviarConvite   = services.SendInviteEmail
)

// AdminUsuarioEmpresaHandler — GET ?company_id= lista os departamentos da empresa (para o
// formulário) | POST cria o usuário da empresa.
func AdminUsuarioEmpresaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adminID := GetUserIDFromContext(r)
		if adminID == "" || !ehAdminDePlataforma(db, adminID) {
			http.Error(w, "Forbidden: só o administrador de plataforma", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
			listarDepartamentosDaEmpresa(w, r, db)
		case http.MethodPost:
			criarUsuarioDaEmpresa(w, r, db, adminID)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// ehAdminDePlataforma confere users.role no banco (a rota já exige admin no token; o
// cadastro é a fonte, um token antigo não vale mais que ele).
func ehAdminDePlataforma(db *sql.DB, userID string) bool {
	var role string
	if err := db.QueryRow(`SELECT COALESCE(role,'') FROM users WHERE id = $1::uuid`, userID).Scan(&role); err != nil {
		return false
	}
	return role == "admin"
}

func listarDepartamentosDaEmpresa(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	id := r.URL.Query().Get("company_id")
	if !reUUIDHandler.MatchString(id) {
		http.Error(w, "company_id inválido", http.StatusBadRequest)
		return
	}
	rows, err := db.Query(`SELECT id::text, codigo, nome, grupo FROM auditoria.departamentos WHERE empresa_id = $1::uuid ORDER BY codigo`, id)
	if err != nil {
		log.Printf("admin usuario empresa: departamentos: %v", err)
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var i, c, n, g string
		if err := rows.Scan(&i, &c, &n, &g); err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		out = append(out, map[string]string{"id": i, "codigo": c, "nome": n, "grupo": g})
	}
	writeJSON(w, out)
}

func criarUsuarioDaEmpresa(w http.ResponseWriter, r *http.Request, db *sql.DB, adminID string) {
	var req struct {
		Email         string    `json:"email"`
		FullName      string    `json:"full_name"`
		Perfil        string    `json:"perfil"`
		Areas         []string  `json:"areas"`
		Departamentos *[]string `json:"departamentos"`
		CompanyID     string    `json:"company_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil {
		http.Error(w, "corpo inválido", http.StatusBadRequest)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.FullName = strings.TrimSpace(req.FullName)
	if req.FullName == "" || len(req.FullName) > 255 {
		http.Error(w, "nome obrigatório", http.StatusBadRequest)
		return
	}
	if a, err := mail.ParseAddress(req.Email); err != nil || a.Address != req.Email || len(req.Email) > 255 {
		http.Error(w, "e-mail inválido", http.StatusBadRequest)
		return
	}
	if matrizPerfis[req.Perfil] == nil {
		http.Error(w, "perfil inválido", http.StatusBadRequest)
		return
	}
	if len(req.Areas) == 0 {
		http.Error(w, "informe ao menos uma área", http.StatusBadRequest)
		return
	}
	for _, a := range req.Areas {
		if !areasValidas[a] {
			http.Error(w, "área inválida: "+a, http.StatusBadRequest)
			return
		}
	}
	if !reUUIDHandler.MatchString(req.CompanyID) {
		http.Error(w, "company_id inválido", http.StatusBadRequest)
		return
	}
	pedidos, msg := departamentosPedidos(req.Perfil, req.Departamentos)
	if msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Senha descartada: ninguém conhece o valor; o hash só existe para que a coluna não seja vazia.
	segredo := make([]byte, 32)
	if _, err := rand.Read(segredo); err != nil {
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(segredo)), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	tokBytes := make([]byte, 32)
	if _, err := rand.Read(tokBytes); err != nil {
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(tokBytes)
	expira := time.Now().Add(validadeConviteHoras * time.Hour)

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var envID string
	err = tx.QueryRow(`SELECT eg.environment_id::text FROM companies c JOIN enterprise_groups eg ON eg.id = c.group_id WHERE c.id = $1::uuid`,
		req.CompanyID).Scan(&envID)
	if err == sql.ErrNoRows {
		http.Error(w, "empresa não encontrada", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("admin usuario empresa: empresa: %v", err)
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	if ok, err := departamentosSaoDaEmpresa(tx, req.CompanyID, pedidos); err != nil {
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	} else if !ok {
		http.Error(w, "departamento não encontrado nesta empresa", http.StatusBadRequest)
		return
	}

	// Sem trial_ends_at: o acesso termina por bloqueio (4.6).
	var userID string
	err = tx.QueryRow(`INSERT INTO users (email, password_hash, full_name, is_verified, role, perfil, areas)
		VALUES ($1, $2, $3, TRUE, 'user', $4, $5) RETURNING id::text`,
		req.Email, string(hash), req.FullName, req.Perfil, pq.Array(req.Areas)).Scan(&userID)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		http.Error(w, "já existe um usuário com este e-mail", http.StatusConflict)
		return
	}
	if err != nil {
		log.Printf("admin usuario empresa: inserir: %v", err)
		http.Error(w, "Erro ao criar usuário", http.StatusInternalServerError)
		return
	}
	// Vínculo à empresa: o mesmo que AudUsuariosHandler usa (ambiente → grupo → empresa);
	// a empresa preferida faz o login cair nela.
	if _, err := tx.Exec(`INSERT INTO user_environments (user_id, environment_id, role, preferred_company_id)
		VALUES ($1::uuid, $2::uuid, 'user', $3::uuid)`, userID, envID, req.CompanyID); err != nil {
		log.Printf("admin usuario empresa: vincular: %v", err)
		http.Error(w, "Erro ao criar usuário", http.StatusInternalServerError)
		return
	}
	for _, id := range pedidos {
		if _, err := tx.Exec(`INSERT INTO auditoria.usuario_departamentos (user_id, departamento_id, empresa_id)
			VALUES ($1::uuid, $2::uuid, $3::uuid)`, userID, id, req.CompanyID); err != nil {
			log.Printf("admin usuario empresa: departamento: %v", err)
			http.Error(w, "Erro ao criar usuário", http.StatusInternalServerError)
			return
		}
	}
	if _, err := tx.Exec(`INSERT INTO verification_tokens (user_id, token, type, expires_at) VALUES ($1::uuid, $2, 'password_reset', $3)`,
		userID, token, expira); err != nil {
		log.Printf("admin usuario empresa: token: %v", err)
		http.Error(w, "Erro ao criar usuário", http.StatusInternalServerError)
		return
	}
	deptos, err := departamentosDoUsuario(tx, req.CompanyID, userID)
	if err != nil {
		http.Error(w, "Erro ao criar usuário", http.StatusInternalServerError)
		return
	}
	areas := append([]string(nil), req.Areas...)
	sort.Strings(areas)
	// Trilha: quem, o quê e até quando vale o convite. Nunca a senha nem o token.
	payload := map[string]any{
		"email": req.Email, "nome": req.FullName, "perfil": req.Perfil, "areas": areas,
		"departamentos": codigosDe(deptos), "convite_validade_horas": validadeConviteHoras,
	}
	if err := writeAuditLogTx(tx, req.CompanyID, adminID, "usuario", userID, "criar", payload); err != nil || tx.Commit() != nil {
		http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
		return
	}

	caminho := services.CaminhoDefinirSenha(token)
	link := services.LinkDefinirSenha(token)
	enviado, motivo := false, ""
	switch {
	case !smtpConfigurado():
		motivo = "SMTP não configurado"
	case link == "":
		motivo = "APP_URL não configurado"
	default:
		if err := enviarConvite(req.Email, req.FullName, link, validadeConviteHoras); err != nil {
			motivo = "falha no envio"
		} else {
			enviado = true
		}
	}
	// Resultado do envio (sem o link): ajuda a explicar um convite que não chegou.
	writeAuditLog(db, req.CompanyID, adminID, "usuario", userID, "convite", map[string]any{"enviado": enviado, "motivo": motivo})

	w.WriteHeader(http.StatusCreated)
	resp := map[string]any{
		"id": userID, "email": req.Email, "full_name": req.FullName, "perfil": req.Perfil,
		"areas": areas, "departamentos": deptos, "email_enviado": enviado,
		"expira_em": expira.UTC().Format(time.RFC3339), "validade_horas": validadeConviteHoras,
	}
	if !enviado {
		// O administrador repassa o link. Com APP_URL, link absoluto; sem, só o caminho.
		resp["link"] = link
		resp["caminho"] = caminho
		resp["motivo_sem_envio"] = motivo
	}
	writeJSON(w, resp)
}
