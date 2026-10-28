package dre

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"fb_auditoria/trilha"
)

// CodigoReservadoGrupo é a palavra reservada da visão "Grupo" (todas as filiais),
// que não tem cadastro: nenhum agrupamento nem filial gerencial pode usá-la como
// código (sem diferenciar maiúsculas).
const CodigoReservadoGrupo = "GRUPO"

// FilialGrupo é o filtro de filial que a leitura do DRE entende como "todas as
// filiais" (Leitor.Ler com filial vazia soma tudo; mês fechado lê o consolidado).
const FilialGrupo = ""

// Agrupamentos cadastra os agrupamentos de filiais gerenciais.
type Agrupamentos struct{ DB *sql.DB }

// Agrupamento é uma visão nomeada que soma filiais gerenciais.
type Agrupamento struct {
	ID      string   `json:"id"`
	Codigo  string   `json:"codigo"`
	Nome    string   `json:"nome"`
	Filiais []string `json:"filiais"` // códigos das filiais gerenciais membros
}

// SalvarAgrupamentoIn é a entrada de Salvar; ID vazio cria, preenchido edita.
type SalvarAgrupamentoIn struct {
	EmpresaID string
	UserID    string
	ID        string
	Codigo    string
	Nome      string
	Filiais   []string // códigos de filiais gerenciais
}

// ErrAgrupamentoNaoEncontrado indica id ou código inexistente na empresa.
type ErrAgrupamentoNaoEncontrado struct{}

func (e *ErrAgrupamentoNaoEncontrado) Error() string { return "Agrupamento não encontrado" }

// ErrAgrupamentoCodigoEmUso recusa código de agrupamento já cadastrado na empresa.
type ErrAgrupamentoCodigoEmUso struct{ Codigo string }

func (e *ErrAgrupamentoCodigoEmUso) Error() string {
	return "Já existe um agrupamento com o código " + e.Codigo
}

// ErrCodigoColide recusa código de agrupamento igual ao de uma filial gerencial
// (ou o inverso) e a palavra reservada do grupo todo.
type ErrCodigoColide struct{ Codigo, Com string }

func (e *ErrCodigoColide) Error() string {
	return "O código " + e.Codigo + " não pode ser usado: " + e.Com
}

// ErrMembroInexistente recusa membro que não é filial gerencial da empresa.
type ErrMembroInexistente struct{ Codigo string }

func (e *ErrMembroInexistente) Error() string {
	return "A filial gerencial " + e.Codigo + " não existe nesta empresa"
}

// ErrFilialEmAgrupamento recusa excluir filial gerencial que é membro de agrupamento.
type ErrFilialEmAgrupamento struct{ Filial, Agrupamento string }

func (e *ErrFilialEmAgrupamento) Error() string {
	return "A filial gerencial " + e.Filial + " não pode ser excluída: faz parte do agrupamento " + e.Agrupamento
}

// ErrAgrupamentoVazio indica agrupamento sem nenhuma filial do ERP para somar.
type ErrAgrupamentoVazio struct{ Codigo string }

func (e *ErrAgrupamentoVazio) Error() string {
	return "O agrupamento " + e.Codigo + " não tem filiais do ERP para somar"
}

// Alvo é o recorte de filiais do ERP que o DRE soma.
type Alvo struct {
	Gerenciais []string // códigos das filiais gerenciais do agrupamento
	ERP        []string // filiais do ERP, por elas, ordenadas
}

