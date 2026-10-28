package dre

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/lib/pq"

	"fb_auditoria/trilha"
)

// Limites de entrada.
const (
	maxCodigoFilial = 40
	maxNomeFilial   = 120
	maxFiliaisERP   = 500
	maxFilialERP    = 20
)

// Filiais cadastra as filiais gerenciais (conjuntos de filiais do ERP).
type Filiais struct{ DB *sql.DB }

// FilialGerencial é uma filial gerencial com as filiais do ERP que a compõem.
type FilialGerencial struct {
	ID      string   `json:"id"`
	Codigo  string   `json:"codigo"`
	Nome    string   `json:"nome"`
	Filiais []string `json:"filiais"`
}

// SalvarFilialIn é a entrada de Salvar; ID vazio cria, preenchido edita.
type SalvarFilialIn struct {
	EmpresaID string
	UserID    string
	ID        string
	Codigo    string
	Nome      string
	Filiais   []string
}

// ErrFilialLigada recusa ligar uma filial do ERP que já pertence a outra filial gerencial.
type ErrFilialLigada struct{ FilialERP, Dono string }

func (e *ErrFilialLigada) Error() string {
	return "A filial " + e.FilialERP + " já está ligada à filial gerencial " + e.Dono
}

// ErrCodigoEmUso recusa código de filial gerencial já cadastrado na empresa.
type ErrCodigoEmUso struct{ Codigo string }

func (e *ErrCodigoEmUso) Error() string {
	return "Já existe uma filial gerencial com o código " + e.Codigo
}

// ErrCodigoIgualFilial recusa código igual a uma filial do ERP ligada a outra filial gerencial.
type ErrCodigoIgualFilial struct{ Codigo, Dono string }

func (e *ErrCodigoIgualFilial) Error() string {
	return "O código " + e.Codigo + " é igual a uma filial do ERP ligada à filial gerencial " + e.Dono
}

// ErrFilialEmUso recusa excluir filial gerencial cujas filiais do ERP têm dados.
type ErrFilialEmUso struct{ Codigo, Onde string }

func (e *ErrFilialEmUso) Error() string {
	return "A filial gerencial " + e.Codigo + " não pode ser excluída: há dados de suas filiais em " + e.Onde
}

// ErrFilialNaoEncontrada indica id inexistente na empresa.
type ErrFilialNaoEncontrada struct{}

func (e *ErrFilialNaoEncontrada) Error() string { return "Filial gerencial não encontrada" }

