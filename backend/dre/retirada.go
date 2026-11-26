package dre

// retirada.go — retirar a movimentação carregada, de um mês ou toda, para receber
// uma carga nova (história 2.23). Apaga só lançamentos e libera a posse das
// contas que ficaram sem lançamento daquela origem (AD-16). Cadastros, orçado,
// planilha do comparativo, fechamentos e trilha ficam. Mês fechado recusa tudo.

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"fb_auditoria/trilha"
)

// ItemRetirada é o que existe numa competência para uma origem.
type ItemRetirada struct {
	Competencia string `json:"competencia"`
	Origem      string `json:"origem"`
	Linhas      int    `json:"linhas"`
}

// Retirada é a prévia (ou o resultado) de uma retirada.
type Retirada struct {
	Competencia string         `json:"competencia"` // vazia = toda a movimentação
	Itens       []ItemRetirada `json:"itens"`
	Linhas      int            `json:"linhas"`
	Fechadas    []string       `json:"fechadas"` // competências com fechamento ativo
	Ambiente    string         `json:"ambiente"`
	// Confirmacao é o texto que o pedido precisa repetir.
	Confirmacao     string `json:"confirmacao"`
	ContasLiberadas int    `json:"contas_liberadas"`
}

// ConfirmacaoRetirarTudo é o texto exigido para retirar toda a movimentação.
const ConfirmacaoRetirarTudo = "RETIRAR TUDO"

func competenciaRetirada(s string) (time.Time, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, "", nil
	}
	c, err := time.Parse("2006-01", s)
	if err != nil {
		return time.Time{}, "", &ErroEntrada{Msg: "competência inválida: use AAAA-MM"}
	}
	return c, c.Format("2006-01-02"), nil
}

func levantarRetirada(ctx context.Context, tx *sql.Tx, empresaID, competencia string) (Retirada, []time.Time, error) {
	_, dia, err := competenciaRetirada(competencia)
	if err != nil {
		return Retirada{}, nil, err
	}
	r := Retirada{Competencia: strings.TrimSpace(competencia), Itens: []ItemRetirada{}, Fechadas: []string{}, Confirmacao: ConfirmacaoRetirarTudo}
	if r.Competencia != "" {
		r.Confirmacao = r.Competencia
	}
	if err := tx.QueryRowContext(ctx, `SELECT ambiente FROM companies WHERE id=$1::uuid`, empresaID).Scan(&r.Ambiente); err != nil {
		return r, nil, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT competencia, origem, COUNT(*) FROM auditoria.lancamentos
		WHERE empresa_id=$1::uuid AND ($2 = '' OR competencia = NULLIF($2,'')::date)
		GROUP BY competencia, origem ORDER BY competencia, origem`, empresaID, dia)
	if err != nil {
		return r, nil, err
	}
	var comps []time.Time
	for rows.Next() {
		var c time.Time
		var it ItemRetirada
		if err := rows.Scan(&c, &it.Origem, &it.Linhas); err != nil {
			rows.Close()
			return r, nil, err
		}
		it.Competencia = c.Format("2006-01")
		if len(comps) == 0 || !comps[len(comps)-1].Equal(c) {
			comps = append(comps, c)
		}
		r.Itens = append(r.Itens, it)
		r.Linhas += it.Linhas
	}
	err = rows.Err()
	rows.Close()
	return r, comps, err
}

// guardarRetirada anota as competências fechadas; a guarda também trava o período
// até o fim da transação.
func guardarRetirada(ctx context.Context, tx *sql.Tx, empresaID string, comps []time.Time, r *Retirada) error {
	var recusados []EscopoFechado
	for _, c := range comps {
		if err := (Periodo{}).Guardar(ctx, tx, empresaID, c); err != nil {
			if err := acumularFechado(err, &recusados); err != nil {
				return err
			}
		}
	}
	for _, e := range recusados {
		r.Fechadas = append(r.Fechadas, e.Competencia)
	}
	return nil
}

// PreverRetirada diz o que seria apagado, sem gravar nada.
func (g *Ingestor) PreverRetirada(ctx context.Context, empresaID, competencia string) (Retirada, error) {
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return Retirada{}, err
	}
	defer tx.Rollback()
	r, comps, err := levantarRetirada(ctx, tx, empresaID, competencia)
	if err != nil {
		return r, err
	}
	return r, guardarRetirada(ctx, tx, empresaID, comps, &r)
}

// Retirar apaga a movimentação do mês (ou toda, só em ambiente de teste), libera
// as contas e grava a trilha, tudo numa transação.
func (g *Ingestor) Retirar(ctx context.Context, empresaID, userID, competencia, confirmacao string) (Retirada, error) {
	tx, err := g.DB.BeginTx(ctx, nil)
	if err != nil {
		return Retirada{}, err
	}
	defer tx.Rollback()
	// A mesma chave da importação do banco do cliente: não retirar no meio dela.
	var livre bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1))`, "importacao_erp:"+empresaID).Scan(&livre); err != nil {
		return Retirada{}, err
	}
	if !livre {
		return Retirada{}, ErrImportacaoEmAndamento
	}
	r, comps, err := levantarRetirada(ctx, tx, empresaID, competencia)
	if err != nil {
		return r, err
	}
	if strings.TrimSpace(confirmacao) != r.Confirmacao {
		return r, &ErroEntrada{Msg: "confirmação não confere: digite " + r.Confirmacao}
	}
	if r.Competencia == "" && r.Ambiente != "teste" {
		return r, &ErroEntrada{Msg: "retirar toda a movimentação só é permitido em ambiente de teste; informe o mês"}
	}
	if err := guardarRetirada(ctx, tx, empresaID, comps, &r); err != nil {
		return r, err
	}
	if len(r.Fechadas) > 0 {
		var e []EscopoFechado
		for _, c := range r.Fechadas {
			e = append(e, EscopoFechado{Competencia: c})
		}
		return r, &ErrPeriodoFechado{Escopos: e}
	}
	for _, c := range comps {
		if err := (Periodo{}).Guardar(ctx, tx, empresaID, c); err != nil {
			return r, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM auditoria.lancamentos WHERE empresa_id=$1::uuid AND competencia=$2`, empresaID, c); err != nil {
			return r, err
		}
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM auditoria.conta_fonte f WHERE f.empresa_id=$1::uuid
		AND NOT EXISTS (SELECT 1 FROM auditoria.lancamentos l
			WHERE l.empresa_id=f.empresa_id AND l.conta=f.conta_padrao AND l.origem=f.origem)`, empresaID)
	if err != nil {
		return r, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return r, err
	}
	r.ContasLiberadas = int(n)
	alvo := r.Competencia
	if alvo == "" {
		alvo = "tudo"
	}
	if err := trilha.Gravar(tx, empresaID, userID, "lancamentos", alvo, "retirar",
		map[string]any{"itens": r.Itens, "linhas": r.Linhas, "contas_liberadas": r.ContasLiberadas}); err != nil {
		return r, err
	}
	return r, tx.Commit()
}