// FiliaisDoAgrupamento resolve o código do agrupamento nas filiais gerenciais
// membros e, por elas (mapa da 2.14), nas filiais do ERP. Código inexistente:
// ErrAgrupamentoNaoEncontrado. A visão "Grupo" não passa por aqui (FilialGrupo).
func FiliaisDoAgrupamento(ctx context.Context, q Consultador, empresaID, codigo string) (Alvo, error) {
	rows, err := q.QueryContext(ctx, `SELECT g.codigo, e.filial_erp
		FROM auditoria.agrupamentos_filiais a
		LEFT JOIN auditoria.agrupamentos_filiais_membros m ON m.agrupamento_id = a.id AND m.empresa_id = a.empresa_id
		LEFT JOIN auditoria.filiais_gerenciais g ON g.id = m.filial_gerencial_id AND g.empresa_id = m.empresa_id
		LEFT JOIN auditoria.filiais_gerenciais_erp e ON e.filial_gerencial_id = g.id AND e.empresa_id = g.empresa_id
		WHERE a.empresa_id = $1::uuid AND a.codigo = $2
		ORDER BY g.codigo, e.filial_erp`, empresaID, codigo)
	if err != nil {
		return Alvo{}, err
	}
	defer rows.Close()
	alvo := Alvo{Gerenciais: []string{}, ERP: []string{}}
	achou := false
	vistas := map[string]bool{}
	for rows.Next() {
		achou = true
		var g, erp sql.NullString
		if err := rows.Scan(&g, &erp); err != nil {
			return Alvo{}, err
		}
		if g.Valid && (len(alvo.Gerenciais) == 0 || alvo.Gerenciais[len(alvo.Gerenciais)-1] != g.String) {
			alvo.Gerenciais = append(alvo.Gerenciais, g.String)
		}
		// Cada filial do ERP pertence a uma só filial gerencial: sem repetição.
		if erp.Valid && !vistas[erp.String] {
			vistas[erp.String] = true
			alvo.ERP = append(alvo.ERP, erp.String)
		}
	}
	if err := rows.Err(); err != nil {
		return Alvo{}, err
	}
	if !achou {
		return Alvo{}, &ErrAgrupamentoNaoEncontrado{}
	}
	sort.Strings(alvo.ERP)
	return alvo, nil
}

// Listar devolve os agrupamentos da empresa, com os códigos das filiais gerenciais, por código.
func (a *Agrupamentos) Listar(ctx context.Context, empresaID string) ([]Agrupamento, error) {
	return listarAgrupamentos(ctx, a.DB, empresaID, "")
}

func listarAgrupamentos(ctx context.Context, q Consultador, empresaID, id string) ([]Agrupamento, error) {
	rows, err := q.QueryContext(ctx, `SELECT a.id::text, a.codigo, a.nome, g.codigo
		FROM auditoria.agrupamentos_filiais a
		LEFT JOIN auditoria.agrupamentos_filiais_membros m ON m.agrupamento_id = a.id AND m.empresa_id = a.empresa_id
		LEFT JOIN auditoria.filiais_gerenciais g ON g.id = m.filial_gerencial_id AND g.empresa_id = m.empresa_id
		WHERE a.empresa_id = $1::uuid AND ($2 = '' OR a.id = NULLIF($2,'')::uuid)
		ORDER BY a.codigo, g.codigo`, empresaID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Agrupamento{}
	for rows.Next() {
		var aid, cod, nome string
		var g sql.NullString
		if err := rows.Scan(&aid, &cod, &nome, &g); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].ID != aid {
			out = append(out, Agrupamento{ID: aid, Codigo: cod, Nome: nome, Filiais: []string{}})
		}
		if g.Valid {
			out[len(out)-1].Filiais = append(out[len(out)-1].Filiais, g.String)
		}
	}
	return out, rows.Err()
}

