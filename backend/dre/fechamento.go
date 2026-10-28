package dre

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"fb_auditoria/trilha"
)

// Fechamento congela o realizado de um mês numa nova versão.
type Fechamento struct{ DB *sql.DB }

// FecharIn é a entrada de Fechar.
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

const maxContasListadas = 20

// Fechar calcula o snapshot dentro da transação (via Realizado), grava o
// fechamento, as linhas e a trilha no mesmo Commit. Tudo ou nada.
func (f *Fechamento) Fechar(ctx context.Context, in FecharIn) (FecharOut, error) {
	if in.Mes < 1 || in.Mes > 12 || in.Ano < 2000 || in.Ano > 2100 {
		return FecharOut{}, &ErroEntrada{Msg: "ano/mes inválidos"}
	}
	comp := time.Date(in.Ano, time.Month(in.Mes), 1, 0, 0, 0, 0, time.UTC)
	tx, err := f.DB.BeginTx(ctx, nil)
	if err != nil {
		return FecharOut{}, err
	}
	defer tx.Rollback()

	linhas, err := CarregarLinhas(ctx, tx, in.EmpresaID)
	if err != nil {
		return FecharOut{}, err
	}
	mapa, err := CarregarMapa(ctx, tx, in.EmpresaID)
	if err != nil {
		return FecharOut{}, err
	}
	real, sem, err := Realizado(ctx, tx, in.EmpresaID, in.Filial, Periodo{Ini: comp, Fim: comp}, mapa)
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

	var versao int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(versao),0)+1 FROM auditoria.dre_fechamentos
		WHERE empresa_id=$1::uuid AND competencia=$2 AND filial=$3`, in.EmpresaID, comp, in.Filial).Scan(&versao); err != nil {
		return FecharOut{}, err
	}
	var id int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por, observacao)
		VALUES ($1::uuid,$2,$3,$4,$5::uuid,$6) RETURNING id`, in.EmpresaID, comp, versao, in.Filial, in.UserID, in.Observacao).Scan(&id); err != nil {
		return FecharOut{}, err
	}
	for _, l := range linhas {
		if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.dre_fechamento_linhas (fechamento_id, linha_codigo, descricao, nivel, ordem, valor)
			VALUES ($1,$2,$3,$4,$5,$6)`, id, l.Codigo, l.Descricao, l.Nivel, l.Ordem, real[l.Codigo]); err != nil {
			return FecharOut{}, err
		}
	}
	if err := trilha.Gravar(tx, in.EmpresaID, in.UserID, "dre_fechamento", strconv.FormatInt(id, 10), "fechar",
		map[string]any{"competencia": comp.Format("2006-01"), "versao": versao, "filial": in.Filial}); err != nil {
		return FecharOut{}, err
	}
	if err := tx.Commit(); err != nil {
		return FecharOut{}, err
	}
	return FecharOut{ID: id, Versao: versao}, nil
}
