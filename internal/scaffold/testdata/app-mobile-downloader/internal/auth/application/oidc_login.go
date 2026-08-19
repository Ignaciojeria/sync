package auth

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"gitinittest5/internal/shared"
	"gitinittest5/internal/shared/configuration"
)

func BuildLoginURL(conf configuration.Conf, state string, preferGoogle bool) (string, error) {
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
	q.Set("scope", shared.FirstNonEmpty(strings.TrimSpace(conf.OIDCScopes), "openid profile email"))
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func BuildDirectGoogleLoginURL(conf configuration.Conf, state string) (string, error) {
	googleClientID := strings.TrimSpace(conf.OIDCUpstreamGoogleClientID)
	if googleClientID == "" {
		return "", fmt.Errorf("OIDC_UPSTREAM_GOOGLE_CLIENT_ID is empty")
	}
	issuer := strings.TrimRight(strings.TrimSpace(conf.OIDCIssuer), "/")
	if issuer == "" {
		return "", fmt.Errorf("OIDC_ISSUER is empty")
	}
	appName, err := DeriveCasdoorAppName(conf.OIDCClientID, conf.PROJECT_NAME)
	if err != nil {
		return "", err
	}

	scope := shared.FirstNonEmpty(strings.TrimSpace(conf.OIDCScopes), "openid profile email")
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

func DeriveCasdoorAppName(clientID, projectSlug string) (string, error) {
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

func IsHTTPS(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https")
}
