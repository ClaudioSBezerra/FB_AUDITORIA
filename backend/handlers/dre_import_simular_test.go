package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fb_auditoria/internal/testdb"
)

// História 2.19: simulação (?simular=1), resumo por competência/filial e avisos.
// Dados fictícios. As permissões (CapCarregar) são do middleware e estão na
// tabela de rotas (rotas_capacidades_test.go); a simulação usa a mesma rota.

type respImport struct {
	Simulacao bool   `json:"simulacao"`
	CargaID   int64  `json:"carga_id"`
	Linhas    int    `json:"linhas"`
	Origem    string `json:"origem"`
	Pares     []struct {
		Competencia string `json:"competencia"`
		Filial      string `json:"filial"`
		Lancamentos int    `json:"lancamentos"`
	} `json:"pares"`
	Avisos []struct {
		Codigo   string `json:"codigo"`
		Mensagem string `json:"mensagem"`
	} `json:"avisos"`
	Conferencia *struct {
		OK bool `json:"ok"`
	} `json:"conferencia_total"`
}

func importarModo(t *testing.T, f testdb.Fixture, nome, conteudo string, simular bool) (*httptest.ResponseRecorder, respImport) {
	t.Helper()
	db := testdb.Open(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("arquivo", nome)
	part.Write([]byte(conteudo))
	mw.Close()
	url := "/api/aud/dre/lancamentos/importar"
	if simular {
		url += "?simular=1"
	}
	r := httptest.NewRequest(http.MethodPost, url, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := chamar(DREImportarHandler(db), reqComCtx(r, f))
	var resp respImport
	json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func (r respImport) tem(codigo string) bool {
	for _, a := range r.Avisos {
		if a.Codigo == codigo {
			return true
		}
	}
	return false
}

const csvDuasCompetencias = "competencia;filial;conta;valor\n2026-08;11;3.1;1,00\n2026-08;11;3.2;2,00\n2026-09;11;3.1;3,00\n2026-09;12;3.1;4,00\n"

func TestImportarSimular_NaoGravaEConfirmarGrava(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)

	w, sim := importarModo(t, f, "x.csv", csvDuasCompetencias, true)
	if w.Code != http.StatusOK || !sim.Simulacao || sim.Linhas != 4 || sim.CargaID != 0 {
		t.Fatalf("simulação: %d %s", w.Code, w.Body.String())
	}
	if len(sim.Pares) != 3 || sim.Pares[0].Competencia != "2026-08" || sim.Pares[0].Filial != "11" || sim.Pares[0].Lancamentos != 2 ||
		sim.Pares[1].Competencia != "2026-09" || sim.Pares[2].Filial != "12" {
		t.Errorf("pares: %+v", sim.Pares)
	}
	for _, q := range []string{
		`SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`,
		`SELECT COUNT(*) FROM auditoria.cargas WHERE empresa_id=$1::uuid`,
		`SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid`,
	} {
		if n := contar(t, db, q, f.EmpresaID); n != 0 {
			t.Errorf("a simulação escreveu (%s): %d", q, n)
		}
	}
	// Confirmar: mesmo arquivo sem simular.
	w, res := importarModo(t, f, "x.csv", csvDuasCompetencias, false)
	if w.Code != http.StatusOK || res.Simulacao || res.CargaID == 0 || res.Linhas != 4 || len(res.Pares) != 3 {
		t.Fatalf("gravação: %d %s", w.Code, w.Body.String())
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 4 {
		t.Errorf("gravou %d, esperado 4", n)
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='lancamentos' AND acao='importar'`, f.EmpresaID); n != 1 {
		t.Errorf("trilha da gravação: %d", n)
	}
}

func TestImportarSimular_MesFechadoRecusadoJaNaSimulacao(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	if _, err := db.Exec(`INSERT INTO auditoria.dre_fechamentos (empresa_id, competencia, versao, filial, fechado_por)
		VALUES ($1::uuid,'2026-09-01',1,'',$2::uuid)`, f.EmpresaID, f.UserID); err != nil {
		t.Fatal(err)
	}
	w, _ := importarModo(t, f, "x.csv", csvDuasCompetencias, true)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "2026-09") {
		t.Fatalf("simulação em mês fechado: %d %s", w.Code, w.Body.String())
	}
}

func TestImportarSimular_AvisoNomeDoArquivoDivergente(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	// Nome com 09-2026 e dados do Mês 202602 (formato da planilha).
	plan := "CONTA;VL. Realizado;Mês;Loja\n3.1;10,00;202602;11\n"
	w, sim := importarModo(t, f, "Entrada_Dados 09 - 2026.csv", plan, true)
	if w.Code != http.StatusOK || !sim.tem("nome_do_arquivo") || sim.Origem != "csv.contas" {
		t.Fatalf("esperava aviso de nome: %d %s", w.Code, w.Body.String())
	}
	for _, c := range []struct {
		nome string
		diz  bool
	}{
		{"carga_202602.csv", false}, {"carga 02-2026.csv", false}, {"carga_02_2026.csv", false}, {"carga022026.csv", false},
		{"carga_202609.csv", true}, {"carga 09_2026.csv", true}, {"carga092026.csv", true}, {"carga.csv", false},
	} {
		_, s := importarModo(t, f, c.nome, plan, true)
		if s.tem("nome_do_arquivo") != c.diz {
			t.Errorf("nome %q: aviso=%v, esperado %v", c.nome, s.tem("nome_do_arquivo"), c.diz)
		}
	}
	// Nada gravado e a gravação direta traz o mesmo aviso.
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.lancamentos WHERE empresa_id=$1::uuid`, f.EmpresaID); n != 0 {
		t.Errorf("simulações gravaram %d", n)
	}
	_, gr := importarModo(t, f, "Entrada_Dados 09 - 2026.csv", plan, false)
	if !gr.tem("nome_do_arquivo") || gr.CargaID == 0 {
		t.Errorf("gravação sem o aviso: %+v", gr)
	}
}

func TestImportarSimular_AvisosDeFilialEOutraOrigem(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	outra := testdb.NovaEmpresa(t, db)
	// Filial 11 já existe em lançamentos de outra origem (csv) em 2026-05.
	if w, _ := importarModo(t, f, "a.csv", "competencia;filial;conta;valor\n2026-05;11;3.1;1,00\n", false); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	plan := "CONTA;VL. Realizado;Mês;Loja\n3.1;10,00;202605;11\n3.1;5,00;202605;99\n"
	_, sim := importarModo(t, f, "p.csv", plan, true)
	if !sim.tem("outra_origem") || !sim.tem("filial_nova") {
		t.Fatalf("avisos: %+v", sim.Avisos)
	}
	for _, a := range sim.Avisos {
		if a.Codigo == "filial_nova" && !strings.Contains(a.Mensagem, "99") {
			t.Errorf("filial nova deveria ser a 99: %s", a.Mensagem)
		}
		if a.Codigo == "filial_nova" && strings.Contains(a.Mensagem, "Filial 11") {
			t.Errorf("filial 11 já existia: %s", a.Mensagem)
		}
	}
	// Mapa de filiais conhece a 99.
	var gid string
	if err := db.QueryRow(`INSERT INTO auditoria.filiais_gerenciais (empresa_id, codigo, nome) VALUES ($1::uuid,'G','Grupo') RETURNING id::text`, f.EmpresaID).Scan(&gid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auditoria.filiais_gerenciais_erp (empresa_id, filial_gerencial_id, filial_erp) VALUES ($1::uuid,$2::uuid,'99')`, f.EmpresaID, gid); err != nil {
		t.Fatal(err)
	}
	if _, sim = importarModo(t, f, "p.csv", plan, true); sim.tem("filial_nova") {
		t.Errorf("filial do Mapa não deveria avisar: %+v", sim.Avisos)
	}
	// Outra empresa não enxerga a primeira: para ela as duas são novas, sem outra origem.
	_, so := importarModo(t, outra, "p.csv", plan, true)
	if so.tem("outra_origem") || !so.tem("filial_nova") || len(so.Avisos) < 2 {
		t.Errorf("outra empresa: %+v", so.Avisos)
	}
}

func TestImportarSimular_ConferenciaTotalEPlanilhaDuasCompetencias(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	plan := "CONTA;VL. Realizado;Mês;Loja\n3.1;10,00;202605;11\n3.2;5,00;202606;11\nTOTAL;15,00;;\n"
	w, sim := importarModo(t, f, "p.csv", plan, true)
	if w.Code != http.StatusOK || len(sim.Pares) != 2 || sim.Conferencia == nil || !sim.Conferencia.OK {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
