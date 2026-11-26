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
		u, _, err := c.a.horarioAte(em(c.agora), loc)
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
	// sexta + primeiro dia útil: 01/12/2026 é terça; 01/11/2026 é domingo.
	s := Agendamento{Frequencia: FrequenciaSemanal, Dia: 5, Hora: "06:30", PrimeiroUtil: true}
	for _, c := range []struct {
		agora, ultimo string
		util          bool
		proximo       string
	}{
		{"2026-11-28 10:00", "2026-11-27 06:30", false, "2026-12-01 06:30"},
		{"2026-12-01 07:00", "2026-12-01 06:30", true, "2026-12-04 06:30"},
		{"2026-11-02 07:00", "2026-11-02 06:30", true, "2026-11-06 06:30"},
	} {
		u, util, err := s.horarioAte(em(c.agora), loc)
		if err != nil {
			t.Fatal(err)
		}
		p, err := s.proximoApos(em(c.agora), loc)
		if err != nil {
			t.Fatal(err)
		}
		if !u.Equal(em(c.ultimo)) || util != c.util || !p.Equal(em(c.proximo)) {
			t.Errorf("em %s: último %s util %v próximo %s", c.agora, u, util, p)
		}
	}
	if m := MesesDoAgendamento(FrequenciaSemanal, true, em("2026-12-01 06:30")); len(m) != 1 || m[0].Month() != time.November {
		t.Errorf("primeiro dia útil relê o mês anterior: %v", m)
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
