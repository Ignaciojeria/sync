package agent

import (
	"net/http"
	"strings"

	"testboi1/internal/shared/server"
	layoutui "testboi1/internal/ui/layout"
	agentapp "testboi1/pkg/agent/application"

	"github.com/go-fuego/fuego"
)

func previewContextUIHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	mw := fuego.OptionMiddleware(requireEditor)
	fuego.Get(s.Server, "/agent/preview-context/ui", renderPreviewContextUI(manager), mw)
	fuego.Post(s.Server, "/agent/sessions/{id}/apply/ui", applyPreviewUI(manager), mw)
}

func renderPreviewContextUI(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		state, err := previewMergeBarState(c.Request(), manager, "")
		if err != nil {
			return nil, mapSessionError(err)
		}
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return nil, layoutui.PreviewMergeBar(state).Render(c.Context(), c.Response())
	}
}

func applyPreviewUI(manager agentapp.AgentService) func(fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		id, err := pathSessionID(c)
		if err != nil {
			return nil, err
		}
		success := false
		errorMessage := ""
		if _, err := manager.ApplyPreview(c.Context(), id); err != nil {
			errorMessage = applyPreviewErrorMessage(err)
		} else {
			success = true
		}
		state, stateErr := previewMergeBarState(c.Request(), manager, errorMessage)
		if stateErr != nil {
			return nil, mapSessionError(stateErr)
		}
		state.Success = success
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return nil, layoutui.PreviewMergeBar(state).Render(c.Context(), c.Response())
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
