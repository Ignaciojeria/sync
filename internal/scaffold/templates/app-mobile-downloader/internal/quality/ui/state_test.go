package ui

import (
	"testing"
	"time"
)

func TestFormatTimestamp(t *testing.T) {
	if got := formatTimestamp(time.Time{}); got != "Nunca" {
		t.Fatalf("formatTimestamp(zero) = %q", got)
	}
	if got := formatTimestamp(time.Date(2026, 6, 15, 20, 30, 0, 0, time.UTC)); got != "15/06/2026 20:30" {
		t.Fatalf("formatTimestamp() = %q", got)
	}
}

func TestStatusHelpers(t *testing.T) {
	if statusLabel(true) != "Éxito" || statusLabel(false) != "Fallido" {
		t.Fatal("unexpected status labels")
	}
	if statusBadgeClass(true) != "badge badge-success" || statusBadgeClass(false) != "badge badge-error" {
		t.Fatal("unexpected status badge classes")
	}
}

func TestCoverageHelpers(t *testing.T) {
	if got := parseCoveragePercent("total: (statements) 88.1%"); got != 0 {
		t.Fatalf("template parseCoveragePercent() = %v, want 0 because it only parses go test format", got)
	}
	if got := parseCoveragePercent("coverage: 88.1% of statements"); got != 88.1 {
		t.Fatalf("template parseCoveragePercent() = %v", got)
	}

	if coverColorClass(85) != "text-success" || coverColorClass(60) != "text-warning" || coverColorClass(10) != "text-error" {
		t.Fatal("unexpected coverColorClass results")
	}
	if coverProgressClass(85) != "progress progress-success" || coverProgressClass(60) != "progress progress-warning" || coverProgressClass(10) != "progress progress-error" {
		t.Fatal("unexpected coverProgressClass results")
	}
	if coverBadgeClass(85) != "badge badge-success badge-sm" || coverBadgeClass(60) != "badge badge-warning badge-sm" || coverBadgeClass(10) != "badge badge-error badge-sm" {
		t.Fatal("unexpected coverBadgeClass results")
	}
}
