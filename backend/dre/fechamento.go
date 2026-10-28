package dre

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fb_auditoria/trilha"
)

// Fechamento congela o realizado de um mês numa nova versão.
type Fechamento struct{ DB *sql.DB }

// FecharIn é a entrada de Fechar. O fechamento é sempre do grupo inteiro:
// Filial preenchida é recusada.
type FecharIn struct {
	EmpresaID  string
	UserID     string
	Ano, Mes   int
	Filial     string
	Observacao string
}

// FecharOut é o resultado de Fechar.
type FecharOut struct {
	ID     int64
	Versao int
}

// ErrContasSemMapa recusa o fechamento quando há conta com lançamento sem linha.
type ErrContasSemMapa struct {
	Contas []string // ordenadas; no máximo 20
	Total  int
}

func (e *ErrContasSemMapa) Error() string {
	msg := "Existem contas sem mapeamento no período — mapeie antes de fechar (" + strconv.Itoa(e.Total) + "): " + strings.Join(e.Contas, ", ")
	if e.Total > len(e.Contas) {
		msg += fmt.Sprintf(" e mais %d", e.Total-len(e.Contas))
	}
	return msg
}

// ErrJaFechado recusa fechar competência que já tem fechamento ativo.
type ErrJaFechado struct{ Competencia string }

func (e *ErrJaFechado) Error() string {
	return "Competência " + e.Competencia + " já está fechada — reabra o fechamento (com motivo) antes de fechar de novo"
}

// ErrNaoFechado recusa reabrir competência sem fechamento ativo.
type ErrNaoFechado struct{ Competencia string }

func (e *ErrNaoFechado) Error() string {
	return "Competência " + e.Competencia + " não tem fechamento ativo para reabrir"
}

const maxContasListadas = 20

// MaxMotivoReabertura é o limite do motivo, em caracteres (runas).
const MaxMotivoReabertura = 500

// Fechar calcula o snapshot dentro da transação (via Realizado), grava o
// fechamento, as linhas e a trilha no mesmo Commit. Tudo ou nada.
func (f *Fechamento) Fechar(ctx context.Context, in FecharIn) (FecharOut, error) {
	if in.Mes < 1 || in.Mes > 12 || in.Ano < 2000 || in.Ano > 2100 {
		return FecharOut{}, &ErroEntrada{Msg: "ano/mes inválidos"}
	}
	if strings.TrimSpace(in.Filial) != "" {
		return FecharOut{}, &ErroEntrada{Msg: "o fechamento é do grupo inteiro: não informe filial"}
	}
	comp := time.Date(in.Ano, time.Month(in.Mes), 1, 0, 0, 0, 0, time.UTC)
	tx, err := f.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return FecharOut{}, err
	}
	defer tx.Rollback()
	// Mesmo lock de Guardar: serializa com importações da competência.
	if err := travar(ctx, tx, in.EmpresaID, comp); err != nil {
		return FecharOut{}, err
	}

	var ativos int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM auditoria.dre_fechamentos
		WHERE empresa_id=$1::uuid AND competencia=$2 AND reaberto_em IS NULL`, in.EmpresaID, comp).Scan(&ativos); err != nil {
		return FecharOut{}, err
	}
	if ativos > 0 {
		return FecharOut{}, &ErrJaFechado{Competencia: comp.Format("2006-01")}
	}

	linhas, err := CarregarLinhas(ctx, tx, in.EmpresaID)
	if err != nil {
		return FecharOut{}, err
	}
	mapa, err := CarregarMapa(ctx, tx, in.EmpresaID)
	if err != nil {
		return FecharOut{}, err
	}
	real, sem, err := Realizado(ctx, tx, in.EmpresaID, "", Periodo{Ini: comp, Fim: comp}, mapa)
	if err != nil {
		return FecharOut{}, err
	}
	if len(sem) > 0 {
		contas := make([]string, 0, len(sem))
		for c := range sem {
			contas = append(contas, c)
		}
		sort.Strings(contas)
		total := len(contas)
		if total > maxContasListadas {
			contas = contas[:maxContasListadas]
		}
		return FecharOut{}, &ErrContasSemMapa{Contas: contas, Total: total}
	}
	AplicarSubtotais(linhas, real)
	pAA := Periodo{Ini: comp.AddDate(-1, 0, 0), Fim: comp.AddDate(-1, 0, 0)}
	filiais, err := filiaisDoSnapshot(ctx, tx, in.EmpresaID, comp)
	if err != nil {
		return FecharOut{}, err
	}

	var versao int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(versao),0)+1 FROM auditoria.dre_fechamentos
		WHERE empresa_id=$1::uuid AND competencia=$2`, in.EmpresaID, comp).Scan(&versao); err != nil {
		return FecharOut{}, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por, observacao)
		VALUES ($1::uuid,$2,$3,'',$4::uuid,$5) RETURNING id`, in.EmpresaID, comp, versao, in.UserID, in.Observacao).Scan(&id); err != nil {
		return FecharOut{}, err
	}
	// Snapshot completo: o consolidado do grupo ('') e cada filial, na mesma transação.
	for _, filial := range append([]string{""}, filiais...) {
		r := real
		if filial != "" {
			if r, _, err = Realizado(ctx, tx, in.EmpresaID, filial, Periodo{Ini: comp, Fim: comp}, mapa); err != nil {
				return FecharOut{}, err
			}
			AplicarSubtotais(linhas, r)
		}
		aa, _, err := Realizado(ctx, tx, in.EmpresaID, filial, pAA, mapa)
		if err != nil {
			return FecharOut{}, err
		}
		AplicarSubtotais(linhas, aa)
		orc, err := OrcadoPorLinha(ctx, tx, in.EmpresaID, filial, Periodo{Ini: comp, Fim: comp})
		if err != nil {
			return FecharOut{}, err
		}
		AplicarSubtotais(linhas, orc)
		for _, l := range linhas {
			if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.dre_fechamento_linhas (fechamento_id, filial, linha_codigo, descricao, nivel, ordem, valor, orcado, ano_anterior, tipo, papel)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''))`, id, filial, l.Codigo, l.Descricao, l.Nivel, l.Ordem, r[l.Codigo], orc[l.Codigo], aa[l.Codigo], l.Tipo, l.Papel); err != nil {
				return FecharOut{}, err
			}
		}
		cr, err := RealizadoPorConta(ctx, tx, in.EmpresaID, filial, Periodo{Ini: comp, Fim: comp}, mapa)
		if err != nil {
			return FecharOut{}, err
		}
		ca, err := RealizadoPorConta(ctx, tx, in.EmpresaID, filial, pAA, mapa)
		if err != nil {
			return FecharOut{}, err
		}
		for conta, v := range cr {
			if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.dre_fechamento_contas (fechamento_id, empresa_id, filial, conta, linha_codigo, realizado, ano_anterior)
				VALUES ($1,$2::uuid,$3,$4,$5,$6,$7)`, id, in.EmpresaID, filial, conta, v.Linha, v.Valor, ca[conta].Valor); err != nil {
				return FecharOut{}, err
			}
		}
		for conta, v := range ca {
			if _, ok := cr[conta]; ok {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.dre_fechamento_contas (fechamento_id, empresa_id, filial, conta, linha_codigo, realizado, ano_anterior)
				VALUES ($1,$2::uuid,$3,$4,$5,0,$6)`, id, in.EmpresaID, filial, conta, v.Linha, v.Valor); err != nil {
				return FecharOut{}, err
			}
		}
	}
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "dre_fechamento", strconv.FormatInt(id, 10), "fechar",
		map[string]any{"competencia": comp.Format("2006-01"), "versao": versao}); err != nil {
		return FecharOut{}, err
	}
	if err := tx.Commit(); err != nil {
		return FecharOut{}, err
	}
	return FecharOut{ID: id, Versao: versao}, nil
}

