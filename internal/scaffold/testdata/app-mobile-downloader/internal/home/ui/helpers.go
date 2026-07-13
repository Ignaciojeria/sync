package ui

import (
	"fmt"
	"strings"
	"time"

	topologyapp "fixtests1/internal/topology/application"
)

func statusBadgeClass(status string) string {
	switch status {
	case topologyapp.StatusRunning:
		return "badge badge-success badge-outline"
	case topologyapp.StatusSyncing:
		return "badge badge-info badge-outline"
	case topologyapp.StatusDegraded:
		return "badge badge-warning badge-outline"
	case topologyapp.StatusOffline:
		return "badge badge-error badge-outline"
	default:
		return "badge badge-ghost"
	}
}

func formatStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return "unknown"
	}
	return strings.ToUpper(status[:1]) + status[1:]
}

func generatedLabel(t time.Time) string {
	if t.IsZero() {
		return "ahora"
	}
	return fmt.Sprintf("Actualizado %s", t.Format("15:04:05"))
}

func sessionPrimaryLabel(clientName string) string {
	primary, _ := splitSessionClientName(clientName)
	return primary
}

func sessionSecondaryLabel(clientName string) string {
	_, secondary := splitSessionClientName(clientName)
	return secondary
}

func splitSessionClientName(clientName string) (string, string) {
	clientName = strings.TrimSpace(clientName)
	parts := strings.SplitN(clientName, " · ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return clientName, ""
}
