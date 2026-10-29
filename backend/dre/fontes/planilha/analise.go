// Package planilha lê a planilha de análise do cliente (história 2.18): as abas
// `Real_AA` (lançamentos do ano, uma linha por conta, mês e loja) e `MENSAL`
// (o DRE do ano numa visão, com os valores já calculados). A leitura é em fluxo
// (linha a linha), porque as abas Real_AA têm centenas de milhares de linhas, e
// só as duas abas necessárias são abertas. Esta camada não acessa o banco.
package planilha

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"fb_auditoria/dre"
)

const (
	abaMensal = "MENSAL"
	// Limites de descompactação: a pasta de trabalho de análise é grande (a aba
	// Real_AA, descompactada, passa de centenas de MB); XML acima do limite de
	// memória é lido de arquivo temporário pelo excelize.
	limiteDescompactado = 4 << 30
	limiteXMLMemoria    = 64 << 20

	primeiraLinhaDRE = 5 // MENSAL: as linhas do DRE começam na linha 5
)

var (
	soDigitos   = regexp.MustCompile(`^\d+$`)
	inteiroComZ = regexp.MustCompile(`^(\d+)[.,]0+$`)
)

// Analise é o conteúdo relevante da planilha.
type Analise struct {
	Ano       int    // ano das contas (o pedido, ou o da MENSAL)
	AnoMensal int    // ano da MENSAL!C2
	Visao     string // MENSAL!C3
	Filial    string // filial resolvida (MENSAL!D1); vazio se não resolvida
	Contas    []dre.ContaExterna
	Linhas    []dre.LinhaExterna
	Ignoradas int // linhas da aba Real descartadas (sem conta, sem mês, mês de outro ano ou valor inválido)
	Avisos    []string
}

func entrada(msg string) error { return &dre.ErroEntrada{Msg: msg} }

// normNumero: número de célula guardado como decimal (123.0) vira "123".
func normNumero(s string) string {
	s = strings.TrimSpace(s)
	if m := inteiroComZ.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return s
}

// normFilial: só dígitos perde os zeros à esquerda, como a ingestão.
func normFilial(s string) string {
	s = normNumero(s)
	if soDigitos.MatchString(s) {
		if s = strings.TrimLeft(s, "0"); s == "" {
			s = "0"
		}
	}
	return s
}

func numero(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if strings.Contains(s, ",") && !strings.Contains(s, ".") {
		s = strings.ReplaceAll(s, ",", ".")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

func celula(c []string, i int) string {
	if i < len(c) {
		return strings.TrimSpace(c[i])
	}
	return ""
}

// nomeAba acha a aba pelo nome, sem diferenciar maiúsculas.
func nomeAba(x *excelize.File, nome string) (string, bool) {
	for _, n := range x.GetSheetList() {
		if strings.EqualFold(strings.TrimSpace(n), nome) {
			return n, true
		}
	}
	return "", false
}

// percorrer lê a aba em fluxo; n é o número da linha (as linhas ausentes no XML
// chegam vazias). f devolve false para parar.
func percorrer(x *excelize.File, aba string, f func(n int, cels []string) bool) error {
	rows, err := x.Rows(aba)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
		cels, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return err
		}
		if !f(n, cels) {
			return nil
		}
	}
	return rows.Error()
}

func mesDe(s string) (time.Time, bool) {
	s = normNumero(s)
	if len(s) != 6 || !soDigitos.MatchString(s) {
		return time.Time{}, false
	}
	t, err := time.Parse("200601", s)
	return t, err == nil
}

// Ler lê a planilha de análise em caminho. ano = 0 usa o da MENSAL!C2.
func Ler(caminho string, ano int) (res *Analise, err error) {
	defer func() {
		if r := recover(); r != nil {
			res, err = nil, entrada("XLSX corrompido ou ilegível")
		}
	}()
	x, e := excelize.OpenFile(caminho, excelize.Options{UnzipSizeLimit: limiteDescompactado, UnzipXMLSizeLimit: limiteXMLMemoria})
	if e != nil {
		return nil, entrada("XLSX corrompido ou ilegível")
	}
	defer x.Close()

	mensal, ok := nomeAba(x, abaMensal)
	if !ok {
		return nil, entrada("a planilha não tem a aba MENSAL")
	}
	a := &Analise{}
	linhasMensal, e := lerMensal(x, mensal, a)
	if e != nil {
		return nil, entrada("não foi possível ler a aba MENSAL: " + e.Error())
	}
	if ano == 0 {
		ano = a.AnoMensal
	}
	if ano < 2000 || ano > 2100 {
		return nil, entrada("informe o ano: a célula C2 da aba MENSAL não tem um ano válido")
	}
	a.Ano = ano
	nomeReal := fmt.Sprintf("Real_%02d", ano%100)
	real, ok := nomeAba(x, nomeReal)
	if !ok {
		return nil, entrada("a planilha não tem a aba " + nomeReal + " (lançamentos do ano " + strconv.Itoa(ano) + ")")
	}
	if e := lerReal(x, real, a); e != nil {
		return nil, entrada("não foi possível ler a aba " + nomeReal + ": " + e.Error())
	}
	if a.AnoMensal == ano {
		a.Linhas = linhasMensal
	} else if len(linhasMensal) > 0 {
		a.Avisos = append(a.Avisos, fmt.Sprintf("A aba MENSAL mostra o ano %d e a carga é do ano %d: as linhas do DRE não foram gravadas, só as contas.", a.AnoMensal, ano))
	}
	if len(a.Contas) == 0 {
		a.Avisos = append(a.Avisos, "A aba "+nomeReal+" não trouxe nenhuma conta com mês e valor válidos.")
	}
	return a, nil
}