// ReabrirIn é a entrada de Reabrir.
type ReabrirIn struct {
	EmpresaID string
	UserID    string
	Ano, Mes  int
	Motivo    string
}

// ReabrirOut é o resultado de Reabrir.
type ReabrirOut struct {
	ID     int64
	Versao int
}

// Reabrir marca o fechamento ativo da competência como reaberto (motivo
// obrigatório), com o mesmo lock de Guardar e Fechar; a versão anterior
// permanece legível e o próximo Fechar cria MAX+1. Tudo ou nada, com a trilha.
func (f *Fechamento) Reabrir(ctx context.Context, in ReabrirIn) (ReabrirOut, error) {
	if in.Mes < 1 || in.Mes > 12 || in.Ano < 2000 || in.Ano > 2100 {
		return ReabrirOut{}, &ErroEntrada{Msg: "ano/mes inválidos"}
	}
	motivo := strings.TrimSpace(in.Motivo)
	if motivo == "" {
		return ReabrirOut{}, &ErroEntrada{Msg: "informe o motivo da reabertura"}
	}
	if utf8.RuneCountInString(motivo) > MaxMotivoReabertura {
		return ReabrirOut{}, &ErroEntrada{Msg: "motivo da reabertura: no máximo 500 caracteres"}
	}
	comp := time.Date(in.Ano, time.Month(in.Mes), 1, 0, 0, 0, 0, time.UTC)
	tx, err := f.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return ReabrirOut{}, err
	}
	defer tx.Rollback()
	if err := travar(ctx, tx, in.EmpresaID, comp); err != nil {
		return ReabrirOut{}, err
	}
	var id int64
	var versao int
	err = tx.QueryRowContext(ctx, `SELECT id, versao FROM auditoria.dre_fechamentos
		WHERE empresa_id=$1::uuid AND competencia=$2 AND reaberto_em IS NULL`, in.EmpresaID, comp).Scan(&id, &versao)
	if errors.Is(err, sql.ErrNoRows) {
		return ReabrirOut{}, &ErrNaoFechado{Competencia: comp.Format("2006-01")}
	}
	if err != nil {
		return ReabrirOut{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auditoria.dre_fechamentos
		SET reaberto_em=now(), reaberto_por=$2::uuid, motivo_reabertura=$3 WHERE id=$1`, id, in.UserID, motivo); err != nil {
		return ReabrirOut{}, err
	}
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "dre_fechamento", strconv.FormatInt(id, 10), "reabrir",
		map[string]any{"competencia": comp.Format("2006-01"), "versao": versao, "motivo": motivo}); err != nil {
		return ReabrirOut{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReabrirOut{}, err
	}
	return ReabrirOut{ID: id, Versao: versao}, nil
}

// filiaisDoSnapshot lista as filiais com lançamento na competência ou no mesmo
// mês do ano anterior, ou com orçado na competência (as que entram no snapshot
// além do consolidado).
func filiaisDoSnapshot(ctx context.Context, tx *sql.Tx, empresaID string, comp time.Time) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT filial FROM auditoria.lancamentos
			WHERE empresa_id = $1::uuid AND filial <> '' AND competencia IN ($2, $3)
		UNION
		SELECT filial FROM auditoria.dre_orcado
			WHERE empresa_id = $1::uuid AND filial <> '' AND competencia = $2
		ORDER BY 1`, empresaID, comp, comp.AddDate(-1, 0, 0))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
