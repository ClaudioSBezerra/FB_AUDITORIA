package handlers

// aud_usuarios.go — gestão de perfil, áreas e departamentos dos usuários DA
// empresa ativa (capacidade gerir_usuarios). O admin de plataforma continua
// usando /api/admin/users. Bloqueio: aud_usuarios_bloqueio.go.

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"sort"

	"github.com/lib/pq"
)

var areasValidas = map[string]bool{AreaEmpresa: true, AreaConfiguracoes: true, AreaComercial: true, AreaOperacao: true, AreaAcompanhamento: true}

var reUUIDHandler = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Usuários que alcançam a empresa ativa pela cadeia ambiente → grupo → empresa
// ($N = id da empresa).
func usuariosDaEmpresa(param string) string {
	return `u.id IN (SELECT ue.user_id FROM user_environments ue
		JOIN enterprise_groups eg ON eg.environment_id = ue.environment_id
		JOIN companies c ON c.group_id = eg.id WHERE c.id = ` + param + `::uuid)`
}

// DeptoRef é o departamento ligado a um usuário.
type DeptoRef struct {
	ID     string `json:"id"`
	Codigo string `json:"codigo"`
	Nome   string `json:"nome"`
}

// departamentosDoUsuario lista os vínculos do usuário na empresa (ordem por código).
func departamentosDoUsuario(q interface {
	Query(string, ...any) (*sql.Rows, error)
}, empresaID, userID string) ([]DeptoRef, error) {
	rows, err := q.Query(`SELECT d.id::text, d.codigo, d.nome FROM auditoria.usuario_departamentos ud
		JOIN auditoria.departamentos d ON d.id = ud.departamento_id AND d.empresa_id = ud.empresa_id
		WHERE ud.empresa_id = $1::uuid AND ud.user_id = $2::uuid ORDER BY d.codigo`, empresaID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeptoRef{}
	for rows.Next() {
		var d DeptoRef
		if err := rows.Scan(&d.ID, &d.Codigo, &d.Nome); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func codigosDe(ds []DeptoRef) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Codigo)
	}
	return out
}

