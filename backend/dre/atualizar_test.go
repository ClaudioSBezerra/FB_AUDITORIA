package dre_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"fb_auditoria/dre"
	"fb_auditoria/internal/testdb"
)

// fonteFake entrega as linhas dadas; faz o papel de uma fonte do ERP.
type fonteFake struct {
	nome  string
	lancs []dre.Lancamento
	err   error
}

func (f *fonteFake) Nome() string { return f.nome }
func (f *fonteFake) Ler(context.Context, dre.Escopo) ([]dre.Lancamento, error) {
	return f.lancs, f.err
}

func lanc(comp, filial, conta string, v float64) dre.Lancamento {
	t, _ := time.Parse("2006-01", comp)
	return dre.Lancamento{Linha: 1, Competencia: t, Filial: filial, Conta: conta, Valor: v}
}

func TestAtualizar_SemFonteNaoFingeSucesso(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	at := &dre.Atualizador{DB: db, Registro: &dre.Registro{}}
	r, err := at.Atualizar(context.Background(), dre.EscopoAtualizacao{EmpresaID: f.EmpresaID, UserID: f.UserID})
	if err != nil || r.Status != dre.AtualizacaoSemFonte || !strings.Contains(r.Mensagem, "nenhuma fonte configurada") || r.Hora.IsZero() || len(r.Fontes) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	// Trilha: pedido e conclusão, com quem pediu e o resultado.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='atualizacao_dados' AND user_id=$2::uuid`, f.EmpresaID, f.UserID); n != 2 {
		t.Errorf("trilha: %d", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='concluir' AND payload->>'status'='sem_fonte'`, f.EmpresaID); n != 1 {
		t.Errorf("resultado na trilha: %d", n)
	}
}

func TestAtualizar_FonteFakeGravaERecusaMesFechado(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	ctx := context.Background()
	reg := &dre.Registro{}
	fake := &fonteFake{nome: "erp-fake", lancs: []dre.Lancamento{lanc("2026-09", "01", "3.1", 10), lanc("2026-09", "02", "3.1", 20)}}
	if err := reg.Registrar(dre.ViaIngestor(&dre.Ingestor{DB: db}, fake)); err != nil {
		t.Fatal(err)
	}
	if err := reg.Registrar(dre.ViaIngestor(&dre.Ingestor{DB: db}, fake)); err == nil {
		t.Error("nome repetido deveria ser recusado")
	}
	at := &dre.Atualizador{DB: db, Registro: reg}
	esc := dre.EscopoAtualizacao{EmpresaID: f.EmpresaID, UserID: f.UserID}

	r, err := at.Atualizar(ctx, esc)
	if err != nil || r.Status != dre.AtualizacaoOK || len(r.Fontes) != 1 || r.Fontes[0].Linhas != 2 {
		t.Fatalf("1ª: %+v %v", r, err)
	}
	// Repetir não duplica (o Ingestor substitui o escopo).
	if r, err = at.Atualizar(ctx, esc); err != nil || r.Status != dre.AtualizacaoOK {
		t.Fatalf("2ª: %+v %v", r, err)
	}
	if n := contar(t, db, total, f.EmpresaID); n != 2 {
		t.Errorf("lançamentos: %d", n)
	}

	// Fecha setembro: a atualização é recusada pela guarda e nada muda.
	db.Exec(`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de) VALUES ($1::uuid,'RB','Receita',1,10,'conta','{}')`, f.EmpresaID)
	db.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'3.1','RB',1)`, f.EmpresaID)
	if _, err := (&dre.Fechamento{DB: db}).Fechar(ctx, dre.FecharIn{EmpresaID: f.EmpresaID, UserID: f.UserID, Ano: 2026, Mes: 9}); err != nil {
		t.Fatal(err)
	}
	fake.lancs = []dre.Lancamento{lanc("2026-09", "01", "3.1", 999)}
	r, err = at.Atualizar(ctx, esc)
	if err != nil || r.Status != dre.AtualizacaoRecusada || r.Fontes[0].Status != dre.AtualizacaoRecusada ||
		!strings.Contains(r.Fontes[0].Mensagem, "Reabra") || len(r.Fontes[0].Fechadas) == 0 {
		t.Fatalf("recusada: %+v %v", r, err)
	}
	var soma float64
	db.QueryRow(`SELECT SUM(valor) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID).Scan(&soma)
	if soma != 30 {
		t.Errorf("mês fechado mudou: %v", soma)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND acao='concluir' AND payload->>'status'='recusada'`, f.EmpresaID); n != 1 {
		t.Errorf("recusa na trilha: %d", n)
	}

	// Falha da fonte: erro, sem vazar detalhe interno.
	fake.lancs, fake.err = nil, errors.New("senha do banco: segredo")
	r, _ = at.Atualizar(ctx, esc)
	if r.Status != dre.AtualizacaoErro || strings.Contains(r.Fontes[0].Mensagem, "segredo") {
		t.Errorf("erro: %+v", r)
	}
	// Entrada inválida mostra a mensagem.
	fake.err = &dre.ErroEntrada{Msg: "arquivo vazio"}
	if r, _ = at.Atualizar(ctx, esc); r.Status != dre.AtualizacaoErro || r.Fontes[0].Mensagem != "arquivo vazio" {
		t.Errorf("entrada: %+v", r)
	}
}

func TestAtualizar_DuasFontesParcial(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	reg := &dre.Registro{}
	ing := &dre.Ingestor{DB: db}
	reg.Registrar(dre.ViaIngestor(ing, &fonteFake{nome: "boa", lancs: []dre.Lancamento{lanc("2026-08", "01", "3.1", 1)}}))
	reg.Registrar(dre.ViaIngestor(ing, &fonteFake{nome: "ruim", err: errors.New("x")}))
	r, err := (&dre.Atualizador{DB: db, Registro: reg}).Atualizar(context.Background(), dre.EscopoAtualizacao{EmpresaID: f.EmpresaID, UserID: f.UserID})
	if err != nil || r.Status != dre.AtualizacaoParcial || r.Fontes[0].Status != "ok" || r.Fontes[1].Status != "erro" {
		t.Fatalf("%+v %v", r, err)
	}
}
