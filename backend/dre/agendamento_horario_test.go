package dre

import (
	"testing"
	"time"
)

func TestAgendamentoHorario(t *testing.T) {
	loc, err := time.LoadLocation(FusoAgendamento)
	if err != nil {
		t.Fatal(err)
	}
	em := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cas := []struct {
		a                      Agendamento
		agora, ultimo, proximo string
	}{
		{Agendamento{Frequencia: FrequenciaMensal, Dia: 5, Hora: "06:30"}, "2026-12-05 06:29", "2026-11-05 06:30", "2026-12-05 06:30"},
		{Agendamento{Frequencia: FrequenciaMensal, Dia: 5, Hora: "06:30"}, "2027-01-02 10:00", "2026-12-05 06:30", "2027-01-05 06:30"},
		{Agendamento{Frequencia: FrequenciaDiaria, Hora: "06:30"}, "2026-11-26 06:30", "2026-11-26 06:30", "2026-11-27 06:30"},
		// 26/11/2026 é quinta-feira; sexta = 5.
		{Agendamento{Frequencia: FrequenciaSemanal, Dia: 5, Hora: "18:00"}, "2026-11-26 12:00", "2026-11-20 18:00", "2026-11-27 18:00"},
	}
	for _, c := range cas {
		u, err := c.a.horarioAte(em(c.agora), loc)
		if err != nil {
			t.Fatal(err)
		}
		p, err := c.a.proximoApos(em(c.agora), loc)
		if err != nil {
			t.Fatal(err)
		}
		if !u.Equal(em(c.ultimo)) || !p.Equal(em(c.proximo)) {
			t.Errorf("%s dia %d em %s: último %s próximo %s", c.a.Frequencia, c.a.Dia, c.agora, u, p)
		}
	}
	for _, ruim := range []Agendamento{
		{Frequencia: FrequenciaMensal, Dia: 31, Hora: "06:30"}, {Frequencia: FrequenciaSemanal, Dia: 7, Hora: "06:30"},
		{Frequencia: FrequenciaDiaria, Hora: "25:00"}, {Frequencia: "anual", Hora: "06:30"},
	} {
		if _, _, err := ruim.Validar(); err == nil {
			t.Errorf("deveria recusar %+v", ruim)
		}
	}
}
