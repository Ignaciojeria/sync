package versions

import (
	"errors"
	"net/http"

	versionsapp "gitinittest5/internal/versions/application"
	versionsui "gitinittest5/internal/versions/ui"
	"gitinittest5/internal/shared/server"
	"gitinittest5/internal/ui/layout"
)

// listPage renderiza la lista de versiones. Limita a 30 para que la
// página cargue rápido incluso con merges frecuentes; si el repo
// tiene muchos merges históricos se puede paginar después.
func listPage(reader versionsapp.Reader) func(server.ContextNoBody) (any, error) {
	return func(c server.ContextNoBody) (any, error) {
		items, err := reader.List(c.Context(), 30)
		if err != nil {
			return writeError(c, err), nil
		}
		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		state := versionsui.ListState{Versions: items}
		page, err := layout.RenderPage(c, "Versiones", versionsui.ListPage(state, nav.PreviewPrefix))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	}
}

// detailPage renderiza el detalle de una versión + su diff vs HEAD.
// Si el SHA no existe o git falla, devolvemos 404 con un mensaje
// claro en vez de un stacktrace.
func detailPage(reader versionsapp.Reader) func(server.ContextNoBody) (any, error) {
	return func(c server.ContextNoBody) (any, error) {
		sha := c.Request().PathValue("sha")
		if sha == "" {
			return writeError(c, errors.New("versions: falta sha")), nil
		}
		v, err := reader.Get(c.Context(), sha)
		if err != nil {
			c.SetHeader("Content-Type", "text/html; charset=utf-8")
			c.Response().WriteHeader(http.StatusNotFound)
			nav := layout.FromRequest(c.Request())
			state := versionsui.ListState{
				Versions: []versionsapp.Version{},
				Error:    "Versión no encontrada: " + sha,
			}
			page, _ := layout.RenderPage(c, "Versión no encontrada", versionsui.ListPage(state, nav.PreviewPrefix))
			_ = page.Render(c.Context(), c.Response())
			return nil, nil
		}
		files, derr := reader.Diff(c.Context(), sha)
		if derr != nil {
			files = nil
		}
		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		state := versionsui.DetailState{Version: v, Files: files}
		page, err := layout.RenderPage(c, "Versión "+v.ShortSHA, versionsui.DetailPage(state, nav.PreviewPrefix))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	}
}

// writeError responde 500 con un mensaje en texto plano. La página
// de versiones no tiene vista de error propia todavía; si se vuelve
// recurrente conviene agregar una plantilla dedicada.
func writeError(c server.ContextNoBody, err error) any {
	c.SetHeader("Content-Type", "text/plain; charset=utf-8")
	c.Response().WriteHeader(http.StatusInternalServerError)
	return err.Error()
}