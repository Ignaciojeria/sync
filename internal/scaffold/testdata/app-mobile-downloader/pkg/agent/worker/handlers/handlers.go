// Package handlers implementa los endpoints de datos del agente (los que
// ejercen acciones o devuelven eventos) usando stdlib net/http. Antes
// estos handlers vivían en pkg/agent/http/ junto al pageHandler, pero
// en la nueva topología de 3 procesos viven aquí (en el worker) para
// que su ciclo de restarts sea independiente del web-server.
//
// Conviven con pkg/agent/http/page.go, que sigue en el web-server
// porque renderiza la UI completa (templ + layout) y depende de varias
// piezas host-side (tema, navegador de sesiones lateral, etc.).
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	agentapp "app-mobile-downloader/pkg/agent/application"
)

// sessionIDFromPath extrae el session id de paths como
//   /agent/sessions/abc123/prompt
//   /agent/sessions/abc123/events
//   /agent/sessions/abc123/abort
func sessionIDFromPath(path string) string {
	const prefix = "/agent/sessions/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.Index(rest, "/"); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.TrimSpace(rest)
}

// extractSessionID devuelve el session id del path. Para URLs como
//   /agent/sessions/abc123/prompt
// devuelve "abc123". Devuelve "" si el path no encaja con el patrón.
func extractSessionID(r *http.Request) string {
	return sessionIDFromPath(r.URL.Path)
}

// readJSON decodifica el body al tipo T. Devuelve error si el body
// falta o si el JSON está malformado. Cap de 1MB para evitar bombas.
func readJSON[T any](r *http.Request) (T, error) {
	var zero T
	if r.Body == nil {
		return zero, errors.New("missing request body")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	var value T
	if err := dec.Decode(&value); err != nil {
		return zero, fmt.Errorf("decode json: %w", err)
	}
	return value, nil
}

// writeJSON serializa value como JSON y lo escribe en w con el status
// dado.
func writeJSON(w http.ResponseWriter, status int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(value)
}

// mapSessionError convierte errores de la capa application a respuestas
// HTTP. ErrSessionNotFound → 404, cualquier otro → 500.
func mapSessionError(err error) (int, string) {
	if errors.Is(err, agentapp.ErrSessionNotFound) {
		return http.StatusNotFound, err.Error()
	}
	return http.StatusInternalServerError, err.Error()
}
