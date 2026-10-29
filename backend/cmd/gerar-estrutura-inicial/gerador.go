package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/unicode/norm"

	"fb_auditoria/dre"
)

// Limites das telas de carga (iguais aos das cargas de estrutura e dicionário).
const (
	maxCodigo    = 40
	maxDescricao = 200
	maxConta     = 60
)

// Opcoes são os argumentos do gerador.
type Opcoes struct {
	Arquivo       string
	Saida         string
	AbaEstrutura  string
	AbaDicionario string
	ColunaValor   string
}

// Linha é uma linha do DRE lida da aba de estrutura.
type Linha struct {
	Row       int // linha na planilha
	Chave     string
	Codigo    string
	Descricao string
	Subtotal  bool
	SomaDe    []string
}

// ContaDic é uma conta da aba do dicionário.
type ContaDic struct {
	Row         int
	Conta       string
	Descricao   string
	Grupo       string
	CentroCusto string
}

// Departamento é um grupo de despesa de um bloco (Comercial, Logística, Administrativo).
type Departamento struct{ Codigo, Nome, Grupo string }

// Resumo traz só contagens, nunca textos da planilha.
type Resumo struct {
	Linhas, Subtotais, LinhasConta      int
	ContasDicionario, ContasMapeadas    int
	SemGrupo, GrupoSemLinha, Duplicadas int
	DepartamentosPorBloco               map[string]int
	Papel                               bool
}

func (r Resumo) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "estrutura: %d linhas (%d subtotais, %d contas); papel faturamento_liquido: %v\n", r.Linhas, r.Subtotais, r.LinhasConta, r.Papel)
	fmt.Fprintf(&b, "dicionario: %d contas (%d repetidas iguais ignoradas)\n", r.ContasDicionario, r.Duplicadas)
	fmt.Fprintf(&b, "mapa: %d contas mapeadas; sem linha: %d (sem grupo %d, grupo sem linha %d)\n", r.ContasMapeadas, r.SemGrupo+r.GrupoSemLinha, r.SemGrupo, r.GrupoSemLinha)
	blocos := make([]string, 0, len(r.DepartamentosPorBloco))
	for k := range r.DepartamentosPorBloco {
		blocos = append(blocos, k)
	}
	sort.Strings(blocos)
	total := 0
	for _, k := range blocos {
		total += r.DepartamentosPorBloco[k]
	}
	fmt.Fprintf(&b, "departamentos: %d", total)
	for _, k := range blocos {
		fmt.Fprintf(&b, " | %s %d", k, r.DepartamentosPorBloco[k])
	}
	b.WriteString("\n")
	return b.String()
}

// normaliza: sem acento, maiúsculas, espaços colapsados.
var reEspacos = regexp.MustCompile(`\s+`)

