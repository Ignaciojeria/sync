package agent

import (
	agentapp "fixtests1/internal/agent/application"
	"fixtests1/internal/shared/mounted"
	"fixtests1/internal/shared/server"
	layoutui "fixtests1/internal/ui/layout"
	"net/http"
	"strings"
)

func previewContextUIHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "GET /agent/preview-context/ui", requireEditor(renderPreviewContextUI(manager)))
	server.Handle(s, "POST /agent/sessions/{id}/apply/ui", requireEditor(applyPreviewUI(manager)))
}

func renderPreviewContextUI(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := previewMergeBarState(r, manager, "")
		if err != nil {
			writeError(w, mapSessionError(err))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := layoutui.PreviewMergeBar(state).Render(r.Context(), w); err != nil {
			writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
		}
	}
}

func applyPreviewUI(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		success := false
		errorMessage := ""
		if _, err := manager.ApplyPreview(r.Context(), id); err != nil {
			errorMessage = applyPreviewErrorMessage(err)
		} else {
			success = true
		}
		state, stateErr := previewMergeBarState(r, manager, errorMessage)
		if stateErr != nil {
			writeError(w, mapSessionError(stateErr))
			return
		}
		state.Success = success
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := layoutui.PreviewMergeBar(state).Render(r.Context(), w); err != nil {
			writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
		}
	}
}

func previewMergeBarState(r *http.Request, manager agentapp.AgentService, errorMessage string) (layoutui.PreviewMergeBarState, error) {
	prefix := currentPreviewPrefixFromRequest(r)
	ownerID := previewOwnerSessionIDFromMountPrefix(prefix)
	if ownerID == "" {
		return layoutui.PreviewMergeBarState{}, nil
	}
	session, err := manager.Get(r.Context(), ownerID)
	if err != nil {
		return layoutui.PreviewMergeBarState{}, err
	}
	return layoutui.PreviewMergeBarState{
		Active:        true,
		SessionID:     session.ID,
		BackURL:       mounted.App(prefix, "/agent?session="+session.ID),
		BaseBranch:    session.BaseBranch,
		PreviewBranch: session.Branch,
		Applicable:    previewApplicable(session),
		ActionPath:    "/agent/sessions/" + session.ID + "/apply/ui",
		ErrorMessage:  strings.TrimSpace(errorMessage),
	}, nil
}

func applyPreviewErrorMessage(err error) string {
	text := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(strings.ToLower(text), "not applicable"):
		return "Esta preview no se puede aplicar todavía."
	default:
		return "No se pudo aplicar esta preview."
	}
}
