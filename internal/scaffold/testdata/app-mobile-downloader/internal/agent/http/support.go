package agent

import (
	"encoding/json"
	"errors"
	agentapp "fixtests1/internal/agent/application"
	"fixtests1/internal/shared/mounted"
	"fixtests1/internal/shared/server"
	"net/http"
	"net/url"
	"strings"
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
	if r == nil || r.Body == nil {
		return zero, errors.New("missing request body")
	}
	defer r.Body.Close()
	var value T
	if err := json.NewDecoder(r.Body).Decode(&value); err != nil {
		return zero, err
	}
	return value, nil
}

func pathSessionID(r *http.Request) (string, error) {
	if r == nil {
		return "", server.HTTPError{Status: http.StatusBadRequest, Detail: "missing session id"}
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		return "", server.HTTPError{Status: http.StatusBadRequest, Detail: "missing session id"}
	}
	if ownerID := previewOwnerSessionIDFromRequest(r); ownerID != "" && id == ownerID {
		return "", server.HTTPError{Status: http.StatusNotFound, Detail: agentapp.ErrSessionNotFound.Error()}
	}
	return id, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	if status > 0 {
		w.WriteHeader(status)
	}
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	detail := strings.TrimSpace(err.Error())
	if he, ok := err.(server.HTTPError); ok {
		if he.Status > 0 {
			status = he.Status
		}
		if strings.TrimSpace(he.Detail) != "" {
			detail = he.Detail
		}
	} else if he, ok := any(err).(*server.HTTPError); ok && he != nil {
		if he.Status > 0 {
			status = he.Status
		}
		if strings.TrimSpace(he.Detail) != "" {
			detail = he.Detail
		}
	}
	writeJSON(w, status, map[string]any{"detail": detail})
}

func previewOwnerSessionIDFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	return previewOwnerSessionIDFromMountPrefix(mounted.Prefix(r))
}

func currentPreviewPrefixFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if prefix := strings.TrimSpace(mounted.Prefix(r)); prefix != "" {
		return prefix
	}
	for _, raw := range []string{r.Header.Get("HX-Current-URL"), r.Referer()} {
		if prefix := previewPrefixFromRawURL(raw); prefix != "" {
			return prefix
		}
	}
	return ""
}

func previewPrefixFromRawURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return previewPrefixFromPath(u.Path)
}

func previewPrefixFromPath(path string) string {
	path = mounted.NormalizePath(path)
	const marker = "/preview"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	prefix := path[:idx+len(marker)]
	if !strings.HasPrefix(prefix, "/agent/sessions/") {
		return ""
	}
	return strings.TrimRight(mounted.NormalizePath(prefix), "/") + "/"
}

func previewOwnerSessionIDFromMountPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	prefix = strings.Trim(prefix, "/")
	parts := strings.Split(prefix, "/")
	if len(parts) < 4 {
		return ""
	}
	if parts[0] != "agent" || parts[1] != "sessions" || parts[3] != "preview" {
		return ""
	}
	return strings.TrimSpace(parts[2])
}

func mapSessionError(err error) error {
	if errors.Is(err, agentapp.ErrSessionNotFound) {
		return server.HTTPError{Status: http.StatusNotFound, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrResumeUnavailable) {
		return server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrPreviewUnavailable) || errors.Is(err, agentapp.ErrPreviewLoopback) || errors.Is(err, agentapp.ErrPreviewNotApplicable) || errors.Is(err, agentapp.ErrPreviewNotMergeable) {
		return server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrPreviewAlreadyMerged) {
		return server.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrPreviewMergeConflict) || errors.Is(err, agentapp.ErrPreviewMergeBlocked) {
		return server.HTTPError{Status: http.StatusConflict, Detail: err.Error()}
	}
	if status, detail, ok := providerHTTPError(err); ok {
		return server.HTTPError{Status: status, Detail: detail}
	}
	return server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
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
