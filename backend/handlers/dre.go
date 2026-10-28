package handlers

// dre.go — DRE automatizado: cálculo (mensal/acumulado, orçado, ano anterior),
// drill-down até o lançamento, mapa de-para, orçado e fechamento versionado.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"fb_auditoria/dre"

	"github.com/lib/pq"
)

type dreLinha = dre.Linha

type dreMapaItem = dre.MapaItem

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ── Estrutura padrão (distribuidora) ─────────────────────────────────────────

var dreLinhasPadrao = []dreLinha{
	{Codigo: "RB", Descricao: "Receita Bruta de Vendas", Nivel: 1, Tipo: "conta"},
	{Codigo: "DED", Descricao: "(-) Deduções e Devoluções", Nivel: 1, Tipo: "conta"},
	{Codigo: "RL", Descricao: "= Receita Líquida", Nivel: 0, Tipo: "subtotal", SomaDe: []string{"RB", "-DED"}},
	{Codigo: "CMV", Descricao: "(-) Custo das Mercadorias Vendidas", Nivel: 1, Tipo: "conta"},
	{Codigo: "LB", Descricao: "= Lucro Bruto", Nivel: 0, Tipo: "subtotal", SomaDe: []string{"RL", "-CMV"}},
	{Codigo: "DV", Descricao: "(-) Despesas com Vendas", Nivel: 1, Tipo: "conta"},
	{Codigo: "DP", Descricao: "(-) Despesas com Pessoal", Nivel: 1, Tipo: "conta"},
	{Codigo: "DA", Descricao: "(-) Despesas Administrativas", Nivel: 1, Tipo: "conta"},
	{Codigo: "OP", Descricao: "(+/-) Outras Receitas/Despesas Operacionais", Nivel: 1, Tipo: "conta"},
	{Codigo: "RO", Descricao: "= Resultado Operacional", Nivel: 0, Tipo: "subtotal", SomaDe: []string{"LB", "-DV", "-DP", "-DA", "OP"}},
	{Codigo: "RF", Descricao: "(+/-) Resultado Financeiro", Nivel: 1, Tipo: "conta"},
	{Codigo: "LAIR", Descricao: "= Resultado antes do IR/CS", Nivel: 0, Tipo: "subtotal", SomaDe: []string{"RO", "RF"}},
	{Codigo: "IR", Descricao: "(-) IRPJ e CSLL", Nivel: 1, Tipo: "conta"},
	{Codigo: "LL", Descricao: "= Lucro Líquido", Nivel: 0, Tipo: "subtotal", SomaDe: []string{"LAIR", "-IR"}},
}

func carregarLinhas(db *sql.DB, empresaID string) ([]dreLinha, error) {
	out, err := dre.CarregarLinhas(context.Background(), db, empresaID)
	if err != nil {
		return nil, err
	}
	if len(out) > 0 {
		return out, nil
	}
	// Primeira vez: semeia a estrutura padrão (a controladoria ajusta depois).
	for i, l := range dreLinhasPadrao {
		if l.SomaDe == nil {
			l.SomaDe = []string{} // soma_de é NOT NULL; nil viraria NULL
		}
		if _, err := db.Exec(`INSERT INTO auditoria.dre_linhas (empresa_id, codigo, descricao, nivel, ordem, tipo, soma_de)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`,
			empresaID, l.Codigo, l.Descricao, l.Nivel, (i+1)*10, l.Tipo, pq.Array(l.SomaDe)); err != nil {
			return nil, err
		}
	}
	return carregarLinhas(db, empresaID)
}

func carregarMapa(db *sql.DB, empresaID string) ([]dreMapaItem, error) {
	return dre.CarregarMapa(context.Background(), db, empresaID)
}

// ── Período ──────────────────────────────────────────────────────────────────

type periodo = dre.Periodo

