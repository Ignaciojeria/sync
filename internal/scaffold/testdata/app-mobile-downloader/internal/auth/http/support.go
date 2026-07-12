package auth

import (
	"net/http"
	"strings"

	authapp "testboi1/internal/auth/application"
	"testboi1/internal/shared/configuration"
	mounted "testboi1/internal/shared/mounted"

	"github.com/go-fuego/fuego"
)

func startGoogleLogin(c fuego.ContextNoBody, conf configuration.Conf, preferGoogle bool) (any, error) {
	state, err := authapp.NewRandomState()
	if err != nil {
		return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "cannot generate oauth state"}
	}

	secure := authapp.IsHTTPS(conf.OIDCRedirectURI)
	http.SetCookie(c.Response(), &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
	returnTo := strings.TrimSpace(c.QueryParam("return_to"))
	if mounted.IsSafeReturnTo(returnTo) {
		mounted.SetReturnToCookie(c.Response(), c.Request(), returnTo, secure)
	} else if mounted.ReadReturnTo(c.Request()) == "" {
		mounted.SetReturnToCookie(c.Response(), c.Request(), mounted.CurrentAppURL(c.Request()), secure)
	}

	if preferGoogle && conf.OIDCUpstreamGoogleClientID != "" {
		googleURL, err := authapp.BuildDirectGoogleLoginURL(conf, state)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusBadGateway, Title: "could not build direct google redirect", Detail: err.Error()}
		}
		http.Redirect(c.Response(), c.Request(), googleURL, http.StatusFound)
		return nil, nil
	}

	loginURL, err := authapp.BuildLoginURL(conf, state, preferGoogle)
	if err != nil {
		return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	http.Redirect(c.Response(), c.Request(), loginURL, http.StatusFound)
	return nil, nil
}

func serveStaticFile(path string) func(c fuego.ContextNoBody) (any, error) {
	return func(c fuego.ContextNoBody) (any, error) {
		http.ServeFile(c.Response(), c.Request(), path)
		return nil, nil
	}
}
