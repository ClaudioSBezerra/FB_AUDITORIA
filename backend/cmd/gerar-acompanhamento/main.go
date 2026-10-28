// Comando gerar-acompanhamento regenera backend/acompanhamento/dados.json a
// partir de _bmad-output (epics.md e sprint-status.yaml). Rode a partir de backend/:
//
//	go run ./cmd/gerar-acompanhamento
package main

import (
	"fmt"
	"os"

	"fb_auditoria/acompanhamento"
)

func main() {
	raiz, destino := "../_bmad-output", "acompanhamento/dados.json"
	if len(os.Args) > 1 {
		raiz = os.Args[1]
	}
	mudou, err := acompanhamento.Escrever(raiz, destino)
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
	if mudou {
		fmt.Println("dados.json atualizado")
	} else {
		fmt.Println("dados.json já estava em dia")
	}
}
