package internal

import (
	"testing"
	"time"
)

func TestPersonalSearchTaskEligible(t *testing.T) {
	activation := time.Date(2026, time.October, 9, 0, 0, 0, 0, time.UTC)
	owner := "owner"
	other := "other"
	tests := []struct {
		name string
		task *SearchTask
		want bool
	}{
		{name: "nil task"},
		{name: "legacy task", task: &SearchTask{CreatedAt: activation}},
		{name: "other owner", task: &SearchTask{UserID: &other, CreatedAt: activation}},
		{name: "before activation", task: &SearchTask{UserID: &owner, CreatedAt: activation.Add(-time.Nanosecond)}},
		{name: "at activation", task: &SearchTask{UserID: &owner, CreatedAt: activation}, want: true},
		{name: "after activation", task: &SearchTask{UserID: &owner, CreatedAt: activation.Add(time.Hour)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PersonalSearchTaskEligible(tt.task, owner, activation); got != tt.want {
				t.Errorf("PersonalSearchTaskEligible(%v) = %t, want %t", tt.task, got, tt.want)
			}
		})
	}
}
