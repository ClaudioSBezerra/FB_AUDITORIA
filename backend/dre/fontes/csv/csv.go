// Package csv é a fonte de lançamentos por arquivo CSV ou XLSX. Não acessa
// banco.
//
// O formato é decidido pelo cabeçalho, procurado nas primeiras 20 linhas:
//
//   - formato do sistema (origem "csv"): competencia, filial, conta e valor
//     obrigatórias; centro_custo, canal, conta_desc, tipo_doc, doc_ref e
//     historico opcionais. competencia: AAAA-MM ou AAAA-MM-DD.
//   - formato da planilha (origem "csv.contas"): CONTA, DESCRIÇÃO, VL.
//     Realizado, Ajustes, Saldo, Grupo, Mês, Loja. Obrigatórias: conta (ou
//     Código), valor realizado, mes e loja. Ajuste/Ajustes não é importado
//     (só contado). Com as colunas Código e Conta, Código é a conta e Conta é
//     a descrição (aba Entrada_Dados). O cabeçalho é refeito a cada linha que
//     o repete, para ler várias tabelas na mesma aba; linhas sem código
//     (subtotais) ou sem Mês e Loja (conferência) são ignoradas e contadas, e
//     a linha TOTAL é conferida contra a soma das contas da mesma tabela.
//
// Arquivo que começa com a assinatura ZIP ("PK") é lido como XLSX; o resto
// como CSV (';' ou ',', BOM aceito).
package csv

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"

	"fb_auditoria/dre"
)

const (
	origemSistema  = "csv"
	origemPlanilha = "csv.contas"

	maxLinhasCabecalho  = 20
	limiteDescompactado = 256 << 20
)

// Fonte lê um CSV ou XLSX de um io.Reader.
type Fonte struct {
	r       io.Reader
	arquivo string
	origem  string
	ajustes int
	xlsx    bool

	ignoradas int
	conf      *ConferenciaTotal
}

// ConferenciaTotal compara a linha TOTAL do arquivo com a soma das contas
// importadas da mesma tabela. Divergência é aviso, não bloqueia.
type ConferenciaTotal struct {
	Esperado  float64 `json:"esperado"`
	Calculado float64 `json:"calculado"`
	OK        bool    `json:"ok"`
}

// Nova cria a fonte; arquivo é só o nome para a trilha.
func Nova(r io.Reader, arquivo string) *Fonte {
	return &Fonte{r: r, arquivo: arquivo, origem: origemSistema}
}

// Nome é a origem gravada; só é definitiva depois de Ler.
func (f *Fonte) Nome() string    { return f.origem }
func (f *Fonte) Arquivo() string { return f.arquivo }

// AjustesIgnorados é a quantidade de linhas com ajuste diferente de zero que
// foram lidas (formato da planilha); o ajuste não é importado.
func (f *Fonte) AjustesIgnorados() int { return f.ajustes }

// LinhasIgnoradas é a quantidade de linhas não importadas por serem subtotais
// (sem código) ou conferência (código e valor, sem Mês e Loja).
func (f *Fonte) LinhasIgnoradas() int { return f.ignoradas }

// Conferencia devolve a conferência da linha TOTAL, ou nil se não houve.
func (f *Fonte) Conferencia() *ConferenciaTotal { return f.conf }

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

var (
	soDigitos   = regexp.MustCompile(`^\d+$`)
	inteiroComZ = regexp.MustCompile(`^(\d+)[.,]0+$`)
	espacos     = regexp.MustCompile(`\s+`)
)

// parseMes aceita AAAAMM em texto ou número (202609, 202609.0).
func parseMes(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if m := inteiroComZ.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if len(s) != 6 || !soDigitos.MatchString(s) {
		return time.Time{}, fmt.Errorf("mês inválido")
	}
	return time.Parse("200601", s)
}

// normFilial: só dígitos perde os zeros à esquerda (como o CODFILIAL do ERP);
// número de célula (11, 11.0) vira 11.
func normFilial(s string) string {
	s = strings.TrimSpace(s)
	if m := inteiroComZ.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if soDigitos.MatchString(s) {
		if s = strings.TrimLeft(s, "0"); s == "" {
			s = "0"
		}
	}
	return s
}

var semAcento = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ç", "c",
)

// normCab: minúsculas, sem acento, sem pontos, espaços repetidos colapsados.
func normCab(h string) string {
	h = strings.TrimPrefix(h, string(rune(0xFEFF)))
	h = semAcento.Replace(strings.ToLower(strings.TrimSpace(h)))
	h = strings.ReplaceAll(h, ".", "")
	return strings.TrimSpace(espacos.ReplaceAllString(h, " "))
}

