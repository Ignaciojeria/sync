package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"app-mobile-downloader/internal/shared/configuration"

	"github.com/MicahParks/keyfunc/v3"
)

const PostLoginRedirectPath = "/"

type CallbackResponse struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
}

func ExchangeAuthorizationCode(conf configuration.Conf, code string) (CallbackResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", strings.TrimSpace(conf.OIDCRedirectURI))
	form.Set("client_id", strings.TrimSpace(conf.OIDCClientID))
	if secret := strings.TrimSpace(conf.OIDCClientSecret); secret != "" {
		form.Set("client_secret", secret)
	}

	resp, err := http.PostForm(strings.TrimSpace(conf.OIDCTokenEndpoint), form)
	if err != nil {
		return CallbackResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CallbackResponse{}, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var out CallbackResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return CallbackResponse{}, err
	}
	return out, nil
}

func ExtractIdentityFromTokens(conf configuration.Conf, jwks keyfunc.Keyfunc, resp CallbackResponse) (Identity, error) {
	return IdentityFromTokens(conf, jwks, resp)
}
