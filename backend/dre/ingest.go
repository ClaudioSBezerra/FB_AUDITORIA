package dre

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"time"

	"fb_auditoria/trilha"
)

// Ingestor é o único escritor de auditoria.lancamentos.
type Ingestor struct {
	DB *sql.DB
	// Agora devolve a data de hoje; nil usa time.Now. Só os avisos de
	// plausibilidade de competência dependem dela (testes a fixam).
	Agora func() time.Time
}

// Resultado da importação. Resumo é o que foi lido do arquivo (história 2.19).
type Resultado struct {
	CargaID int64
	Linhas  int
	Resumo  Resumo
}

type escopoImp struct {
	Competencia string
	Filial      string
}

// entradaLida é o arquivo já lido e validado, pronto para simular ou gravar.
type entradaLida struct {
	lancs   []Lancamento
	origem  string
	arquivo string
}

// ler lê a fonte e valida tudo antes de qualquer transação: linhas com problema
// e arquivo sem linhas são recusados aqui.
func (g *Ingestor) ler(ctx context.Context, empresaID string, fonte Fonte) (entradaLida, error) {
	lancs, err := fonte.Ler(ctx, Escopo{EmpresaID: empresaID})
	var probs []Problema
	if err != nil {
		var el *ErroLinhas
		if !errors.As(err, &el) {
			return entradaLida{}, err
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
		if l.Competencia.IsZero() {
			probs = append(probs, Problema{l.Linha, "competência ausente"})
		}
	}
	if len(probs) > 0 {
		sort.SliceStable(probs, func(i, j int) bool { return probs[i].Linha < probs[j].Linha })
		return entradaLida{}, &ErroLinhas{Problemas: probs}
	}
	if len(lancs) == 0 {
		return entradaLida{}, &ErroEntrada{Msg: "nenhuma linha de dados"}
	}
	origem := fonte.Nome()
	arquivo := origem
	if a, ok := fonte.(interface{ Arquivo() string }); ok {
		arquivo = a.Arquivo()
	}
	return entradaLida{lancs, origem, arquivo}, nil
}

// competenciasOrdenadas devolve os meses das linhas, sem repetição, em ordem
// crescente (a ordem em que a guarda trava, para não haver deadlock).
func competenciasOrdenadas(lancs []Lancamento) []time.Time {
	comps := map[string]time.Time{}
	for _, l := range lancs {
		c := time.Date(l.Competencia.Year(), l.Competencia.Month(), 1, 0, 0, 0, 0, time.UTC)
		comps[c.Format("2006-01-02")] = c
	}
	ordem := make([]time.Time, 0, len(comps))
	for _, c := range comps {
		ordem = append(ordem, c)
	}
	sort.Slice(ordem, func(i, j int) bool { return ordem[i].Before(ordem[j]) })
	return ordem
}

// acumularFechado junta o escopo fechado de um erro da guarda; devolve o erro
// original se não for *ErrPeriodoFechado (falha de verdade).
func acumularFechado(err error, recusados *[]EscopoFechado) error {
	var pf *ErrPeriodoFechado
	if !errors.As(err, &pf) {
		return err
	}
	*recusados = append(*recusados, pf.Escopos...)
	return nil
}

// Simular lê e valida o arquivo, aplica a guarda de período e devolve o resumo
// e os avisos SEM gravar nada: a transação é só de leitura e termina em
// rollback, sem carga e sem trilha. Mês fechado volta como *ErrPeriodoFechado,
// como na gravação.
func (g *Ingestor) Simular(ctx context.Context, empresaID string, fonte Fonte) (Resumo, error) {
	e, err := g.ler(ctx, empresaID, fonte)
	if err != nil {
		return Resumo{}, err
	}
	tx, err := g.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Resumo{}, err
	}
	defer tx.Rollback()
	var recusados []EscopoFechado
	for _, c := range competenciasOrdenadas(e.lancs) {
		if err := (Periodo{}).Guardar(ctx, tx, empresaID, c); err != nil {
			if err := acumularFechado(err, &recusados); err != nil {
				return Resumo{}, err
			}
		}
	}
	if len(recusados) > 0 {
		return Resumo{}, &ErrPeriodoFechado{Escopos: recusados}
	}
	return g.montarResumo(ctx, tx, empresaID, e.origem, e.arquivo, e.lancs)
}

// Importar lê a fonte, valida tudo antes de abrir a transação e então, numa só
// transação, grava a carga, substitui cada escopo (empresa, origem,
// competência, filial) presente nas linhas e registra a trilha. Leitura vazia
// nunca apaga nada. O resumo e os avisos (história 2.19) vão em Resultado.Resumo.
func (g *Ingestor) Importar(ctx context.Context, empresaID, userID string, fonte Fonte) (Resultado, error) {
	e, err := g.ler(ctx, empresaID, fonte)
	if err != nil {
		return Resultado{}, err
	}
	lancs, origem, arquivo := e.lancs, e.origem, e.arquivo
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
	// Guarda de período (AD-8), antes de qualquer escrita.
	var recusados []EscopoFechado
	for _, c := range competenciasOrdenadas(lancs) {
		if err := (Periodo{}).Guardar(ctx, tx, empresaID, c); err != nil {
			if err := acumularFechado(err, &recusados); err != nil {
				return Resultado{}, err
			}
		}
	}
	if len(recusados) > 0 {
		return Resultado{}, &ErrPeriodoFechado{Escopos: recusados}
	}
	// Resumo e avisos antes de substituir os escopos (a conferência de outras
	// origens e de filial nova lê o estado anterior à carga).
	resumo, err := g.montarResumo(ctx, tx, empresaID, origem, arquivo, lancs)
	if err != nil {
		return Resultado{}, err
	}
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
	return Resultado{CargaID: cargaID, Linhas: len(lancs), Resumo: resumo}, nil
}
