// Comando gerar-estrutura-inicial (história 3.3): lê a planilha de análise do
// cliente (abas MENSAL e METRICAS2) e grava, em uma pasta FORA do repositório,
// os quatro CSVs que as telas carregam: estrutura_dre.csv, mapa_dre.csv,
// departamentos.csv e dicionario.csv, mais contas_sem_linha.csv (relatório).
//
// Só estrutura: nenhum valor da planilha é lido, e nenhum nome de conta, código
// ou texto de linha está neste programa; tudo vem do arquivo. Os arquivos
// gerados trazem dados do cliente e nunca entram no repositório.
//
//	go run ./cmd/gerar-estrutura-inicial -arquivo "analise.xlsx" -saida ../../carga_inicial
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	arquivo := flag.String("arquivo", "", "planilha de análise do cliente (.xlsx)")
	saida := flag.String("saida", "", "pasta de saída, fora do repositório")
	abaEstrutura := flag.String("aba-estrutura", "MENSAL", "aba com o DRE (estrutura e fórmulas dos subtotais)")
	abaDicionario := flag.String("aba-dicionario", "METRICAS2", "aba com o dicionário de contas")
	colunaValor := flag.String("coluna-valor", "D", "coluna do DRE cujas fórmulas definem os subtotais")
	flag.Parse()
	if *arquivo == "" || *saida == "" {
		fmt.Fprintln(os.Stderr, "uso: gerar-estrutura-inicial -arquivo planilha.xlsx -saida pasta")
		os.Exit(2)
	}
	r, err := Gerar(Opcoes{Arquivo: *arquivo, Saida: *saida, AbaEstrutura: *abaEstrutura, AbaDicionario: *abaDicionario, ColunaValor: *colunaValor})
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
	fmt.Print(r.String())
}
