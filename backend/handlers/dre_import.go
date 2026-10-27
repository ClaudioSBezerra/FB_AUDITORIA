package handlers

// dre_import.go — carga de lançamentos por CSV (enquanto a ingestão
// incremental do Winthor/Oracle não existe; grava na mesma tabela).
//
// Cabeçalho aceito (qualquer ordem, ';' ou ','): competencia, filial,
// centro_custo, canal, conta, conta_desc, valor, tipo_doc, doc_ref, historico.
// competencia: AAAA-MM ou AAAA-MM-DD. valor: 1234.56 ou 1.234,56.

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func parseValorBR(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(strings.ReplaceAll(s, ".", ""), ",", ".")
	}
	return strconv.ParseFloat(s, 64)
}

func parseCompetencia(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 7 {
		if t, err := time.Parse("2006-01", s[:7]); err == nil {
			return t, nil
		}
	}
	return time.Parse("02/01/2006", "01/"+s) // MM/AAAA
}

// DREImportarHandler — POST /api/aud/dre/lancamentos/importar (multipart "arquivo").
// Tudo ou nada: qualquer linha inválida aborta e informa a linha.
func DREImportarHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := GetAudContext(r)
		r.Body = http.MaxBytesReader(w, r.Body, 64<<20)
		f, hdr, err := r.FormFile("arquivo")
		if err != nil {
			http.Error(w, "arquivo obrigatório", http.StatusBadRequest)
			return
		}
		defer f.Close()

		br := bufio.NewReader(f)
		first, _ := br.Peek(2048)
		cr := csv.NewReader(br)
		cr.Comma = ';'
		if strings.Count(string(first), ",") > strings.Count(string(first), ";") {
			cr.Comma = ','
		}
		cr.LazyQuotes = true
		head, err := cr.Read()
		if err != nil {
			http.Error(w, "CSV vazio ou ilegível", http.StatusBadRequest)
			return
		}
		col := map[string]int{}
		for i, h := range head {
			col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, string(rune(0xFEFF)))))] = i
		}
		for _, req := range []string{"competencia", "conta", "valor"} {
			if _, ok := col[req]; !ok {
				http.Error(w, "coluna obrigatória ausente: "+req, http.StatusBadRequest)
				return
			}
		}
		get := func(rec []string, name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}

		tx, err := db.Begin()
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()
		var cargaID int64
		if err := tx.QueryRow(`INSERT INTO auditoria.cargas (empresa_id, user_id, arquivo) VALUES ($1::uuid,$2::uuid,$3) RETURNING id`,
			c.EmpresaID, c.UserID, hdr.Filename).Scan(&cargaID); err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		stmt, err := tx.Prepare(`INSERT INTO auditoria.lancamentos
			(empresa_id, competencia, filial, centro_custo, canal, conta, conta_desc, valor, tipo_doc, doc_ref, historico, carga_id)
			VALUES ($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`)
		if err != nil {
			http.Error(w, "Erro", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		n := 0
		for line := 2; ; line++ {
			rec, err := cr.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, fmt.Sprintf("linha %d: %v", line, err), http.StatusBadRequest)
				return
			}
			comp, err := parseCompetencia(get(rec, "competencia"))
			if err != nil {
				http.Error(w, fmt.Sprintf("linha %d: competencia inválida", line), http.StatusBadRequest)
				return
			}
			conta := get(rec, "conta")
			valor, err := parseValorBR(get(rec, "valor"))
			if conta == "" || err != nil {
				http.Error(w, fmt.Sprintf("linha %d: conta/valor inválidos", line), http.StatusBadRequest)
				return
			}
			if _, err := stmt.Exec(c.EmpresaID, comp, get(rec, "filial"), get(rec, "centro_custo"), get(rec, "canal"),
				conta, get(rec, "conta_desc"), valor, get(rec, "tipo_doc"), get(rec, "doc_ref"), get(rec, "historico"), cargaID); err != nil {
				http.Error(w, fmt.Sprintf("linha %d: %v", line, err), http.StatusBadRequest)
				return
			}
			n++
		}
		if n == 0 {
			http.Error(w, "nenhuma linha de dados", http.StatusBadRequest)
			return
		}
		_, _ = tx.Exec(`UPDATE auditoria.cargas SET linhas=$1 WHERE id=$2`, n, cargaID)
		if err := writeAuditLogTx(tx, c.EmpresaID, c.UserID, "lancamentos", strconv.FormatInt(cargaID, 10), "importar",
			map[string]any{"arquivo": hdr.Filename, "linhas": n}); err != nil || tx.Commit() != nil {
			http.Error(w, "Erro ao registrar auditoria", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"carga_id": cargaID, "linhas": n})
	}
}
