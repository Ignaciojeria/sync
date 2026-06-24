package layout

import (
	"net/http"

	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	designapp "app-mobile-downloader/internal/design/application"
	"app-mobile-downloader/internal/shared"
)

// NavigationContext contiene todo el contexto necesario para renderizar navegacion.
// El backend lo construye desde request/claims/session. Nunca desde templates.
type NavigationContext struct {
	CurrentPath   string
	IsEditor      bool
	ActiveThemeID string
	ThemeCSSHref  string
	Themes        []designapp.ResolvedTheme
	ActiveTheme   designapp.ResolvedTheme
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
	if !ok {
		return NavigationContext{
			CurrentPath:   r.URL.Path,
			IsEditor:      false,
			ActiveThemeID: activeThemeID,
			ThemeCSSHref:  themeCSSHref,
			Themes:        themes,
			ActiveTheme:   activeTheme,
		}
	}

	email := shared.FirstStringClaim(claims, "email")
	return NavigationContext{
		CurrentPath:   r.URL.Path,
		IsEditor:      shared.IsAllowedEditorEmail(email),
		ActiveThemeID: activeThemeID,
		ThemeCSSHref:  themeCSSHref,
		Themes:        themes,
		ActiveTheme:   activeTheme,
	}
}
