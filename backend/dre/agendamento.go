package dre

// agendamento.go — importação automática do banco do cliente (história 2.13;
// AD-15). A configuração fica no banco e nasce desligada. O relógio é o de
// America/Sao_Paulo. A exclusão mútua é o mesmo advisory lock da importação
// manual (ImportacaoERP.Iniciar).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"fb_auditoria/trilha"
)

// Frequências do agendamento.
const (
	FrequenciaDiaria  = "diaria"
	FrequenciaSemanal = "semanal"
	FrequenciaMensal  = "mensal"
)

// GatilhoAgendador identifica a execução automática em execucoes e na trilha.
const GatilhoAgendador = "agendador"

// FusoAgendamento é o fuso em que a hora configurada vale.
const FusoAgendamento = "America/Sao_Paulo"

// Agendamento é a configuração da importação automática de uma empresa.
type Agendamento struct {
	Ativo      bool   `json:"ativo"`
	Frequencia string `json:"frequencia"`
	Dia        int    `json:"dia"`  // mensal: 1 a 28; semanal: 0 (domingo) a 6
	Hora       string `json:"hora"` // HH:MM

	Desde  time.Time  `json:"-"`
	Ultimo *time.Time `json:"ultimo,omitempty"`
	// Proxima só vem preenchida com o agendamento ligado.
	Proxima *time.Time `json:"proxima,omitempty"`
}

// AgendamentoPadrao é o que vale enquanto ninguém salvou.
func AgendamentoPadrao() Agendamento {
	return Agendamento{Frequencia: FrequenciaMensal, Dia: 5, Hora: "06:30"}
}

// Validar confere os campos e devolve a hora e o minuto.
func (a Agendamento) Validar() (int, int, error) {
	h, err := time.Parse("15:04", a.Hora)
	if err != nil {
		return 0, 0, &ErroEntrada{Msg: "hora inválida: use HH:MM"}
	}
	switch a.Frequencia {
	case FrequenciaDiaria:
	case FrequenciaSemanal:
		if a.Dia < 0 || a.Dia > 6 {
			return 0, 0, &ErroEntrada{Msg: "dia da semana inválido: 0 (domingo) a 6 (sábado)"}
		}
	case FrequenciaMensal:
		if a.Dia < 1 || a.Dia > 28 {
			return 0, 0, &ErroEntrada{Msg: "dia do mês inválido: 1 a 28"}
		}
	default:
		return 0, 0, &ErroEntrada{Msg: "frequência inválida: diaria, semanal ou mensal"}
	}
	return h.Hour(), h.Minute(), nil
}

// horarioAte devolve o último horário previsto que não passa de agora.
func (a Agendamento) horarioAte(agora time.Time, loc *time.Location) (time.Time, error) {
	h, m, err := a.Validar()
	if err != nil {
		return time.Time{}, err
	}
	l := agora.In(loc)
	switch a.Frequencia {
	case FrequenciaMensal:
		s := time.Date(l.Year(), l.Month(), a.Dia, h, m, 0, 0, loc)
		if s.After(l) {
			s = time.Date(l.Year(), l.Month()-1, a.Dia, h, m, 0, 0, loc)
		}
		return s, nil
	default:
		s := time.Date(l.Year(), l.Month(), l.Day(), h, m, 0, 0, loc)
		for s.After(l) || (a.Frequencia == FrequenciaSemanal && int(s.Weekday()) != a.Dia) {
			s = s.AddDate(0, 0, -1)
		}
		return s, nil
	}
}

// proximoApos devolve o primeiro horário previsto depois de agora.
func (a Agendamento) proximoApos(agora time.Time, loc *time.Location) (time.Time, error) {
	s, err := a.horarioAte(agora, loc)
	if err != nil {
		return s, err
	}
	switch a.Frequencia {
	case FrequenciaMensal:
		return s.AddDate(0, 1, 0), nil
	case FrequenciaSemanal:
		return s.AddDate(0, 0, 7), nil
	}
	return s.AddDate(0, 0, 1), nil
}

// MesesDoAgendamento diz o que reler: no mensal, o mês anterior (fechado); no
// diário e no semanal, o anterior e o corrente.
func MesesDoAgendamento(frequencia string, horario time.Time) []time.Time {
	atual := time.Date(horario.Year(), horario.Month(), 1, 0, 0, 0, 0, time.UTC)
	if frequencia == FrequenciaMensal {
		return []time.Time{atual.AddDate(0, -1, 0)}
	}
	return []time.Time{atual.AddDate(0, -1, 0), atual}
}

