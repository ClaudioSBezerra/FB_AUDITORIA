package csv

import (
	"io"
	"strings"
	"unicode/utf8"

	"fb_auditoria/dre"
)

// Limites iguais aos do cadastro manual (dre.SalvarDepartamento).
const (
	maxCodigoDepto = 40
	maxNomeDepto   = 120

	colunasDepartamentos = "codigo, nome, grupo (Comercial, Logística ou Administrativo), todas obrigatórias"
)

func colunaDepartamento(h string) string {
	switch normColuna(h) {
	case "codigo", "cod":
		return "codigo"
	case "nome", "descricao":
		return "nome"
	case "grupo":
		return "grupo"
	}
	return ""
}

func cabecalhoDepartamentos(cels []string) bool {
	var cod, nome, grupo bool
	for _, c := range cels {
		switch colunaDepartamento(c) {
		case "codigo":
			cod = true
		case "nome":
			nome = true
		case "grupo":
			grupo = true
		}
	}
	return cod && nome && grupo
}

// grupoCanonico aceita o grupo sem acento e em qualquer caixa ("logistica") e
// devolve o nome gravado ("Logística"); ok=false se não for um dos grupos.
func grupoCanonico(s string) (string, bool) {
	n := normCab(s)
	for _, g := range dre.GruposDepartamento {
		if normCab(g) == n {
			return g, true
		}
	}
	return "", false
}

// LerDepartamentos lê os departamentos de um CSV ou XLSX (colunas codigo, nome e
// grupo). Devolve as linhas válidas e os problemas de leitura; código repetido no
// arquivo e a conferência contra o banco são de dre.Dicionario.CarregarDepartamentos.
func LerDepartamentos(r io.Reader) ([]dre.DepartamentoArquivo, []dre.Problema, error) {
	t, col, err := leTabelaCarga(r, cabecalhoDepartamentos, colunaDepartamento, colunasDepartamentos)
	if err != nil {
		return nil, nil, err
	}
	get := func(rec []string, nome string) string {
		if i, ok := col[nome]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}
	probs := t.probs
	var out []dre.DepartamentoArquivo
	for _, r := range t.rows[t.hi+1:] {
		if vazia(r.cels) {
			continue
		}
		if !t.xlsx && len(r.cels) != len(t.rows[t.hi].cels) {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "número de colunas diferente do cabeçalho"})
			continue
		}
		d := dre.DepartamentoArquivo{Linha: r.n, Codigo: inteiroTexto(get(r.cels, "codigo")), Nome: get(r.cels, "nome")}
		grupo, grupoOK := grupoCanonico(get(r.cels, "grupo"))
		bad := func(msg string) { probs = append(probs, dre.Problema{Linha: r.n, Msg: msg}) }
		switch {
		case d.Codigo == "":
			bad("codigo vazio")
		case utf8.RuneCountInString(d.Codigo) > maxCodigoDepto:
			bad("codigo: no máximo 40 caracteres")
		case d.Nome == "":
			bad("nome vazio")
		case utf8.RuneCountInString(d.Nome) > maxNomeDepto:
			bad("nome: no máximo 120 caracteres")
		case !grupoOK:
			bad("grupo deve ser Comercial, Logística ou Administrativo")
		default:
			d.Grupo = grupo
			out = append(out, d)
		}
	}
	return out, probs, nil
}
