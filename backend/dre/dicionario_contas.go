package dre

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"fb_auditoria/trilha"
)

// ContaDic é uma linha do arquivo do dicionário. Departamento é o código do
// departamento quando o arquivo o informa; vazio deriva do grupo.
type ContaDic struct {
	Linha        int
	Conta        string
	Descricao    string
	Grupo        string
	Departamento string
}

// CargaDicionarioIn é a entrada de Carregar. Problemas traz o que a leitura do
// arquivo já recusou; nada é gravado se houver qualquer problema.
type CargaDicionarioIn struct {
	EmpresaID string
	UserID    string
	Arquivo   string
	Linhas    []ContaDic
	Problemas []Problema
}

// CargaDicionarioOut resume a carga.
type CargaDicionarioOut struct {
	Linhas            int `json:"linhas"`
	Novas             int `json:"novas"`
	Atualizadas       int `json:"atualizadas"`
	VinculosGrupo     int `json:"vinculos_grupo"`
	VinculosInformado int `json:"vinculos_informados"`
	MantidosManual    int `json:"mantidos_manual"`
	VinculosRemovidos int `json:"vinculos_removidos"`
	SemDepartamento   int `json:"sem_departamento"`
}

// ErrContaNaoEncontrada indica conta fora do dicionário da empresa.
type ErrContaNaoEncontrada struct{ Conta string }

func (e *ErrContaNaoEncontrada) Error() string {
	return "A conta " + e.Conta + " não está no dicionário"
}

func travarDicionario(ctx context.Context, tx *sql.Tx, empresaID string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('dicionario:' || $1))`, empresaID)
	return err
}

// Carregar grava o dicionário do arquivo: conta repetida ou departamento
// inexistente são erros listados e nada é gravado. O reenvio atualiza descrição,
// grupo e vínculo; vínculo manual só muda por departamento informado no arquivo.
// Nunca toca em dre_mapa.
func (d *Dicionario) Carregar(ctx context.Context, in CargaDicionarioIn) (CargaDicionarioOut, error) {
	var out CargaDicionarioOut
	if len(in.Linhas) == 0 && len(in.Problemas) == 0 {
		return out, &ErroEntrada{Msg: "o arquivo não tem contas"}
	}
	tx, err := d.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err := travarDicionario(ctx, tx, in.EmpresaID); err != nil {
		return out, err
	}
	deptos, err := listarDepartamentos(ctx, tx, in.EmpresaID, "")
	if err != nil {
		return out, err
	}
	porCodigo := map[string]string{}
	for _, x := range deptos {
		porCodigo[x.Codigo] = x.ID
	}

	probs := append([]Problema(nil), in.Problemas...)
	vista := map[string]int{}
	for _, l := range in.Linhas {
		if prim, ok := vista[l.Conta]; ok {
			probs = append(probs, Problema{Linha: l.Linha, Msg: fmt.Sprintf("conta repetida %s (já na linha %d)", l.Conta, prim)})
			continue
		}
		vista[l.Conta] = l.Linha
		if l.Departamento != "" {
			if _, ok := porCodigo[l.Departamento]; !ok {
				probs = append(probs, Problema{Linha: l.Linha, Msg: "departamento inexistente: " + l.Departamento})
			}
		}
	}
	if len(probs) > 0 {
		sort.SliceStable(probs, func(i, j int) bool { return probs[i].Linha < probs[j].Linha })
		return out, &ErroLinhas{Problemas: probs}
	}

	for _, l := range in.Linhas {
		var nova bool
		if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.conta_dicionario (empresa_id, conta, descricao, grupo)
			VALUES ($1::uuid,$2,$3,$4)
			ON CONFLICT (empresa_id, conta) DO UPDATE SET descricao = EXCLUDED.descricao, grupo = EXCLUDED.grupo
			RETURNING (xmax = 0)`, in.EmpresaID, l.Conta, l.Descricao, l.Grupo).Scan(&nova); err != nil {
			return out, err
		}
		if nova {
			out.Novas++
		} else {
			out.Atualizadas++
		}
		switch {
		case l.Departamento != "": // informado no arquivo: decisão explícita, vale como manual
			if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.conta_departamento (empresa_id, conta_padrao, departamento_id, origem)
				VALUES ($1::uuid,$2,$3::uuid,'manual')
				ON CONFLICT (empresa_id, conta_padrao) DO UPDATE SET departamento_id = EXCLUDED.departamento_id, origem = 'manual'`,
				in.EmpresaID, l.Conta, porCodigo[l.Departamento]); err != nil {
				return out, err
			}
			out.VinculosInformado++
		case l.Grupo != "" && porCodigo[l.Grupo] != "":
			res, err := tx.ExecContext(ctx, `INSERT INTO auditoria.conta_departamento (empresa_id, conta_padrao, departamento_id, origem)
				VALUES ($1::uuid,$2,$3::uuid,'grupo')
				ON CONFLICT (empresa_id, conta_padrao) DO UPDATE SET departamento_id = EXCLUDED.departamento_id, origem = 'grupo'
				WHERE auditoria.conta_departamento.origem <> 'manual'`, in.EmpresaID, l.Conta, porCodigo[l.Grupo])
			if err != nil {
				return out, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				out.VinculosGrupo++
			} else {
				out.MantidosManual++
			}
		default: // sem departamento derivável: some o vínculo derivado antigo, o manual fica
			res, err := tx.ExecContext(ctx, `DELETE FROM auditoria.conta_departamento
				WHERE empresa_id=$1::uuid AND conta_padrao=$2 AND origem='grupo'`, in.EmpresaID, l.Conta)
			if err != nil {
				return out, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				out.VinculosRemovidos++
			}
			var tem bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auditoria.conta_departamento
				WHERE empresa_id=$1::uuid AND conta_padrao=$2)`, in.EmpresaID, l.Conta).Scan(&tem); err != nil {
				return out, err
			}
			if tem {
				out.MantidosManual++
			} else {
				out.SemDepartamento++
			}
		}
	}
	out.Linhas = len(in.Linhas)
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "dicionario", "carga", "carregar",
		map[string]any{"arquivo": in.Arquivo, "resultado": out}); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// ContaDepartamento é uma conta do dicionário com o departamento resolvido.
