package layout

import (
	"net/http"

	authmiddleware "scaffoldxd1/internal/auth/middleware"
	designapp "scaffoldxd1/internal/design/application"
	"scaffoldxd1/internal/shared"
	mounted "scaffoldxd1/internal/shared/mounted"
)

// NavigationContext contiene todo el contexto necesario para renderizar navegacion.
// El backend lo construye desde request/claims/session. Nunca desde templates.
type NavigationContext struct {
	CurrentPath   string
	PreviewPrefix string
	IsEditor      bool
	ActiveThemeID string
	ThemeCSSHref  string
	Themes        []designapp.ResolvedTheme
	ActiveTheme   designapp.ResolvedTheme
}

func (n NavigationContext) AppPath(path string) string {
	return mounted.App(n.PreviewPrefix, path)
}

func (n NavigationContext) HostPath(path string) string {
	return mounted.Host(path)
}

// FromRequest construye el contexto desde el request actual.
func FromRequest(r *http.Request) NavigationContext {
	ctx := r.Context()
	claims, ok := authmiddleware.JWTClaimsFromContext(ctx)
	activeThemeID := "light"
	themeCSSHref := ""
	var themes []designapp.ResolvedTheme
	var activeTheme designapp.ResolvedTheme
	if catalog, err := designapp.DefaultCatalog(); err == nil {
		activeThemeID = catalog.ActiveThemeIDFromRequest(r)
		themeCSSHref = "/design/theme/" + activeThemeID
		themes = catalog.Themes()
		activeTheme, _ = catalog.ThemeByID(activeThemeID)
	}
	previewPrefix := mounted.Prefix(r)
	themeCSSHref = mounted.App(previewPrefix, themeCSSHref)
	currentPath := mounted.Relative(previewPrefix, r.URL.Path)
	if !ok {
		return NavigationContext{
			CurrentPath:   currentPath,
			PreviewPrefix: previewPrefix,
			IsEditor:      false,
			ActiveThemeID: activeThemeID,
			ThemeCSSHref:  themeCSSHref,
			Themes:        themes,
			ActiveTheme:   activeTheme,
		}
	}

	email := shared.FirstStringClaim(claims, "email")
	return NavigationContext{
		CurrentPath:   currentPath,
		PreviewPrefix: previewPrefix,
		IsEditor:      shared.IsAllowedEditorEmail(email),
		ActiveThemeID: activeThemeID,
		ThemeCSSHref:  themeCSSHref,
		Themes:        themes,
		ActiveTheme:   activeTheme,
	}
}
