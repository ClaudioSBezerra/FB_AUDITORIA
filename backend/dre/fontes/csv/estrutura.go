package csv

import (
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"fb_auditoria/dre"
)

const (
	maxCodigoLinha    = 40
	maxDescricaoLinha = 200
	maxContaMapa      = 60

	colunasEstrutura = "codigo, descricao, tipo (conta|subtotal) obrigatórias; nivel, ordem, soma_de (códigos separados por vírgula ou |) e papel opcionais"
	colunasMapa      = "conta_padrao e linha_codigo obrigatórias; sinal (1 ou -1; vazio = 1) opcional"
)

// normColuna normaliza o nome de coluna para comparar com os aliases: como
// normCab, e "_" ou "-" viram espaço (soma_de, soma de e Soma-De são a mesma).
func normColuna(h string) string {
	h = normCab(h)
	h = strings.NewReplacer("_", " ", "-", " ").Replace(h)
	return strings.TrimSpace(espacos.ReplaceAllString(h, " "))
}

// colunaEstrutura devolve o nome canônico da coluna (ou "").
func colunaEstrutura(h string) string {
	switch normColuna(h) {
	case "codigo", "cod", "chave":
		return "codigo"
	case "descricao", "nome":
		return "descricao"
	case "nivel":
		return "nivel"
	case "ordem":
		return "ordem"
	case "tipo":
		return "tipo"
	case "soma de", "somade", "soma":
		return "soma_de"
	case "papel":
		return "papel"
	}
	return ""
}

func colunaMapa(h string) string {
	switch normColuna(h) {
	case "conta padrao", "conta", "contapadrao":
		return "conta_padrao"
	case "linha codigo", "linha", "linhacodigo":
		return "linha_codigo"
	case "sinal":
		return "sinal"
	}
	return ""
}

func cabecalhoEstrutura(cels []string) bool {
	var cod, desc, tipo bool
	for _, c := range cels {
		switch colunaEstrutura(c) {
		case "codigo":
			cod = true
		case "descricao":
			desc = true
		case "tipo":
			tipo = true
		}
	}
	return cod && desc && tipo
}

func cabecalhoMapa(cels []string) bool {
	var conta, linha bool
	for _, c := range cels {
		switch colunaMapa(c) {
		case "conta_padrao":
			conta = true
		case "linha_codigo":
			linha = true
		}
	}
	return conta && linha
}

// inteiroPlanilha lê "10", "10.0" (XLSX) como inteiro; ok=false se não for inteiro.
func inteiroPlanilha(s string) (int, bool) {
	s = inteiroTexto(s)
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// leTabelaCarga lê o arquivo e devolve a tabela com a posição das colunas.
func leTabelaCarga(r io.Reader, cab func([]string) bool, canon func(string) string, esperado string) (tabela, map[string]int, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return tabela{}, nil, &dre.ErroEntrada{Msg: "arquivo ilegível"}
	}
	if len(data) == 0 {
		return tabela{}, nil, &dre.ErroEntrada{Msg: "arquivo vazio ou ilegível"}
	}
	t, err := lerTabela(data, cab, nil)
	if err != nil {
		return t, nil, err
	}
	if t.hi < 0 {
		return t, nil, &dre.ErroEntrada{Msg: "cabeçalho não reconhecido; esperado " + esperado}
	}
	col := map[string]int{}
	for i, h := range t.rows[t.hi].cels {
		if n := canon(h); n != "" {
			if _, ok := col[n]; !ok {
				col[n] = i
			}
		}
	}
	return t, col, nil
}

