package http

import (
	"strings"

	backlogapp "gitinittest5/internal/backlog/application"
	backlogui "gitinittest5/internal/backlog/ui"
	"gitinittest5/internal/shared/server"
	"gitinittest5/internal/ui/layout"

	"github.com/a-h/templ"
)

// cardDetailHandler expone /backlog/cards/{slug}/detail con doble
// comportamiento según HX-Request:
//   - Si NO es HTMX: devuelve la página completa (deep linkeable).
//   - Si ES HTMX: devuelve solo el fragmento del detalle, pensado
//     para inyectarse dentro del <dialog> del board.
func cardDetailHandler(s *server.Server, svc *backlogapp.Service, mw server.RouteOption) {
	server.Get(s, "/backlog/cards/{slug}/detail", func(c server.ContextNoBody) (any, error) {
		slug := strings.TrimSpace(c.PathParam("slug"))
		if slug == "" {
			return nil, server.HTTPError{Status: 400, Detail: "missing card slug"}
		}
		card, err := svc.Get(c.Context(), slug)
		if err != nil {
			return nil, translateError(err)
		}

		previewPrefix := extractPreviewPrefix(c)
		isHTMX := strings.EqualFold(c.Request().Header.Get("HX-Request"), "true")

		if !isHTMX {
			// Navegación directa → página completa con layout.
			page, err := layout.RenderPage(c, card.Title,
				backlogui.Detail(backlogui.ToCardView(card), previewPrefix))
			if err != nil {
				return nil, err
			}
			return nil, page.Render(c.Context(), c.Response())
		}

		// HTMX → fragmento del detalle para inyectar en el modal.
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		if err := backlogui.RenderDetailFragment(c.Response(), c.Context(), card, previewPrefix); err != nil {
			return nil, err
		}
		return nil, nil
	}, mw)
}

// guard: templ import se mantiene para futuras extensiones (p. ej.
// headers personalizados desde los handlers de detail).
var _ = templ.Component(nil)
