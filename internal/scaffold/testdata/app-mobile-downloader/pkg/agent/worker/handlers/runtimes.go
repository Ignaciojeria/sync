package handlers

import (
	"context"
	"net/http"
	"time"

	agentapp "testboi1/pkg/agent/application"
)

// handleRuntimes devuelve la lista de procesos `pi` corriendo en el
// host del worker.
//
// Ruta: GET /agent/runtimes
// Auth: requerida (igual que el resto de los endpoints de datos).
//
// El body JSON incluye `count`, `rssKb` (suma) y `runtimes` (lista
// detallada con pid/elapsed/cwd/owner). El cliente usa count + rssKb
// para el pill del header; la lista completa se muestra en un panel
// bajo demanda.
func handleRuntimes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	procs, err := agentapp.ListAgentRuntimes(ctx)
	if err != nil {
		http.Error(w, "list runtimes: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var totalRSS int64
	for _, p := range procs {
		totalRSS += p.RSSKB
	}

	_ = writeJSON(w, http.StatusOK, map[string]any{
		"count":    len(procs),
		"rssKb":    totalRSS,
		"runtimes": procs,
	})
}
