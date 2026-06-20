package auth

import (
	"net/http"
	"strings"

	authapp "app-mobile-downloader/internal/auth/application"
	"app-mobile-downloader/internal/shared/configuration"

	"github.com/go-fuego/fuego"
)

func startGoogleLogin(c fuego.ContextNoBody, conf configuration.Conf, preferGoogle bool) (any, error) {
	return startGoogleLoginWithStateGenerator(c, conf, preferGoogle, authapp.NewRandomState)
}

func startGoogleLoginWithStateGenerator(c fuego.ContextNoBody, conf configuration.Conf, preferGoogle bool, generateState func() (string, error)) (any, error) {
	state, err := generateState()
	if err != nil {
		return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "cannot generate oauth state"}
	}

	http.SetCookie(c.Response(), &http.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   authapp.IsHTTPS(conf.OIDCRedirectURI),
		SameSite: http.SameSiteLaxMode,
	})

	if preferGoogle && strings.TrimSpace(conf.OIDCUpstreamGoogleClientID) != "" {
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