func normalizarMembros(in []string) ([]string, error) {
	if len(in) > maxFiliaisERP {
		return nil, &ErroEntrada{Msg: "no máximo 500 filiais gerenciais por agrupamento"}
	}
	vistos := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, &ErroEntrada{Msg: "filial gerencial em branco na lista"}
		}
		if vistos[s] {
			return nil, &ErroEntrada{Msg: "filial gerencial repetida na lista: " + s}
		}
		vistos[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

// Salvar cria ou edita o agrupamento e substitui a lista de membros de forma
// atômica, com a trilha na mesma transação.
func (a *Agrupamentos) Salvar(ctx context.Context, in SalvarAgrupamentoIn) (Agrupamento, error) {
	codigo, nome := strings.TrimSpace(in.Codigo), strings.TrimSpace(in.Nome)
	if codigo == "" || nome == "" {
		return Agrupamento{}, &ErroEntrada{Msg: "código e nome são obrigatórios"}
	}
	if utf8.RuneCountInString(codigo) > maxCodigoFilial || utf8.RuneCountInString(nome) > maxNomeFilial {
		return Agrupamento{}, &ErroEntrada{Msg: "código: no máximo 40 caracteres; nome: no máximo 120"}
	}
	if strings.EqualFold(codigo, CodigoReservadoGrupo) {
		return Agrupamento{}, &ErrCodigoColide{Codigo: codigo, Com: "é a palavra reservada da visão de todas as filiais"}
	}
	if in.ID != "" && !reUUID.MatchString(in.ID) {
		return Agrupamento{}, &ErrAgrupamentoNaoEncontrado{}
	}
	membros, err := normalizarMembros(in.Filiais)
	if err != nil {
		return Agrupamento{}, err
	}
	tx, err := a.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Agrupamento{}, err
	}
	defer tx.Rollback()

	id := in.ID
	acao := "criar"
	var antes *Agrupamento
	if id != "" {
		ant, err := listarAgrupamentos(ctx, tx, in.EmpresaID, id)
		if err != nil {
			return Agrupamento{}, err
		}
		if len(ant) == 0 {
			return Agrupamento{}, &ErrAgrupamentoNaoEncontrado{}
		}
		antes = &ant[0]
	}
	// O código não pode coincidir com o de uma filial gerencial.
	var existe bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auditoria.filiais_gerenciais
		WHERE empresa_id=$1::uuid AND codigo=$2)`, in.EmpresaID, codigo).Scan(&existe); err != nil {
		return Agrupamento{}, err
	}
	if existe {
		return Agrupamento{}, &ErrCodigoColide{Codigo: codigo, Com: "já é o código de uma filial gerencial"}
	}

	if id == "" {
		err = tx.QueryRowContext(ctx, `INSERT INTO auditoria.agrupamentos_filiais (empresa_id, codigo, nome)
			VALUES ($1::uuid,$2,$3) RETURNING id::text`, in.EmpresaID, codigo, nome).Scan(&id)
		if violaUnico(err) {
			return Agrupamento{}, &ErrAgrupamentoCodigoEmUso{Codigo: codigo}
		}
		if err != nil {
			return Agrupamento{}, err
		}
	} else {
		acao = "editar"
		res, err := tx.ExecContext(ctx, `UPDATE auditoria.agrupamentos_filiais SET codigo=$3, nome=$4
			WHERE empresa_id=$1::uuid AND id=$2::uuid`, in.EmpresaID, id, codigo, nome)
		if violaUnico(err) {
			return Agrupamento{}, &ErrAgrupamentoCodigoEmUso{Codigo: codigo}
		}
		if err != nil {
			return Agrupamento{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return Agrupamento{}, &ErrAgrupamentoNaoEncontrado{}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.agrupamentos_filiais_membros
		WHERE empresa_id=$1::uuid AND agrupamento_id=$2::uuid`, in.EmpresaID, id); err != nil {
		return Agrupamento{}, err
	}
	for _, m := range membros {
		var gid string
		err := tx.QueryRowContext(ctx, `SELECT id::text FROM auditoria.filiais_gerenciais
			WHERE empresa_id=$1::uuid AND codigo=$2`, in.EmpresaID, m).Scan(&gid)
		if errors.Is(err, sql.ErrNoRows) {
			return Agrupamento{}, &ErrMembroInexistente{Codigo: m}
		}
		if err != nil {
			return Agrupamento{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.agrupamentos_filiais_membros
			(empresa_id, agrupamento_id, filial_gerencial_id) VALUES ($1::uuid,$2::uuid,$3::uuid)`,
			in.EmpresaID, id, gid); err != nil {
			return Agrupamento{}, err
		}
	}
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "agrupamento_filiais", id, acao,
		map[string]any{"codigo": codigo, "nome": nome, "filiais": membros, "antes": antes}); err != nil {
		return Agrupamento{}, err
	}
	if err := tx.Commit(); err != nil {
		return Agrupamento{}, err
	}
	return Agrupamento{ID: id, Codigo: codigo, Nome: nome, Filiais: membros}, nil
}

// Excluir remove o agrupamento e a lista de membros (as filiais gerenciais ficam).
func (a *Agrupamentos) Excluir(ctx context.Context, empresaID, userID, id string) error {
	if !reUUID.MatchString(id) {
		return &ErrAgrupamentoNaoEncontrado{}
	}
	tx, err := a.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var codigo string
	err = tx.QueryRowContext(ctx, `SELECT codigo FROM auditoria.agrupamentos_filiais
		WHERE empresa_id=$1::uuid AND id=$2::uuid FOR UPDATE`, empresaID, id).Scan(&codigo)
	if errors.Is(err, sql.ErrNoRows) {
		return &ErrAgrupamentoNaoEncontrado{}
	}
	if err != nil {
		return err
	}
	atuais, err := listarAgrupamentos(ctx, tx, empresaID, id)
	if err != nil {
		return err
	}
	var antes *Agrupamento
	if len(atuais) == 1 {
		antes = &atuais[0]
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.agrupamentos_filiais
		WHERE empresa_id=$1::uuid AND id=$2::uuid`, empresaID, id); err != nil { // membros caem em cascata
		return err
	}
	if err := trilha.Gravar(tx, empresaID, userID, "agrupamento_filiais", id, "excluir",
		map[string]any{"codigo": codigo, "antes": antes}); err != nil {
		return err
	}
	return tx.Commit()
}

// OrigemFiliais diz de onde se lê o mês para um recorte de várias filiais do
// ERP: Snapshot se o fechamento é completo (vale para toda filial), Legado se
// alguma filial tem fechamento legado, senão Vivo.
func (a Ativo) OrigemFiliais(filiais []string) Origem {
	if a.Completo {
		return Snapshot
	}
	for _, f := range filiais {
		if a.Origem(f) == Legado {
			return Legado
		}
	}
	return Vivo
}

// LerFiliais lê o período para cada filial do ERP do recorte e soma os
// resultados linha a linha (os subtotais são aditivos). A lógica de leitura e de
// snapshot é a de Ler, sem mudança: mês fechado vem do snapshot de cada filial.
func (l Leitor) LerFiliais(ctx context.Context, empresaID string, filiais []string, p Periodo, linhas []Linha, mapa []MapaItem) (Leitura, error) {
	if len(filiais) == 1 {
		return l.Ler(ctx, empresaID, filiais[0], p, linhas, mapa)
	}
	var tot Leitura
	for i, f := range filiais {
		r, err := l.Ler(ctx, empresaID, f, p, linhas, mapa)
		if err != nil {
			return Leitura{}, err
		}
		if i == 0 {
			tot = r
			continue
		}
		somar(tot.Real, r.Real)
		somar(tot.Orc, r.Orc)
		somar(tot.AA, r.AA)
		somar(tot.Sem, r.Sem)
	}
	if tot.Real == nil {
		tot = Leitura{Real: map[string]float64{}, Orc: map[string]float64{}, AA: map[string]float64{}, Sem: map[string]float64{}, Ativos: map[string]Ativo{}}
	}
	return tot, nil
}

// ContasPorDepartamentoFiliais soma ContasPorDepartamento das filiais do recorte.
func (l Leitor) ContasPorDepartamentoFiliais(ctx context.Context, empresaID string, filiais []string, p Periodo, mapa []MapaItem, ativos map[string]Ativo) ([]ContaDepto, []string, error) {
	if len(filiais) == 1 {
		return l.ContasPorDepartamento(ctx, empresaID, filiais[0], p, mapa, ativos)
	}
	type chave struct{ conta, dep string }
	acc := map[chave]*ContaDepto{}
	sem := map[string]bool{}
	for _, f := range filiais {
		contas, semContas, err := l.ContasPorDepartamento(ctx, empresaID, f, p, mapa, ativos)
		if err != nil {
			return nil, nil, err
		}
		for _, c := range contas {
			k := chave{c.Conta, c.Departamento}
			if x, ok := acc[k]; ok {
				x.Valor += c.Valor
			} else {
				c := c
				acc[k] = &c
			}
		}
		for _, m := range semContas {
			sem[m] = true
		}
	}
	var out []ContaDepto
	for _, c := range acc {
		if c.Valor != 0 {
			out = append(out, *c)
		}
	}
	var semOut []string
	for m := range sem {
		semOut = append(semOut, m)
	}
	sort.Strings(semOut)
	return out, semOut, nil
}