func parsePeriodo(r *http.Request) (periodo, string, bool) {
	q := r.URL.Query()
	ano, e1 := strconv.Atoi(q.Get("ano"))
	mes, e2 := strconv.Atoi(q.Get("mes"))
	if e1 != nil || e2 != nil || mes < 1 || mes > 12 || ano < 2000 || ano > 2100 {
		return periodo{}, "", false
	}
	visao := q.Get("visao")
	if visao != "acumulada" {
		visao = "mensal"
	}
	fim := time.Date(ano, time.Month(mes), 1, 0, 0, 0, 0, time.UTC)
	ini := fim
	if visao == "acumulada" {
		ini = time.Date(ano, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	return periodo{Ini: ini, Fim: fim}, visao, true
}

func orcadoPorLinha(db *sql.DB, empresaID, filial string, p periodo) (map[string]float64, error) {
	rows, err := db.Query(`SELECT linha_codigo, SUM(valor) FROM auditoria.dre_orcado
		WHERE empresa_id = $1::uuid AND competencia BETWEEN $2 AND $3 AND ($4 = '' OR filial = $4)
		GROUP BY linha_codigo`, empresaID, p.Ini, p.Fim, filial)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]float64{}
	for rows.Next() {
		var c string
		var v float64
		if err := rows.Scan(&c, &v); err != nil {
			return nil, err
		}
		out[c] = v
	}
	return out, rows.Err()
}

func pct(atual, base float64) *float64 {
	if base == 0 {
		return nil
	}
	x := (atual - base) / absf(base) * 100
	return &x
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// DREHandler — GET /api/aud/dre?ano=&mes=&visao=mensal|acumulada&filial=
func DREHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		p, visao, ok := parsePeriodo(r)
		if !ok {
			http.Error(w, "ano/mes inválidos", http.StatusBadRequest)
			return
		}
		filial := r.URL.Query().Get("filial")

		linhas, err := carregarLinhas(db, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro ao carregar estrutura do DRE", http.StatusInternalServerError)
			return
		}
		mapa, err := carregarMapa(db, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro ao carregar mapa", http.StatusInternalServerError)
			return
		}

		// Período congelado (só mensal): serve o snapshot do fechamento ativo
		// (a leitura completa do snapshot é a história 1.8).
		var real map[string]float64
		congelado, versao := false, 0
		var fechID int64
		if visao == "mensal" {
			err := db.QueryRow(`SELECT id, versao FROM auditoria.dre_fechamentos
				WHERE empresa_id = $1::uuid AND competencia = $2 AND filial = $3 AND reaberto_em IS NULL
				ORDER BY versao DESC LIMIT 1`, c.EmpresaID, p.Fim, filial).Scan(&fechID, &versao)
			if err == nil {
				if rows, err := db.Query(`SELECT linha_codigo, valor FROM auditoria.dre_fechamento_linhas WHERE fechamento_id = $1`, fechID); err == nil {
					real = map[string]float64{}
					for rows.Next() {
						var cod string
						var v float64
						if rows.Scan(&cod, &v) == nil {
							real[cod] = v
						}
					}
					rows.Close()
					congelado = true
				}
			}
		}
		// Existe fechamento ativo do mês (qualquer filial)? Independe do filtro de filial.
		fechAtivo, fechVersao := false, 0
		if visao == "mensal" {
			if err := db.QueryRow(`SELECT versao FROM auditoria.dre_fechamentos
				WHERE empresa_id = $1::uuid AND competencia = $2 AND reaberto_em IS NULL
				ORDER BY versao DESC LIMIT 1`, c.EmpresaID, p.Fim).Scan(&fechVersao); err == nil {
				fechAtivo = true
			}
		}
		var sem map[string]float64
		if real == nil {
			real, sem, err = dre.Realizado(r.Context(), db, c.EmpresaID, filial, p, mapa)
			if err != nil {
				http.Error(w, "Erro ao calcular realizado", http.StatusInternalServerError)
				return
			}
			dre.AplicarSubtotais(linhas, real)
		}

		pAA := periodo{Ini: p.Ini.AddDate(-1, 0, 0), Fim: p.Fim.AddDate(-1, 0, 0)}
		aa, _, err := dre.Realizado(r.Context(), db, c.EmpresaID, filial, pAA, mapa)
		if err != nil {
			http.Error(w, "Erro ao calcular ano anterior", http.StatusInternalServerError)
			return
		}
		dre.AplicarSubtotais(linhas, aa)

		orc, err := orcadoPorLinha(db, c.EmpresaID, filial, p)
		if err != nil {
			http.Error(w, "Erro ao carregar orçado", http.StatusInternalServerError)
			return
		}
		dre.AplicarSubtotais(linhas, orc)

		type out struct {
			Codigo      string   `json:"codigo"`
			Descricao   string   `json:"descricao"`
			Nivel       int      `json:"nivel"`
			Tipo        string   `json:"tipo"`
			Realizado   float64  `json:"realizado"`
			Orcado      float64  `json:"orcado"`
			AnoAnterior float64  `json:"ano_anterior"`
			VarOrcado   *float64 `json:"var_orcado_pct"`
			VarAno      *float64 `json:"var_ano_pct"`
		}
		res := make([]out, 0, len(linhas))
		for _, l := range linhas {
			res = append(res, out{l.Codigo, l.Descricao, l.Nivel, l.Tipo, real[l.Codigo], orc[l.Codigo], aa[l.Codigo],
				pct(real[l.Codigo], orc[l.Codigo]), pct(real[l.Codigo], aa[l.Codigo])})
		}
		type nm struct {
			Conta string  `json:"conta"`
			Valor float64 `json:"valor"`
		}
		naoMapeado := []nm{}
		for k, v := range sem {
			naoMapeado = append(naoMapeado, nm{k, v})
		}
		sort.Slice(naoMapeado, func(i, j int) bool { return absf(naoMapeado[i].Valor) > absf(naoMapeado[j].Valor) })

		writeAuditLog(db, c.EmpresaID, c.UserID, "dre", p.Fim.Format("2006-01"), "visualizar", map[string]any{"visao": visao, "filial": filial})
		writeJSON(w, map[string]any{
			"ini": p.Ini.Format("2006-01-02"), "fim": p.Fim.Format("2006-01-02"), "visao": visao,
			"congelado": congelado, "versao": versao,
			"fechamento_ativo": fechAtivo, "fechamento_versao": fechVersao, "linhas": res, "nao_mapeado": naoMapeado,
		})
	}
}

