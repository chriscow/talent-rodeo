package main

import (
	"errors"
	"testing"
	"time"

	"talent-rodeo/internal"
)

func TestValidateOutboundAppointment(t *testing.T) {
	now := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	request := outboundCallRequest{AppointmentID: "appointment", UserID: "owner"}
	appointment := internal.Appointment{
		ID: "appointment", UserID: "owner", Status: internal.AppointmentStatusScheduled,
		ScheduledTime: now.Add(-time.Minute),
	}
	tests := []struct {
		name        string
		appointment internal.Appointment
		want        error
	}{
		{name: "due", appointment: appointment},
		{name: "early", appointment: func() internal.Appointment { a := appointment; a.ScheduledTime = now.Add(time.Second); return a }(), want: errOutboundNotDue},
		{name: "stale", appointment: func() internal.Appointment { a := appointment; a.ScheduledTime = now.Add(-time.Hour); return a }(), want: errOutboundExpired},
		{name: "cancelled", appointment: func() internal.Appointment {
			a := appointment
			a.Status = internal.AppointmentStatusCancelled
			return a
		}(), want: errOutboundNotScheduled},
		{name: "different owner", appointment: func() internal.Appointment { a := appointment; a.UserID = "other"; return a }(), want: errOutboundNotScheduled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateOutboundAppointment(tt.appointment, request, now); !errors.Is(err, tt.want) {
				t.Errorf("validateOutboundAppointment(%s) = %v, want %v", tt.name, err, tt.want)
			}
		})
	}
}
