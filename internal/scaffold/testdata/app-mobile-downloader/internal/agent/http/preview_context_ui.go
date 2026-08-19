package agent

import (
	"context"
	"errors"

	agentapp "lastmile-agents/internal/agent/application"
	"lastmile-agents/internal/shared/mounted"
	"lastmile-agents/internal/shared/server"
	layoutui "lastmile-agents/internal/ui/layout"
	"net/http"
	"strings"
)

// ponytail: el flujo de Apply previo quedó obsoleto. Sólo queda el
// merge como acción: traer la rama de preview a la base es la
// integración real. El bar expone /agent/sessions/{id}/merge/ui y
// POST dispara m.MergePreview. El handler detecta tres outcomes
// (integró commits, no había nada, error) y crea una sesión nueva
// sólo cuando realmente integró algo.
func previewContextUIHandler(s *server.Server, manager agentapp.AgentService, requireEditor func(http.Handler) http.Handler) {
	server.Handle(s, "GET /agent/preview-context/ui", requireEditor(renderPreviewContextUI(manager)))
	server.Handle(s, "POST /agent/sessions/{id}/merge/ui", requireEditor(mergePreviewUI(manager)))
}

func renderPreviewContextUI(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := previewMergeBarState(r, manager, mergeBarInputs{})
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

// mergeBarInputs empaqueta los tres flags que el templ entiende para
// describir el estado del bar después de una operación. success +
// noChanges es válido (merge integró cero commits nuevos).
type mergeBarInputs struct {
	errorMessage     string
	success          bool
	noChanges        bool
	createdSessionID string
}

func mergePreviewUI(manager agentapp.AgentService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathSessionID(r)
		if err != nil {
			writeError(w, err)
			return
		}
		owner, ownerErr := manager.Get(r.Context(), id)
		if ownerErr != nil {
			writeError(w, mapSessionError(ownerErr))
			return
		}
		inputs := mergeBarInputs{}
		result, err := manager.MergePreview(r.Context(), id)
		if err != nil {
			inputs.errorMessage = mergePreviewErrorMessage(err)
		} else {
			inputs.success = true
			inputs.noChanges = result.NoChanges
			// Sólo creamos sesión nueva cuando el merge real
			// integró commits (NoChanges=false). Si no había nada,
			// quedarse en la sesión actual es lo más útil.
			if !result.NoChanges {
				if newID, ok := newSessionAfterMerge(r.Context(), manager, owner); ok {
					inputs.createdSessionID = newID
				}
			}
		}
		state, stateErr := previewMergeBarState(r, manager, inputs)
		if stateErr != nil {
			writeError(w, mapSessionError(stateErr))
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := layoutui.PreviewMergeBar(state).Render(r.Context(), w); err != nil {
			writeError(w, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()})
		}
	}
}

func previewMergeBarState(r *http.Request, manager agentapp.AgentService, inputs mergeBarInputs) (layoutui.PreviewMergeBarState, error) {
	prefix := currentPreviewPrefixFromRequest(r)
	ownerID := previewOwnerSessionIDFromMountPrefix(prefix)
	if ownerID == "" {
		return layoutui.PreviewMergeBarState{}, nil
	}
	session, err := manager.Get(r.Context(), ownerID)
	if err != nil {
		return layoutui.PreviewMergeBarState{}, err
	}
	// ponytail: el BackURL siempre es el del host (NO el del mount
	// del preview), para sacar al usuario de la preview del dueño
	// y dejarlo en el chat regular del agente. Si CreatedSessionID
	// viene informado (caso feliz tras un merge con cambios),
	// apuntamos a la sesión nueva; si no, caemos al dueño.
	backURL := mounted.Host("/agent?session=" + session.ID)
	if strings.TrimSpace(inputs.createdSessionID) != "" {
		backURL = mounted.Host("/agent?session=" + strings.TrimSpace(inputs.createdSessionID))
	}
	return layoutui.PreviewMergeBarState{
		Active:        true,
		SessionID:     session.ID,
		BackURL:       backURL,
		NoChanges:     inputs.noChanges,
		BaseBranch:    session.BaseBranch,
		PreviewBranch: session.Branch,
		Applicable:    previewMergeable(session),
		ActionPath:    "/agent/sessions/" + session.ID + "/merge/ui",
		ErrorMessage:  inputs.errorMessage,
		Success:       inputs.success,
	}, nil
}

// previewMergeable reemplaza al viejo previewApplicable para el flujo
// de merge: necesita Branch + BaseBranch no vacíos (no requiere
// SourcePath como el apply viejo, que era file-copy based).
func previewMergeable(session agentapp.Session) bool {
	if strings.TrimSpace(session.ID) == "" {
		return false
	}
	if strings.TrimSpace(session.Branch) == "" || strings.TrimSpace(session.BaseBranch) == "" {
		return false
	}
	return true
}

// newSessionAfterMerge crea una sesión nueva heredando CWD y Model
// del dueño. Devuelve (newID, true) si la creación fue exitosa; en
// caso contrario cae al comportamiento histórico de apuntar al
// dueño. Heredar Model garantiza que la nueva conversación use el
// mismo modelo que la dueña; el sandbox CWD lo resuelve
// pirpc.resolveCWD como cualquier sesión nueva.
func newSessionAfterMerge(ctx context.Context, manager agentapp.AgentService, owner agentapp.Session) (string, bool) {
	created, err := manager.Create(ctx, agentapp.CreateSessionInput{
		Title: "",
		CWD:   owner.CWD,
		Model: owner.Model,
	})
	if err != nil {
		return "", false
	}
	id := strings.TrimSpace(created.ID)
	if id == "" {
		return "", false
	}
	return id, true
}

// mergePreviewErrorMessage mapea errores del Manager a strings
// amigables para el bar. Las Err* son los sentinels definidos en
// internal/agent/application/manager.go. Detectamos por errors.Is
// (no por substring) porque los mensajes envueltos los wrappea el
// Manager con fmt.Errorf("%w: %v", ...).
func mergePreviewErrorMessage(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, agentapp.ErrPreviewAlreadyMerged):
		return "Esta preview ya fue mergeada."
	case errors.Is(err, agentapp.ErrPreviewMergeConflict):
		return "Conflicto de merge entre la preview y el branch base. Resolvelo manualmente."
	case errors.Is(err, agentapp.ErrPreviewMergeBlocked):
		return "El workspace base tiene cambios sin commitear o la preview no está lista para merge."
	case errors.Is(err, agentapp.ErrPreviewNotMergeable):
		return "La preview no está lista para merge (sin branch o sin base branch)."
	default:
		return "No se pudo mergear la preview."
	}
}