type ContaDepartamento struct {
	Conta              string `json:"conta"`
	Descricao          string `json:"descricao"`
	Grupo              string `json:"grupo"`
	DepartamentoID     string `json:"departamento_id,omitempty"`
	DepartamentoCodigo string `json:"departamento_codigo,omitempty"`
	DepartamentoNome   string `json:"departamento_nome,omitempty"`
	Origem             string `json:"origem,omitempty"`
	// Padrao é o padrão (com '%') que resolveu a conta, quando difere dela.
	Padrao string `json:"padrao,omitempty"`
}

// ListaDicionario é a página de contas e as contas sem departamento, à parte.
type ListaDicionario struct {
	Total          int                 `json:"total"`
	Pagina         int                 `json:"pagina"`
	Limite         int                 `json:"limite"`
	Contas         []ContaDepartamento `json:"contas"`
	PendentesTotal int                 `json:"pendentes_total"`
	Pendentes      []ContaDepartamento `json:"pendentes"`
}

const (
	maxLimiteLista = 200
	maxPendentes   = 500
)

type vinculo struct {
	padrao string
	origem string
	dep    Departamento
}

func carregarVinculos(ctx context.Context, q Consultador, empresaID string) ([]vinculo, error) {
	rows, err := q.QueryContext(ctx, `SELECT cd.conta_padrao, cd.origem, d.id::text, d.codigo, d.nome, d.grupo
		FROM auditoria.conta_departamento cd
		JOIN auditoria.departamentos d ON d.id = cd.departamento_id AND d.empresa_id = cd.empresa_id
		WHERE cd.empresa_id = $1::uuid`, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []vinculo
	for rows.Next() {
		var v vinculo
		if err := rows.Scan(&v.padrao, &v.origem, &v.dep.ID, &v.dep.Codigo, &v.dep.Nome, &v.dep.Grupo); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ResolvedorDepartamento resolve o departamento de muitas contas com uma única
// leitura dos vínculos (a mesma regra do Mapa: exato vence, senão o prefixo mais longo).
type ResolvedorDepartamento struct{ vs []vinculo }

// CarregarResolvedorDepartamento lê os vínculos da empresa uma vez.
func CarregarResolvedorDepartamento(ctx context.Context, q Consultador, empresaID string) (*ResolvedorDepartamento, error) {
	vs, err := carregarVinculos(ctx, q, empresaID)
	if err != nil {
		return nil, err
	}
	return &ResolvedorDepartamento{vs: vs}, nil
}

// De devolve o departamento da conta (nil se não houver).
func (r *ResolvedorDepartamento) De(conta string) *Departamento {
	i := MelhorPadrao(len(r.vs), func(i int) string { return r.vs[i].padrao }, conta)
	if i < 0 {
		return nil
	}
	dep := r.vs[i].dep
	return &dep
}

// DepartamentoDe devolve o departamento de UMA conta (nil se não houver). Para
// várias contas use CarregarResolvedorDepartamento: aqui cada chamada consulta.
func DepartamentoDe(ctx context.Context, q Consultador, empresaID, conta string) (*Departamento, error) {
	r, err := CarregarResolvedorDepartamento(ctx, q, empresaID)
	if err != nil {
		return nil, err
	}
	return r.De(conta), nil
}

// DescritorConta resolve a descrição de contas pelo dicionário (mesma regra de padrão).
type DescritorConta struct{ contas, descs []string }

// CarregarDescritorConta lê o dicionário da empresa uma vez.
func CarregarDescritorConta(ctx context.Context, q Consultador, empresaID string) (*DescritorConta, error) {
	rows, err := q.QueryContext(ctx, `SELECT conta, descricao FROM auditoria.conta_dicionario
		WHERE empresa_id = $1::uuid`, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	d := &DescritorConta{}
	for rows.Next() {
		var c, desc string
		if err := rows.Scan(&c, &desc); err != nil {
			return nil, err
		}
		d.contas, d.descs = append(d.contas, c), append(d.descs, desc)
	}
	return d, rows.Err()
}

// De devolve a descrição da conta ('' se não estiver no dicionário).
func (d *DescritorConta) De(conta string) string {
	i := MelhorPadrao(len(d.contas), func(i int) string { return d.contas[i] }, conta)
	if i < 0 {
		return ""
	}
	return d.descs[i]
}

// ListarContas devolve a página de contas (por conta, filtro opcional por trecho
// da conta ou da descrição) e as contas sem departamento.
func (d *Dicionario) ListarContas(ctx context.Context, empresaID, busca string, pagina, limite int) (ListaDicionario, error) {
	if limite <= 0 {
		limite = 50
	}
	if limite > maxLimiteLista {
		limite = maxLimiteLista
	}
	if pagina < 1 {
		pagina = 1
	}
	vs, err := carregarVinculos(ctx, d.DB, empresaID)
	if err != nil {
		return ListaDicionario{}, err
	}
	rows, err := d.DB.QueryContext(ctx, `SELECT conta, descricao, grupo FROM auditoria.conta_dicionario
		WHERE empresa_id = $1::uuid ORDER BY conta`, empresaID)
	if err != nil {
		return ListaDicionario{}, err
	}
	defer rows.Close()
	busca = strings.ToLower(strings.TrimSpace(busca))
	res := ListaDicionario{Pagina: pagina, Limite: limite, Contas: []ContaDepartamento{}, Pendentes: []ContaDepartamento{}}
	var todas []ContaDepartamento
	for rows.Next() {
		var c ContaDepartamento
		if err := rows.Scan(&c.Conta, &c.Descricao, &c.Grupo); err != nil {
			return ListaDicionario{}, err
		}
		if i := MelhorPadrao(len(vs), func(i int) string { return vs[i].padrao }, c.Conta); i >= 0 {
			v := vs[i]
			c.DepartamentoID, c.DepartamentoCodigo, c.DepartamentoNome, c.Origem = v.dep.ID, v.dep.Codigo, v.dep.Nome, v.origem
			if v.padrao != c.Conta {
				c.Padrao = v.padrao
			}
		} else {
			res.PendentesTotal++
			if len(res.Pendentes) < maxPendentes {
				res.Pendentes = append(res.Pendentes, c)
			}
		}
		if busca == "" || strings.Contains(strings.ToLower(c.Conta), busca) || strings.Contains(strings.ToLower(c.Descricao), busca) {
			todas = append(todas, c)
		}
	}
	if err := rows.Err(); err != nil {
		return ListaDicionario{}, err
	}
	res.Total = len(todas)
	ini := (pagina - 1) * limite
	if ini < len(todas) {
		fim := ini + limite
		if fim > len(todas) {
			fim = len(todas)
		}
		res.Contas = todas[ini:fim]
	}
	return res, nil
}

// CorrigirDepartamento define o departamento de UMA conta (origem manual). Com
// departamentoID vazio, desfaz a correção e volta a derivar do grupo.
func (d *Dicionario) CorrigirDepartamento(ctx context.Context, empresaID, userID, conta, departamentoID string) (ContaDepartamento, error) {
	conta = strings.TrimSpace(conta)
	if conta == "" {
		return ContaDepartamento{}, &ErroEntrada{Msg: "conta obrigatória"}
	}
	if departamentoID != "" && !reUUID.MatchString(departamentoID) {
		return ContaDepartamento{}, &ErrDepartamentoNaoEncontrado{}
	}
	tx, err := d.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return ContaDepartamento{}, err
	}
	defer tx.Rollback()
	if err := travarDicionario(ctx, tx, empresaID); err != nil {
		return ContaDepartamento{}, err
	}
	var grupo string
	err = tx.QueryRowContext(ctx, `SELECT grupo FROM auditoria.conta_dicionario
		WHERE empresa_id=$1::uuid AND conta=$2 FOR UPDATE`, empresaID, conta).Scan(&grupo)
	if errors.Is(err, sql.ErrNoRows) {
		return ContaDepartamento{}, &ErrContaNaoEncontrada{Conta: conta}
	}
	if err != nil {
		return ContaDepartamento{}, err
	}
	antes := ""
	_ = tx.QueryRowContext(ctx, `SELECT d.codigo FROM auditoria.conta_departamento cd
		JOIN auditoria.departamentos d ON d.id = cd.departamento_id AND d.empresa_id = cd.empresa_id
		WHERE cd.empresa_id=$1::uuid AND cd.conta_padrao=$2`, empresaID, conta).Scan(&antes)

	alvo := departamentoID
	origem := "manual"
	if alvo == "" { // volta a derivar do grupo
		origem = "grupo"
		err = tx.QueryRowContext(ctx, `SELECT id::text FROM auditoria.departamentos
			WHERE empresa_id=$1::uuid AND codigo=$2`, empresaID, grupo).Scan(&alvo)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ContaDepartamento{}, err
		}
		if alvo == "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.conta_departamento
				WHERE empresa_id=$1::uuid AND conta_padrao=$2`, empresaID, conta); err != nil {
				return ContaDepartamento{}, err
			}
		}
	} else {
		var existe bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auditoria.departamentos
			WHERE empresa_id=$1::uuid AND id=$2::uuid)`, empresaID, alvo).Scan(&existe); err != nil {
			return ContaDepartamento{}, err
		}
		if !existe {
			return ContaDepartamento{}, &ErrDepartamentoNaoEncontrado{}
		}
	}
	if alvo != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.conta_departamento (empresa_id, conta_padrao, departamento_id, origem)
			VALUES ($1::uuid,$2,$3::uuid,$4)
			ON CONFLICT (empresa_id, conta_padrao) DO UPDATE SET departamento_id = EXCLUDED.departamento_id, origem = EXCLUDED.origem`,
			empresaID, conta, alvo, origem); err != nil {
			return ContaDepartamento{}, err
		}
	}
	depois := ""
	if alvo != "" {
		if err := tx.QueryRowContext(ctx, `SELECT codigo FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND id=$2::uuid`,
			empresaID, alvo).Scan(&depois); err != nil {
			return ContaDepartamento{}, err
		}
	}
	if err := trilha.Gravar(tx, empresaID, userID, "dicionario", conta, "corrigir_departamento",
		map[string]any{"conta": conta, "antes": antes, "depois": depois, "origem": origem}); err != nil {
		return ContaDepartamento{}, err
	}
	if err := tx.Commit(); err != nil {
		return ContaDepartamento{}, err
	}
	l, err := d.ListarContas(ctx, empresaID, conta, 1, maxLimiteLista)
	if err != nil {
		return ContaDepartamento{}, err
	}
	for _, c := range l.Contas {
		if c.Conta == conta {
			return c, nil
		}
	}
	return ContaDepartamento{Conta: conta}, nil
}
