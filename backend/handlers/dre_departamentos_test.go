package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"fb_auditoria/internal/testdb"
)

type visaoDep struct {
	PctStatus string   `json:"pct_status"`
	Avisos    []string `json:"avisos"`
	Grupos    []struct {
		Grupo         string  `json:"grupo"`
		Total         float64 `json:"total"`
		Departamentos []struct {
			Codigo string   `json:"codigo"`
			Total  float64  `json:"total"`
			Pct    *float64 `json:"pct_faturamento"`
			Contas []struct {
				Conta     string   `json:"conta"`
				Descricao string   `json:"descricao"`
				Valor     float64  `json:"valor"`
				Pct       *float64 `json:"pct_faturamento"`
			} `json:"contas"`
		} `json:"departamentos"`
	} `json:"grupos"`
	SemDepartamento *struct {
		Total  float64 `json:"total"`
		Contas []struct {
			Conta string `json:"conta"`
		} `json:"contas"`
	} `json:"sem_departamento"`
}

// Cenário (fictício): FL = RB - DED; contas 3.1 (RB), 3.2 (DED), 4.1 e 4.2 (CMV), 5.1 (CMV, sem departamento).
// Departamentos D1 (Comercial) e D2 (Logística); 4.1 -> D1 exato, 4.% -> D2 por prefixo.
func cenarioDep(t *testing.T, f testdb.Fixture) {
	t.Helper()
	estruturaPct(t, f, true)
	exec(t, `INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'4.2','CMV',-1)`, f.EmpresaID)
	exec(t, `INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal) VALUES ($1::uuid,'5.1','CMV',1)`, f.EmpresaID)
	exec(t, `INSERT INTO auditoria.departamentos (empresa_id, codigo, nome, grupo) VALUES ($1::uuid,'D1','Depto 1','Comercial'),($1::uuid,'D2','Depto 2','Logística')`, f.EmpresaID)
	exec(t, `INSERT INTO auditoria.conta_departamento (empresa_id, conta_padrao, departamento_id, origem)
		SELECT $1::uuid, '4.1', id, 'manual' FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND codigo='D1'`, f.EmpresaID)
	exec(t, `INSERT INTO auditoria.conta_departamento (empresa_id, conta_padrao, departamento_id, origem)
		SELECT $1::uuid, '4.%', id, 'grupo' FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND codigo='D2'`, f.EmpresaID)
	exec(t, `INSERT INTO auditoria.conta_departamento (empresa_id, conta_padrao, departamento_id, origem)
		SELECT $1::uuid, '3.1', id, 'manual' FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND codigo='D1'`, f.EmpresaID)
	exec(t, `INSERT INTO auditoria.conta_dicionario (empresa_id, conta, descricao, grupo) VALUES ($1::uuid,'4.1','Frete teste','')`, f.EmpresaID)
	lancPct(t, f, "2026-09-01", "01", "3.1", 1000)
	lancPct(t, f, "2026-09-01", "02", "3.1", 200)
	lancPct(t, f, "2026-09-01", "01", "3.2", 200)
	lancPct(t, f, "2026-09-01", "01", "4.1", 100)
	lancPct(t, f, "2026-09-01", "01", "4.2", 30)
	lancPct(t, f, "2026-09-01", "01", "5.1", 20)
}

func chamarDep(t *testing.T, f testdb.Fixture, perfil, url string) (int, visaoDep) {
	t.Helper()
	w := chamar(DREDepartamentosHandler(testdb.Open(t)), reqFilial(http.MethodGet, url, "", f, perfil))
	var v visaoDep
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, v
}

func achar(v visaoDep, grupo, depto string) (float64, int, bool) {
	for _, g := range v.Grupos {
		if g.Grupo != grupo {
			continue
		}
		for _, d := range g.Departamentos {
			if d.Codigo == depto {
				return d.Total, len(d.Contas), true
			}
		}
	}
	return 0, 0, false
}

