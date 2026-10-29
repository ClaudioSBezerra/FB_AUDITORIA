package dre

// conferencia.go — relatório de conferência entre duas origens de lançamentos
// (história 2.12). Só leitura: compara a soma bruta de `valor` por (filial,
// conta) de cada origem na mesma competência, sem passar pelo DRE e sem aplicar
// o sinal do Mapa.

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"
)

// ToleranciaPadrao é a diferença aceita entre as duas origens.
const ToleranciaPadrao = 0.01

// ErrConferencia é uma entrada inválida do relatório (o handler responde 400).
var ErrConferencia = errors.New("conferência inválida")

// ParamsConferencia são os parâmetros do relatório. Filial vazia = todas.
type ParamsConferencia struct {
	Competencia time.Time
	OrigemA     string
	OrigemB     string
	Filial      string
	Tolerancia  float64
	Limite      int
	Offset      int
}

// TotalOrigem resume uma origem na competência.
type TotalOrigem struct {
	Origem string  `json:"origem"`
	Total  float64 `json:"total"`
	Contas int     `json:"contas"` // pares (filial, conta) distintos
	Linhas int     `json:"linhas"` // lançamentos
}

// ItemConferencia é um par (filial, conta) com valor nas duas origens.
type ItemConferencia struct {
	Filial    string  `json:"filial"`
	Conta     string  `json:"conta"`
	Descricao string  `json:"descricao"`
	ValorA    float64 `json:"valor_a"`
	ValorB    float64 `json:"valor_b"`
	Diferenca float64 `json:"diferenca"`
}

// ItemSoUmLado é um par (filial, conta) presente só numa origem.
type ItemSoUmLado struct {
	Filial    string  `json:"filial"`
	Conta     string  `json:"conta"`
	Descricao string  `json:"descricao"`
	Valor     float64 `json:"valor"`
}

// ResumoLinha soma as duas origens por linha do DRE (Mapa); Codigo vazio = contas sem Mapa.
type ResumoLinha struct {
	Codigo    string  `json:"codigo"`
	Descricao string  `json:"descricao"`
	ValorA    float64 `json:"valor_a"`
	ValorB    float64 `json:"valor_b"`
	Diferenca float64 `json:"diferenca"`
}

// Conferencia é o resultado do relatório.
type Conferencia struct {
	Competencia string      `json:"competencia"`
	Tolerancia  float64     `json:"tolerancia"`
	Filial      string      `json:"filial"`
	A           TotalOrigem `json:"a"`
	B           TotalOrigem `json:"b"`
	Fechada     bool        `json:"fechada"`
	SomaNoDRE   bool        `json:"soma_no_dre"` // as duas origens têm lançamentos: o DRE as soma
	Aviso       string      `json:"aviso,omitempty"`

	ParesComparados int `json:"pares_comparados"`
	Iguais          int `json:"iguais"` // dentro da tolerância

	TotalDiferencas int               `json:"total_diferencas"`
	Diferencas      []ItemConferencia `json:"diferencas"`
	TotalSoEmA      int               `json:"total_so_em_a"`
	SoEmA           []ItemSoUmLado    `json:"so_em_a"`
	TotalSoEmB      int               `json:"total_so_em_b"`
	SoEmB           []ItemSoUmLado    `json:"so_em_b"`
	ResumoLinhas    []ResumoLinha     `json:"resumo_linhas"`
	todasDif        []ItemConferencia
	todosA, todosB  []ItemSoUmLado
}

// Todas devolve as listas completas (sem paginação), para exportação.
func (c *Conferencia) Todas() ([]ItemConferencia, []ItemSoUmLado, []ItemSoUmLado) {
	return c.todasDif, c.todosA, c.todosB
}

// AvisoSomaNoDRE é o texto fixo do alerta de dupla contagem (2.1 ainda não existe).
const AvisoSomaNoDRE = "As duas origens têm lançamentos nesta competência. Enquanto as duas estiverem carregadas, o DRE soma as duas (a história 2.1, uma conta uma fonte, ainda não existe): confira e remova a origem que não deve valer."

