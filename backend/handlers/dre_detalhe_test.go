package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"fb_auditoria/internal/testdb"
)

type detConta struct {
	Total            float64  `json:"total"`
	SomaLancamentos  float64  `json:"soma_lancamentos"`
	Divergente       bool     `json:"divergente"`
	AvisoDivergencia string   `json:"aviso_divergencia"`
	MesesFechados    []string `json:"meses_fechados"`
	TotalLancamentos int      `json:"total_lancamentos"`
	Lancamentos      []struct {
		ID          int64   `json:"id"`
		Competencia string  `json:"competencia"`
		Filial      string  `json:"filial"`
		TipoDoc     string  `json:"tipo_doc"`
		DocRef      string  `json:"doc_ref"`
		Historico   string  `json:"historico"`
		Valor       float64 `json:"valor"`
	} `json:"lancamentos"`
}

func lancDoc(t *testing.T, f testdb.Fixture, comp, filial, conta, doc, hist string, valor float64) {
	t.Helper()
	exec(t, `INSERT INTO auditoria.lancamentos (empresa_id, competencia, filial, conta, valor, tipo_doc, doc_ref, historico)
		VALUES ($1::uuid,$2,$3,$4,$5,'nota',$6,$7)`, f.EmpresaID, comp, filial, conta, valor, doc, hist)
}

func chamarDet(t *testing.T, f testdb.Fixture, perfil, url string) (int, detConta) {
	t.Helper()
	w := chamar(DREContaLancamentosHandler(testdb.Open(t)), reqFilial(http.MethodGet, url, "", f, perfil))
	var d detConta
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, d
}

const urlDet = "/x?ano=2026&mes=9&visao=mensal&conta=3.1"

