package auth

import (
	authapp "gitinittest5/internal/auth/application"
	"gitinittest5/internal/shared/configuration"
	mounted "gitinittest5/internal/shared/mounted"
	"gitinittest5/internal/shared/server"
	"net/http"
	"strings"
)

func startGoogleLogin(c server.ContextNoBody, conf configuration.Conf, preferGoogle bool) (any, error) {
	state, err := authapp.NewRandomState()
	if err != nil {
		return nil, server.HTTPError{Status: http.StatusInternalServerError, Detail: "cannot generate oauth state"}
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
			return nil, server.HTTPError{Status: http.StatusBadGateway, Title: "could not build direct google redirect", Detail: err.Error()}
		}
		http.Redirect(c.Response(), c.Request(), googleURL, http.StatusFound)
		return nil, nil
	}

	loginURL, err := authapp.BuildLoginURL(conf, state, preferGoogle)
	if err != nil {
		return nil, server.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
	}

	http.Redirect(c.Response(), c.Request(), loginURL, http.StatusFound)
	return nil, nil
}

func serveStaticFile(path string) func(c server.ContextNoBody) (any, error) {
	return func(c server.ContextNoBody) (any, error) {
		http.ServeFile(c.Response(), c.Request(), path)
		return nil, nil
	}
}