func ehValorPlanilha(h string) bool {
	return h == "vl realizado" || h == "valor realizado" || h == "realizado"
}

// ehCabecalho: a linha traz o conjunto obrigatório de um dos formatos —
// sistema: competencia, conta e valor; planilha: conta (ou código), valor
// realizado, mes e loja. (A falta de filial no formato do sistema é apontada depois.)
func ehCabecalho(cels []string) bool {
	var comp, conta, codigo, valor, vreal, mes, loja bool
	for _, c := range cels {
		switch h := normCab(c); {
		case h == "competencia":
			comp = true
		case h == "conta":
			conta = true
		case h == "codigo":
			codigo = true
		case h == "valor":
			valor = true
		case ehValorPlanilha(h):
			vreal = true
		case h == "mes":
			mes = true
		case h == "loja":
			loja = true
		}
	}
	return (comp && conta && valor) || ((conta || codigo) && vreal && mes && loja)
}

// cabecalhoLancamentos reconhece o cabeçalho dos formatos de lançamentos.
func cabecalhoLancamentos(cels []string) bool { return ehCabecalho(cels) }

// primeiraComCompetencia: uma primeira linha com competencia ainda vale como
// cabeçalho do formato do sistema (para apontar a coluna ausente).
func primeiraComCompetencia(cels []string) bool {
	for _, c := range cels {
		if normCab(c) == "competencia" {
			return true
		}
	}
	return false
}

// acharCabecalho devolve o índice do cabeçalho nas primeiras 20 linhas, ou -1.
// cab reconhece o cabeçalho completo; primeira (opcional) aceita a primeira
// linha como cabeçalho quando nenhuma linha passou em cab.
func acharCabecalho(rows []linha, cab, primeira func([]string) bool) int {
	for i, r := range rows {
		if i >= maxLinhasCabecalho {
			break
		}
		if cab(r.cels) {
			return i
		}
	}
	if primeira != nil && len(rows) > 0 && primeira(rows[0].cels) {
		return 0
	}
	return -1
}

type linha struct {
	n    int
	cels []string
}

func vazia(cels []string) bool {
	for _, c := range cels {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func lerCSV(data []byte, sep rune) ([]linha, []dre.Problema) {
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = sep
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	var rows []linha
	var probs []dre.Problema
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			n := 0
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				n = pe.Line
			}
			probs = append(probs, dre.Problema{Linha: n, Msg: fmt.Sprint(err)})
			continue
		}
		n, _ := cr.FieldPos(0)
		rows = append(rows, linha{n: n, cels: rec})
	}
	return rows, probs
}

// decodificar converte CSV não UTF-8 (Excel exporta em Windows-1252).
func decodificar(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	if b, err := charmap.Windows1252.NewDecoder().Bytes(data); err == nil {
		return b
	}
	return data
}

// lerXLSX devolve as linhas da primeira planilha que tenha o cabeçalho nas
// primeiras 20 linhas; valores brutos das células.
func lerXLSX(data []byte, cab, primeira func([]string) bool) (rows []linha, err error) {
	defer func() {
		if recover() != nil {
			rows, err = nil, &dre.ErroEntrada{Msg: "XLSX corrompido ou ilegível"}
		}
	}()
	x, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		UnzipSizeLimit: limiteDescompactado, UnzipXMLSizeLimit: limiteDescompactado,
		RawCellValue: true,
	})
	if err != nil {
		return nil, &dre.ErroEntrada{Msg: "XLSX corrompido ou ilegível"}
	}
	defer x.Close()
	for _, nome := range x.GetSheetList() {
		cels, err := x.GetRows(nome)
		if err != nil {
			return nil, &dre.ErroEntrada{Msg: "XLSX corrompido ou ilegível"}
		}
		rs := make([]linha, len(cels))
		for j, cc := range cels {
			rs[j] = linha{n: j + 1, cels: cc}
		}
		if acharCabecalho(rs, cab, primeira) >= 0 {
			return rs, nil
		}
	}
	return nil, nil // sem cabeçalho em nenhuma planilha
}

// tabela é o conteúdo lido de um CSV ou XLSX: as linhas, o índice do cabeçalho
// (hi, -1 se não achado) e os problemas de leitura posteriores ao cabeçalho.
type tabela struct {
	rows  []linha
	probs []dre.Problema
	hi    int
	xlsx  bool
}