// NormFilialConferencia normaliza a filial como a ingestão da planilha: só
// dígitos perde os zeros à esquerda.
func NormFilialConferencia(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return s
		}
	}
	if s = strings.TrimLeft(s, "0"); s == "" {
		return "0"
	}
	return s
}

func cents(v float64) float64 { return math.Round(v*100) / 100 }

type parConf struct{ filial, conta string }

type lado struct {
	valor float64
	desc  string
	ok    bool
}

// Conferir compara as duas origens na competência.
func Conferir(ctx context.Context, q Consultador, empresaID string, p ParamsConferencia) (*Conferencia, error) {
	p.OrigemA, p.OrigemB = strings.TrimSpace(p.OrigemA), strings.TrimSpace(p.OrigemB)
	if p.OrigemA == "" || p.OrigemB == "" {
		return nil, errors.Join(ErrConferencia, errors.New("origem_a e origem_b são obrigatórias"))
	}
	if p.OrigemA == p.OrigemB {
		return nil, errors.Join(ErrConferencia, errors.New("origem_a e origem_b devem ser diferentes"))
	}
	if p.Tolerancia < 0 || math.IsNaN(p.Tolerancia) || math.IsInf(p.Tolerancia, 0) {
		return nil, errors.Join(ErrConferencia, errors.New("tolerância inválida"))
	}
	comp := time.Date(p.Competencia.Year(), p.Competencia.Month(), 1, 0, 0, 0, 0, time.UTC)
	filtro := NormFilialConferencia(p.Filial)

	const norm = `CASE WHEN filial ~ '^[0-9]+$' THEN COALESCE(NULLIF(LTRIM(filial, '0'), ''), '0') ELSE filial END`
	rows, err := q.QueryContext(ctx, `SELECT `+norm+` AS f, conta, origem, SUM(valor)::float8, COUNT(*), COALESCE(MAX(NULLIF(conta_desc, '')), '')
		FROM auditoria.lancamentos
		WHERE empresa_id = $1::uuid AND competencia = $2 AND origem IN ($3, $4) AND ($5 = '' OR `+norm+` = $5)
		GROUP BY 1, 2, 3`, empresaID, comp, p.OrigemA, p.OrigemB, filtro)
	if err != nil {
		return nil, err
	}
	pares := map[parConf][2]lado{}
	out := &Conferencia{
		Competencia: comp.Format("2006-01"), Tolerancia: p.Tolerancia, Filial: filtro,
		A: TotalOrigem{Origem: p.OrigemA}, B: TotalOrigem{Origem: p.OrigemB},
		Diferencas: []ItemConferencia{}, SoEmA: []ItemSoUmLado{}, SoEmB: []ItemSoUmLado{}, ResumoLinhas: []ResumoLinha{},
	}
	for rows.Next() {
		var f, conta, origem, desc string
		var v float64
		var n int
		if err := rows.Scan(&f, &conta, &origem, &v, &n, &desc); err != nil {
			rows.Close()
			return nil, err
		}
		par := pares[parConf{f, conta}]
		i, t := 0, &out.A
		if origem == p.OrigemB {
			i, t = 1, &out.B
		}
		par[i] = lado{valor: v, desc: desc, ok: true}
		pares[parConf{f, conta}] = par
		t.Total += v
		t.Contas++
		t.Linhas += n
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	out.A.Total, out.B.Total = cents(out.A.Total), cents(out.B.Total)

	desc, err := CarregarDescritorConta(ctx, q, empresaID)
	if err != nil {
		return nil, err
	}
	mapa, err := CarregarMapa(ctx, q, empresaID)
	if err != nil {
		return nil, err
	}
	linhas, err := CarregarLinhas(ctx, q, empresaID)
	if err != nil {
		return nil, err
	}
	descLinha, ordemLinha := map[string]string{}, map[string]int{}
	for _, l := range linhas {
		descLinha[l.Codigo], ordemLinha[l.Codigo] = l.Descricao, l.Ordem
	}
	descDe := func(conta string, ls ...lado) string {
		if d := desc.De(conta); d != "" {
			return d
		}
		for _, l := range ls {
			if l.desc != "" {
				return l.desc
			}
		}
		return ""
	}

	resumo := map[string]*ResumoLinha{}
	somar := func(conta string, a, b float64) {
		cod := ""
		if m, ok := ResolverConta(mapa, conta); ok {
			cod = m.LinhaCodigo
		}
		r := resumo[cod]
		if r == nil {
			r = &ResumoLinha{Codigo: cod, Descricao: descLinha[cod]}
			if cod == "" {
				r.Descricao = "Contas sem Mapa"
			} else if r.Descricao == "" {
				r.Descricao = cod
			}
			resumo[cod] = r
		}
		r.ValorA += a
		r.ValorB += b
	}

	for k, par := range pares {
		a, b := par[0], par[1]
		somar(k.conta, a.valor, b.valor)
		switch {
		case a.ok && b.ok:
			out.ParesComparados++
			dif := cents(a.valor - b.valor)
			if math.Abs(dif) > p.Tolerancia+1e-9 {
				out.todasDif = append(out.todasDif, ItemConferencia{k.filial, k.conta, descDe(k.conta, a, b), cents(a.valor), cents(b.valor), dif})
			} else {
				out.Iguais++
			}
		case a.ok:
			out.todosA = append(out.todosA, ItemSoUmLado{k.filial, k.conta, descDe(k.conta, a), cents(a.valor)})
		default:
			out.todosB = append(out.todosB, ItemSoUmLado{k.filial, k.conta, descDe(k.conta, b), cents(b.valor)})
		}
	}
	sort.Slice(out.todasDif, func(i, j int) bool {
		x, y := out.todasDif[i], out.todasDif[j]
		if ax, ay := math.Abs(x.Diferenca), math.Abs(y.Diferenca); ax != ay {
			return ax > ay
		}
		if x.Filial != y.Filial {
			return x.Filial < y.Filial
		}
		return x.Conta < y.Conta
	})
	ordSo := func(l []ItemSoUmLado) {
		sort.Slice(l, func(i, j int) bool {
			if l[i].Filial != l[j].Filial {
				return l[i].Filial < l[j].Filial
			}
			return l[i].Conta < l[j].Conta
		})
	}
	ordSo(out.todosA)
	ordSo(out.todosB)

	for _, r := range resumo {
		r.ValorA, r.ValorB = cents(r.ValorA), cents(r.ValorB)
		r.Diferenca = cents(r.ValorA - r.ValorB)
		out.ResumoLinhas = append(out.ResumoLinhas, *r)
	}
	sort.Slice(out.ResumoLinhas, func(i, j int) bool {
		x, y := out.ResumoLinhas[i], out.ResumoLinhas[j]
		if (x.Codigo == "") != (y.Codigo == "") {
			return y.Codigo == ""
		}
		if ordemLinha[x.Codigo] != ordemLinha[y.Codigo] {
			return ordemLinha[x.Codigo] < ordemLinha[y.Codigo]
		}
		return x.Codigo < y.Codigo
	})

	out.TotalDiferencas, out.TotalSoEmA, out.TotalSoEmB = len(out.todasDif), len(out.todosA), len(out.todosB)
	lim := p.Limite
	if lim <= 0 {
		lim = 200
	}
	out.Diferencas = pagina(out.todasDif, p.Offset, lim, out.Diferencas)
	out.SoEmA = pagina(out.todosA, p.Offset, lim, out.SoEmA)
	out.SoEmB = pagina(out.todosB, p.Offset, lim, out.SoEmB)

	fr, err := q.QueryContext(ctx, `SELECT EXISTS (SELECT 1 FROM auditoria.dre_fechamentos
		WHERE empresa_id = $1::uuid AND competencia = $2 AND reaberto_em IS NULL)`, empresaID, comp)
	if err != nil {
		return nil, err
	}
	if fr.Next() {
		err = fr.Scan(&out.Fechada)
	}
	fr.Close()
	if err != nil {
		return nil, err
	}
	if out.A.Linhas > 0 && out.B.Linhas > 0 {
		out.SomaNoDRE, out.Aviso = true, AvisoSomaNoDRE
	}
	return out, nil
}

func pagina[T any](todas []T, offset, limite int, vazio []T) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(todas) {
		return vazio
	}
	fim := offset + limite
	if fim > len(todas) {
		fim = len(todas)
	}
	return todas[offset:fim]
}
