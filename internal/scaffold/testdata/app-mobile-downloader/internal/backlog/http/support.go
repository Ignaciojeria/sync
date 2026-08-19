package http

import (
	"errors"
	"strconv"

	backlogapp "gitinittest5/internal/backlog/application"
	"gitinittest5/internal/shared/mounted"
	"gitinittest5/internal/shared/server"
)

// parsePriority acepta el valor de un form o query param como string
// ("P0".."P3"). Si es inválido, devuelve ErrInvalidInput.
func parsePriority(raw string) (backlogapp.Priority, error) {
	p := backlogapp.Priority(raw)
	if !p.Valid() {
		return "", backlogapp.ErrInvalidInput
	}
	return p, nil
}

// parsePriorityInt acepta el valor numérico de un form (delta) y
// devuelve el string Priority correspondiente.
func parsePriorityInt(raw string) (backlogapp.Priority, error) {
	v, err := strconv.Atoi(raw)
	if err != nil {
		return "", backlogapp.ErrInvalidInput
	}
	p := backlogapp.Priority("P" + strconv.Itoa(v))
	if !p.Valid() {
		return "", backlogapp.ErrInvalidInput
	}
	return p, nil
}

// translateError mapea errores de application a HTTPError.
func translateError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, backlogapp.ErrNotFound) {
		return server.HTTPError{Status: 404, Detail: "not found"}
	}
	if errors.Is(err, backlogapp.ErrInvalidInput) {
		return server.HTTPError{Status: 400, Detail: err.Error()}
	}
	return server.HTTPError{Status: 500, Detail: err.Error()}
}

// extractPreviewPrefix devuelve el prefijo de preview (si lo hay) a
// partir del request. Centralizado para que todos los handlers del
// módulo compongan URLs idénticas a las del Page principal.
func extractPreviewPrefix(c server.ContextNoBody) string {
	if c == nil {
		return ""
	}
	r := c.Request()
	if r == nil {
		return ""
	}
	return mounted.Prefix(r)
}