// separadorSomaDe divide soma_de por vírgula ou barra vertical.
func separadorSomaDe(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '|' || r == ';' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// LerEstrutura lê a estrutura do DRE de um CSV ou XLSX (colunas codigo,
// descricao, tipo e, opcionalmente, nivel, ordem, soma_de, papel). Devolve as
// linhas lidas e os problemas de leitura; a validação cruzada (referências,
// ciclos, papel) é de dre.ValidarEstrutura.
func LerEstrutura(r io.Reader) ([]dre.LinhaEstrutura, []dre.Problema, error) {
	t, col, err := leTabelaCarga(r, cabecalhoEstrutura, colunaEstrutura, colunasEstrutura)
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
	var out []dre.LinhaEstrutura
	seq := 0
	for _, r := range t.rows[t.hi+1:] {
		if vazia(r.cels) {
			continue
		}
		if !t.xlsx && len(r.cels) != len(t.rows[t.hi].cels) {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "número de colunas diferente do cabeçalho"})
			continue
		}
		seq++
		l := dre.LinhaEstrutura{
			Linha: r.n, Codigo: inteiroTexto(get(r.cels, "codigo")), Descricao: get(r.cels, "descricao"),
			Tipo: strings.ToLower(normCab(get(r.cels, "tipo"))), Nivel: 1, Ordem: seq * 10,
			SomaDe: separadorSomaDe(get(r.cels, "soma_de")), Papel: strings.ToLower(strings.TrimSpace(get(r.cels, "papel"))),
		}
		bad := func(msg string) { probs = append(probs, dre.Problema{Linha: r.n, Msg: msg}) }
		if v := get(r.cels, "nivel"); v != "" {
			n, ok := inteiroPlanilha(v)
			if !ok || n < 0 {
				bad("nivel deve ser um inteiro maior ou igual a zero")
				continue
			}
			l.Nivel = n
		}
		if v := get(r.cels, "ordem"); v != "" {
			n, ok := inteiroPlanilha(v)
			if !ok {
				bad("ordem deve ser um número inteiro")
				continue
			}
			l.Ordem = n
		}
		switch {
		case l.Codigo == "":
			bad("codigo vazio")
		case utf8.RuneCountInString(l.Codigo) > maxCodigoLinha:
			bad("codigo: no máximo 40 caracteres")
		case strings.HasPrefix(l.Codigo, "-"):
			bad("codigo não pode começar com '-' (o hífen indica subtração em soma_de)")
		case l.Descricao == "":
			bad("descricao vazia")
		case utf8.RuneCountInString(l.Descricao) > maxDescricaoLinha:
			bad("descricao: no máximo 200 caracteres")
		default:
			out = append(out, l)
		}
	}
	return out, probs, nil
}

// LerMapa lê o Mapa de contas de um CSV ou XLSX (conta_padrao, linha_codigo e,
// opcionalmente, sinal). A existência da linha é conferida por dre.CargaMapa.
func LerMapa(r io.Reader) ([]dre.ItemMapaArquivo, []dre.Problema, error) {
	t, col, err := leTabelaCarga(r, cabecalhoMapa, colunaMapa, colunasMapa)
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
	var out []dre.ItemMapaArquivo
	for _, r := range t.rows[t.hi+1:] {
		if vazia(r.cels) {
			continue
		}
		if !t.xlsx && len(r.cels) != len(t.rows[t.hi].cels) {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "número de colunas diferente do cabeçalho"})
			continue
		}
		it := dre.ItemMapaArquivo{Linha: r.n, ContaPadrao: inteiroTexto(get(r.cels, "conta_padrao")), LinhaCodigo: inteiroTexto(get(r.cels, "linha_codigo")), Sinal: 1}
		bad := func(msg string) { probs = append(probs, dre.Problema{Linha: r.n, Msg: msg}) }
		if v := get(r.cels, "sinal"); v != "" {
			switch n, ok := inteiroPlanilha(strings.TrimPrefix(v, "+")); {
			case ok && (n == 1 || n == -1):
				it.Sinal = n
			default:
				bad("sinal deve ser 1 ou -1")
				continue
			}
		}
		switch {
		case it.ContaPadrao == "":
			bad("conta_padrao vazia")
		case utf8.RuneCountInString(it.ContaPadrao) > maxContaMapa:
			bad("conta_padrao: no máximo 60 caracteres")
		case strings.Contains(strings.TrimSuffix(it.ContaPadrao, "%"), "%"):
			bad("conta_padrao: o % só vale no fim (prefixo)")
		case it.LinhaCodigo == "":
			bad("linha_codigo vazio")
		default:
			out = append(out, it)
		}
	}
	return out, probs, nil
}
