package csv

import (
	"context"
	"errors"
	"strings"
	"testing"

	"fb_auditoria/dre"
)

func ler(conteudo string) ([]dre.Lancamento, error) {
	return Nova(strings.NewReader(conteudo), "t.csv").Ler(context.Background(), dre.Escopo{})
}

func TestLer(t *testing.T) {
	casos := []struct {
		nome, csv string
		n         int
		entrada   string // mensagem de ErroEntrada esperada
		linhas    []int  // linhas com problema esperadas
	}{
		{nome: "ponto_e_virgula", csv: "competencia;filial;conta;valor\n2026-09;01;3.1;1.234,56\n", n: 1},
		{nome: "virgula", csv: "competencia,filial,conta,valor\n2026-09,01,3.1,1234.56\n", n: 1},
		{nome: "bom", csv: "\xef\xbb\xbfcompetencia;filial;conta;valor\n2026-09;01;3.1;1,00\n", n: 1},
		{nome: "mm_aaaa", csv: "competencia;filial;conta;valor\n09/2026;01;3.1;1,00\n", n: 1},
		{nome: "data_completa", csv: "competencia;filial;conta;valor\n2026-09-01;01;3.1;1,00\n", n: 1},
		{nome: "sem_filial", csv: "competencia;conta;valor\n2026-09;3.1;1,00\n", entrada: "coluna obrigatória ausente: filial"},
		{nome: "sem_conta", csv: "competencia;filial;valor\n2026-09;01;1,00\n", entrada: "coluna obrigatória ausente: conta"},
		{nome: "competencia_invalida", csv: "competencia;filial;conta;valor\n2026-09;01;3.1;1,00\nxx;01;3.1;1,00\n", n: 1, linhas: []int{3}},
		{nome: "nan", csv: "competencia;filial;conta;valor\n2026-09;01;3.1;NaN\n", linhas: []int{2}},
		{nome: "inf", csv: "competencia;filial;conta;valor\n2026-09;01;3.1;Inf\n", linhas: []int{2}},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			out, err := ler(c.csv)
			if c.entrada != "" {
				var ee *dre.ErroEntrada
				if !errors.As(err, &ee) || ee.Msg != c.entrada {
					t.Fatalf("esperava ErroEntrada %q: %v", c.entrada, err)
				}
				return
			}
			if c.linhas != nil {
				var el *dre.ErroLinhas
				if !errors.As(err, &el) || len(el.Problemas) != len(c.linhas) {
					t.Fatalf("esperava problemas nas linhas %v: %v", c.linhas, err)
				}
				for i, l := range c.linhas {
					if el.Problemas[i].Linha != l {
						t.Errorf("problema %d: linha %d, esperada %d", i, el.Problemas[i].Linha, l)
					}
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(out) != c.n {
				t.Errorf("linhas válidas: %d, esperado %d", len(out), c.n)
			}
		})
	}
}
