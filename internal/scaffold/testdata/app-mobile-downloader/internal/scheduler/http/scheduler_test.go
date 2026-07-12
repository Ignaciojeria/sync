package scheduler

import (
	"testing"
	"time"

	schedulerapp "testboi1/internal/scheduler/application"
	"testboi1/internal/shared"
)

func TestShouldRun(t *testing.T) {
	t.Run("returns true when LastRunAt is nil", func(t *testing.T) {
		job := schedulerapp.JobConfig{Name: "j", Schedule: "0 * * * *"}
		if !shouldRun(job, time.Now()) {
			t.Fatal("expected true when job has never run")
		}
	})

	t.Run("returns false when schedule is invalid", func(t *testing.T) {
		now := time.Now()
		last := now.Add(-2 * time.Hour)
		job := schedulerapp.JobConfig{Name: "j", Schedule: "not-a-cron", LastRunAt: &last}
		if shouldRun(job, now) {
			t.Fatal("expected false for invalid cron schedule")
		}
	})

	t.Run("returns true for hourly job whose window has passed", func(t *testing.T) {
		// At hour H:30, the previous hour H:00 already passed. LastRunAt = H:30 → shouldRun -> false
		// At hour H:30, LastRunAt = H:15 → prevRun = H:00 should be before LastRunAt → shouldRun false
		// We craft a scenario where prevRun (H:00) is later than LastRunAt (yesterday) → shouldRun true.
		now := time.Date(2026, 6, 1, 12, 30, 0, 0, time.UTC)
		yesterdayHour0 := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
		job := schedulerapp.JobConfig{
			Name:      "hourly",
			Schedule:  "0 * * * *",
			LastRunAt: &yesterdayHour0,
		}
		if !shouldRun(job, now) {
			t.Fatal("expected true when last run is older than the previous schedule boundary")
		}
	})

	t.Run("returns false when last run covers the previous schedule window", func(t *testing.T) {
		now := time.Date(2026, 6, 1, 12, 30, 0, 0, time.UTC)
		later := now.Add(-1 * time.Minute) // < a previous hourly tick
		job := schedulerapp.JobConfig{
			Name:      "hourly",
			Schedule:  "0 * * * *",
			LastRunAt: &later,
		}
		// prevRun = 12:00 → 12:00 > later → shouldRun returns false.
		if shouldRun(job, now) {
			t.Fatal("expected false when last run is newer than the previous schedule boundary")
		}
	})
}

func TestStartSchedulerSkipsWhenDBNil(t *testing.T) {
	hooks := &shared.Hooks{}
	err := startScheduler(nil, hooks)
	if err != nil {
		t.Fatalf("startScheduler(nil) error = %v", err)
	}
	// Sin acceso al campo privado, verificamos que un Shutdown sin hooks registrados retorna nil.
	if err := hooks.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}
