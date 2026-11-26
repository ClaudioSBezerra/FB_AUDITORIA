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
	UF          string // UF de destino, quando a fonte a traz (AD-3)
	// Detalhe do lançamento, quando a fonte o traz (história 2.24).
	DataLancamento, DataReferencia      time.Time
	ParceiroTipo, ParceiroCod, Parceiro string
	ERPID                               string
}

// Fonte é a porta de entrada de lançamentos. Nome() é gravado em
// lancamentos.origem. Ler pode devolver *ErroLinhas (com as linhas que leu) ou
// *ErroEntrada.
type Fonte interface {
	Nome() string
	Ler(ctx context.Context, e Escopo) ([]Lancamento, error)
}

// PrefixoOrigemERP marca as origens lidas do banco do cliente.
const PrefixoOrigemERP = "oracle."

// FonteERP marca a fonte que lê a competência inteira do ERP. Para ela o
// Ingestor substitui a competência toda daquela origem (uma filial que sumiu no
// reprocessamento não fica para trás) e recusa conviver com lançamentos de outra
// origem nas mesmas contas e competência.
type FonteERP interface {
	Fonte
	CompetenciaInteira()
}

// ErroFonte é falha ao ler a fonte (rede, credencial, objeto). Msg é segura para
// o usuário: nunca leva credencial nem endereço.
type ErroFonte struct{ Msg string }

func (e *ErroFonte) Error() string { return e.Msg }

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
