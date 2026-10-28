package dre

import "context"

// EscopoDepto é o que o usuário pode ver por departamento (AD-10, história 4.7). O
// valor ZERO NÃO DÁ ACESSO: quem não foi calculado pelo servidor nega. Só
// Irrestrito=true (perfis com a capacidade ler) enxerga tudo, inclusive as
// contas sem departamento; Deptos lista os códigos de departamento permitidos
// (gestor). Escopo vazio e não irrestrito = nada.
type EscopoDepto struct {
	Irrestrito bool
	Deptos     []string
}

// Vazio diz que o escopo não alcança nenhuma conta.
func (e EscopoDepto) Vazio() bool { return !e.Irrestrito && len(e.Deptos) == 0 }

// Permite diz se as contas do departamento (código; ” = sem departamento) entram no escopo.
func (e EscopoDepto) Permite(departamento string) bool {
	if e.Irrestrito {
		return true
	}
	if departamento == "" {
		return false // "Sem departamento" nunca é de um escopo restrito
	}
	for _, d := range e.Deptos {
		if d == departamento {
			return true
		}
	}
	return false
}

// Filtrar devolve só as contas cujo departamento o escopo permite. O departamento
// de cada conta já vem resolvido por MelhorPadrao (aberto) ou do snapshot
// (fechado); nenhum casamento por prefixo acontece em SQL.
func (e EscopoDepto) Filtrar(contas []ContaDepto) []ContaDepto {
	if e.Irrestrito {
		return contas
	}
	out := make([]ContaDepto, 0, len(contas))
	if e.Vazio() {
		return out
	}
	for _, c := range contas {
		if e.Permite(c.Departamento) {
			out = append(out, c)
		}
	}
	return out
}

// ContasDoEscopo lê as contas do período por departamento e devolve só as do
// escopo. Escopo vazio devolve nada sem consultar. Em mês fechado o filtro vale
// sobre o snapshot por contas (dre_fechamento_contas.departamento).
func (l Leitor) ContasDoEscopo(ctx context.Context, empresaID string, filiais []string, p Periodo, mapa []MapaItem, ativos map[string]Ativo, esc EscopoDepto) ([]ContaDepto, []string, error) {
	if esc.Vazio() {
		return []ContaDepto{}, nil, nil
	}
	contas, semContas, err := l.ContasPorDepartamentoFiliais(ctx, empresaID, filiais, p, mapa, ativos)
	if err != nil {
		return nil, nil, err
	}
	return esc.Filtrar(contas), semContas, nil
}

// ContaNoEscopo diz se a conta tem valor no período em algum departamento do
// escopo. Conta que não existe, ou só de outro departamento, devolve false.
func (l Leitor) ContaNoEscopo(ctx context.Context, empresaID string, filiais []string, p Periodo, mapa []MapaItem, conta string, esc EscopoDepto) (bool, error) {
	if esc.Irrestrito {
		return true, nil
	}
	if esc.Vazio() {
		return false, nil
	}
	ativos, err := l.Ativos(ctx, empresaID, p)
	if err != nil {
		return false, err
	}
	contas, _, err := l.ContasDoEscopo(ctx, empresaID, filiais, p, mapa, ativos, esc)
	if err != nil {
		return false, err
	}
	for _, c := range contas {
		if c.Conta == conta {
			return true, nil
		}
	}
	return false, nil
}