// lerMensal lê C2 (ano), C3 (visão), D1 (filial) e as linhas do DRE (linha 5 em
// diante: A chave, C descrição, D..O janeiro a dezembro já calculados).
func lerMensal(x *excelize.File, aba string, a *Analise) ([]dre.LinhaExterna, error) {
	type bruta struct {
		chave, desc string
		vals        [12]*float64
	}
	var brutas []bruta
	d1 := ""
	err := percorrer(x, aba, func(n int, c []string) bool {
		switch {
		case n == 1:
			d1 = celula(c, 3)
		case n == 2:
			if v, ok := numero(celula(c, 2)); ok && v == math.Trunc(v) {
				a.AnoMensal = int(v)
			}
		case n == 3:
			a.Visao = celula(c, 2)
		case n >= primeiraLinhaDRE:
			b := bruta{chave: normNumero(celula(c, 0)), desc: celula(c, 2)}
			if b.chave == "0" {
				b.chave = ""
			}
			if b.chave == "" && b.desc == "" {
				return true
			}
			tem := false
			for m := 0; m < 12; m++ {
				if v, ok := numero(celula(c, 3+m)); ok {
					vv := v
					b.vals[m], tem = &vv, true
				}
			}
			if tem {
				brutas = append(brutas, b)
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if len(brutas) == 0 {
		a.Avisos = append(a.Avisos, "A aba MENSAL não trouxe nenhuma linha com valor numérico (a partir da linha 5, colunas D a O). Se ela tem fórmulas, salve a planilha no Excel para gravar o valor calculado.")
	}
	if a.AnoMensal == 0 {
		a.Avisos = append(a.Avisos, "A célula C2 da aba MENSAL não tem um ano válido.")
	}
	if v, ok := numero(d1); ok && v == math.Trunc(v) && v >= 0 {
		a.Filial = normFilial(d1)
	} else {
		a.Avisos = append(a.Avisos, "A aba MENSAL está numa visão sem filial resolvida (célula D1 vazia ou sem número): as linhas do DRE foram gravadas só para exibição e o comparativo por linha precisa de uma filial. Selecione uma filial na aba MENSAL, salve e envie de novo.")
	}
	var out []dre.LinhaExterna
	usadas := map[string]int{}
	repetidas := 0
	for _, b := range brutas {
		chave := b.chave
		if chave == "" {
			chave = dre.SlugLinha(b.desc)
		}
		if usadas[chave]++; usadas[chave] > 1 {
			repetidas++
			chave = fmt.Sprintf("%s_%d", chave, usadas[chave])
		}
		desc := b.desc
		if desc == "" {
			desc = b.chave
		}
		for m := 0; m < 12; m++ {
			if b.vals[m] != nil && a.AnoMensal > 0 {
				out = append(out, dre.LinhaExterna{
					Competencia: time.Date(a.AnoMensal, time.Month(m+1), 1, 0, 0, 0, 0, time.UTC),
					Chave:       chave, Descricao: desc, Valor: *b.vals[m],
				})
			}
		}
	}
	if repetidas > 0 {
		a.Avisos = append(a.Avisos, fmt.Sprintf("%d linha(s) da aba MENSAL repetem a chave de outra: ganharam um sufixo numérico.", repetidas))
	}
	return out, nil
}

type chaveReal struct {
	comp          time.Time
	filial, conta string
}

// ehLinhaDeTotal reconhece os totais e as provas de rotina que a aba Real_AA
// traz junto das contas (tabela dinâmica): não são contas.
func ehLinhaDeTotal(conta string) bool {
	c := strings.ToLower(strings.TrimSpace(conta))
	switch c {
	case "total", "total geral", "(vazio)", "resultado resumo":
		return true
	}
	return strings.HasPrefix(c, "calculo para prova real") || strings.HasPrefix(c, "cálculo para prova real") ||
		strings.HasPrefix(c, "diferença (tem que") || strings.HasPrefix(c, "diferenca (tem que")
}

// lerReal soma VALOR por (mês, loja, conta) da aba Real_AA: A conta, B valor,
// C mês (AAAAMM), D loja. Linha sem conta ou sem mês válido do ano é ignorada.
func lerReal(x *excelize.File, aba string, a *Analise) error {
	soma := map[chaveReal]float64{}
	err := percorrer(x, aba, func(n int, c []string) bool {
		conta, mesS := normNumero(celula(c, 0)), celula(c, 2)
		if conta == "" && mesS == "" && celula(c, 1) == "" {
			return true // linha em branco
		}
		valor, okV := numero(celula(c, 1))
		mes, okM := mesDe(mesS)
		if n == 1 && !okV && !okM {
			return true // cabeçalho
		}
		if conta == "" || !okM || !okV || mes.Year() != a.Ano {
			a.Ignoradas++
			return true
		}
		soma[chaveReal{mes, normFilial(celula(c, 3)), conta}] += valor
		return true
	})
	if err != nil {
		return err
	}
	a.Contas = make([]dre.ContaExterna, 0, len(soma))
	for k, v := range soma {
		a.Contas = append(a.Contas, dre.ContaExterna{Competencia: k.comp, Filial: k.filial, Conta: k.conta, Valor: v})
	}
	return nil
}