// lerTabela decide o formato pela assinatura (ZIP é XLSX) e acha o cabeçalho com
// cab/primeira. Serve a todas as cargas por arquivo.
func lerTabela(data []byte, cab, primeira func([]string) bool) (tabela, error) {
	t := tabela{hi: -1}
	var err error
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		t.xlsx = true
		if t.rows, err = lerXLSX(data, cab, primeira); err != nil {
			return t, err
		}
		t.hi = acharCabecalho(t.rows, cab, primeira)
		return t, nil
	}
	data = decodificar(data)
	for _, sep := range []rune{';', ',', '\t'} {
		t.rows, t.probs = lerCSV(data, sep)
		if t.hi = acharCabecalho(t.rows, cab, primeira); t.hi >= 0 {
			break
		}
	}
	if t.hi >= 0 { // erros de parse antes do cabeçalho (títulos) são descartados
		kept := t.probs[:0]
		for _, p := range t.probs {
			if p.Linha > t.rows[t.hi].n {
				kept = append(kept, p)
			}
		}
		t.probs = kept
	}
	return t, nil
}

const colunasEsperadas = "formato do sistema: competencia, filial, conta, valor; " +
	"formato da planilha: CONTA (ou Código), VL. Realizado, Mês, Loja (DESCRIÇÃO, Ajuste, Saldo e Grupo opcionais)"

// num lê um número: no XLSX o valor bruto vem com ponto decimal; texto como
// 1.234,56 cai no parseValorBR.
func (f *Fonte) num(s string) (float64, error) {
	if f.xlsx {
		if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && !math.IsNaN(v) && !math.IsInf(v, 0) {
			return v, nil
		}
	}
	return parseValorBR(s)
}

// Ler devolve as linhas válidas; linhas com problema vêm num *dre.ErroLinhas.
func (f *Fonte) Ler(ctx context.Context, _ dre.Escopo) ([]dre.Lancamento, error) {
	f.origem, f.ajustes, f.xlsx, f.ignoradas, f.conf = origemSistema, 0, false, 0, nil
	data, err := io.ReadAll(f.r)
	if err != nil {
		return nil, &dre.ErroEntrada{Msg: "arquivo ilegível"}
	}
	if len(data) == 0 {
		return nil, &dre.ErroEntrada{Msg: "CSV vazio ou ilegível"}
	}
	t, err := lerTabela(data, cabecalhoLancamentos, primeiraComCompetencia)
	if err != nil {
		return nil, err
	}
	f.xlsx = t.xlsx
	rows, probs, hi, ehCSV := t.rows, t.probs, t.hi, !t.xlsx
	if hi < 0 {
		if ehCSV && len(rows) == 0 && len(probs) == 0 {
			return nil, &dre.ErroEntrada{Msg: "arquivo vazio ou ilegível"}
		}
		return nil, &dre.ErroEntrada{Msg: "cabeçalho não reconhecido; esperado " + colunasEsperadas}
	}
	head := rows[hi].cels
	col := map[string]int{}
	for i, h := range head {
		if n := normCab(h); n != "" {
			if _, ok := col[n]; !ok {
				col[n] = i
			}
		}
	}
	if _, sistema := col["competencia"]; !sistema {
		f.origem = origemPlanilha
		return f.lerPlanilha(ctx, rows, hi, ehCSV, probs)
	}
	f.origem = origemSistema
	for _, req := range []string{"competencia", "filial", "conta", "valor"} {
		if _, ok := col[req]; !ok {
			return nil, &dre.ErroEntrada{Msg: "coluna obrigatória ausente: " + req}
		}
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i >= 0 && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	var out []dre.Lancamento
	for _, r := range rows[hi+1:] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if vazia(r.cels) {
			continue
		}
		if ehCSV && len(r.cels) != len(head) {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "número de colunas diferente do cabeçalho"})
			continue
		}
		comp, err := parseCompetencia(get(r.cels, "competencia"))
		if err != nil {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "competencia inválida"})
			continue
		}
		conta := get(r.cels, "conta")
		valor, err := f.num(get(r.cels, "valor"))
		if conta == "" || err != nil {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "conta/valor inválidos"})
			continue
		}
		out = append(out, dre.Lancamento{
			Linha: r.n, Competencia: comp, Filial: get(r.cels, "filial"),
			CentroCusto: get(r.cels, "centro_custo"), Canal: get(r.cels, "canal"),
			Conta: conta, ContaDesc: get(r.cels, "conta_desc"), Valor: valor,
			TipoDoc: get(r.cels, "tipo_doc"), DocRef: get(r.cels, "doc_ref"), Historico: get(r.cels, "historico"),
		})
		continue
	}
	if len(probs) > 0 {
		return out, &dre.ErroLinhas{Problemas: probs}
	}
	return out, nil
}

// mapaPlanilha são as colunas de uma tabela do formato da planilha, refeitas a
// cada cabeçalho.
type mapaPlanilha struct {
	n                           int // colunas do cabeçalho
	codigo, desc, valor, ajuste int
	mes, loja                   int
}

