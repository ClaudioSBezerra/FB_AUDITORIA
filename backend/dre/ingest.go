package dre

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"

	"fb_auditoria/trilha"
)

// Ingestor é o único escritor de auditoria.lancamentos.
type Ingestor struct{ DB *sql.DB }

// Resultado da importação.
type Resultado struct {
	CargaID int64
	Linhas  int
}

type escopoImp struct {
	Competencia string
	Filial      string
}

// Importar lê a fonte, valida tudo antes de abrir a transação e então, numa só
// transação, grava a carga, substitui cada escopo (empresa, origem,
// competência, filial) presente nas linhas e registra a trilha. Leitura vazia
// nunca apaga nada.
func (g *Ingestor) Importar(ctx context.Context, empresaID, userID string, fonte Fonte) (Resultado, error) {
	lancs, err := fonte.Ler(ctx, Escopo{EmpresaID: empresaID})
	var probs []Problema
	if err != nil {
		var el *ErroLinhas
		if !errors.As(err, &el) {
			return Resultado{}, err
		}
		probs = append(probs, el.Problemas...)
	}
	for _, l := range lancs {
		if l.Conta == "" {
			probs = append(probs, Problema{l.Linha, "conta vazia"})
		}
		if l.Filial == "" {
			probs = append(probs, Problema{l.Linha, "filial obrigatória"})
		}
	}
	if len(probs) > 0 {
		sort.SliceStable(probs, func(i, j int) bool { return probs[i].Linha < probs[j].Linha })
		return Resultado{}, &ErroLinhas{Problemas: probs}
	}
	if len(lancs) == 0 {
		return Resultado{}, &ErroEntrada{Msg: "nenhuma linha de dados"}
	}

	origem := fonte.Nome()
	arquivo := origem
	if a, ok := fonte.(interface{ Arquivo() string }); ok {
		arquivo = a.Arquivo()
	}
	var escopos []escopoImp
	vistos := map[escopoImp]bool{}
	for _, l := range lancs {
		k := escopoImp{l.Competencia.Format("2006-01-02"), l.Filial}
		if !vistos[k] {
			vistos[k] = true
			escopos = append(escopos, k)
		}
	}

	tx, err := g.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Resultado{}, err
	}
	defer tx.Rollback()
	var cargaID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO auditoria.cargas (empresa_id, user_id, arquivo) VALUES ($1::uuid,$2::uuid,$3) RETURNING id`,
		empresaID, userID, arquivo).Scan(&cargaID); err != nil {
		return Resultado{}, err
	}
	// Serializa importações da mesma (empresa, origem): sem isso, duas cargas
	// simultâneas do mesmo escopo (READ COMMITTED) duplicariam linhas.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, empresaID+":"+origem); err != nil {
		return Resultado{}, err
	}
	type substituido struct {
		Competencia string `json:"competencia"`
		Filial      string `json:"filial"`
		Removidas   int64  `json:"removidas"`
	}
	subst := make([]substituido, 0, len(escopos))
	for _, e := range escopos {
		res, err := tx.ExecContext(ctx, `DELETE FROM auditoria.lancamentos
			WHERE empresa_id=$1::uuid AND origem=$2 AND competencia=$3 AND filial=$4`,
			empresaID, origem, e.Competencia, e.Filial)
		if err != nil {
			return Resultado{}, err
		}
		n, _ := res.RowsAffected()
		subst = append(subst, substituido{e.Competencia[:7], e.Filial, n})
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO auditoria.lancamentos
		(empresa_id, competencia, filial, centro_custo, canal, conta, conta_desc, valor, origem, tipo_doc, doc_ref, historico, carga_id)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`)
	if err != nil {
		return Resultado{}, err
	}
	defer stmt.Close()
	for _, l := range lancs {
		if _, err := stmt.ExecContext(ctx, empresaID, l.Competencia, l.Filial, l.CentroCusto, l.Canal, l.Conta, l.ContaDesc,
			l.Valor, origem, l.TipoDoc, l.DocRef, l.Historico, cargaID); err != nil {
			return Resultado{}, &ErroLinhas{Problemas: []Problema{{Linha: l.Linha, Msg: err.Error()}}}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE auditoria.cargas SET linhas=$1 WHERE id=$2`, len(lancs), cargaID); err != nil {
		return Resultado{}, err
	}
	if err := trilha.Gravar(tx, empresaID, userID, "lancamentos", strconv.FormatInt(cargaID, 10), "importar",
		map[string]any{"arquivo": arquivo, "origem": origem, "linhas": len(lancs), "escopos": subst}); err != nil {
		return Resultado{}, err
	}
	if err := tx.Commit(); err != nil {
		return Resultado{}, err
	}
	return Resultado{CargaID: cargaID, Linhas: len(lancs)}, nil
}
