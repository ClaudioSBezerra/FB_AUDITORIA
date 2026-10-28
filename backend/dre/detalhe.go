package dre

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/lib/pq"
)

// ContaDaLinha é o realizado de uma conta dentro de uma linha do DRE, já com o
// sinal do Mapa.
type ContaDaLinha struct {
	Conta string
	Linha string
	Valor float64
}

// ContasDaLinha lista as contas das linhas em alvo (a linha 'conta' ou, num
// subtotal, as que o compõem) no recorte e período, somando as filiais. Mês
// fechado vem do snapshot das contas, mês aberto ao vivo (mesma leitura da visão
// por departamento, sem expor o departamento). semContas traz os meses de
// fechamento legado, que não têm contas congeladas.
func (l Leitor) ContasDaLinha(ctx context.Context, empresaID string, filiais []string, p Periodo, mapa []MapaItem, ativos map[string]Ativo, alvo map[string]bool) ([]ContaDaLinha, []string, error) {
	contas, semContas, err := l.ContasPorDepartamentoFiliais(ctx, empresaID, filiais, p, mapa, ativos)
	if err != nil {
		return nil, nil, err
	}
	acc := map[string]*ContaDaLinha{}
	for _, c := range contas {
		if !alvo[c.Linha] {
			continue
		}
		if x, ok := acc[c.Conta]; ok {
			x.Valor += c.Valor
			continue
		}
		acc[c.Conta] = &ContaDaLinha{Conta: c.Conta, Linha: c.Linha, Valor: c.Valor}
	}
	out := make([]ContaDaLinha, 0, len(acc))
	for _, c := range acc {
		c.Valor = arredondaCentavos(c.Valor)
		if c.Valor != 0 {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Conta < out[j].Conta })
	return out, semContas, nil
}

// LancamentoConta é um lançamento de uma conta, como gravado (histórico
// inclusive), com o valor no sinal do Mapa.
type LancamentoConta struct {
	ID          int64   `json:"id"`
	Competencia string  `json:"competencia"`
	Filial      string  `json:"filial"`
	CentroCusto string  `json:"centro_custo"`
	Canal       string  `json:"canal"`
	Origem      string  `json:"origem"`
	TipoDoc     string  `json:"tipo_doc"`
	DocRef      string  `json:"doc_ref"`
	Historico   string  `json:"historico"`
	Valor       float64 `json:"valor"`
}

// Divergencia é um mês fechado em que a soma dos lançamentos ao vivo difere do
// valor congelado da conta.
type Divergencia struct {
	Competencia string  `json:"competencia"`
	Congelado   float64 `json:"congelado"`
	AoVivo      float64 `json:"ao_vivo"`
	Diferenca   float64 `json:"diferenca"`
}

// DetalheConta é o resultado de Leitor.DetalheConta.
type DetalheConta struct {
	Conta            string
	Linha            string
	Sinal            int
	Total            float64 // mês fechado: do snapshot; mês aberto: ao vivo
	SomaLancamentos  float64 // soma ao vivo de todos os lançamentos do período (não só da página)
	TotalLancamentos int
	Lancamentos      []LancamentoConta
	Divergencias     []Divergencia
	MesesFechados    []string // meses do período lidos do snapshot
	SemSnapshot      []string // meses de fechamento legado: sem valor congelado da conta
}

// ErrContaSemMapa indica conta que o Mapa não liga a nenhuma linha (fora do DRE).
type ErrContaSemMapa struct{ Conta string }

func (e *ErrContaSemMapa) Error() string { return "A conta " + e.Conta + " não está no Mapa do DRE" }

