package csv

import (
	"io"
	"strings"
	"unicode/utf8"

	"fb_auditoria/dre"
)

// Limites de tamanho das células do dicionário.
const (
	maxConta     = 60
	maxDescricao = 200
	maxGrupoDic  = 40
)

const colunasDicionario = "conta, grupo (obrigatórias); descricao e departamento (código do departamento) opcionais"

// cabecalhoDicionario: a linha traz conta e grupo.
func cabecalhoDicionario(cels []string) bool {
	var conta, grupo bool
	for _, c := range cels {
		switch normCab(c) {
		case "conta":
			conta = true
		case "grupo":
			grupo = true
		}
	}
	return conta && grupo
}

// inteiroTexto tira o ".0" que a planilha põe em número inteiro (402.0 vira 402).
func inteiroTexto(s string) string {
	s = strings.TrimSpace(s)
	if m := inteiroComZ.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// LerDicionario lê o dicionário de contas de um CSV ou XLSX (colunas conta,
// grupo, descricao e, opcionalmente, departamento). Devolve as linhas válidas e
// os problemas de leitura; a conferência contra o banco é de dre.Dicionario.
func LerDicionario(r io.Reader) ([]dre.ContaDic, []dre.Problema, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, &dre.ErroEntrada{Msg: "arquivo ilegível"}
	}
	if len(data) == 0 {
		return nil, nil, &dre.ErroEntrada{Msg: "arquivo vazio ou ilegível"}
	}
	t, err := lerTabela(data, cabecalhoDicionario, nil)
	if err != nil {
		return nil, nil, err
	}
	if t.hi < 0 {
		return nil, nil, &dre.ErroEntrada{Msg: "cabeçalho não reconhecido; esperado " + colunasDicionario}
	}
	col := map[string]int{}
	for i, h := range t.rows[t.hi].cels {
		if n := normCab(h); n != "" {
			if _, ok := col[n]; !ok {
				col[n] = i
			}
		}
	}
	get := func(rec []string, nomes ...string) string {
		for _, n := range nomes {
			if i, ok := col[n]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
		}
		return ""
	}
	probs := t.probs
	var out []dre.ContaDic
	for _, r := range t.rows[t.hi+1:] {
		if vazia(r.cels) {
			continue
		}
		if !t.xlsx && len(r.cels) != len(t.rows[t.hi].cels) {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "número de colunas diferente do cabeçalho"})
			continue
		}
		c := dre.ContaDic{
			Linha: r.n, Conta: get(r.cels, "conta"), Descricao: get(r.cels, "descricao"),
			Grupo: inteiroTexto(get(r.cels, "grupo")), Departamento: inteiroTexto(get(r.cels, "departamento", "codigo departamento")),
		}
		switch {
		case c.Conta == "":
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "conta vazia"})
		case utf8.RuneCountInString(c.Conta) > maxConta:
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "conta: no máximo 60 caracteres"})
		case utf8.RuneCountInString(c.Descricao) > maxDescricao:
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "descrição: no máximo 200 caracteres"})
		case utf8.RuneCountInString(c.Grupo) > maxGrupoDic || utf8.RuneCountInString(c.Departamento) > maxGrupoDic:
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "grupo e departamento: no máximo 40 caracteres"})
		default:
			out = append(out, c)
		}
	}
	return out, probs, nil
}
