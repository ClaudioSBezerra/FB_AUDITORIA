package handlers

// escopo.go — escopo por departamento que nega por padrão (AD-10, história 4.7).
//
// O servidor calcula o escopo a partir do perfil; nunca vem do cliente. Quem tem
// a capacidade `ler` é irrestrito (controladoria, analista, diretoria, auditoria,
// admin de plataforma). Quem só tem `ler_no_escopo` (gestor) enxerga os
// departamentos de usuario_departamentos na empresa ativa. Quem não tem nenhuma
// das duas, ou o gestor sem departamento, recebe o valor zero, que não dá acesso.

import (
	"context"
	"database/sql"
	"net/http"

	"fb_auditoria/dre"
)

// escopoDe calcula o escopo do usuário na empresa ativa.
func escopoDe(ctx context.Context, db *sql.DB, c *AudContext) (dre.EscopoDepto, error) {
	if c == nil {
		return dre.EscopoDepto{}, nil
	}
	if c.Pode(CapLer) {
		return dre.EscopoDepto{Irrestrito: true}, nil
	}
	if !c.Pode(CapLerNoEscopo) {
		return dre.EscopoDepto{}, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT d.codigo FROM auditoria.usuario_departamentos ud
		JOIN auditoria.departamentos d ON d.id = ud.departamento_id AND d.empresa_id = ud.empresa_id
		WHERE ud.user_id = $1::uuid AND ud.empresa_id = $2::uuid ORDER BY d.codigo`, c.UserID, c.EmpresaID)
	if err != nil {
		return dre.EscopoDepto{}, err
	}
	defer rows.Close()
	esc := dre.EscopoDepto{}
	for rows.Next() {
		var cod string
		if err := rows.Scan(&cod); err != nil {
			return dre.EscopoDepto{}, err
		}
		esc.Deptos = append(esc.Deptos, cod)
	}
	return esc, rows.Err()
}

// exigeLer recusa (403, registrado na trilha) quem não tem a leitura sem restrição:
// a rota devolve valor do grupo (faturamento, margens), que o escopo por
// departamento não cobre.
func exigeLer(w http.ResponseWriter, c *AudContext, db *sql.DB, entidade string) bool {
	if c.Pode(CapLer) {
		return true
	}
	writeAuditLog(db, c.EmpresaID, c.UserID, entidade, "", "negado", map[string]any{"perfil": c.Perfil})
	http.Error(w, "Forbidden: o perfil só vê o seu departamento", http.StatusForbidden)
	return false
}

// exigeLerNoEscopo recusa (403, registrado) o perfil sem nenhuma leitura. Quem
// passa ainda precisa do escopo calculado por escopoDe.
func exigeLerNoEscopo(w http.ResponseWriter, c *AudContext, db *sql.DB, entidade string) bool {
	if c.Pode(CapLerNoEscopo) {
		return true
	}
	writeAuditLog(db, c.EmpresaID, c.UserID, entidade, "", "negado", map[string]any{"perfil": c.Perfil})
	http.Error(w, "Forbidden: o perfil não tem acesso a este detalhe", http.StatusForbidden)
	return false
}

// baseFL é a base dos percentuais (Faturamento Líquido) do recorte e a situação do
// cálculo. Valor nunca vai para quem tem escopo restrito: só o percentual.
type baseFL struct {
	Codigo string
	Status string
	Valor  float64
	Ativos map[string]dre.Ativo
}

// baseFaturamentoLiquido lê o recorte (mês fechado do snapshot, aberto ao vivo) e
// devolve a linha-base, a situação do percentual e o valor da base.
func baseFaturamentoLiquido(ctx context.Context, leitor dre.Leitor, empresaID string, filiais []string, p dre.Periodo, linhas []dre.Linha, mapa []dre.MapaItem) (baseFL, error) {
	lei, err := leitor.LerFiliais(ctx, empresaID, filiais, p, linhas, mapa)
	if err != nil {
		return baseFL{}, err
	}
	exibidas, congelada := linhas, false
	if cong, ok, err := leitor.EstruturaCongelada(ctx, filiais[0], p, lei.Ativos); err != nil {
		return baseFL{}, err
	} else if ok {
		exibidas, congelada = cong, true
	}
	b := baseFL{Ativos: lei.Ativos}
	b.Codigo = dre.CodigoBase(exibidas, linhas, congelada)
	_, b.Status = dre.PercentuaisSobreFaturamento(exibidas, lei.Real, b.Codigo)
	if b.Codigo != "" {
		b.Valor = lei.Real[b.Codigo]
	}
	return b, nil
}