// FilialGerencialDe devolve o código da filial gerencial que contém a filial do
// ERP, ou a própria filial do ERP quando não há ligação (filial solta).
func FilialGerencialDe(ctx context.Context, q Consultador, empresaID, filialERP string) (string, error) {
	rows, err := q.QueryContext(ctx, `SELECT g.codigo FROM auditoria.filiais_gerenciais_erp e
		JOIN auditoria.filiais_gerenciais g ON g.id = e.filial_gerencial_id AND g.empresa_id = e.empresa_id
		WHERE e.empresa_id = $1::uuid AND e.filial_erp = $2`, empresaID, filialERP)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if rows.Next() {
		var cod string
		if err := rows.Scan(&cod); err != nil {
			return "", err
		}
		return cod, rows.Err()
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return filialERP, nil
}

// Listar devolve as filiais gerenciais da empresa, com as filiais do ERP, por código.
func (f *Filiais) Listar(ctx context.Context, empresaID string) ([]FilialGerencial, error) {
	return listarFiliais(ctx, f.DB, empresaID, "")
}

func listarFiliais(ctx context.Context, q Consultador, empresaID, id string) ([]FilialGerencial, error) {
	rows, err := q.QueryContext(ctx, `SELECT g.id::text, g.codigo, g.nome, e.filial_erp
		FROM auditoria.filiais_gerenciais g
		LEFT JOIN auditoria.filiais_gerenciais_erp e ON e.filial_gerencial_id = g.id AND e.empresa_id = g.empresa_id
		WHERE g.empresa_id = $1::uuid AND ($2 = '' OR g.id = NULLIF($2,'')::uuid)
		ORDER BY g.codigo, e.filial_erp`, empresaID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FilialGerencial{}
	for rows.Next() {
		var gid, cod, nome string
		var erp sql.NullString
		if err := rows.Scan(&gid, &cod, &nome, &erp); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].ID != gid {
			out = append(out, FilialGerencial{ID: gid, Codigo: cod, Nome: nome, Filiais: []string{}})
		}
		if erp.Valid {
			out[len(out)-1].Filiais = append(out[len(out)-1].Filiais, erp.String)
		}
	}
	return out, rows.Err()
}

var reUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func normalizarFiliais(in []string) ([]string, error) {
	if len(in) > maxFiliaisERP {
		return nil, &ErroEntrada{Msg: "no máximo 500 filiais do ERP por filial gerencial"}
	}
	vistos := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, &ErroEntrada{Msg: "filial do ERP em branco na lista"}
		}
		if utf8.RuneCountInString(s) > maxFilialERP {
			return nil, &ErroEntrada{Msg: "filial do ERP: no máximo 20 caracteres"}
		}
		if vistos[s] {
			return nil, &ErroEntrada{Msg: "filial do ERP repetida na lista: " + s}
		}
		vistos[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

func violaUnico(err error) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Code == "23505"
}

// Salvar cria ou edita a filial gerencial e substitui a lista de filiais do ERP
// de forma atômica, com a trilha na mesma transação.
func (f *Filiais) Salvar(ctx context.Context, in SalvarFilialIn) (FilialGerencial, error) {
	codigo, nome := strings.TrimSpace(in.Codigo), strings.TrimSpace(in.Nome)
	if codigo == "" || nome == "" {
		return FilialGerencial{}, &ErroEntrada{Msg: "código e nome são obrigatórios"}
	}
	if utf8.RuneCountInString(codigo) > maxCodigoFilial || utf8.RuneCountInString(nome) > maxNomeFilial {
		return FilialGerencial{}, &ErroEntrada{Msg: "código: no máximo 40 caracteres; nome: no máximo 120"}
	}
	if strings.EqualFold(codigo, CodigoReservadoGrupo) {
		return FilialGerencial{}, &ErrCodigoColide{Codigo: codigo, Com: "é a palavra reservada da visão de todas as filiais"}
	}
	if in.ID != "" && !reUUID.MatchString(in.ID) {
		return FilialGerencial{}, &ErrFilialNaoEncontrada{}
	}
	filiais, err := normalizarFiliais(in.Filiais)
	if err != nil {
		return FilialGerencial{}, err
	}
	tx, err := f.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return FilialGerencial{}, err
	}
	defer tx.Rollback()

	var doAgrupamento bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auditoria.agrupamentos_filiais
		WHERE empresa_id=$1::uuid AND codigo=$2)`, in.EmpresaID, codigo).Scan(&doAgrupamento); err != nil {
		return FilialGerencial{}, err
	}
	if doAgrupamento {
		return FilialGerencial{}, &ErrCodigoColide{Codigo: codigo, Com: "já é o código de um agrupamento"}
	}

	id := in.ID
	acao := "criar"
	var antes *FilialGerencial
	if id != "" {
		ant, err := listarFiliais(ctx, tx, in.EmpresaID, id)
		if err != nil {
			return FilialGerencial{}, err
		}
		if len(ant) == 0 {
			return FilialGerencial{}, &ErrFilialNaoEncontrada{}
		}
		antes = &ant[0]
	}
	if id == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO auditoria.filiais_gerenciais (empresa_id, codigo, nome)
			VALUES ($1::uuid,$2,$3) RETURNING id::text`, in.EmpresaID, codigo, nome).Scan(&id)
		if violaUnico(err) {
			return FilialGerencial{}, &ErrCodigoEmUso{Codigo: codigo}
		}
		if err != nil {
			return FilialGerencial{}, err
		}
	} else {
		acao = "editar"
		res, err := tx.ExecContext(ctx, `UPDATE auditoria.filiais_gerenciais SET codigo=$3, nome=$4
			WHERE empresa_id=$1::uuid AND id=$2::uuid`, in.EmpresaID, id, codigo, nome)
		if violaUnico(err) {
			return FilialGerencial{}, &ErrCodigoEmUso{Codigo: codigo}
		}
		if err != nil {
			return FilialGerencial{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return FilialGerencial{}, &ErrFilialNaoEncontrada{}
		}
	}

	// O código não pode coincidir com filial do ERP de OUTRA filial gerencial
	// (o resolvedor devolveria a mesma chave para coisas diferentes).
	var outro string
	err = tx.QueryRowContext(ctx, `SELECT g.codigo FROM auditoria.filiais_gerenciais_erp x
		JOIN auditoria.filiais_gerenciais g ON g.id = x.filial_gerencial_id
		WHERE x.empresa_id=$1::uuid AND x.filial_erp=$2 AND x.filial_gerencial_id <> $3::uuid`, in.EmpresaID, codigo, id).Scan(&outro)
	if err == nil {
		return FilialGerencial{}, &ErrCodigoIgualFilial{Codigo: codigo, Dono: outro}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return FilialGerencial{}, err
	}
	// Conflito: filial do ERP ligada a OUTRA filial gerencial da empresa.
	for _, e := range filiais {
		var dono string
		err := tx.QueryRowContext(ctx, `SELECT g.codigo FROM auditoria.filiais_gerenciais_erp x
			JOIN auditoria.filiais_gerenciais g ON g.id = x.filial_gerencial_id
			WHERE x.empresa_id=$1::uuid AND x.filial_erp=$2 AND x.filial_gerencial_id <> $3::uuid`,
			in.EmpresaID, e, id).Scan(&dono)
		if err == nil {
			return FilialGerencial{}, &ErrFilialLigada{FilialERP: e, Dono: dono}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return FilialGerencial{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.filiais_gerenciais_erp
		WHERE empresa_id=$1::uuid AND filial_gerencial_id=$2::uuid`, in.EmpresaID, id); err != nil {
		return FilialGerencial{}, err
	}
	for _, e := range filiais {
		if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.filiais_gerenciais_erp (empresa_id, filial_gerencial_id, filial_erp)
			VALUES ($1::uuid,$2::uuid,$3)`, in.EmpresaID, id, e); err != nil {
			if violaUnico(err) { // corrida com outra gravação
				return FilialGerencial{}, &ErrFilialLigada{FilialERP: e, Dono: "outra filial gerencial"}
			}
			return FilialGerencial{}, err
		}
	}
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "filial_gerencial", id, acao,
		map[string]any{"codigo": codigo, "nome": nome, "filiais": filiais, "antes": antes}); err != nil {
		return FilialGerencial{}, err
	}
	if err := tx.Commit(); err != nil {
		return FilialGerencial{}, err
	}
	return FilialGerencial{ID: id, Codigo: codigo, Nome: nome, Filiais: filiais}, nil
}