// DREFiliaisHandler — GET /api/aud/dre/filiais: filiais com lançamento.
func DREFiliaisHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		rows, err := db.Query(`SELECT DISTINCT filial FROM auditoria.lancamentos
			WHERE empresa_id = $1::uuid AND filial <> '' ORDER BY filial`, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		out := []string{}
		for rows.Next() {
			var f string
			if rows.Scan(&f) == nil {
				out = append(out, f)
			}
		}
		writeJSON(w, out)
	}
}

// DRELancamentosHandler — drill-down: GET /api/aud/dre/lancamentos?linha=&ano=&mes=&visao=&filial=
// Para linha de subtotal, devolve os lançamentos de todas as linhas 'conta' que a compõem.
func DRELancamentosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		p, _, ok := parsePeriodo(r)
		linha := r.URL.Query().Get("linha")
		if !ok || linha == "" {
			http.Error(w, "linha/ano/mes obrigatórios", http.StatusBadRequest)
			return
		}
		filial := r.URL.Query().Get("filial")
		linhas, err := carregarLinhas(db, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		alvo := alcancarContas(linhas, linha)
		if len(alvo) == 0 {
			http.Error(w, "linha desconhecida", http.StatusNotFound)
			return
		}
		mapa, err := carregarMapa(db, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		rows, err := db.Query(`SELECT id, competencia, filial, centro_custo, canal, conta, conta_desc, valor,
			origem, tipo_doc, doc_ref, historico FROM auditoria.lancamentos
			WHERE empresa_id = $1::uuid AND competencia BETWEEN $2 AND $3 AND ($4 = '' OR filial = $4)
			ORDER BY competencia, conta, id`, c.EmpresaID, p.Ini, p.Fim, filial)
		if err != nil {
			http.Error(w, "Erro ao consultar lançamentos", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type lanc struct {
			ID          int64   `json:"id"`
			Competencia string  `json:"competencia"`
			Filial      string  `json:"filial"`
			CentroCusto string  `json:"centro_custo"`
			Canal       string  `json:"canal"`
			Conta       string  `json:"conta"`
			ContaDesc   string  `json:"conta_desc"`
			Valor       float64 `json:"valor"`
			Linha       string  `json:"linha"`
			Origem      string  `json:"origem"`
			TipoDoc     string  `json:"tipo_doc"`
			DocRef      string  `json:"doc_ref"`
			Historico   string  `json:"historico"`
		}
		out := []lanc{}
		const limite = 2000
		for rows.Next() {
			var l lanc
			var comp time.Time
			if err := rows.Scan(&l.ID, &comp, &l.Filial, &l.CentroCusto, &l.Canal, &l.Conta, &l.ContaDesc, &l.Valor,
				&l.Origem, &l.TipoDoc, &l.DocRef, &l.Historico); err != nil {
				continue
			}
			m, ok := dre.ResolverConta(mapa, l.Conta)
			if !ok || !alvo[m.LinhaCodigo] {
				continue
			}
			l.Competencia = comp.Format("2006-01")
			l.Linha = m.LinhaCodigo
			l.Valor *= float64(m.Sinal)
			out = append(out, l)
			if len(out) >= limite {
				break
			}
		}
		writeJSON(w, map[string]any{"lancamentos": out, "truncado": len(out) >= limite})
	}
}

// alcancarContas expande subtotal recursivamente até as linhas 'conta'.
func alcancarContas(linhas []dreLinha, codigo string) map[string]bool {
	idx := map[string]dreLinha{}
	for _, l := range linhas {
		idx[l.Codigo] = l
	}
	res := map[string]bool{}
	var visita func(c string, depth int)
	visita = func(c string, depth int) {
		c = strings.TrimPrefix(c, "-")
		l, ok := idx[c]
		if !ok || depth > 20 {
			return
		}
		if l.Tipo == "conta" {
			res[c] = true
			return
		}
		for _, s := range l.SomaDe {
			visita(s, depth+1)
		}
	}
	visita(codigo, 0)
	return res
}

// DREMapaHandler — GET lista / PUT upsert {conta_padrao, linha_codigo, sinal} / DELETE ?conta_padrao=
func DREMapaHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		// Leitura: qualquer perfil da área. Escrita: só controladoria (auditoria
		// confere o mapa, não altera).
		if r.Method != http.MethodGet && !c.Tem("controladoria") {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet:
			m, err := carregarMapa(db, c.EmpresaID)
			if err != nil {
				http.Error(w, "Erro", http.StatusInternalServerError)
				return
			}
			writeJSON(w, m)
		case http.MethodPut:
			var m dreMapaItem
			if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&m) != nil ||
				strings.TrimSpace(m.ContaPadrao) == "" || m.LinhaCodigo == "" || (m.Sinal != 1 && m.Sinal != -1) {
				http.Error(w, "conta_padrao, linha_codigo e sinal (1|-1) obrigatórios", http.StatusBadRequest)
				return
			}
			var existe bool
			_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND codigo=$2 AND tipo='conta')`,
				c.EmpresaID, m.LinhaCodigo).Scan(&existe)
			if !existe {
				http.Error(w, "linha_codigo não é uma linha de conta do DRE", http.StatusBadRequest)
				return
			}
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Erro", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`INSERT INTO auditoria.dre_mapa (empresa_id, conta_padrao, linha_codigo, sinal)
				VALUES ($1::uuid,$2,$3,$4)
				ON CONFLICT (empresa_id, conta_padrao) DO UPDATE SET linha_codigo=EXCLUDED.linha_codigo, sinal=EXCLUDED.sinal`,
				c.EmpresaID, strings.TrimSpace(m.ContaPadrao), m.LinhaCodigo, m.Sinal); err != nil {
				http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
				return
			}
			if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "dre_mapa", m.ContaPadrao, "salvar", m); err != nil || tx.Commit() != nil {
				http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
				return
			}
			writeJSON(w, m)
		case http.MethodDelete:
			conta := r.URL.Query().Get("conta_padrao")
			tx, err := db.Begin()
			if err != nil {
				http.Error(w, "Erro", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`DELETE FROM auditoria.dre_mapa WHERE empresa_id=$1::uuid AND conta_padrao=$2`, c.EmpresaID, conta); err != nil {
				http.Error(w, "Erro", http.StatusInternalServerError)
				return
			}
			if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "dre_mapa", conta, "remover", nil); err != nil || tx.Commit() != nil {
				http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// DRELinhasHandler — GET /api/aud/dre/linhas (estrutura; semeia o padrão na 1ª vez).
func DRELinhasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		l, err := carregarLinhas(db, c.EmpresaID)
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		writeJSON(w, l)
	}
}

// DREOrcadoHandler — PUT /api/aud/dre/orcado [{competencia:"2026-09", filial, linha_codigo, valor}]
func DREOrcadoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		var itens []struct {
			Competencia string  `json:"competencia"`
			Filial      string  `json:"filial"`
			LinhaCodigo string  `json:"linha_codigo"`
			Valor       float64 `json:"valor"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&itens) != nil || len(itens) == 0 {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		for _, it := range itens {
			comp, err := time.Parse("2006-01", it.Competencia)
			if err != nil || it.LinhaCodigo == "" {
				http.Error(w, "competencia (AAAA-MM) e linha_codigo obrigatórios", http.StatusBadRequest)
				return
			}
			if _, err := tx.Exec(`INSERT INTO auditoria.dre_orcado (empresa_id, competencia, filial, linha_codigo, valor)
				VALUES ($1::uuid,$2,$3,$4,$5)
				ON CONFLICT (empresa_id, competencia, filial, linha_codigo) DO UPDATE SET valor = EXCLUDED.valor`,
				c.EmpresaID, comp, it.Filial, it.LinhaCodigo, it.Valor); err != nil {
				http.Error(w, "Erro ao salvar orçado", http.StatusInternalServerError)
				return
			}
		}
		if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "dre_orcado", "", "importar", map[string]int{"itens": len(itens)}); err != nil || tx.Commit() != nil {
			http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]int{"itens": len(itens)})
	}
}