func normaliza(s string) string {
	d := norm.NFD.String(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range d {
		if r >= 0x300 && r <= 0x36f { // marcas de acento
			continue
		}
		b.WriteRune(r)
	}
	return strings.ToUpper(reEspacos.ReplaceAllString(strings.TrimSpace(b.String()), " "))
}

var reNaoAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// slug gera um código a partir da descrição: sem marcas como "(=)" e "(-)", sem
// acento, minúsculas, separado por "_".
func slug(desc string) string {
	d := strings.TrimSpace(desc)
	for strings.HasPrefix(d, "(") {
		i := strings.Index(d, ")")
		if i < 0 {
			break
		}
		d = strings.TrimSpace(d[i+1:])
	}
	s := strings.ToLower(normaliza(d))
	s = strings.Trim(reNaoAlnum.ReplaceAllString(s, "_"), "_")
	if s == "" {
		s = "linha"
	}
	if len(s) > maxCodigo {
		s = strings.Trim(s[:maxCodigo], "_")
	}
	return s
}

// textoCelula devolve o valor da célula como texto; número inteiro guardado como
// decimal (402.0) vira "402".
func textoCelula(rows [][]string, r, c int) string {
	if r >= len(rows) || c >= len(rows[r]) {
		return ""
	}
	s := strings.TrimSpace(rows[r][c])
	if f, err := strconv.ParseFloat(s, 64); err == nil && s != "" && !strings.ContainsAny(s, "eE") && f == float64(int64(f)) && strings.Contains(s, ".") {
		return strconv.FormatInt(int64(f), 10)
	}
	return s
}

func colunaIndice(letra string) (int, error) {
	n, err := excelize.ColumnNameToNumber(strings.ToUpper(strings.TrimSpace(letra)))
	if err != nil {
		return 0, fmt.Errorf("coluna inválida %q", letra)
	}
	return n - 1, nil
}

var (
	reRef   = regexp.MustCompile(`^([A-Z]+)(\d+)$`)
	reTermo = regexp.MustCompile(`^([+-]?)([A-Z]+)(\d+)`)
)

type refSoma struct {
	Row int
	Neg bool
}

// parseSomaSimples reconhece a fórmula de soma simples: SUM(Dx:Dy[,Dz...]) ou
// Dx+Dy-Dz, sempre na coluna col. ok=false para qualquer outra fórmula (SUMIFS,
// IF etc.), que faz da linha uma linha de conta.
func parseSomaSimples(formula, aba, col string) (refs []refSoma, ok bool) {
	f := strings.ToUpper(strings.NewReplacer(" ", "", "$", "").Replace(strings.TrimSpace(formula)))
	f = strings.TrimPrefix(f, "=")
	f = strings.TrimPrefix(f, strings.ToUpper(aba)+"!")
	f = strings.ReplaceAll(f, strings.ToUpper(aba)+"!", "")
	if f == "" {
		return nil, false
	}
	col = strings.ToUpper(col)
	if strings.HasPrefix(f, "SUM(") && strings.HasSuffix(f, ")") {
		for _, arg := range strings.FieldsFunc(f[4:len(f)-1], func(r rune) bool { return r == ',' || r == ';' }) {
			ini, fim, temFim := arg, "", false
			if i := strings.Index(arg, ":"); i >= 0 {
				ini, fim, temFim = arg[:i], arg[i+1:], true
			}
			a := reRef.FindStringSubmatch(ini)
			if a == nil || a[1] != col {
				return nil, false
			}
			r1, _ := strconv.Atoi(a[2])
			r2 := r1
			if temFim {
				b := reRef.FindStringSubmatch(fim)
				if b == nil || b[1] != col {
					return nil, false
				}
				r2, _ = strconv.Atoi(b[2])
			}
			if r2 < r1 {
				r1, r2 = r2, r1
			}
			for r := r1; r <= r2; r++ {
				refs = append(refs, refSoma{Row: r})
			}
		}
		return refs, len(refs) > 0
	}
	for f != "" {
		m := reTermo.FindStringSubmatch(f)
		if m == nil || m[2] != col {
			return nil, false
		}
		if len(refs) > 0 && m[1] == "" {
			return nil, false // termos seguintes precisam de sinal
		}
		r, _ := strconv.Atoi(m[3])
		refs = append(refs, refSoma{Row: r, Neg: m[1] == "-"})
		f = f[len(m[0]):]
	}
	return refs, len(refs) > 0
}

// LerEstrutura lê a aba do DRE: coluna A = chave, C = descrição; o cabeçalho tem
// "GRUPO" na coluna A e "DESCRIÇÃO" na C. Linha cuja célula da coluna de valor
// tem fórmula de soma simples é subtotal (soma_de vem das linhas referenciadas
// que existem no DRE); as demais são contas.
func LerEstrutura(x *excelize.File, aba, colValor string) ([]Linha, error) {
	cv, err := colunaIndice(colValor)
	if err != nil {
		return nil, err
	}
	rows, err := x.GetRows(aba, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("aba %q: %w", aba, err)
	}
	hi := -1
	for i := 0; i < len(rows) && i < 30; i++ {
		if normaliza(textoCelula(rows, i, 0)) == "GRUPO" && strings.HasPrefix(normaliza(textoCelula(rows, i, 2)), "DESCRI") {
			hi = i
			break
		}
	}
	if hi < 0 {
		return nil, fmt.Errorf("aba %q: cabeçalho não encontrado (esperado GRUPO na coluna A e DESCRIÇÃO na coluna C, nas primeiras 30 linhas)", aba)
	}
	var linhas []Linha
	porRow := map[int]int{} // linha da planilha -> índice em linhas
	usados := map[string]int{}
	for i := hi + 1; i < len(rows); i++ {
		chave, desc := textoCelula(rows, i, 0), textoCelula(rows, i, 2)
		if chave == "0" {
			chave = "" // marcador de "sem chave" (subtotais)
		}
		if desc == "" {
			desc = chave
		}
		if desc == "" {
			continue
		}
		cel, _ := excelize.CoordinatesToCellName(cv+1, i+1)
		formula, _ := x.GetCellFormula(aba, cel)
		l := Linha{Row: i + 1, Chave: chave, Descricao: desc}
		if _, ok := parseSomaSimples(formula, aba, colValor); ok {
			l.Subtotal = true
		}
		if chave != "" {
			l.Codigo = chave
		} else {
			l.Codigo = slug(desc)
		}
		porRow[i+1] = len(linhas)
		linhas = append(linhas, l)
	}
	if len(linhas) == 0 {
		return nil, fmt.Errorf("aba %q: nenhuma linha de DRE abaixo do cabeçalho", aba)
	}
	// Códigos únicos: chave repetida é erro; código gerado de descrição ganha sufixo.
	var erros []string
	for i := range linhas {
		l := &linhas[i]
		if l.Chave != "" {
			if prim, ok := usados[l.Codigo]; ok {
				erros = append(erros, fmt.Sprintf("chave repetida na linha %d da planilha (já na linha %d)", l.Row, prim))
				continue
			}
			usados[l.Codigo] = l.Row
		}
	}
	for i := range linhas {
		l := &linhas[i]
		if l.Chave == "" {
			base, cod := l.Codigo, l.Codigo
			for n := 2; ; n++ {
				if _, ok := usados[cod]; !ok {
					break
				}
				suf := "_" + strconv.Itoa(n)
				cod = strings.Trim(base[:min(len(base), maxCodigo-len(suf))], "_") + suf
			}
			l.Codigo = cod
			usados[cod] = l.Row
		}
		if utf8.RuneCountInString(l.Codigo) > maxCodigo {
			erros = append(erros, fmt.Sprintf("linha %d da planilha: código com mais de %d caracteres", l.Row, maxCodigo))
		}
		if utf8.RuneCountInString(l.Descricao) > maxDescricao {
			erros = append(erros, fmt.Sprintf("linha %d da planilha: descrição com mais de %d caracteres", l.Row, maxDescricao))
		}
	}
	// soma_de dos subtotais.
	for i := range linhas {
		l := &linhas[i]
		if !l.Subtotal {
			continue
		}
		cel, _ := excelize.CoordinatesToCellName(cv+1, l.Row)
		formula, _ := x.GetCellFormula(aba, cel)
		refs, _ := parseSomaSimples(formula, aba, colValor)
		for _, r := range refs {
			j, ok := porRow[r.Row]
			if !ok {
				if len(refs) > 1 && isRange(formula) {
					continue // linha em branco dentro de um intervalo
				}
				erros = append(erros, fmt.Sprintf("linha %d da planilha: a fórmula referencia a linha %d, que não é linha do DRE", l.Row, r.Row))
				continue
			}
			if j == i {
				erros = append(erros, fmt.Sprintf("linha %d da planilha: a fórmula soma a própria linha", l.Row))
				continue
			}
			cod := linhas[j].Codigo
			if r.Neg {
				cod = "-" + cod
			}
			l.SomaDe = append(l.SomaDe, cod)
		}
		if len(l.SomaDe) == 0 {
			erros = append(erros, fmt.Sprintf("linha %d da planilha: subtotal sem nenhuma linha somada", l.Row))
		}
	}
	if len(erros) > 0 {
		return nil, listaErros("estrutura", erros)
	}
	return linhas, nil
}

func isRange(f string) bool { return strings.Contains(strings.ToUpper(f), "SUM(") }

func listaErros(onde string, erros []string) error {
	const max = 20
	msg := fmt.Sprintf("%s: %d problema(s)", onde, len(erros))
	for i, e := range erros {
		if i == max {
			msg += fmt.Sprintf("\n  ... e mais %d", len(erros)-max)
			break
		}
		msg += "\n  " + e
	}
	return errors.New(msg)
}

// LerDicionario lê a aba do dicionário: cabeçalho com CONTA, DESCRIÇÃO e GRUPO
// (e, se houver, CENTRO DE CUSTO). Linhas sem conta (resumos) são ignoradas.
func LerDicionario(x *excelize.File, aba string) ([]ContaDic, error) {
	rows, err := x.GetRows(aba, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fmt.Errorf("aba %q: %w", aba, err)
	}
	hi, cConta, cDesc, cGrupo, cCC := -1, -1, -1, -1, -1
	for i := 0; i < len(rows) && i < 30 && hi < 0; i++ {
		conta, grupo := -1, -1
		for j := range rows[i] {
			switch n := normaliza(rows[i][j]); {
			case n == "CONTA" && conta < 0:
				conta = j
			case n == "GRUPO" && grupo < 0:
				grupo = j
			}
		}
		if conta >= 0 && grupo >= 0 {
			hi, cConta, cGrupo = i, conta, grupo
			cDesc = -1
			for j := range rows[i] {
				switch n := normaliza(rows[i][j]); {
				case strings.HasPrefix(n, "DESCRI") && cDesc < 0:
					cDesc = j
				case n == "CENTRO DE CUSTO" && cCC < 0:
					cCC = j
				}
			}
		}
	}
	if hi < 0 {
		return nil, fmt.Errorf("aba %q: cabeçalho não encontrado (esperado CONTA e GRUPO, nas primeiras 30 linhas)", aba)
	}
	var out []ContaDic
	for i := hi + 1; i < len(rows); i++ {
		conta := textoCelula(rows, i, cConta)
		if conta == "" || normaliza(conta) == "TOTAL" {
			continue
		}
		c := ContaDic{Row: i + 1, Conta: conta, Grupo: textoCelula(rows, i, cGrupo)}
		if cDesc >= 0 {
			c.Descricao = textoCelula(rows, i, cDesc)
		}
		if cCC >= 0 {
			c.CentroCusto = textoCelula(rows, i, cCC)
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("aba %q: nenhuma conta abaixo do cabeçalho", aba)
	}
	return out, nil
}

// Saidas é o conteúdo dos CSVs, antes de gravar.
type Saidas struct {
	Estrutura     [][]string
	Mapa          [][]string
	Departamentos [][]string
	Dicionario    [][]string
	SemLinha      [][]string
	Resumo        Resumo
}

var reLiquida = regexp.MustCompile(`VENDA LIQUIDA|FATURAMENTO LIQUIDO`)

// papelFaturamento é o código do papel gravado na estrutura.
const papelFaturamento = dre.PapelFaturamentoLiquido

// Montar cruza a estrutura com o dicionário e produz os CSVs.
func Montar(linhas []Linha, contas []ContaDic) (Saidas, error) {
	var s Saidas
	porCodigo := map[string]Linha{}
	porNorm := map[string]string{} // grupo normalizado -> código da linha de conta
	var erros []string
	for _, l := range linhas {
		porCodigo[l.Codigo] = l
		if !l.Subtotal {
			n := normaliza(l.Codigo)
			if outro, ok := porNorm[n]; ok && outro != l.Codigo {
				erros = append(erros, fmt.Sprintf("linhas %d e %s: códigos iguais depois de normalizar acento, caixa e espaços", l.Row, outro))
				continue
			}
			porNorm[n] = l.Codigo
		}
	}
	if len(erros) > 0 {
		return s, listaErros("estrutura", erros)
	}

	// Estrutura.
	s.Estrutura = [][]string{{"codigo", "descricao", "nivel", "ordem", "tipo", "soma_de", "papel"}}
	papelUsado := false
	for i, l := range linhas {
		tipo, nivel, papel := "conta", "1", ""
		if l.Subtotal {
			tipo, nivel = "subtotal", "0"
			if !papelUsado && reLiquida.MatchString(normaliza(l.Descricao)) {
				papel, papelUsado = papelFaturamento, true
			}
			s.Resumo.Subtotais++
		} else {
			s.Resumo.LinhasConta++
		}
		s.Estrutura = append(s.Estrutura, []string{l.Codigo, l.Descricao, nivel, strconv.Itoa((i + 1) * 10), tipo, strings.Join(l.SomaDe, "|"), papel})
	}
	s.Resumo.Linhas = len(linhas)
	s.Resumo.Papel = papelUsado

	// Departamentos: as contas somadas (direta ou indiretamente) pelos subtotais
	// cuja descrição é o nome de um bloco.
	blocoDe := map[string]string{} // código da linha de conta -> bloco
	for _, bloco := range dre.GruposDepartamento {
		for _, l := range linhas {
			if l.Subtotal && normaliza(descricaoSemMarca(l.Descricao)) == normaliza(bloco) {
				coletaContas(l, porCodigo, bloco, blocoDe, map[string]bool{})
			}
		}
	}
	s.Departamentos = [][]string{{"codigo", "nome", "grupo"}}
	s.Resumo.DepartamentosPorBloco = map[string]int{}
	for _, l := range linhas {
		if b, ok := blocoDe[l.Codigo]; ok {
			s.Departamentos = append(s.Departamentos, []string{l.Codigo, l.Descricao, b})
			s.Resumo.DepartamentosPorBloco[b]++
		}
	}

	// Dicionário e Mapa.
	s.Mapa = [][]string{{"conta_padrao", "linha_codigo", "sinal"}}
	s.Dicionario = [][]string{{"conta", "descricao", "grupo", "departamento", "centro_custo"}}
	s.SemLinha = [][]string{{"conta", "descricao", "grupo", "centro_custo", "motivo"}}
	vistas := map[string]ContaDic{}
	for _, c := range contas {
		if prim, ok := vistas[c.Conta]; ok {
			if normaliza(prim.Grupo) != normaliza(c.Grupo) {
				erros = append(erros, fmt.Sprintf("conta repetida com grupos diferentes (linhas %d e %d da planilha)", prim.Row, c.Row))
			} else {
				s.Resumo.Duplicadas++
			}
			continue
		}
		vistas[c.Conta] = c
		if utf8.RuneCountInString(c.Conta) > maxConta || utf8.RuneCountInString(c.Descricao) > maxDescricao || utf8.RuneCountInString(c.Grupo) > maxCodigo {
			erros = append(erros, fmt.Sprintf("linha %d da planilha: conta, descrição ou grupo acima do limite da carga", c.Row))
			continue
		}
		cod, motivo := "", ""
		switch {
		case c.Grupo == "":
			motivo = "sem_grupo"
			s.Resumo.SemGrupo++
		default:
			if k, ok := porCodigo[c.Grupo]; ok && !k.Subtotal {
				cod = k.Codigo
			} else if k, ok := porNorm[normaliza(c.Grupo)]; ok {
				cod = k
			} else {
				motivo = "grupo_sem_linha"
				s.Resumo.GrupoSemLinha++
			}
		}
		depto := ""
		if cod != "" {
			s.Mapa = append(s.Mapa, []string{c.Conta, cod, "1"})
			s.Resumo.ContasMapeadas++
			if _, ok := blocoDe[cod]; ok {
				depto = cod
			}
		} else {
			s.SemLinha = append(s.SemLinha, []string{c.Conta, c.Descricao, c.Grupo, c.CentroCusto, motivo})
		}
		grupo := c.Grupo
		if cod != "" {
			grupo = cod // grupo do dicionário = código do departamento quando a conta é de despesa
		}
		s.Dicionario = append(s.Dicionario, []string{c.Conta, c.Descricao, grupo, depto, c.CentroCusto})
		s.Resumo.ContasDicionario++
	}
	if len(erros) > 0 {
		return s, listaErros("dicionário", erros)
	}
	return s, nil
}

func descricaoSemMarca(d string) string {
	d = strings.TrimSpace(d)
	for strings.HasPrefix(d, "(") {
		i := strings.Index(d, ")")
		if i < 0 {
			break
		}
		d = strings.TrimSpace(d[i+1:])
	}
	return d
}

// coletaContas marca no mapa as linhas de conta somadas pelo subtotal, descendo
// pelos subtotais aninhados.
func coletaContas(l Linha, porCodigo map[string]Linha, bloco string, saida map[string]string, vistos map[string]bool) {
	if vistos[l.Codigo] {
		return
	}
	vistos[l.Codigo] = true
	for _, ref := range l.SomaDe {
		k, ok := porCodigo[strings.TrimPrefix(ref, "-")]
		if !ok {
			continue
		}
		if k.Subtotal {
			coletaContas(k, porCodigo, bloco, saida, vistos)
		} else if _, ja := saida[k.Codigo]; !ja {
			saida[k.Codigo] = bloco
		}
	}
}

func gravarCSV(caminho string, linhas [][]string) error {
	f, err := os.OpenFile(caminho, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString("\xef\xbb\xbf"); err != nil {
		f.Close()
		return err
	}
	w := csv.NewWriter(f)
	w.Comma = ';'
	if err := w.WriteAll(linhas); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// dentroDeRepositorio diz se a pasta (ou algum ancestral) tem .git.
func dentroDeRepositorio(pasta string) bool {
	abs, err := filepath.Abs(pasta)
	if err != nil {
		return true
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return true
		}
		if filepath.Dir(d) == d {
			return false
		}
	}
}

// Gerar lê a planilha e grava os CSVs na pasta de saída (criada se preciso).
func Gerar(o Opcoes) (Resumo, error) {
	if dentroDeRepositorio(o.Saida) {
		return Resumo{}, errors.New("a pasta de saída está dentro de um repositório git; os arquivos gerados têm dados do cliente e não podem ir para o repositório: escolha uma pasta fora dele")
	}
	x, err := excelize.OpenFile(o.Arquivo, excelize.Options{UnzipSizeLimit: 4 << 30, UnzipXMLSizeLimit: 64 << 20})
	if err != nil {
		return Resumo{}, fmt.Errorf("abrir a planilha: %w", err)
	}
	defer x.Close()
	linhas, err := LerEstrutura(x, o.AbaEstrutura, o.ColunaValor)
	if err != nil {
		return Resumo{}, err
	}
	contas, err := LerDicionario(x, o.AbaDicionario)
	if err != nil {
		return Resumo{}, err
	}
	s, err := Montar(linhas, contas)
	if err != nil {
		return Resumo{}, err
	}
	if err := os.MkdirAll(o.Saida, 0o700); err != nil {
		return Resumo{}, err
	}
	for nome, dados := range map[string][][]string{
		"estrutura_dre.csv": s.Estrutura, "mapa_dre.csv": s.Mapa, "departamentos.csv": s.Departamentos,
		"dicionario.csv": s.Dicionario, "contas_sem_linha.csv": s.SemLinha,
	} {
		if err := gravarCSV(filepath.Join(o.Saida, nome), dados); err != nil {
			return Resumo{}, fmt.Errorf("gravar %s: %w", nome, err)
		}
	}
	return s.Resumo, nil
}
