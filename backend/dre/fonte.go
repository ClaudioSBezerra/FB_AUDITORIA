// Package dre concentra a regra do DRE gerencial. Só o Ingestor escreve em
// lancamentos; as fontes (dre/fontes/...) apenas leem e entregam linhas.
package dre

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Escopo é o pedido de leitura feito a uma fonte. Para o CSV traz só a
// empresa: os escopos de substituição saem das próprias linhas.
type Escopo struct {
	EmpresaID string
}

// Lancamento é uma linha lida de uma fonte. Linha é a posição no arquivo de
// origem (0 quando a fonte não tem esse conceito), usada nas mensagens.
type Lancamento struct {
	Linha       int
	Competencia time.Time
	Filial      string
	CentroCusto string
	Canal       string
	Conta       string
	ContaDesc   string
	Valor       float64
	TipoDoc     string
	DocRef      string
	Historico   string
}

// Fonte é a porta de entrada de lançamentos. Nome() é gravado em
// lancamentos.origem. Ler pode devolver *ErroLinhas (com as linhas que leu) ou
// *ErroEntrada.
type Fonte interface {
	Nome() string
	Ler(ctx context.Context, e Escopo) ([]Lancamento, error)
}

// ErroEntrada é uma entrada recusada por ser inválida como um todo
// (cabeçalho, arquivo vazio). O handler a mapeia para 400.
type ErroEntrada struct{ Msg string }

func (e *ErroEntrada) Error() string { return e.Msg }

// Problema descreve uma linha recusada.
type Problema struct {
	Linha int
	Msg   string
}

// ErroLinhas reúne as linhas com problema; nada é gravado.
type ErroLinhas struct{ Problemas []Problema }

const maxProblemasListados = 20

func (e *ErroLinhas) Error() string {
	var b strings.Builder
	total := len(e.Problemas)
	fmt.Fprintf(&b, "%d linha(s) com problema; nada foi gravado", total)
	for i, p := range e.Problemas {
		if i == maxProblemasListados {
			fmt.Fprintf(&b, "\n... e mais %d", total-maxProblemasListados)
			break
		}
		fmt.Fprintf(&b, "\nlinha %d: %s", p.Linha, p.Msg)
	}
	return b.String()
}
