package layout

import (
	"context"
	"fixtests1/internal/shared/server"
	"github.com/a-h/templ"
	"io"
)

// RenderPage renderiza una pagina completa con el layout autenticado.
// Extrae el contexto de navegacion desde el request (path, permisos, etc.).
func RenderPage(c server.ContextNoBody, title string, content templ.Component) (templ.Component, error) {
	nav := FromRequest(c.Request())
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		ctx = templ.WithChildren(ctx, content)
		return LayoutWithNav(title, nav).Render(ctx, w)
	}), nil
}

// RenderPublicPage renderiza una página pública usando el tema activo, sin sidenav.
func RenderPublicPage(c server.ContextNoBody, title string, content templ.Component) (templ.Component, error) {
	nav := FromRequest(c.Request())
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		ctx = templ.WithChildren(ctx, content)
		return Layout(title, nav.ActiveThemeID, nav.ThemeCSSHref).Render(ctx, w)
	}), nil
}