func TestDetalheConta_MesAbertoEContasDaLinha(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	lancDoc(t, f, "2026-09-01", "01", "3.1", "NF1", "Venda a Fulano de Tal", 100)
	lancDoc(t, f, "2026-09-01", "02", "3.1", "NF2", "Venda B", 50)
	lancDoc(t, f, "2026-09-01", "01", "3.2", "NF3", "Devolução", 10)

	code, d := chamarDet(t, f, "controladoria", urlDet)
	if code != http.StatusOK || d.Total != 150 || d.SomaLancamentos != 150 || d.Divergente || len(d.Lancamentos) != 2 || d.TotalLancamentos != 2 {
		t.Fatalf("aberto: %d %+v", code, d)
	}
	if l := d.Lancamentos[0]; l.Historico != "Venda a Fulano de Tal" || l.DocRef != "NF1" || l.TipoDoc != "nota" || l.Filial != "01" || l.Competencia != "2026-09" {
		t.Errorf("campos do lançamento: %+v", l)
	}
	// Filial: só os lançamentos dela.
	if _, d1 := chamarDet(t, f, "diretoria", urlDet+"&filial=02"); d1.Total != 50 || len(d1.Lancamentos) != 1 {
		t.Errorf("filial 02: %+v", d1)
	}
	// Contas da linha FL (subtotal) = RB + DED.
	w := chamar(DREContasHandler(db), reqFilial(http.MethodGet, "/x?ano=2026&mes=9&visao=mensal&linha=FL", "", f, "auditoria"))
	var cs struct {
		Contas []struct {
			Conta string  `json:"conta"`
			Valor float64 `json:"valor"`
		} `json:"contas"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &cs) != nil || len(cs.Contas) != 2 || cs.Contas[0].Conta != "3.1" || cs.Contas[0].Valor != 150 || cs.Contas[1].Valor != 10 {
		t.Fatalf("contas: %d %s", w.Code, w.Body.String())
	}
	// Conta fora do Mapa: 404; linha inexistente: 404.
	if code, _ := chamarDet(t, f, "controladoria", "/x?ano=2026&mes=9&conta=9.9"); code != http.StatusNotFound {
		t.Errorf("conta sem mapa: %d", code)
	}
	w = chamar(DREContasHandler(db), reqFilial(http.MethodGet, "/x?ano=2026&mes=9&linha=ZZ", "", f, "controladoria"))
	if w.Code != http.StatusNotFound {
		t.Errorf("linha desconhecida: %d", w.Code)
	}
}

func TestDetalheConta_MesFechadoIgualEDivergente(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	lancDoc(t, f, "2026-09-01", "01", "3.1", "NF1", "H1", 100)
	lancDoc(t, f, "2026-09-01", "02", "3.1", "NF2", "H2", 50)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	// Igual ao snapshot: sem aviso, lançamentos vêm de lancamentos ao vivo.
	code, d := chamarDet(t, f, "controladoria", urlDet)
	if code != 200 || d.Total != 150 || d.Divergente || d.AvisoDivergencia != "" || len(d.Lancamentos) != 2 || len(d.MesesFechados) != 1 {
		t.Fatalf("fechado igual: %d %+v", code, d)
	}
	// Mudar o Mapa depois não altera o total do mês fechado.
	exec(t, `UPDATE auditoria.dre_mapa SET sinal=-1 WHERE empresa_id=$1::uuid AND conta_padrao='3.1'`, f.EmpresaID)
	if _, d := chamarDet(t, f, "controladoria", urlDet); d.Total != 150 {
		t.Errorf("total depois do Mapa: %v", d.Total)
	}
	exec(t, `UPDATE auditoria.dre_mapa SET sinal=1 WHERE empresa_id=$1::uuid AND conta_padrao='3.1'`, f.EmpresaID)

	// Divergência: lançamento novo por fora da guarda; total fica o do snapshot e a resposta avisa.
	lancDoc(t, f, "2026-09-01", "01", "3.1", "NF9", "Depois do fechamento", 5000)
	_, d = chamarDet(t, f, "controladoria", urlDet)
	if d.Total != 150 || !d.Divergente || d.AvisoDivergencia == "" || d.SomaLancamentos != 5150 || len(d.Lancamentos) != 3 {
		t.Fatalf("divergente: %+v", d)
	}
	// Filial 02 não foi tocada: sem divergência.
	if _, d2 := chamarDet(t, f, "controladoria", urlDet+"&filial=02"); d2.Divergente || d2.Total != 50 {
		t.Errorf("filial 02: %+v", d2)
	}

	// Reaberto e atualizado: vê os lançamentos novos, total ao vivo.
	if w := reabrir(t, f, "controladoria", `{"ano":2026,"mes":9,"motivo":"ajuste"}`); w.Code != 200 {
		t.Fatalf("reabrir: %d", w.Code)
	}
	_, d = chamarDet(t, f, "controladoria", urlDet)
	if d.Total != 5150 || d.Divergente || len(d.MesesFechados) != 0 {
		t.Errorf("reaberto: %+v", d)
	}
}

func TestDetalheConta_AgrupamentoEPaginacao(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	for _, c := range []string{`{"codigo":"G1","nome":"G","filiais":["01","02"]}`, `{"codigo":"G2","nome":"G","filiais":["03"]}`} {
		if w := chamar(FiliaisGerenciaisHandler(db), reqFilial(http.MethodPost, "/x", c, f, "controladoria")); w.Code != http.StatusCreated {
			t.Fatal(w.Body.String())
		}
	}
	if w := chamar(AgrupamentosFiliaisHandler(db), reqFilial(http.MethodPost, "/x", `{"codigo":"A1","nome":"A","filiais":["G1"]}`, f, "controladoria")); w.Code != http.StatusCreated {
		t.Fatal(w.Body.String())
	}
	for i, fil := range []string{"01", "01", "02", "03", "04"} {
		lancDoc(t, f, "2026-09-01", fil, "3.1", "NF"+string(rune('A'+i)), "h", 10)
	}
	if _, d := chamarDet(t, f, "controladoria", urlDet+"&agrupamento=A1"); d.Total != 30 || d.TotalLancamentos != 3 {
		t.Errorf("agrupamento: %+v", d)
	}
	if _, d := chamarDet(t, f, "controladoria", urlDet+"&grupo=1"); d.Total != 50 {
		t.Errorf("grupo: %+v", d)
	}
	// Paginação: total_lancamentos é o do recorte, a página é limitada.
	_, d := chamarDet(t, f, "controladoria", urlDet+"&grupo=1&limite=2&offset=4")
	if d.TotalLancamentos != 5 || len(d.Lancamentos) != 1 {
		t.Errorf("página: %+v", d)
	}
	for _, q := range []string{"&limite=0", "&limite=5000", "&offset=-1", "&limite=x", "&agrupamento=A1&filial=01"} {
		if code, _ := chamarDet(t, f, "controladoria", urlDet+q); code != http.StatusBadRequest {
			t.Errorf("%s: %d", q, code)
		}
	}
	if code, _ := chamarDet(t, f, "controladoria", urlDet+"&agrupamento=XX"); code != http.StatusNotFound {
		t.Errorf("agrupamento inexistente: %d", code)
	}
	// Outra empresa não enxerga os lançamentos.
	outra := testdb.NovaEmpresa(t, db)
	if _, d := chamarDet(t, outra, "controladoria", urlDet); d.TotalLancamentos != 0 {
		t.Errorf("outra empresa: %+v", d)
	}
}

func TestDetalheConta_GestorRecebe403(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	estruturaPct(t, f, true)
	lancDoc(t, f, "2026-09-01", "01", "3.1", "NF1", "h", 10)
	for _, perfil := range []string{"gestor", "", "qualquer"} {
		if code, d := chamarDet(t, f, perfil, urlDet); code != http.StatusForbidden || len(d.Lancamentos) != 0 {
			t.Errorf("lançamentos, perfil %q: %d", perfil, code)
		}
		w := chamar(DREContasHandler(db), reqFilial(http.MethodGet, "/x?ano=2026&mes=9&linha=RB", "", f, perfil))
		if w.Code != http.StatusForbidden {
			t.Errorf("contas, perfil %q: %d", perfil, w.Code)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade IN ('dre_contas','dre_lancamentos') AND acao='negado'`, f.EmpresaID); n != 6 {
		t.Errorf("negativas registradas = %d", n)
	}
}
