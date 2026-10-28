package dre

import (
	"context"
	"testing"
)

func contasDeTeste() []ContaDepto {
	return []ContaDepto{
		{Conta: "4.1", Departamento: "A", Valor: 10},
		{Conta: "4.2", Departamento: "B", Valor: 20},
		{Conta: "4.3", Departamento: "C", Valor: 30},
		{Conta: "5.1", Departamento: "", Valor: 40},
	}
}

// O valor zero não dá acesso a nada, nem às contas sem departamento.
func TestEscopo_ValorZeroNega(t *testing.T) {
	var zero EscopoDepto
	if !zero.Vazio() {
		t.Error("valor zero deveria ser vazio")
	}
	for _, d := range []string{"A", "", "%", "A%"} {
		if zero.Permite(d) {
			t.Errorf("valor zero permitiu %q", d)
		}
	}
	if got := zero.Filtrar(contasDeTeste()); len(got) != 0 {
		t.Errorf("valor zero devolveu %d contas", len(got))
	}
	// Leitor com escopo vazio nem consulta (Q nil não pode ser tocado).
	l := Leitor{}
	contas, sem, err := l.ContasDoEscopo(context.Background(), "e", []string{"01"}, Periodo{}, nil, nil, zero)
	if err != nil || len(contas) != 0 || len(sem) != 0 {
		t.Errorf("ContasDoEscopo(zero) = %v %v %v", contas, sem, err)
	}
	if ok, err := l.ContaNoEscopo(context.Background(), "e", []string{"01"}, Periodo{}, nil, "4.1", zero); ok || err != nil {
		t.Errorf("ContaNoEscopo(zero) = %v %v", ok, err)
	}
	// Deptos vazio e não irrestrito também é nada.
	if !(EscopoDepto{Deptos: []string{}}).Vazio() {
		t.Error("Deptos vazio deveria ser vazio")
	}
}

func TestEscopo_RestritoEIrrestrito(t *testing.T) {
	ab := EscopoDepto{Deptos: []string{"A", "B"}}
	got := ab.Filtrar(contasDeTeste())
	if len(got) != 2 || got[0].Conta != "4.1" || got[1].Conta != "4.2" {
		t.Errorf("A e B: %+v", got)
	}
	for _, d := range []string{"", "C", "a", "A%", "%"} {
		if ab.Permite(d) {
			t.Errorf("A,B permitiu %q", d)
		}
	}
	// o código do escopo é comparado por igualdade, nunca como padrão
	pct := EscopoDepto{Deptos: []string{"%"}}
	if got := pct.Filtrar(contasDeTeste()); len(got) != 0 {
		t.Errorf("departamento '%%' vazou %d contas", len(got))
	}
	todos := EscopoDepto{Irrestrito: true}
	if got := todos.Filtrar(contasDeTeste()); len(got) != 4 || !todos.Permite("") {
		t.Errorf("irrestrito: %d", len(got))
	}
}