// DREFechamentoHandler — GET lista fechamentos (com situação) / POST {ano, mes, observacao}
// congela o realizado do mês, do grupo inteiro, numa NOVA versão (as anteriores permanecem).
// 409 se a competência já tem fechamento ativo; 400 se vier filial.
func DREFechamentoHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if r.Method == http.MethodGet {
			rows, err := db.Query(`SELECT f.id, f.competencia, f.versao, f.filial, COALESCE(u.full_name,''), f.fechado_em, f.observacao,
					f.reaberto_em, COALESCE(ur.full_name,''), COALESCE(f.motivo_reabertura,'')
				FROM auditoria.dre_fechamentos f LEFT JOIN users u ON u.id = f.fechado_por LEFT JOIN users ur ON ur.id = f.reaberto_por
				WHERE f.empresa_id = $1::uuid ORDER BY f.competencia DESC, f.versao DESC LIMIT 200`, c.EmpresaID)
			if err != nil {
				http.Error(w, "Erro", http.StatusInternalServerError)
				return
			}
			defer rows.Close()
			type f struct {
				ID          int64  `json:"id"`
				Competencia string `json:"competencia"`
				Versao      int    `json:"versao"`
				Filial      string `json:"filial"`
				FechadoPor  string `json:"fechado_por"`
				FechadoEm   string `json:"fechado_em"`
				Observacao  string `json:"observacao"`
				Ativo       bool   `json:"ativo"`
				ReabertoEm  string `json:"reaberto_em"`
				ReabertoPor string `json:"reaberto_por"`
				MotivoReab  string `json:"motivo_reabertura"`
			}
			out := []f{}
			for rows.Next() {
				var x f
				var comp time.Time
				var em time.Time
				var reab sql.NullTime
				if rows.Scan(&x.ID, &comp, &x.Versao, &x.Filial, &x.FechadoPor, &em, &x.Observacao, &reab, &x.ReabertoPor, &x.MotivoReab) == nil {
					x.Competencia, x.FechadoEm = comp.Format("2006-01"), em.Format(time.RFC3339)
					x.Ativo = !reab.Valid
					if reab.Valid {
						x.ReabertoEm = reab.Time.Format(time.RFC3339)
					}
					out = append(out, x)
				}
			}
			writeJSON(w, out)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !c.Tem("controladoria") {
			http.Error(w, "Forbidden: só a controladoria fecha períodos", http.StatusForbidden)
			return
		}
		var req struct {
			Ano        int    `json:"ano"`
			Mes        int    `json:"mes"`
			Filial     string `json:"filial"`
			Observacao string `json:"observacao"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil || req.Mes < 1 || req.Mes > 12 || req.Ano < 2000 {
			http.Error(w, "ano/mes inválidos", http.StatusBadRequest)
			return
		}
		// Garante a estrutura padrão semeada (primeira vez) antes de fechar.
		if _, err := carregarLinhas(db, c.EmpresaID); err != nil {
			http.Error(w, "Erro ao carregar DRE", http.StatusInternalServerError)
			return
		}
		out, err := (&dre.Fechamento{DB: db}).Fechar(r.Context(), dre.FecharIn{
			EmpresaID: c.EmpresaID, UserID: c.UserID, Ano: req.Ano, Mes: req.Mes, Filial: req.Filial, Observacao: req.Observacao,
		})
		var semMapa *dre.ErrContasSemMapa
		if errors.As(err, &semMapa) {
			http.Error(w, semMapa.Error(), http.StatusConflict)
			return
		}
		var jaFechado *dre.ErrJaFechado
		if errors.As(err, &jaFechado) {
			http.Error(w, jaFechado.Error(), http.StatusConflict)
			return
		}
		var entrada *dre.ErroEntrada
		if errors.As(err, &entrada) {
			http.Error(w, entrada.Msg, http.StatusBadRequest)
			return
		}
		if err != nil {
			log.Printf("dre fechar: %v", err)
			http.Error(w, "Erro ao fechar período", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]any{"id": out.ID, "versao": out.Versao})
	}
}

// DREReabrirHandler — POST {ano, mes, motivo} reabre o fechamento ativo do mês.
// Só controladoria (403); motivo obrigatório (400); sem ativo, 409.
func DREReabrirHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !c.Tem("controladoria") {
			http.Error(w, "Forbidden: só a controladoria reabre períodos", http.StatusForbidden)
			return
		}
		var req struct {
			Ano    int    `json:"ano"`
			Mes    int    `json:"mes"`
			Motivo string `json:"motivo"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil {
			http.Error(w, "corpo inválido", http.StatusBadRequest)
			return
		}
		out, err := (&dre.Fechamento{DB: db}).Reabrir(r.Context(), dre.ReabrirIn{
			EmpresaID: c.EmpresaID, UserID: c.UserID, Ano: req.Ano, Mes: req.Mes, Motivo: req.Motivo,
		})
		var naoFechado *dre.ErrNaoFechado
		if errors.As(err, &naoFechado) {
			http.Error(w, naoFechado.Error(), http.StatusConflict)
			return
		}
		var entrada *dre.ErroEntrada
		if errors.As(err, &entrada) {
			http.Error(w, entrada.Msg, http.StatusBadRequest)
			return
		}
		if err != nil {
			log.Printf("dre reabrir: %v", err)
			http.Error(w, "Erro ao reabrir período", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"id": out.ID, "versao": out.Versao})
	}
}
