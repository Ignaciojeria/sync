package layout

import (
	"net/http"

	authmiddleware "app-mobile-downloader/internal/auth/middleware"
	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/access"
)

// NavigationContext contiene todo el contexto necesario para renderizar navegacion.
// El backend lo construye desde request/claims/session. Nunca desde templates.
type NavigationContext struct {
	CurrentPath string
	IsEditor    bool
}

// FromRequest construye el contexto desde el request actual.
func FromRequest(r *http.Request) NavigationContext {
	ctx := r.Context()
	claims, ok := authmiddleware.JWTClaimsFromContext(ctx)
	if !ok {
		return NavigationContext{
			CurrentPath: r.URL.Path,
			IsEditor:    false,
		}
	}

	email := shared.FirstStringClaim(claims, "email")
	return NavigationContext{
		CurrentPath: r.URL.Path,
		IsEditor:    access.IsAllowedEditorEmail(email),
	}
}
