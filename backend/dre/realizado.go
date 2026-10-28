package dre

import (
	"context"
	"database/sql"
	"math"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Linha é uma linha da estrutura do DRE.
type Linha struct {
	ID        int64    `json:"id,omitempty"`
	Codigo    string   `json:"codigo"`
	Descricao string   `json:"descricao"`
	Nivel     int      `json:"nivel"`
	Ordem     int      `json:"ordem"`
	Tipo      string   `json:"tipo"`
	SomaDe    []string `json:"soma_de"`
	Ativo     bool     `json:"ativo"`
	// Papel marca a linha-base de percentuais ('' = nenhum); ver PapelFaturamentoLiquido.
	Papel string `json:"papel,omitempty"`
}

// MapaItem liga uma conta padrão (ou prefixo terminado em '%') a uma linha.
type MapaItem struct {
	ContaPadrao string `json:"conta_padrao"`
	LinhaCodigo string `json:"linha_codigo"`
	Sinal       int    `json:"sinal"`
}

// Periodo é um intervalo de competências (ambas as pontas inclusivas).
type Periodo struct{ Ini, Fim time.Time }

// Consultador é satisfeito por *sql.DB e *sql.Tx.
type Consultador interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// CarregarLinhas lê as linhas ativas da estrutura (sem semear).
func CarregarLinhas(ctx context.Context, q Consultador, empresaID string) ([]Linha, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, codigo, descricao, nivel, ordem, tipo, soma_de, ativo, COALESCE(papel,'')
		FROM auditoria.dre_linhas WHERE empresa_id = $1::uuid AND ativo ORDER BY ordem`, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Linha
	for rows.Next() {
		var l Linha
		var soma pq.StringArray
		if err := rows.Scan(&l.ID, &l.Codigo, &l.Descricao, &l.Nivel, &l.Ordem, &l.Tipo, &soma, &l.Ativo, &l.Papel); err != nil {
			return nil, err
		}
		l.SomaDe = []string(soma)
		out = append(out, l)
	}
	return out, rows.Err()
}

// CarregarMapa lê o de-para conta → linha.
func CarregarMapa(ctx context.Context, q Consultador, empresaID string) ([]MapaItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT conta_padrao, linha_codigo, sinal FROM auditoria.dre_mapa
		WHERE empresa_id = $1::uuid ORDER BY conta_padrao`, empresaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MapaItem{}
	for rows.Next() {
		var m MapaItem
		if err := rows.Scan(&m.ContaPadrao, &m.LinhaCodigo, &m.Sinal); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MelhorPadrao é a regra única de resolução de conta (Mapa e dicionário):
// casamento exato vence; senão o prefixo ('%' no fim) mais longo. Devolve o
// índice do padrão escolhido entre n, ou -1.
func MelhorPadrao(n int, padrao func(i int) string, conta string) int {
	best, bestLen := -1, -1
	for i := 0; i < n; i++ {
		p := padrao(i)
		if p == conta {
			return i
		}
		if strings.HasSuffix(p, "%") {
			pre := strings.TrimSuffix(p, "%")
			if strings.HasPrefix(conta, pre) && len(pre) > bestLen {
				best, bestLen = i, len(pre)
			}
		}
	}
	return best
}

// ResolverConta: casamento exato vence; senão o prefixo ('%') mais longo.
func ResolverConta(mapa []MapaItem, conta string) (MapaItem, bool) {
	i := MelhorPadrao(len(mapa), func(i int) string { return mapa[i].ContaPadrao }, conta)
	if i < 0 {
		return MapaItem{}, false
	}
	return mapa[i], true
}

// somaPorConta soma os lançamentos por conta (valor bruto, sem sinal do mapa).
func somaPorConta(ctx context.Context, q Consultador, empresaID, filial string, p Periodo) (map[string]float64, error) {
	rows, err := q.QueryContext(ctx, `SELECT conta, SUM(valor) FROM auditoria.lancamentos
		WHERE empresa_id = $1::uuid AND competencia BETWEEN $2 AND $3 AND ($4 = '' OR filial = $4)
		GROUP BY conta`, empresaID, p.Ini, p.Fim, filial)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var conta string
		var v float64
		if err := rows.Scan(&conta, &v); err != nil {
			return nil, err
		}
		out[conta] = v
	}
	return out, rows.Err()
}

// Realizado devolve o realizado já normalizado (sinal do mapa) por linha
// 'conta', além do que ficou sem mapa. É o único caminho que calcula o
// realizado exibido ou congelado.
func Realizado(ctx context.Context, q Consultador, empresaID, filial string, p Periodo, mapa []MapaItem) (map[string]float64, map[string]float64, error) {
	contas, err := somaPorConta(ctx, q, empresaID, filial, p)
	if err != nil {
		return nil, nil, err
	}
	por := map[string]float64{}
	sem := map[string]float64{}
	for conta, v := range contas {
		if m, ok := ResolverConta(mapa, conta); ok {
			por[m.LinhaCodigo] += v * float64(m.Sinal)
		} else {
			sem[conta] += v
		}
	}
	return por, sem, nil
}

// ContaValor é o realizado de uma conta já normalizado, com a linha do mapa.
type ContaValor struct {
	Linha string
	Valor float64
}

// RealizadoPorConta é o detalhe por conta do mesmo cálculo de Realizado
// (contas sem mapa ficam de fora).
func RealizadoPorConta(ctx context.Context, q Consultador, empresaID, filial string, p Periodo, mapa []MapaItem) (map[string]ContaValor, error) {
	contas, err := somaPorConta(ctx, q, empresaID, filial, p)
	if err != nil {
		return nil, err
	}
	out := map[string]ContaValor{}
	for conta, v := range contas {
		if m, ok := ResolverConta(mapa, conta); ok {
			out[conta] = ContaValor{Linha: m.LinhaCodigo, Valor: v * float64(m.Sinal)}
		}
	}
	return out, nil
}

// AplicarSubtotais completa as linhas 'subtotal' a partir de soma_de ("-X" subtrai).
func AplicarSubtotais(linhas []Linha, v map[string]float64) {
	for _, l := range linhas {
		if l.Tipo != "subtotal" {
			continue
		}
		var s float64
		for _, c := range l.SomaDe {
			if strings.HasPrefix(c, "-") {
				s -= v[strings.TrimPrefix(c, "-")]
			} else {
				s += v[c]
			}
		}
		v[l.Codigo] = arredondaCentavos(s)
	}
}

// arredondaCentavos elimina o resíduo binário da soma de valores de 2 casas
// (0,1+0,2 vira 0,3 exato), de modo que o subtotal seja exatamente a soma.
func arredondaCentavos(x float64) float64 { return math.Round(x*100) / 100 }
