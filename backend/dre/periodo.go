package dre

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// EscopoFechado identifica um fechamento que recusa a escrita.
type EscopoFechado struct {
	Competencia string // AAAA-MM
	Filial      string // vazio = grupo inteiro
}

// ErrPeriodoFechado recusa escrita em competência com fechamento (FR-11).
type ErrPeriodoFechado struct{ Escopos []EscopoFechado }

func (e *ErrPeriodoFechado) Error() string {
	partes := make([]string, 0, len(e.Escopos))
	for _, s := range e.Escopos {
		if s.Filial == "" {
			partes = append(partes, s.Competencia+" (todas as filiais)")
		} else {
			partes = append(partes, s.Competencia+" (filial "+s.Filial+")")
		}
	}
	return "Competência fechada: " + strings.Join(partes, ", ") + " — é preciso reabrir o fechamento antes de gravar"
}

// ChaveLock é a única função de chave do advisory lock por (empresa,
// competência): hash estável (FNV-1a 64) do mês de calendário.
func ChaveLock(empresa string, competencia time.Time) int64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%s", empresa, competencia.Format("2006-01"))
	return int64(h.Sum64())
}

// Trava toma o lock de transação da (empresa, competência); solta no
// Commit/Rollback. Quem escreve na competência (Guardar, Fechar) passa por aqui.
func travar(ctx context.Context, tx *sql.Tx, empresa string, competencia time.Time) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, ChaveLock(empresa, competencia))
	return err
}

// Guardar toma o lock da (empresa, competência) e só então confere se existe
// fechamento ativo (não reaberto; de qualquer filial); se existir, devolve *ErrPeriodoFechado.
// Lock primeiro, conferência depois: em READ COMMITTED o SELECT novo enxerga o
// Commit de um Fechar que acabou de soltar o lock. Método de Periodo apenas por
// nome (AD-8); não usa os campos Ini/Fim.
func (Periodo) Guardar(ctx context.Context, tx *sql.Tx, empresa string, competencia time.Time) error {
	competencia = time.Date(competencia.Year(), competencia.Month(), 1, 0, 0, 0, 0, time.UTC)
	if err := travar(ctx, tx, empresa, competencia); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT COALESCE(filial,'') AS filial FROM auditoria.dre_fechamentos
		WHERE empresa_id=$1::uuid AND competencia=$2 AND reaberto_em IS NULL ORDER BY 1`, empresa, competencia.Format("2006-01-02"))
	if err != nil {
		return err
	}
	defer rows.Close()
	var esc []EscopoFechado
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return err
		}
		esc = append(esc, EscopoFechado{Competencia: competencia.Format("2006-01"), Filial: f})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(esc) > 0 {
		return &ErrPeriodoFechado{Escopos: esc}
	}
	return nil
}