// DetalheConta lê os lançamentos de uma conta no recorte e período. O TOTAL é o
// do DRE (mês fechado do snapshot); os lançamentos são sempre lidos ao vivo,
// o que é consistente porque a guarda de período (1.6) impede escrita em mês
// fechado. Se a soma ao vivo de um mês fechado diferir do valor congelado, o mês
// vem em Divergencias. limite/offset paginam os lançamentos.
func (l Leitor) DetalheConta(ctx context.Context, empresaID string, filiais []string, p Periodo, conta string, mapa []MapaItem, limite, offset int) (DetalheConta, error) {
	m, ok := ResolverConta(mapa, conta)
	if !ok {
		return DetalheConta{}, &ErrContaSemMapa{Conta: conta}
	}
	sinal := float64(m.Sinal)
	out := DetalheConta{Conta: conta, Linha: m.LinhaCodigo, Sinal: m.Sinal, Lancamentos: []LancamentoConta{}, Divergencias: []Divergencia{}, MesesFechados: []string{}, SemSnapshot: []string{}}
	ativos, err := l.Ativos(ctx, empresaID, p)
	if err != nil {
		return DetalheConta{}, err
	}
	todas := len(filiais) == 1 && filiais[0] == FilialGrupo
	filtro := pq.Array(filiais)

	// Soma ao vivo por mês (todas as competências do período).
	vivo := map[string]float64{}
	rows, err := l.Q.QueryContext(ctx, `SELECT competencia, SUM(valor) FROM auditoria.lancamentos
		WHERE empresa_id = $1::uuid AND conta = $2 AND competencia BETWEEN $3 AND $4 AND ($5 OR filial = ANY($6))
		GROUP BY competencia`, empresaID, conta, p.Ini, p.Fim, todas, filtro)
	if err != nil {
		return DetalheConta{}, err
	}
	for rows.Next() {
		var comp time.Time
		var v float64
		if err := rows.Scan(&comp, &v); err != nil {
			rows.Close()
			return DetalheConta{}, err
		}
		vivo[chaveMes(comp)] += v * sinal
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return DetalheConta{}, err
	}
	rows.Close()

	for _, mes := range meses(p) {
		k := chaveMes(mes)
		a, fechado := ativos[k]
		switch {
		case fechado && a.Completo:
			var cong float64
			for _, f := range filiais {
				var v float64
				if err := lerLinha(ctx, l.Q, `SELECT COALESCE(SUM(realizado),0) FROM auditoria.dre_fechamento_contas
					WHERE fechamento_id = $1 AND filial = $2 AND conta = $3`, []any{a.ID, f, conta}, &v); err != nil {
					return DetalheConta{}, err
				}
				cong += v
			}
			cong = arredondaCentavos(cong)
			ao := arredondaCentavos(vivo[k])
			out.Total += cong
			out.MesesFechados = append(out.MesesFechados, k)
			if math.Abs(cong-ao) >= 0.005 {
				out.Divergencias = append(out.Divergencias, Divergencia{Competencia: k, Congelado: cong, AoVivo: ao, Diferenca: arredondaCentavos(ao - cong)})
			}
		case fechado && a.OrigemFiliais(filiais) == Legado:
			out.SemSnapshot = append(out.SemSnapshot, k)
			out.Total += vivo[k]
		default:
			out.Total += vivo[k]
		}
	}
	out.Total = arredondaCentavos(out.Total)

	// Contagem exata, soma e a página de lançamentos.
	if err := lerLinha(ctx, l.Q, `SELECT COUNT(*), COALESCE(SUM(valor),0) FROM auditoria.lancamentos
		WHERE empresa_id = $1::uuid AND conta = $2 AND competencia BETWEEN $3 AND $4 AND ($5 OR filial = ANY($6))`,
		[]any{empresaID, conta, p.Ini, p.Fim, todas, filtro}, &out.TotalLancamentos, &out.SomaLancamentos); err != nil {
		return DetalheConta{}, err
	}
	out.SomaLancamentos = arredondaCentavos(out.SomaLancamentos * sinal)
	rows, err = l.Q.QueryContext(ctx, `SELECT id, competencia, filial, centro_custo, canal, origem, tipo_doc, doc_ref, historico, valor
		FROM auditoria.lancamentos
		WHERE empresa_id = $1::uuid AND conta = $2 AND competencia BETWEEN $3 AND $4 AND ($5 OR filial = ANY($6))
		ORDER BY competencia, id LIMIT $7 OFFSET $8`, empresaID, conta, p.Ini, p.Fim, todas, filtro, limite, offset)
	if err != nil {
		return DetalheConta{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var x LancamentoConta
		var comp time.Time
		if err := rows.Scan(&x.ID, &comp, &x.Filial, &x.CentroCusto, &x.Canal, &x.Origem, &x.TipoDoc, &x.DocRef, &x.Historico, &x.Valor); err != nil {
			return DetalheConta{}, err
		}
		x.Competencia = chaveMes(comp)
		x.Valor *= sinal
		out.Lancamentos = append(out.Lancamentos, x)
	}
	return out, rows.Err()
}

// lerLinha executa uma consulta que devolve uma linha e a escaneia em dest.
func lerLinha(ctx context.Context, q Consultador, query string, args []any, dest ...any) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			return err
		}
	}
	return rows.Err()
}
