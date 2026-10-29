package dre

import (
	"context"
	"sort"

	"github.com/lib/pq"
)

// ContaSemMapa é uma conta com lançamento no período que o Mapa não liga a
// nenhuma linha do DRE, com a soma bruta dos lançamentos.
type ContaSemMapa struct {
	Conta     string  `json:"conta"`
	Descricao string  `json:"descricao"`
	Valor     float64 `json:"valor"`
}

// ContasSemMapa lista as contas sem Mapa do período, maiores valores (em
// módulo) primeiro. Só olha meses ao vivo: mês fechado não aceita conta sem
// Mapa (o fechamento recusa), então o snapshot não tem o que listar.
func (l Leitor) ContasSemMapa(ctx context.Context, empresaID string, p Periodo, mapa []MapaItem) ([]ContaSemMapa, error) {
	lei, err := l.Ler(ctx, empresaID, "", p, nil, mapa)
	if err != nil {
		return nil, err
	}
	out := make([]ContaSemMapa, 0, len(lei.Sem))
	contas := make([]string, 0, len(lei.Sem))
	for c, v := range lei.Sem {
		out = append(out, ContaSemMapa{Conta: c, Valor: arredondaCentavos(v)})
		contas = append(contas, c)
	}
	if len(contas) > 0 {
		rows, err := l.Q.QueryContext(ctx, `SELECT conta, COALESCE(MAX(NULLIF(conta_desc,'')),'') FROM auditoria.lancamentos
			WHERE empresa_id=$1::uuid AND competencia BETWEEN $2 AND $3 AND conta = ANY($4) GROUP BY conta`,
			empresaID, p.Ini, p.Fim, pq.Array(contas))
		if err != nil {
			return nil, err
		}
		desc := map[string]string{}
		for rows.Next() {
			var c, d string
			if err := rows.Scan(&c, &d); err != nil {
				rows.Close()
				return nil, err
			}
			desc[c] = d
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		for i := range out {
			out[i].Descricao = desc[out[i].Conta]
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Valor, out[j].Valor
		if a < 0 {
			a = -a
		}
		if b < 0 {
			b = -b
		}
		if a != b {
			return a > b
		}
		return out[i].Conta < out[j].Conta
	})
	return out, nil
}
