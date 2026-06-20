package ui

import (
	"regexp"
	"strconv"
	"time"
)

type TestRunState struct {
	Success       bool
	Output        string
	CoverPath     string
	CoverPercent  float64
	Timestamp     time.Time
	HasResult     bool
}

func formatTimestamp(t time.Time) string {
	if t.IsZero() {
		return "Nunca"
	}
	return t.Format("02/01/2006 15:04")
}

func statusLabel(success bool) string {
	if success {
		return "Éxito"
	}
	return "Fallido"
}

func statusBadgeClass(success bool) string {
	if success {
		return "badge badge-success"
	}
	return "badge badge-error"
}

func parseCoveragePercent(output string) float64 {
	re := regexp.MustCompile(`coverage:\s*([0-9]+\.[0-9]+)%`)
	matches := re.FindStringSubmatch(output)
	if len(matches) >= 2 {
		p, _ := strconv.ParseFloat(matches[1], 64)
		return p
	}
	return 0
}

func coverColorClass(percent float64) string {
	if percent >= 80 {
		return "text-success"
	}
	if percent >= 50 {
		return "text-warning"
	}
	return "text-error"
}

func coverProgressClass(percent float64) string {
	if percent >= 80 {
		return "progress progress-success"
	}
	if percent >= 50 {
		return "progress progress-warning"
	}
	return "progress progress-error"
}

func coverBadgeClass(percent float64) string {
	if percent >= 80 {
		return "badge badge-success badge-sm"
	}
	if percent >= 50 {
		return "badge badge-warning badge-sm"
	}
	return "badge badge-error badge-sm"
}
