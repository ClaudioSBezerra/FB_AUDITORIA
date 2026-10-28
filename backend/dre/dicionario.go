package dre

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"fb_auditoria/trilha"
)

const (
	maxCodigoDepto = 40
	maxNomeDepto   = 120
)

// GruposDepartamento são os grupos aceitos (mesma lista do CHECK da tabela).
var GruposDepartamento = []string{"Comercial", "Logística", "Administrativo"}

// Dicionario cadastra departamentos e o dicionário de contas; é o único escritor
// dessas tabelas.
type Dicionario struct{ DB *sql.DB }

// Departamento é um departamento da empresa.
type Departamento struct {
	ID     string `json:"id"`
	Codigo string `json:"codigo"`
	Nome   string `json:"nome"`
	Grupo  string `json:"grupo"`
}

// SalvarDepartamentoIn é a entrada de SalvarDepartamento; ID vazio cria.
type SalvarDepartamentoIn struct {
	EmpresaID string
	UserID    string
	ID        string
	Codigo    string
	Nome      string
	Grupo     string
}

// ErrDepartamentoCodigoEmUso recusa código já cadastrado na empresa.
type ErrDepartamentoCodigoEmUso struct{ Codigo string }

func (e *ErrDepartamentoCodigoEmUso) Error() string {
	return "Já existe um departamento com o código " + e.Codigo
}

// ErrDepartamentoNaoEncontrado indica id inexistente na empresa.
type ErrDepartamentoNaoEncontrado struct{}

func (e *ErrDepartamentoNaoEncontrado) Error() string { return "Departamento não encontrado" }

// ListarDepartamentos devolve os departamentos da empresa, por código.
func (d *Dicionario) ListarDepartamentos(ctx context.Context, empresaID string) ([]Departamento, error) {
	return listarDepartamentos(ctx, d.DB, empresaID, "")
}

func listarDepartamentos(ctx context.Context, q Consultador, empresaID, id string) ([]Departamento, error) {
	rows, err := q.QueryContext(ctx, `SELECT id::text, codigo, nome, grupo FROM auditoria.departamentos
		WHERE empresa_id = $1::uuid AND ($2 = '' OR id = NULLIF($2,'')::uuid) ORDER BY codigo`, empresaID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Departamento{}
	for rows.Next() {
		var x Departamento
		if err := rows.Scan(&x.ID, &x.Codigo, &x.Nome, &x.Grupo); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func grupoValido(g string) bool {
	for _, x := range GruposDepartamento {
		if x == g {
			return true
		}
	}
	return false
}

// SalvarDepartamento cria ou edita o departamento, com a trilha na mesma transação.
func (d *Dicionario) SalvarDepartamento(ctx context.Context, in SalvarDepartamentoIn) (Departamento, error) {
	codigo, nome, grupo := strings.TrimSpace(in.Codigo), strings.TrimSpace(in.Nome), strings.TrimSpace(in.Grupo)
	if codigo == "" || nome == "" {
		return Departamento{}, &ErroEntrada{Msg: "código e nome são obrigatórios"}
	}
	if utf8.RuneCountInString(codigo) > maxCodigoDepto || utf8.RuneCountInString(nome) > maxNomeDepto {
		return Departamento{}, &ErroEntrada{Msg: "código: no máximo 40 caracteres; nome: no máximo 120"}
	}
	if !grupoValido(grupo) {
		return Departamento{}, &ErroEntrada{Msg: "grupo deve ser Comercial, Logística ou Administrativo"}
	}
	if in.ID != "" && !reUUID.MatchString(in.ID) {
		return Departamento{}, &ErrDepartamentoNaoEncontrado{}
	}
	tx, err := d.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Departamento{}, err
	}
	defer tx.Rollback()

	id, acao := in.ID, "criar"
	var antes *Departamento
	if id == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO auditoria.departamentos (empresa_id, codigo, nome, grupo)
			VALUES ($1::uuid,$2,$3,$4) RETURNING id::text`, in.EmpresaID, codigo, nome, grupo).Scan(&id)
		if violaUnico(err) {
			return Departamento{}, &ErrDepartamentoCodigoEmUso{Codigo: codigo}
		}
		if err != nil {
			return Departamento{}, err
		}
	} else {
		acao = "editar"
		ant, err := listarDepartamentos(ctx, tx, in.EmpresaID, id)
		if err != nil {
			return Departamento{}, err
		}
		if len(ant) == 0 {
			return Departamento{}, &ErrDepartamentoNaoEncontrado{}
		}
		antes = &ant[0]
		res, err := tx.ExecContext(ctx, `UPDATE auditoria.departamentos SET codigo=$3, nome=$4, grupo=$5
			WHERE empresa_id=$1::uuid AND id=$2::uuid`, in.EmpresaID, id, codigo, nome, grupo)
		if violaUnico(err) {
			return Departamento{}, &ErrDepartamentoCodigoEmUso{Codigo: codigo}
		}
		if err != nil {
			return Departamento{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Departamento{}, &ErrDepartamentoNaoEncontrado{}
		}
	}
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "departamento", id, acao,
		map[string]any{"codigo": codigo, "nome": nome, "grupo": grupo, "antes": antes}); err != nil {
		return Departamento{}, err
	}
	if err := tx.Commit(); err != nil {
		return Departamento{}, err
	}
	return Departamento{ID: id, Codigo: codigo, Nome: nome, Grupo: grupo}, nil
}

// ExcluirDepartamento remove o departamento. A recusa por contas ligadas
// (ErrDepartamentoEmUso) entra com a história 2.10, quando a tabela
// conta_departamento passa a existir.
func (d *Dicionario) ExcluirDepartamento(ctx context.Context, empresaID, userID, id string) error {
	if !reUUID.MatchString(id) {
		return &ErrDepartamentoNaoEncontrado{}
	}
	tx, err := d.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var x Departamento
	err = tx.QueryRowContext(ctx, `SELECT id::text, codigo, nome, grupo FROM auditoria.departamentos
		WHERE empresa_id=$1::uuid AND id=$2::uuid FOR UPDATE`, empresaID, id).Scan(&x.ID, &x.Codigo, &x.Nome, &x.Grupo)
	if errors.Is(err, sql.ErrNoRows) {
		return &ErrDepartamentoNaoEncontrado{}
	}
	if err != nil {
		return err
	}
	if err := d.verificarUsoDepartamento(ctx, tx, empresaID, x); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND id=$2::uuid`, empresaID, id); err != nil {
		return err
	}
	if err := trilha.Gravar(tx, empresaID, userID, "departamento", id, "excluir",
		map[string]any{"codigo": x.Codigo, "nome": x.Nome, "grupo": x.Grupo}); err != nil {
		return err
	}
	return tx.Commit()
}

// verificarUsoDepartamento é o ponto de extensão da recusa por contas ligadas.
func (d *Dicionario) verificarUsoDepartamento(ctx context.Context, tx *sql.Tx, empresaID string, x Departamento) error {
	return nil
}