func TestDepartamentos_ContasLigadasSemDepartamentoEPercentual(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	code, v := chamarDep(t, f, "controladoria", "/x?ano=2026&mes=9&visao=mensal")
	if code != http.StatusOK {
		t.Fatalf("code %d", code)
	}
	// FL = 1200 - 200 = 1000. D1 = 3.1 (1200) + 4.1 (100); D2 = 4.2 (sinal -1: -30).
	if tot, n, ok := achar(v, "Comercial", "D1"); !ok || tot != 1300 || n != 2 {
		t.Errorf("D1: %v %v %v", tot, n, ok)
	}
	if tot, n, ok := achar(v, "Logística", "D2"); !ok || tot != -30 || n != 1 {
		t.Errorf("D2: %v %v %v", tot, n, ok)
	}
	if _, _, ok := achar(v, "Administrativo", "D1"); ok {
		t.Error("D1 no grupo errado")
	}
	if len(v.Grupos) != 3 || v.Grupos[0].Grupo != "Comercial" || v.Grupos[1].Grupo != "Logística" || v.Grupos[2].Grupo != "Administrativo" {
		t.Errorf("grupos: %+v", v.Grupos)
	}
	for _, d := range v.Grupos[0].Departamentos {
		if d.Pct == nil || *d.Pct != 130 {
			t.Errorf("pct D1 = %v", d.Pct)
		}
		for _, c := range d.Contas {
			if c.Conta == "4.1" && (c.Descricao != "Frete teste" || *c.Pct != 10) {
				t.Errorf("conta 4.1: %+v", c)
			}
		}
	}
	// 3.2 (DED) e 5.1 não têm departamento.
	if v.SemDepartamento == nil || len(v.SemDepartamento.Contas) != 2 || v.SemDepartamento.Total != 220 {
		t.Errorf("sem departamento: %+v", v.SemDepartamento)
	}
	// Filial: só os lançamentos dela.
	_, v1 := chamarDep(t, f, "diretoria", "/x?ano=2026&mes=9&visao=mensal&filial=02")
	if tot, _, ok := achar(v1, "Comercial", "D1"); !ok || tot != 200 {
		t.Errorf("filial 02 D1: %v %v", tot, ok)
	}
}

// Perfil sem nenhuma leitura recebe 403 (o Gerente tem escopo: ver escopo_test.go).
func TestDepartamentos_PerfilSemLeituraRecebe403(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	for _, perfil := range []string{"", "qualquer"} {
		if code, _ := chamarDep(t, f, perfil, "/x?ano=2026&mes=9&visao=mensal"); code != http.StatusForbidden {
			t.Errorf("perfil %q: %d", perfil, code)
		}
	}
	if n := contar(t, db, `SELECT COUNT(*) FROM auditoria.audit_log WHERE empresa_id=$1::uuid AND entidade='dre_departamentos' AND acao='negado'`, f.EmpresaID); n != 2 {
		t.Errorf("negativas registradas = %d", n)
	}
}

func TestDepartamentos_SemPapelAvisa(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	exec(t, `UPDATE auditoria.dre_linhas SET papel=NULL WHERE empresa_id=$1::uuid`, f.EmpresaID)
	code, v := chamarDep(t, f, "controladoria", "/x?ano=2026&mes=9&visao=mensal")
	if code != http.StatusOK || v.PctStatus != "sem_papel" || len(v.Avisos) == 0 {
		t.Fatalf("%d %+v", code, v)
	}
	if _, _, ok := achar(v, "Comercial", "D1"); !ok {
		t.Error("valores devem aparecer mesmo sem percentual")
	}
}

