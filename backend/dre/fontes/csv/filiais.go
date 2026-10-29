package csv

import (
	"io"
	"strings"

	"fb_auditoria/dre"
)

const (
	colunasMapaFiliais  = "filial_gerencial, filial_erp (e, opcionalmente, nome)"
	colunasAgrupamentos = "agrupamento, filial (e, opcionalmente, nome)"
)

func colunaMapaFilial(h string) string {
	switch normColuna(h) {
	case "filial gerencial", "gerencial", "codigo gerencial":
		return "gerencial"
	case "filial erp", "erp", "filial do erp":
		return "erp"
	case "nome":
		return "nome"
	}
	return ""
}

func cabecalhoMapaFiliais(cels []string) bool {
	var g, e bool
	for _, c := range cels {
		switch colunaMapaFilial(c) {
		case "gerencial":
			g = true
		case "erp":
			e = true
		}
	}
	return g && e
}

// LerMapaFiliais lê o Mapa de filiais de um CSV ou XLSX (colunas filial_gerencial
// e filial_erp, uma linha por filial do ERP; nome é opcional). Os códigos de
// filial perdem os zeros à esquerda ("01" = "1"), como na ingestão. Devolve as
// linhas válidas e os problemas de leitura; a conferência contra o banco é de
// dre.Filiais.CarregarMapa.
func LerMapaFiliais(r io.Reader) ([]dre.FilialMapaArquivo, []dre.Problema, error) {
	t, col, err := leTabelaCarga(r, cabecalhoMapaFiliais, colunaMapaFilial, colunasMapaFiliais)
	if err != nil {
		return nil, nil, err
	}
	get := celulaDe(col)
	probs := t.probs
	var out []dre.FilialMapaArquivo
	for _, r := range t.rows[t.hi+1:] {
		if vazia(r.cels) {
			continue
		}
		if !t.xlsx && len(r.cels) != len(t.rows[t.hi].cels) {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "número de colunas diferente do cabeçalho"})
			continue
		}
		l := dre.FilialMapaArquivo{Linha: r.n, Gerencial: normFilial(get(r.cels, "gerencial")),
			FilialERP: normFilial(get(r.cels, "erp")), Nome: get(r.cels, "nome")}
		switch {
		case l.Gerencial == "":
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "filial_gerencial vazia"})
		case l.FilialERP == "":
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "filial_erp vazia"})
		default:
			out = append(out, l)
		}
	}
	return out, probs, nil
}

func colunaAgrupamento(h string) string {
	switch normColuna(h) {
	case "agrupamento", "codigo", "codigo agrupamento":
		return "codigo"
	case "filial", "filial gerencial", "gerencial":
		return "filial"
	case "nome":
		return "nome"
	}
	return ""
}

func cabecalhoAgrupamentos(cels []string) bool {
	var a, f bool
	for _, c := range cels {
		switch colunaAgrupamento(c) {
		case "codigo":
			a = true
		case "filial":
			f = true
		}
	}
	return a && f
}

// LerAgrupamentos lê os agrupamentos de um CSV ou XLSX (colunas agrupamento e
// filial, uma linha por filial gerencial membro; nome é opcional). O código do
// agrupamento é gravado como veio; o da filial perde os zeros à esquerda.
func LerAgrupamentos(r io.Reader) ([]dre.MembroAgrupamentoArquivo, []dre.Problema, error) {
	t, col, err := leTabelaCarga(r, cabecalhoAgrupamentos, colunaAgrupamento, colunasAgrupamentos)
	if err != nil {
		return nil, nil, err
	}
	get := celulaDe(col)
	probs := t.probs
	var out []dre.MembroAgrupamentoArquivo
	for _, r := range t.rows[t.hi+1:] {
		if vazia(r.cels) {
			continue
		}
		if !t.xlsx && len(r.cels) != len(t.rows[t.hi].cels) {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "número de colunas diferente do cabeçalho"})
			continue
		}
		l := dre.MembroAgrupamentoArquivo{Linha: r.n, Codigo: get(r.cels, "codigo"),
			Filial: normFilial(get(r.cels, "filial")), Nome: get(r.cels, "nome")}
		switch {
		case l.Codigo == "":
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "agrupamento vazio"})
		case l.Filial == "":
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "filial vazia"})
		default:
			out = append(out, l)
		}
	}
	return out, probs, nil
}

func celulaDe(col map[string]int) func([]string, string) string {
	return func(rec []string, nome string) string {
		if i, ok := col[nome]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
}