// LerAgendamento devolve a configuração salva, ou a padrão.
func LerAgendamento(ctx context.Context, db *sql.DB, empresaID string, agora time.Time) (Agendamento, error) {
	a := AgendamentoPadrao()
	var ultimo sql.NullTime
	err := db.QueryRowContext(ctx, `SELECT ativo, frequencia, dia, hora, desde, ultimo_horario
		FROM auditoria.agendamento_erp WHERE empresa_id=$1::uuid`, empresaID).
		Scan(&a.Ativo, &a.Frequencia, &a.Dia, &a.Hora, &a.Desde, &ultimo)
	if errors.Is(err, sql.ErrNoRows) {
		return a, nil
	}
	if err != nil {
		return a, err
	}
	if ultimo.Valid {
		a.Ultimo = &ultimo.Time
	}
	if a.Ativo {
		loc, err := time.LoadLocation(FusoAgendamento)
		if err != nil {
			return a, err
		}
		p, err := a.proximoApos(agora, loc)
		if err != nil {
			return a, err
		}
		a.Proxima = &p
	}
	return a, nil
}

// SalvarAgendamento grava a configuração e a trilha na mesma transação. Salvar
// reinicia o marco "desde": um horário que já passou não dispara na hora.
func SalvarAgendamento(ctx context.Context, db *sql.DB, empresaID, userID string, a Agendamento, agora time.Time) error {
	if _, _, err := a.Validar(); err != nil {
		return err
	}
	if a.Frequencia == FrequenciaDiaria {
		a.Dia = 0
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO auditoria.agendamento_erp
			(empresa_id, ativo, frequencia, dia, hora, desde, atualizado_por, atualizado_em)
		VALUES ($1::uuid,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,$6)
		ON CONFLICT (empresa_id) DO UPDATE SET ativo=$2, frequencia=$3, dia=$4, hora=$5, desde=$6,
			atualizado_por=NULLIF($7,'')::uuid, atualizado_em=$6`,
		empresaID, a.Ativo, a.Frequencia, a.Dia, a.Hora, agora, userID); err != nil {
		return err
	}
	if err := trilha.Gravar(tx, empresaID, userID, "agendamento_erp", empresaID, "salvar",
		map[string]any{"ativo": a.Ativo, "frequencia": a.Frequencia, "dia": a.Dia, "hora": a.Hora}); err != nil {
		return err
	}
	return tx.Commit()
}

// Agendar dispara a importação se houver um horário vencido e ainda não
// disparado. Devolve nil quando não há o que fazer. Com outra importação em
// andamento, o horário continua pendente para a próxima volta.
func (i *ImportacaoERP) Agendar(ctx context.Context, agora time.Time) (*Execucao, error) {
	a, err := LerAgendamento(ctx, i.DB, i.EmpresaID, agora)
	if err != nil || !a.Ativo {
		return nil, err
	}
	loc, err := time.LoadLocation(FusoAgendamento)
	if err != nil {
		return nil, err
	}
	slot, err := a.horarioAte(agora, loc)
	if err != nil {
		return nil, err
	}
	if !slot.After(a.Desde) || (a.Ultimo != nil && !slot.After(*a.Ultimo)) {
		return nil, nil
	}
	// Reserva o horário antes de rodar: nunca dispara duas vezes o mesmo.
	r, err := i.DB.ExecContext(ctx, `UPDATE auditoria.agendamento_erp SET ultimo_horario=$2
		WHERE empresa_id=$1::uuid AND ativo AND (ultimo_horario IS NULL OR ultimo_horario < $2)`, i.EmpresaID, slot)
	if err != nil {
		return nil, err
	}
	if n, err := r.RowsAffected(); err != nil || n == 0 {
		return nil, err
	}
	p, err := i.Iniciar(ctx, EscopoAtualizacao{EmpresaID: i.EmpresaID}, GatilhoAgendador, MesesDoAgendamento(a.Frequencia, slot))
	if err != nil {
		if _, e := i.DB.ExecContext(ctx, `UPDATE auditoria.agendamento_erp SET ultimo_horario=$2 WHERE empresa_id=$1::uuid`,
			i.EmpresaID, a.Ultimo); e != nil {
			log.Printf("agendador: devolver o horário: %v", e)
		}
		return nil, fmt.Errorf("horário de %s não iniciado: %w", slot.Format("02/01 15:04"), err)
	}
	ex, err := i.Executar(ctx, p)
	return &ex, err
}

// RodarAgendador verifica o agendamento a cada intervalo, até o contexto acabar.
func (i *ImportacaoERP) RodarAgendador(ctx context.Context, intervalo time.Duration) {
	t := time.NewTicker(intervalo)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			i.volta(ctx)
		}
	}
}

func (i *ImportacaoERP) volta(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("agendador: pânico: %v", r)
		}
	}()
	ex, err := i.Agendar(ctx, i.Ingestor.agora())
	if err != nil {
		log.Printf("agendador: %v", err)
		return
	}
	if ex != nil {
		log.Printf("agendador: importação %d %s", ex.ID, ex.Status)
	}
}