// Excluir remove a filial gerencial e suas ligações, desde que nenhuma filial do
// ERP dela apareça em lançamentos, orçado ou fechamentos da empresa.
func (f *Filiais) Excluir(ctx context.Context, empresaID, userID, id string) error {
	tx, err := f.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var codigo string
	if !reUUID.MatchString(id) {
		return &ErrFilialNaoEncontrada{}
	}
	err = tx.QueryRowContext(ctx, `SELECT codigo FROM auditoria.filiais_gerenciais
		WHERE empresa_id=$1::uuid AND id=$2::uuid FOR UPDATE`, empresaID, id).Scan(&codigo)
	if errors.Is(err, sql.ErrNoRows) {
		return &ErrFilialNaoEncontrada{}
	}
	if err != nil {
		return err
	}
	var agrup string
	err = tx.QueryRowContext(ctx, `SELECT a.codigo FROM auditoria.agrupamentos_filiais_membros m
		JOIN auditoria.agrupamentos_filiais a ON a.id = m.agrupamento_id AND a.empresa_id = m.empresa_id
		WHERE m.empresa_id=$1::uuid AND m.filial_gerencial_id=$2::uuid ORDER BY a.codigo LIMIT 1`, empresaID, id).Scan(&agrup)
	if err == nil {
		return &ErrFilialEmAgrupamento{Filial: codigo, Agrupamento: agrup}
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	atuais, err := listarFiliais(ctx, tx, empresaID, id)
	if err != nil {
		return err
	}
	erps := []string{}
	nome := ""
	if len(atuais) == 1 {
		erps, nome = atuais[0].Filiais, atuais[0].Nome
	}
	usos := []struct{ onde, q string }{
		{"lançamentos", `SELECT EXISTS(SELECT 1 FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND filial = ANY($2))`},
		{"orçado", `SELECT EXISTS(SELECT 1 FROM auditoria.dre_orcado WHERE empresa_id=$1::uuid AND filial = ANY($2))`},
		{"fechamentos", `SELECT EXISTS(SELECT 1 FROM auditoria.dre_fechamentos WHERE empresa_id=$1::uuid AND filial = ANY($2))
			OR EXISTS(SELECT 1 FROM auditoria.dre_fechamento_linhas l
			JOIN auditoria.dre_fechamentos f ON f.id = l.fechamento_id
			WHERE f.empresa_id=$1::uuid AND l.filial = ANY($2))
			OR EXISTS(SELECT 1 FROM auditoria.dre_fechamento_contas WHERE empresa_id=$1::uuid AND filial = ANY($2))`},
	}
	if len(erps) > 0 {
		for _, u := range usos {
			var em bool
			if err := tx.QueryRowContext(ctx, u.q, empresaID, pq.Array(erps)).Scan(&em); err != nil {
				return err
			}
			if em {
				return &ErrFilialEmUso{Codigo: codigo, Onde: u.onde}
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.filiais_gerenciais_erp
		WHERE empresa_id=$1::uuid AND filial_gerencial_id=$2::uuid`, empresaID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.filiais_gerenciais
		WHERE empresa_id=$1::uuid AND id=$2::uuid`, empresaID, id); err != nil {
		return err
	}
	if err := trilha.Gravar(tx, empresaID, userID, "filial_gerencial", id, "excluir",
		map[string]any{"codigo": codigo, "nome": nome, "filiais": erps}); err != nil {
		return err
	}
	return tx.Commit()
}
