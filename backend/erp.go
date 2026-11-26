package main

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/dre/fontes/oracle"
	"fb_auditoria/handlers"
)

// configurarERP liga a importação do banco do cliente quando o ambiente a
// configura (AD-15: credencial só em variável de ambiente; inquilino único).
func configurarERP(db *sql.DB, getenv func(string) string) {
	cfg, ok, err := oracle.ConfigDoAmbiente(getenv)
	empresa := strings.TrimSpace(getenv("JC_EMPRESA_ID"))
	if err != nil {
		log.Printf("[config] banco do cliente: configuração inválida: %v", err)
		return
	}
	if !ok || empresa == "" {
		log.Printf("[config] banco do cliente: não configurado")
		return
	}
	contas, err := oracle.CarregarContas(cfg.ArquivoContas)
	if err != nil {
		log.Printf("[config] banco do cliente: %v", err)
		return
	}
	cli := oracle.NovoCliente(cfg)
	erp := &dre.ImportacaoERP{
		DB: db, Ingestor: &dre.Ingestor{DB: db}, EmpresaID: empresa,
		Fontes: func(c time.Time) []dre.Fonte {
			return []dre.Fonte{oracle.NovaConsolidado(cli, c, contas), oracle.NovaRealizado(cli, c, cfg.ContasExcluidas...)}
		},
		Observacoes: observacoesERP(cfg, contas),
	}
	handlers.ERPPadrao.Store(erp)
	// 2.13: a configuração nasce desligada; o laço só verifica o relógio.
	go erp.RodarAgendador(context.Background(), time.Minute)
	if err := dre.RegistroPadrao.Registrar(erp); err != nil {
		log.Printf("[config] banco do cliente: %v", err)
	}
	log.Printf("[config] banco do cliente: configurado (%s por %s)", cfg.ObjetoRealizado, cfg.ColunaData)
}

func observacoesERP(cfg oracle.Config, contas map[string]oracle.ContaColuna) []string {
	obs := []string{
		"O CMV não vem do banco do cliente: a view consolidada não traz o CMV bruto.",
		"Realizado por conta: " + cfg.ObjetoRealizado + ", pela data " + cfg.ColunaData + "." + detalheDo(cfg),
	}
	if len(cfg.ContasExcluidas) > 0 {
		obs = append(obs, "Contas fora do DRE (não importadas): "+strings.Join(cfg.ContasExcluidas, ", ")+".")
	}
	var fora []string
	for _, col := range oracle.ColunasConsolidado {
		if contas[col].Conta == "" {
			fora = append(fora, col)
		}
	}
	if len(fora) > 0 {
		obs = append(obs, "Colunas da view consolidada que não são importadas: "+strings.Join(fora, ", ")+".")
	}
	return obs
}

func detalheDo(cfg oracle.Config) string {
	if cfg.Detalhado {
		return " Um lançamento por linha, com parceiro e nota."
	}
	return " Somado por filial, conta e dia, sem fornecedor nem número de nota."
}
