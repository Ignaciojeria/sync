package agent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"testboi1/internal/shared/mounted"
	agentapp "testboi1/pkg/agent/application"

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
	if ownerID := previewOwnerSessionIDFromRequest(c.Request()); ownerID != "" && id == ownerID {
		return "", fuego.HTTPError{Status: http.StatusNotFound, Detail: agentapp.ErrSessionNotFound.Error()}
	}
	return id, nil
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
		return fuego.HTTPError{Status: http.StatusNotFound, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrResumeUnavailable) {
		return fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrPreviewUnavailable) || errors.Is(err, agentapp.ErrPreviewLoopback) || errors.Is(err, agentapp.ErrPreviewNotApplicable) || errors.Is(err, agentapp.ErrPreviewNotMergeable) {
		return fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrPreviewAlreadyMerged) {
		return fuego.HTTPError{Status: http.StatusBadRequest, Detail: err.Error()}
	}
	if errors.Is(err, agentapp.ErrPreviewMergeConflict) || errors.Is(err, agentapp.ErrPreviewMergeBlocked) {
		return fuego.HTTPError{Status: http.StatusConflict, Detail: err.Error()}
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
