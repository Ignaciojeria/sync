package app

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	appui "app-mobile-downloader/internal/app/ui"
	"app-mobile-downloader/internal/shared/configuration"
	"app-mobile-downloader/internal/shared/server"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(authLoginHandler)

var randRead = rand.Read

func authLoginHandler(s *server.Server, conf configuration.Conf) {
	startGoogleLogin := func(c fuego.ContextNoBody, preferGoogle bool) (any, error) {
		state, err := randomState()
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: "cannot generate oauth state"}
		}

		http.SetCookie(c.Response(), &http.Cookie{
			Name:     "oidc_state",
			Value:    state,
			Path:     "/",
			HttpOnly: true,
			Secure:   isHTTPS(conf.OIDCRedirectURI),
			SameSite: http.SameSiteLaxMode,
		})

		if preferGoogle && strings.TrimSpace(conf.OIDCUpstreamGoogleClientID) != "" {
			googleURL, err := buildDirectGoogleLoginURL(conf, state)
			if err != nil {
				return nil, fuego.HTTPError{Status: http.StatusBadGateway, Title: "could not build direct google redirect", Detail: err.Error()}
			}
			http.Redirect(c.Response(), c.Request(), googleURL, http.StatusFound)
			return nil, nil
		}

		loginURL, err := buildLoginURL(conf, state, preferGoogle)
		if err != nil {
			return nil, fuego.HTTPError{Status: http.StatusInternalServerError, Detail: err.Error()}
		}

		http.Redirect(c.Response(), c.Request(), loginURL, http.StatusFound)
		return nil, nil
	}

	fuego.Get(s.Server, "/auth/login", func(c fuego.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		return nil, appui.LoginPage().Render(c.Request().Context(), c.Response())
	})
	fuego.Get(s.Server, "/auth/login/google", func(c fuego.ContextNoBody) (any, error) {
		return startGoogleLogin(c, true)
	})

	fuego.Get(s.Server, "/logo.jpeg", func(c fuego.ContextNoBody) (any, error) {
		http.ServeFile(c.Response(), c.Request(), "logo.jpeg")
		return nil, nil
	})
	fuego.Get(s.Server, "/logo.svg", func(c fuego.ContextNoBody) (any, error) {
		http.ServeFile(c.Response(), c.Request(), "logo.svg")
		return nil, nil
	})
	fuego.Get(s.Server, "/login.jpeg", func(c fuego.ContextNoBody) (any, error) {
		http.ServeFile(c.Response(), c.Request(), "login.jpeg")
		return nil, nil
	})
	fuego.Get(s.Server, "/login-bg.svg", func(c fuego.ContextNoBody) (any, error) {
		http.ServeFile(c.Response(), c.Request(), "login-bg.svg")
		return nil, nil
	})
}

func buildLoginURL(conf configuration.Conf, state string, preferGoogle bool) (string, error) {
	base := strings.TrimSpace(conf.OIDCLoginURL)
	if preferGoogle && strings.TrimSpace(conf.OIDCGoogleLoginURL) != "" {
		base = strings.TrimSpace(conf.OIDCGoogleLoginURL)
	}
	if base == "" {
		base = strings.TrimSpace(conf.OIDCAuthorizationEndpoint)
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", strings.TrimSpace(conf.OIDCClientID))
	q.Set("redirect_uri", strings.TrimSpace(conf.OIDCRedirectURI))
	q.Set("response_type", "code")
	q.Set("scope", firstNonEmptyScope(strings.TrimSpace(conf.OIDCScopes), "openid profile email"))
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func buildDirectGoogleLoginURL(conf configuration.Conf, state string) (string, error) {
	googleClientID := strings.TrimSpace(conf.OIDCUpstreamGoogleClientID)
	if googleClientID == "" {
		return "", fmt.Errorf("OIDC_UPSTREAM_GOOGLE_CLIENT_ID is empty")
	}
	issuer := strings.TrimRight(strings.TrimSpace(conf.OIDCIssuer), "/")
	if issuer == "" {
		return "", fmt.Errorf("OIDC_ISSUER is empty")
	}
	appName, err := deriveCasdoorAppName(conf.OIDCClientID, conf.PROJECT_NAME)
	if err != nil {
		return "", err
	}

	scope := firstNonEmptyScope(strings.TrimSpace(conf.OIDCScopes), "openid profile email")
	packedQ := url.Values{}
	packedQ.Set("client_id", strings.TrimSpace(conf.OIDCClientID))
	packedQ.Set("redirect_uri", strings.TrimSpace(conf.OIDCRedirectURI))
	packedQ.Set("response_type", "code")
	packedQ.Set("scope", scope)
	packedQ.Set("state", state)

	packed := "?" + packedQ.Encode() +
		"&application=" + url.QueryEscape(appName) +
		"&provider=" + url.QueryEscape("provider_google_einar") +
		"&method=" + url.QueryEscape("signup")
	packedState := base64.StdEncoding.EncodeToString([]byte(packed))

	googleQ := url.Values{}
	googleQ.Set("client_id", googleClientID)
	googleQ.Set("redirect_uri", issuer+"/callback")
	googleQ.Set("scope", "openid email profile")
	googleQ.Set("response_type", "code")
	googleQ.Set("state", packedState)
	return "https://accounts.google.com/signin/oauth?" + googleQ.Encode(), nil
}

func deriveCasdoorAppName(clientID, projectSlug string) (string, error) {
	clientID = strings.TrimSpace(clientID)
	projectSlug = strings.TrimSpace(projectSlug)
	if clientID == "" || projectSlug == "" {
		return "", fmt.Errorf("cannot derive app name: empty client id or project slug")
	}
	needle := "-" + projectSlug + "-"
	idx := strings.Index(clientID, needle)
	if idx < 0 {
		return "", fmt.Errorf("cannot derive app name from client id")
	}
	return clientID[:idx+len(needle)-1], nil
}

func randomState() (string, error) {
	b := make([]byte, 32)
	if _, err := randRead(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func isHTTPS(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https")
}

func firstNonEmptyScope(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
