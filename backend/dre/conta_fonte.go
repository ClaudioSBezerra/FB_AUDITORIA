package dre

// conta_fonte.go — AD-16, uma conta, uma fonte (histórias 2.1 e 2.4). A primeira
// origem que grava uma conta passa a ser a dona; outra origem é recusada. Só o
// Ingestor lê e escreve em auditoria.conta_fonte. Trocar a dona é a história 2.8.

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/lib/pq"

	"fb_auditoria/trilha"
)

// ContaDona é uma conta recusada porque pertence a outra origem.
type ContaDona struct{ Conta, Origem string }

// ErrContaDeOutraFonte recusa a carga inteira: há contas de outra fonte.
type ErrContaDeOutraFonte struct {
	Origem string // origem que tentou gravar
	Contas []ContaDona
}

func (e *ErrContaDeOutraFonte) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d conta(s) pertencem a outra fonte; nada foi gravado (origem desta carga: %s)", len(e.Contas), e.Origem)
	for i, c := range e.Contas {
		if i == maxProblemasListados {
			fmt.Fprintf(&b, "\n... e mais %d", len(e.Contas)-maxProblemasListados)
			break
		}
		fmt.Fprintf(&b, "\nconta %s: pertence à fonte %s", c.Conta, c.Origem)
	}
	return b.String()
}

// ErrDadosDeOutraOrigem recusa a importação do ERP numa competência que já tem,
// para as mesmas contas e em QUALQUER filial, lançamentos de outra origem: somar
// os dois dobraria o valor (um arquivo pode trazer o consolidado numa só filial).
// Nada é apagado.
type ErrDadosDeOutraOrigem struct {
	Competencia string
	Origens     []string
	Contas      []string
}

func (e *ErrDadosDeOutraOrigem) Error() string {
	ex := e.Contas
	if len(ex) > 8 {
		ex = ex[:8]
	}
	return fmt.Sprintf("%s já tem lançamentos de %s em %d conta(s) que o banco do cliente também traz (ex.: %s); importar somaria em dobro. Nada foi gravado: é preciso retirar a carga anterior desse mês antes",
		e.Competencia, strings.Join(e.Origens, ", "), len(e.Contas), strings.Join(ex, ", "))
}

// contasDe devolve as contas distintas das linhas, ordenadas (ordem fixa de
// inserção evita deadlock entre cargas simultâneas).
func contasDe(lancs []Lancamento) []string {
	vistas := map[string]bool{}
	var out []string
	for _, l := range lancs {
		if !vistas[l.Conta] {
			vistas[l.Conta] = true
			out = append(out, l.Conta)
		}
	}
	sort.Strings(out)
	return out
}

// conferirDonas recusa se alguma conta pertence a outra origem. Só leitura.
func conferirDonas(ctx context.Context, q Consultador, empresaID, origem string, contas []string) error {
	rows, err := q.QueryContext(ctx, `SELECT conta_padrao, origem FROM auditoria.conta_fonte
		WHERE empresa_id=$1::uuid AND conta_padrao = ANY($2) AND origem <> $3 ORDER BY conta_padrao`,
		empresaID, pq.Array(contas), origem)
	if err != nil {
		return err
	}
	defer rows.Close()
	var donas []ContaDona
	for rows.Next() {
		var c ContaDona
		if err := rows.Scan(&c.Conta, &c.Origem); err != nil {
			return err
		}
		donas = append(donas, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(donas) > 0 {
		return &ErrContaDeOutraFonte{Origem: origem, Contas: donas}
	}
	return nil
}

// registrarDonas dá dona às contas ainda sem dona e depois confere todas, na
// transação da carga: se outra carga ganhou a corrida, esta é recusada e o
// rollback desfaz o registro. A posse nova vai à trilha.
func registrarDonas(ctx context.Context, tx *sql.Tx, empresaID, userID, origem string, contas []string) error {
	rows, err := tx.QueryContext(ctx, `INSERT INTO auditoria.conta_fonte (empresa_id, conta_padrao, origem)
		SELECT $1::uuid, c, $2 FROM unnest($3::text[]) AS c
		ON CONFLICT (empresa_id, conta_padrao) DO NOTHING RETURNING conta_padrao`, empresaID, origem, pq.Array(contas))
	if err != nil {
		return err
	}
	var novas []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			rows.Close()
			return err
		}
		novas = append(novas, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if err := conferirDonas(ctx, tx, empresaID, origem, contas); err != nil {
		return err
	}
	if len(novas) == 0 {
		return nil
	}
	sort.Strings(novas)
	return trilha.Gravar(tx, empresaID, userID, "conta_fonte", origem, "registrar",
		map[string]any{"origem": origem, "quantidade": len(novas), "contas": novas})
}

// conferirOutraOrigem recusa a fonte do ERP quando a competência já tem as mesmas
// contas vindas de uma origem que não é do ERP.
func conferirOutraOrigem(ctx context.Context, q Consultador, empresaID, competencia string, contas []string) error {
	rows, err := q.QueryContext(ctx, `SELECT DISTINCT origem, conta FROM auditoria.lancamentos
		WHERE empresa_id=$1::uuid AND competencia=$2 AND conta = ANY($3) AND origem NOT LIKE $4
		ORDER BY conta, origem`, empresaID, competencia, pq.Array(contas), PrefixoOrigemERP+"%")
	if err != nil {
		return err
	}
	defer rows.Close()
	origens, cs := map[string]bool{}, map[string]bool{}
	out := &ErrDadosDeOutraOrigem{Competencia: competencia[:7]}
	for rows.Next() {
		var o, c string
		if err := rows.Scan(&o, &c); err != nil {
			return err
		}
		if !origens[o] {
			origens[o] = true
			out.Origens = append(out.Origens, o)
		}
		if !cs[c] {
			cs[c] = true
			out.Contas = append(out.Contas, c)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(out.Contas) > 0 {
		sort.Strings(out.Origens)
		return out
	}
	return nil
}