func TestDepartamentos_SnapshotGravaELeDepartamento(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d %s", w.Code, w.Body.String())
	}
	rows, err := db.Query(`SELECT conta, departamento FROM auditoria.dre_fechamento_contas c JOIN auditoria.dre_fechamentos x ON x.id=c.fechamento_id
		WHERE x.empresa_id=$1::uuid AND c.filial=''`, f.EmpresaID)
	if err != nil {
		t.Fatal(err)
	}
	gravado := map[string]string{}
	for rows.Next() {
		var c, d string
		rows.Scan(&c, &d)
		gravado[c] = d
	}
	rows.Close()
	want := map[string]string{"3.1": "D1", "4.1": "D1", "4.2": "D2", "3.2": "", "5.1": ""}
	for c, d := range want {
		if g, ok := gravado[c]; !ok || g != d {
			t.Errorf("conta %s: departamento %q (existe=%v), esperado %q", c, g, ok, d)
		}
	}

	_, antes := chamarDep(t, f, "controladoria", "/x?ano=2026&mes=9&visao=mensal")
	// Muda tudo que é vivo: vínculos, departamentos, descrição, lançamentos, mapa.
	exec(t, `DELETE FROM auditoria.conta_departamento WHERE empresa_id=$1::uuid`, f.EmpresaID)
	exec(t, `INSERT INTO auditoria.conta_departamento (empresa_id, conta_padrao, departamento_id, origem)
		SELECT $1::uuid, '%', id, 'manual' FROM auditoria.departamentos WHERE empresa_id=$1::uuid AND codigo='D2'`, f.EmpresaID)
	exec(t, `UPDATE auditoria.conta_dicionario SET descricao='mudou' WHERE empresa_id=$1::uuid`, f.EmpresaID)
	lancPct(t, f, "2026-09-01", "01", "4.1", 999)
	_, depois := chamarDep(t, f, "controladoria", "/x?ano=2026&mes=9&visao=mensal")
	// A descrição é rótulo lido do dicionário vivo; valores e departamentos vêm do snapshot.
	semDescricao := func(v visaoDep) string {
		for gi := range v.Grupos {
			for di := range v.Grupos[gi].Departamentos {
				for ci := range v.Grupos[gi].Departamentos[di].Contas {
					v.Grupos[gi].Departamentos[di].Contas[ci].Descricao = ""
				}
			}
		}
		b, _ := json.Marshal(v.Grupos)
		return string(b)
	}
	a, b := semDescricao(antes), semDescricao(depois)
	if a != b {
		t.Errorf("mês fechado mudou com o dicionário:\n%s\n%s", a, b)
	}
	if depois.SemDepartamento == nil || depois.SemDepartamento.Total != 220 {
		t.Errorf("sem departamento mudou: %+v", depois.SemDepartamento)
	}
	// Acumulado mistura mês fechado (snapshot) e aberto (ao vivo, com o dicionário novo).
	lancPct(t, f, "2026-10-01", "01", "4.1", 50)
	_, mix := chamarDep(t, f, "controladoria", "/x?ano=2026&mes=10&visao=acumulada")
	if tot, _, ok := achar(mix, "Logística", "D2"); !ok || tot != -30+50 {
		t.Errorf("acumulado D2 = %v (ok=%v)", tot, ok)
	}
}

func TestDepartamentos_FechamentoAntigoSemDepartamentoCaiEmSem(t *testing.T) {
	db := testdb.Open(t)
	f := testdb.NovaEmpresa(t, db)
	cenarioDep(t, f)
	if w := fechar(t, f, corpoSetembro); w.Code != http.StatusCreated {
		t.Fatalf("fechar: %d", w.Code)
	}
	// Simula fechamento anterior à 3.5: departamento vazio no snapshot.
	exec(t, `SELECT set_config('auditoria.limpando_teste', $1, false)`, f.EmpresaID)
	exec(t, `UPDATE auditoria.dre_fechamento_contas SET departamento='' WHERE empresa_id=$1::uuid`, f.EmpresaID)
	_, v := chamarDep(t, f, "controladoria", "/x?ano=2026&mes=9&visao=mensal")
	for _, g := range v.Grupos {
		if len(g.Departamentos) != 0 {
			t.Errorf("grupo %s deveria estar vazio: %+v", g.Grupo, g.Departamentos)
		}
	}
	if v.SemDepartamento == nil || len(v.SemDepartamento.Contas) != 5 {
		t.Errorf("sem departamento: %+v", v.SemDepartamento)
	}
}
