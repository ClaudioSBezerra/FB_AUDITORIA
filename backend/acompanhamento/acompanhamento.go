// Package acompanhamento serve o quadro Kanban somente leitura do andamento do
// projeto (história 4.10). Os dados vêm de dados.json, gerado a partir de
// epics.md e sprint-status.yaml (ver gerador.go) e embutido no binário, porque o
// container de produção não tem _bmad-output.
package acompanhamento

import (
	_ "embed"
	"encoding/json"
)

//go:embed dados.json
var dadosJSON []byte

// Colunas do quadro, na ordem de exibição.
const (
	ColBacklog      = "Backlog"
	ColPronto       = "Pronto"
	ColEmAndamento  = "Em andamento"
	ColEmRevisao    = "Em revisão"
	ColFeito        = "Feito"
	ReleaseAtual    = "A1"
	formatoDataGera = "2006-01-02"
)

type Epico struct {
	Numero int    `json:"numero"`
	Nome   string `json:"nome"`
}

type Historia struct {
	ID               string   `json:"id"`
	Titulo           string   `json:"titulo"`
	Epico            Epico    `json:"epico"`
	Release          string   `json:"release"`
	Status           string   `json:"status"`
	BloqueadaCliente bool     `json:"bloqueada_cliente"`
	MotivoBloqueio   string   `json:"motivo_bloqueio,omitempty"`
	Resumo           string   `json:"resumo"`
	Criterios        []string `json:"criterios"`
}

type Dados struct {
	GeradoEm  string     `json:"gerado_em"`
	Versao    string     `json:"versao"`
	Historias []Historia `json:"historias"`
}

// Embutido devolve os dados versionados que foram embutidos no binário.
func Embutido() (Dados, error) {
	var d Dados
	err := json.Unmarshal(dadosJSON, &d)
	return d, err
}

// JSON devolve os bytes embutidos, para a rota servir sem reserializar.
func JSON() []byte { return dadosJSON }