// mapearPlanilha lê um cabeçalho (que já passou em ehCabecalho). Com Código e
// Conta, Código é a conta e Conta é a descrição; só com Conta, Conta é o código
// e a descrição vem de DESCRIÇÃO.
func mapearPlanilha(head []string) mapaPlanilha {
	col := map[string]int{}
	for i, h := range head {
		if n := normCab(h); n != "" {
			if _, ok := col[n]; !ok {
				col[n] = i
			}
		}
	}
	idx := func(nomes ...string) int {
		for _, n := range nomes {
			if i, ok := col[n]; ok {
				return i
			}
		}
		return -1
	}
	m := mapaPlanilha{
		n:      len(head),
		valor:  idx("vl realizado", "valor realizado", "realizado"),
		ajuste: idx("ajustes", "ajuste"),
		mes:    idx("mes"),
		loja:   idx("loja"),
	}
	if c := idx("codigo"); c >= 0 {
		m.codigo = c
		m.desc = idx("descricao", "conta")
	} else {
		m.codigo = idx("conta")
		m.desc = idx("descricao")
	}
	return m
}

func celula(rec []string, i int) string {
	if i >= 0 && i < len(rec) {
		return strings.TrimSpace(rec[i])
	}
	return ""
}

func ehTotal(codigo string) bool { return normCab(codigo) == "total" }

const msgSemCache = "Mês e Loja vazios: as fórmulas não têm valor calculado; abra o arquivo no Excel e salve de novo, ou preencha Mês e Loja"

// lerPlanilha lê o formato da planilha varrendo todas as linhas a partir do
// primeiro cabeçalho: cada linha que repete o conjunto obrigatório refaz o
// mapeamento de colunas (duas tabelas na mesma aba). Linha sem código é
// subtotal; linha com código mas sem Mês e sem Loja é conferência; ambas são
// ignoradas e contadas. Só uma das duas células vazia, ou valor que não é
// número, é erro de linha.
func (f *Fonte) lerPlanilha(ctx context.Context, rows []linha, hi int, ehCSV bool, probs []dre.Problema) ([]dre.Lancamento, error) {
	var out []dre.Lancamento
	var m mapaPlanilha
	var soma float64 // das contas importadas da tabela atual
	semMesLoja := 0
	for _, r := range rows[hi:] {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if ehCabecalho(r.cels) {
			m, soma = mapearPlanilha(r.cels), 0
			continue
		}
		if vazia(r.cels) {
			continue
		}
		codigo := celula(r.cels, m.codigo)
		if f.xlsx {
			codigo = inteiroTexto(codigo)
		}
		if codigo == "" {
			f.ignoradas++
			continue
		}
		mesTxt, lojaTxt := celula(r.cels, m.mes), celula(r.cels, m.loja)
		valor, errValor := f.num(celula(r.cels, m.valor))
		if ehTotal(codigo) {
			f.ignoradas++
			if errValor == nil {
				esp, calc := math.Round(valor*100), math.Round(soma*100)
				c := &ConferenciaTotal{Esperado: esp / 100, Calculado: calc / 100, OK: math.Abs(esp-calc) <= 1}
				if f.conf == nil || f.conf.OK {
					f.conf = c
				}
			}
			continue
		}
		if mesTxt == "" && lojaTxt == "" {
			f.ignoradas++
			if errValor == nil {
				semMesLoja++
			}
			continue
		}
		if ehCSV && len(r.cels) != m.n {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "número de colunas diferente do cabeçalho"})
			continue
		}
		if mesTxt == "" {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "Mês vazio"})
			continue
		}
		comp, err := parseMes(mesTxt)
		if err != nil {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "Mês inválido (esperado AAAAMM)"})
			continue
		}
		filial := normFilial(lojaTxt)
		if filial == "" {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "Loja vazia"})
			continue
		}
		aj, errAj := f.num(celula(r.cels, m.ajuste))
		if celula(r.cels, m.valor) == "" { // valor em branco: zero/ausente, não grava
			f.ignoradas++
			if errAj == nil && aj != 0 {
				f.ajustes++
			}
			continue
		}
		if errValor != nil {
			probs = append(probs, dre.Problema{Linha: r.n, Msg: "conta/valor inválidos"})
			continue
		}
		if errAj == nil && aj != 0 {
			f.ajustes++
		}
		soma += valor
		out = append(out, dre.Lancamento{
			Linha: r.n, Competencia: comp, Filial: filial,
			Conta: codigo, ContaDesc: celula(r.cels, m.desc), Valor: valor,
		})
	}
	if len(probs) > 0 {
		return out, &dre.ErroLinhas{Problemas: probs}
	}
	if len(out) == 0 && semMesLoja > 0 {
		return nil, &dre.ErroEntrada{Msg: msgSemCache}
	}
	return out, nil
}
