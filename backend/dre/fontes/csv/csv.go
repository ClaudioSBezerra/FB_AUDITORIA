// Package csv é a fonte de lançamentos por arquivo CSV. Não acessa banco.
//
// Cabeçalho aceito (qualquer ordem, ';' ou ','): competencia, filial, conta e
// valor são obrigatórias; centro_custo, canal, conta_desc, tipo_doc, doc_ref e
// historico são opcionais.
// competencia: AAAA-MM ou AAAA-MM-DD. valor: 1234.56 ou 1.234,56.
package csv

import (
	"bufio"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"fb_auditoria/dre"
)

// Fonte lê um CSV de um io.Reader.
type Fonte struct {
	r       io.Reader
	arquivo string
}

// Nova cria a fonte; arquivo é só o nome para a trilha.
func Nova(r io.Reader, arquivo string) *Fonte { return &Fonte{r: r, arquivo: arquivo} }

func (f *Fonte) Nome() string    { return "csv" }
func (f *Fonte) Arquivo() string { return f.arquivo }

func parseValorBR(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err == nil && (math.IsNaN(v) || math.IsInf(v, 0)) {
		return 0, fmt.Errorf("valor não finito")
	}
	return v, err
}

func parseCompetencia(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 7 {
		if t, err := time.Parse("2006-01", s[:7]); err == nil {
			return t, nil
		}
	}
	return time.Parse("02/01/2006", "01/"+s) // MM/AAAA
}

// Ler devolve as linhas válidas; linhas com problema vêm num *dre.ErroLinhas.
func (f *Fonte) Ler(ctx context.Context, _ dre.Escopo) ([]dre.Lancamento, error) {
	br := bufio.NewReader(f.r)
	first, _ := br.Peek(2048)
	cr := csv.NewReader(br)
	cr.Comma = ';'
	if strings.Count(string(first), ",") > strings.Count(string(first), ";") {
		cr.Comma = ','
	}
	cr.LazyQuotes = true
	head, err := cr.Read()
	if err != nil {
		return nil, &dre.ErroEntrada{Msg: "CSV vazio ou ilegível"}
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, string(rune(0xFEFF)))))] = i
	}
	for _, req := range []string{"competencia", "filial", "conta", "valor"} {
		if _, ok := col[req]; !ok {
			return nil, &dre.ErroEntrada{Msg: "coluna obrigatória ausente: " + req}
		}
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	var out []dre.Lancamento
	var probs []dre.Problema
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			linha := 0
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				linha = pe.Line
			}
			probs = append(probs, dre.Problema{Linha: linha, Msg: fmt.Sprint(err)})
			continue
		}
		line, _ := cr.FieldPos(0)
		comp, err := parseCompetencia(get(rec, "competencia"))
		if err != nil {
			probs = append(probs, dre.Problema{Linha: line, Msg: "competencia inválida"})
			continue
		}
		conta := get(rec, "conta")
		valor, err := parseValorBR(get(rec, "valor"))
		if conta == "" || err != nil {
			probs = append(probs, dre.Problema{Linha: line, Msg: "conta/valor inválidos"})
			continue
		}
		out = append(out, dre.Lancamento{
			Linha: line, Competencia: comp, Filial: get(rec, "filial"),
			CentroCusto: get(rec, "centro_custo"), Canal: get(rec, "canal"),
			Conta: conta, ContaDesc: get(rec, "conta_desc"), Valor: valor,
			TipoDoc: get(rec, "tipo_doc"), DocRef: get(rec, "doc_ref"), Historico: get(rec, "historico"),
		})
	}
	if len(probs) > 0 {
		return out, &dre.ErroLinhas{Problemas: probs}
	}
	return out, nil
}
