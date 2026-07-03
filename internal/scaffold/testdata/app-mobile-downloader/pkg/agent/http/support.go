package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	agentapp "app-mobile-downloader/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

type createSessionRequest struct {
	Title string `json:"title"`
	CWD   string `json:"cwd"`
	Model string `json:"model"`
}

type messageRequest struct {
	Message string `json:"message"`
}

func decodeJSON[T any](r *http.Request) (T, error) {
	var zero T
	if r.Body == nil {
		return zero, errors.New("missing request body")
	}
	defer r.Body.Close()
	var value T
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		return zero, err
	}
	return value, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(value)
}

func pathSessionID(c fuego.ContextNoBody) (string, error) {
	id := strings.TrimSpace(c.PathParam("id"))
	if id == "" {
		return "", fuego.HTTPError{Status: http.StatusBadRequest, Detail: "missing session id"}
	}
	return id, nil
}

func mapSessionError(err error) error {
	if errors.Is(err, agentapp.ErrSessionNotFound) {
		return fuego.HTTPError{Status: http.StatusNotFound, Detail: err.Error()}
	}
	return fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
}
