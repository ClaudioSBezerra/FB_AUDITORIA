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

// DREHandler — GET /api/aud/dre?ano=&mes=&visao=mensal|acumulada&filial=|agrupamento=<codigo>|grupo=1
func DREHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		p, visao, ok := parsePeriodo(r)
		if !ok {
			http.Error(w, "ano/mes inválidos", http.StatusBadRequest)
			return
		}
		rec, ok := resolverRecorte(w, r, db, c.EmpresaID)
		if !ok {
			return
		}

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

		// Leitura mês a mês: mês fechado vem do snapshot do fechamento ativo,
		// mês aberto ou reaberto ao vivo (dre.Leitor).
		res0, err := (dre.Leitor{Q: db}).LerFiliais(r.Context(), c.EmpresaID, rec.Filiais, p, linhas, mapa)
		if err != nil {
			log.Printf("dre ler: %v", err)
			http.Error(w, "Erro ao calcular o DRE", http.StatusInternalServerError)
			return
		}
		// Estrutura exibida: se todos os meses vêm do snapshot, a do fechamento
		// (renomear/reordenar/apagar linha depois não muda o mês fechado). Em
		// período misto vale a estrutura viva.
		vivas := linhas
		estruturaCongelada := false
		if cong, ok, err := (dre.Leitor{Q: db}).EstruturaCongelada(r.Context(), rec.Filiais[0], p, res0.Ativos); err != nil {
			log.Printf("dre estrutura congelada: %v", err)
			http.Error(w, "Erro ao calcular o DRE", http.StatusInternalServerError)
			return
		} else if ok {
			linhas, estruturaCongelada = cong, true
		}
		// Linha-base dos percentuais: o papel da estrutura exibida; fechamento
		// anterior à 3.4 (sem papel gravado) usa o papel vivo, se a linha existir nele.
		codFat := dre.CodigoBase(linhas, vivas, estruturaCongelada)
		real, aa, orc, sem := res0.Real, res0.AA, res0.Orc, res0.Sem
		congelado, versao := false, 0
		fechAtivo, fechVersao := false, 0
		if visao == "mensal" {
			if a, ok := res0.Ativos[p.Fim.Format("2006-01")]; ok {
				// Existe fechamento ativo do mês (qualquer filial)? Independe do filtro.
				fechAtivo, fechVersao = true, a.Versao
				if a.OrigemFiliais(rec.Filiais) != dre.Vivo {
					congelado, versao = true, a.Versao
				}
			}
		}

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
			PctFat      *float64 `json:"pct_faturamento"`
		}
		pcts, pctStatus := dre.PercentuaisSobreFaturamento(linhas, real, codFat)
		res := make([]out, 0, len(linhas))
		for _, l := range linhas {
			res = append(res, out{l.Codigo, l.Descricao, l.Nivel, l.Tipo, real[l.Codigo], orc[l.Codigo], aa[l.Codigo],
				pct(real[l.Codigo], orc[l.Codigo]), pct(real[l.Codigo], aa[l.Codigo]), pcts[l.Codigo]})
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

		writeAuditLog(db, c.EmpresaID, c.UserID, "dre", p.Fim.Format("2006-01"), "visualizar", rec.comVisao(visao))
		aviso := ""
		if pctStatus == dre.PctSemPapel {
			aviso = "Nenhuma linha do DRE está marcada como Faturamento Líquido — marque uma em Mapa do DRE para ver os percentuais."
		}
		writeJSON(w, map[string]any{
			"pct_status": pctStatus, "faturamento_liquido_linha": codFat, "aviso_faturamento": aviso,
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
		// Mês fechado vem do snapshot (contas congeladas); só os meses ao vivo
		// consultam lançamentos.
		leitor := dre.Leitor{Q: db}
		ativos, err := leitor.Ativos(r.Context(), c.EmpresaID, p)
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		congeladas := []string{}
		for _, a := range ativos {
			if a.Origem(filial) != dre.Vivo {
				congeladas = append(congeladas, a.Competencia.Format("2006-01-02"))
			}
		}
		contasCong, semContas, err := leitor.ContasCongeladas(r.Context(), filial, ativos, alvo)
		if err != nil {
			http.Error(w, "Erro ao consultar o fechamento", http.StatusInternalServerError)
			return
		}
		rows, err := db.Query(`SELECT id, competencia, filial, centro_custo, canal, conta, conta_desc, valor,
			origem, tipo_doc, doc_ref, historico FROM auditoria.lancamentos
			WHERE empresa_id = $1::uuid AND competencia BETWEEN $2 AND $3 AND ($4 = '' OR filial = $4)
			AND NOT (competencia = ANY($5::date[]))
			ORDER BY competencia, conta, id`, c.EmpresaID, p.Ini, p.Fim, filial, pq.Array(congeladas))
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
		for _, cc := range contasCong {
			out = append(out, lanc{Competencia: cc.Competencia.Format("2006-01"), Filial: filial, Conta: cc.Conta,
				Valor: cc.Valor, Linha: cc.Linha, Origem: "snapshot"})
		}
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
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Competencia != out[j].Competencia {
				return out[i].Competencia < out[j].Competencia
			}
			return out[i].Conta < out[j].Conta
		})
		resp := map[string]any{"lancamentos": out, "truncado": len(out) >= limite}
		if len(semContas) > 0 {
			sort.Strings(semContas)
			resp["aviso"] = "Fechamento anterior ao detalhe por conta: sem contas congeladas em " + strings.Join(semContas, ", ") + "."
		}
		writeJSON(w, resp)
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
		if r.Method != http.MethodGet && !c.Pode(CapEditarMapa) {
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
		if !c.Pode(CapFechar) {
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
		if !c.Pode(CapReabrir) {
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

// DRELinhaPapelHandler — PUT {codigo, papel}: marca a linha com o papel
// (hoje 'faturamento_liquido'); papel vazio desmarca. A marca anterior da
// empresa sai na mesma transação. Só controladoria (rota).
func DRELinhaPapelHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		if !c.Pode(CapEditarEstrutura) {
			http.Error(w, "Forbidden: perfil insuficiente", http.StatusForbidden)
			return
		}
		var req struct {
			Codigo string `json:"codigo"`
			Papel  string `json:"papel"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req) != nil || req.Codigo == "" ||
			(req.Papel != "" && req.Papel != dre.PapelFaturamentoLiquido) {
			http.Error(w, "codigo e papel (faturamento_liquido ou vazio) obrigatórios", http.StatusBadRequest)
			return
		}
		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		var id int64
		if err := tx.QueryRow(`SELECT id FROM auditoria.dre_linhas WHERE empresa_id=$1::uuid AND codigo=$2 AND ativo FOR UPDATE`,
			c.EmpresaID, req.Codigo).Scan(&id); err != nil {
			http.Error(w, "linha não encontrada", http.StatusNotFound)
			return
		}
		if req.Papel != "" {
			if _, err := tx.Exec(`UPDATE auditoria.dre_linhas SET papel = NULL WHERE empresa_id=$1::uuid AND papel=$2 AND id<>$3`,
				c.EmpresaID, req.Papel, id); err != nil {
				http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
				return
			}
		}
		if _, err := tx.Exec(`UPDATE auditoria.dre_linhas SET papel = NULLIF($2,'') WHERE id=$1`, id, req.Papel); err != nil {
			http.Error(w, "Erro ao salvar", http.StatusInternalServerError)
			return
		}
		if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "dre_linha", req.Codigo, "definir_papel", req); err != nil || tx.Commit() != nil {
			http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
			return
		}
		writeJSON(w, req)
	}
}

// DREDepartamentosHandler — GET /api/aud/dre/departamentos?ano=&mes=&visao=&filial=|agrupamento=<codigo>|grupo=1
// DRE por departamento (Comercial, Logística, Administrativo), com as contas, o
// total e o percentual sobre o Faturamento Líquido. O perfil `gestor` (sem escopo
// até o épico 4) não recebe dado de departamento: 403, fail closed. "Sem
// departamento" só vai a quem não tem restrição (o mesmo conjunto de perfis).
func DREDepartamentosHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c := GetAudContext(r)
		if !c.Pode(CapLer) {
			writeAuditLog(db, c.EmpresaID, c.UserID, "dre_departamentos", "", "negado", map[string]any{"perfil": c.Perfil})
			http.Error(w, "Forbidden: o perfil não tem acesso ao DRE por departamento", http.StatusForbidden)
			return
		}
		semRestricao := true // todos os perfis acima; vira checagem de escopo no épico 4
		p, visao, ok := parsePeriodo(r)
		if !ok {
			http.Error(w, "ano/mes inválidos", http.StatusBadRequest)
			return
		}
		rec, ok := resolverRecorte(w, r, db, c.EmpresaID)
		if !ok {
			return
		}
		erro := func(err error) {
			log.Printf("dre departamentos: %v", err)
			http.Error(w, "Erro ao calcular o DRE por departamento", http.StatusInternalServerError)
		}
		linhas, err := carregarLinhas(db, c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		mapa, err := carregarMapa(db, c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		leitor := dre.Leitor{Q: db}
		lei, err := leitor.LerFiliais(r.Context(), c.EmpresaID, rec.Filiais, p, linhas, mapa)
		if err != nil {
			erro(err)
			return
		}
		exibidas, congelada := linhas, false
		if cong, ok, err := leitor.EstruturaCongelada(r.Context(), rec.Filiais[0], p, lei.Ativos); err != nil {
			erro(err)
			return
		} else if ok {
			exibidas, congelada = cong, true
		}
		codFat := dre.CodigoBase(exibidas, linhas, congelada)
		_, status := dre.PercentuaisSobreFaturamento(exibidas, lei.Real, codFat)
		base := 0.0
		if codFat != "" {
			base = lei.Real[codFat]
		}
		contas, semContas, err := leitor.ContasPorDepartamentoFiliais(r.Context(), c.EmpresaID, rec.Filiais, p, mapa, lei.Ativos)
		if err != nil {
			erro(err)
			return
		}
		deps, err := leitor.Departamentos(r.Context(), c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		desc, err := dre.CarregarDescritorConta(r.Context(), db, c.EmpresaID)
		if err != nil {
			erro(err)
			return
		}
		visaoDeps := dre.MontarVisao(contas, deps, desc, base, semRestricao)
		avisos := []string{}
		if status == dre.PctSemPapel {
			avisos = append(avisos, "Nenhuma linha do DRE está marcada como Faturamento Líquido — marque uma em Mapa do DRE para ver os percentuais.")
		}
		if len(semContas) > 0 {
			avisos = append(avisos, "Fechamento anterior ao detalhe por conta: sem contas por departamento em "+strings.Join(semContas, ", ")+".")
		}
		writeAuditLog(db, c.EmpresaID, c.UserID, "dre_departamentos", p.Fim.Format("2006-01"), "visualizar", rec.comVisao(visao))
		writeJSON(w, map[string]any{
			"ini": p.Ini.Format("2006-01-02"), "fim": p.Fim.Format("2006-01-02"), "visao": visao,
			"pct_status": status, "faturamento_liquido_linha": codFat, "faturamento_liquido": base,
			"avisos": avisos, "grupos": visaoDeps.Grupos, "sem_departamento": visaoDeps.SemDepartamento,
		})
	}
}