// AudUsuariosHandler — GET lista | PUT {user_id, perfil, areas, departamentos?}.
// departamentos (ids) ausente = mantém os vínculos; presente (mesmo []) = substitui.
// Só o perfil gestor tem departamentos; os vínculos somem se o perfil muda.
func AudUsuariosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if c == nil || !c.Pode(CapGerirUsuarios) {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
			listarUsuarios(w, db, c)
		case http.MethodPut:
			salvarUsuario(w, r, db, c)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func listarUsuarios(w http.ResponseWriter, db *sql.DB, c *AudContext) {
	rows, err := db.Query(`SELECT u.id, u.full_name, u.email, u.perfil, u.areas,
		u.bloqueado_em IS NOT NULL, COALESCE(u.motivo_bloqueio,''),
		COALESCE(to_char(u.bloqueado_em AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),''), u.role = 'admin'
		FROM users u WHERE `+usuariosDaEmpresa("$1")+` ORDER BY u.full_name`, c.EmpresaID)
	if err != nil {
		log.Printf("aud usuarios listar: %v", err)
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	type u struct {
		ID            string     `json:"id"`
		Nome          string     `json:"full_name"`
		Email         string     `json:"email"`
		Perfil        string     `json:"perfil"`
		Areas         []string   `json:"areas"`
		Departamentos []DeptoRef `json:"departamentos"`
		Bloqueado     bool       `json:"bloqueado"`
		Motivo        string     `json:"motivo_bloqueio"`
		BloqueadoEm   string     `json:"bloqueado_em"`
		AdminPlat     bool       `json:"admin_plataforma"`
	}
	out := []u{}
	for rows.Next() {
		var x u
		var areas pq.StringArray
		if err := rows.Scan(&x.ID, &x.Nome, &x.Email, &x.Perfil, &areas, &x.Bloqueado, &x.Motivo, &x.BloqueadoEm, &x.AdminPlat); err != nil {
			log.Printf("aud usuarios listar: %v", err)
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		x.Areas = []string(areas)
		x.Departamentos = []DeptoRef{}
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	rows.Close()
	porUsuario := map[string][]DeptoRef{}
	vr, err := db.Query(`SELECT ud.user_id::text, d.id::text, d.codigo, d.nome FROM auditoria.usuario_departamentos ud
		JOIN auditoria.departamentos d ON d.id = ud.departamento_id AND d.empresa_id = ud.empresa_id
		WHERE ud.empresa_id = $1::uuid ORDER BY d.codigo`, c.EmpresaID)
	if err != nil {
		log.Printf("aud usuarios departamentos: %v", err)
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	defer vr.Close()
	for vr.Next() {
		var uid string
		var d DeptoRef
		if err := vr.Scan(&uid, &d.ID, &d.Codigo, &d.Nome); err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		porUsuario[uid] = append(porUsuario[uid], d)
	}
	for i := range out {
		if ds := porUsuario[out[i].ID]; ds != nil {
			out[i].Departamentos = ds
		}
	}
	writeJSON(w, out)
}

func salvarUsuario(w http.ResponseWriter, r *http.Request, db *sql.DB, c *AudContext) {
	var req struct {
		UserID        string    `json:"user_id"`
		Perfil        string    `json:"perfil"`
		Areas         []string  `json:"areas"`
		Departamentos *[]string `json:"departamentos"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil || req.UserID == "" || matrizPerfis[req.Perfil] == nil || len(req.Areas) == 0 {
		http.Error(w, "user_id, perfil e areas obrigatórios", http.StatusBadRequest)
		return
	}
	for _, a := range req.Areas {
		if !areasValidas[a] {
			http.Error(w, "área inválida: "+a, http.StatusBadRequest)
			return
		}
	}
	if !reUUIDHandler.MatchString(req.UserID) {
		http.Error(w, "usuário não encontrado nesta empresa", http.StatusNotFound)
		return
	}
	pedidos, msg := departamentosPedidos(req.Perfil, req.Departamentos)
	if msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		http.Error(w, "Erro", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var perfilAntes string
	var areasAntes pq.StringArray
	err = tx.QueryRow(`SELECT u.perfil, u.areas FROM users u WHERE u.id = $1::uuid AND `+usuariosDaEmpresa("$2")+` FOR UPDATE OF u`,
		req.UserID, c.EmpresaID).Scan(&perfilAntes, &areasAntes)
	if err == sql.ErrNoRows {
		http.Error(w, "usuário não encontrado nesta empresa", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("aud usuarios salvar: %v", err)
		http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
		return
	}
	deptosAntes, err := departamentosDoUsuario(tx, c.EmpresaID, req.UserID)
	if err != nil {
		log.Printf("aud usuarios salvar: %v", err)
		http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
		return
	}

	if _, err := tx.Exec(`UPDATE users SET perfil = $2, areas = $3, updated_at = now() WHERE id = $1::uuid`,
		req.UserID, req.Perfil, pq.Array(req.Areas)); err != nil {
		log.Printf("aud usuarios salvar: %v", err)
		http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
		return
	}

	// Vínculos: perfil que não é gestor não tem nenhum (apaga em todas as empresas);
	// gestor com lista no pedido troca pela lista; sem lista, mantém.
	switch {
	case req.Perfil != "gestor":
		if _, err := tx.Exec(`DELETE FROM auditoria.usuario_departamentos WHERE user_id = $1::uuid`, req.UserID); err != nil {
			http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
			return
		}
	case req.Departamentos != nil:
		if ok, err := departamentosSaoDaEmpresa(tx, c.EmpresaID, pedidos); err != nil {
			http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
			return
		} else if !ok {
			http.Error(w, "departamento não encontrado nesta empresa", http.StatusBadRequest)
			return
		}
		if _, err := tx.Exec(`DELETE FROM auditoria.usuario_departamentos WHERE user_id = $1::uuid AND empresa_id = $2::uuid`,
			req.UserID, c.EmpresaID); err != nil {
			http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
			return
		}
		for _, id := range pedidos {
			if _, err := tx.Exec(`INSERT INTO auditoria.usuario_departamentos (user_id, departamento_id, empresa_id)
				VALUES ($1::uuid, $2::uuid, $3::uuid)`, req.UserID, id, c.EmpresaID); err != nil {
				log.Printf("aud usuarios vincular: %v", err)
				http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
				return
			}
		}
	}
	deptosDepois, err := departamentosDoUsuario(tx, c.EmpresaID, req.UserID)
	if err != nil {
		http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
		return
	}

	sort.Strings(areasAntes)
	areasDepois := append([]string(nil), req.Areas...)
	sort.Strings(areasDepois)
	payload := map[string]any{
		"antes":  map[string]any{"perfil": perfilAntes, "areas": []string(areasAntes), "departamentos": codigosDe(deptosAntes)},
		"depois": map[string]any{"perfil": req.Perfil, "areas": areasDepois, "departamentos": codigosDe(deptosDepois)},
	}
	if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "usuario", req.UserID, "alterar_perfil", payload); err != nil || tx.Commit() != nil {
		http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"user_id": req.UserID, "perfil": req.Perfil, "areas": req.Areas, "departamentos": deptosDepois})
}

// departamentosPedidos valida a lista de ids de departamento do pedido (sem repetição).
// msg não vazia = erro 400. Só o perfil gestor tem departamentos. Regra única da 4.5,
// usada também na criação de usuário (4.9).
func departamentosPedidos(perfil string, ids *[]string) (pedidos []string, msg string) {
	if ids == nil {
		return nil, ""
	}
	vistos := map[string]bool{}
	for _, id := range *ids {
		if !reUUIDHandler.MatchString(id) {
			return nil, "departamento inválido: " + id
		}
		if !vistos[id] {
			vistos[id] = true
			pedidos = append(pedidos, id)
		}
	}
	if len(pedidos) > 0 && perfil != "gestor" {
		return nil, "departamentos só podem ser ligados ao perfil Gerente (gestor)"
	}
	return pedidos, ""
}

// departamentosSaoDaEmpresa confere que todos os ids existem na empresa.
func departamentosSaoDaEmpresa(tx *sql.Tx, empresaID string, pedidos []string) (bool, error) {
	if len(pedidos) == 0 {
		return true, nil
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM auditoria.departamentos WHERE empresa_id = $1::uuid AND id::text = ANY($2)`,
		empresaID, pq.Array(lower(pedidos))).Scan(&n); err != nil {
		return false, err
	}
	return n == len(pedidos), nil
}

func lower(ids []string) []string {
	out := make([]string, len(ids))
	for i, s := range ids {
		b := []byte(s)
		for j, ch := range b {
			if ch >= 'A' && ch <= 'F' {
				b[j] = ch + 32
			}
		}
		out[i] = string(b)
	}
	return out
}
