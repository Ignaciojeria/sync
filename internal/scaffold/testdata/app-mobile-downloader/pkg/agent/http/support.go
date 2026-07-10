package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	agentapp "scaffoldxd1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

type createSessionRequest struct {
	Title string `json:"title"`
	CWD   string `json:"cwd"`
	Model string `json:"model"`
}

type messageRequest struct {
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
	TurnID  string `json:"turnId,omitempty"`
}

type previewRequest struct {
	Port       int    `json:"port"`
	HealthPath string `json:"healthPath,omitempty"`
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
	if errors.Is(err, agentapp.ErrResumeUnavailable) {
		return fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrPreviewUnavailable) || errors.Is(err, agentapp.ErrPreviewLoopback) {
		return fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
	}
	if status, detail, ok := providerHTTPError(err); ok {
		return fuego.HTTPError{Status: status, Detail: detail}
	}
	return fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
}

func providerHTTPError(err error) (int, string, bool) {
	text := strings.TrimSpace(strings.ToLower(err.Error()))
	if text == "" {
		return 0, "", false
	}
	if strings.Contains(text, "insufficient_credits") ||
		strings.Contains(text, "créditos insuficientes") ||
		strings.Contains(text, "payment required") ||
		strings.Contains(text, `"status": 402`) ||
		strings.Contains(text, `"status":402`) {
		return http.StatusPaymentRequired, "Créditos insuficientes", true
	}
	return 0, "", false
}
