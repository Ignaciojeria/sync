package auth

import (
	"fmt"
	"strings"

	"testboi1/internal/shared"
	"testboi1/internal/shared/configuration"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type Identity struct {
	Subject     string
	Email       string
	DisplayName string
}

func IdentityFromTokens(conf configuration.Conf, jwks keyfunc.Keyfunc, resp CallbackResponse) (Identity, error) {
	issuer := strings.TrimSpace(conf.OIDCIssuer)
	audience := shared.FirstNonEmpty(strings.TrimSpace(conf.JWTAudience), strings.TrimSpace(conf.OIDCClientID))
	for _, tokenString := range []string{strings.TrimSpace(resp.IDToken), strings.TrimSpace(resp.AccessToken)} {
		if tokenString == "" {
			continue
		}
		claims := jwt.MapClaims{}
		opts := []jwt.ParserOption{}
		if issuer != "" {
			opts = append(opts, jwt.WithIssuer(issuer))
		}
		if audience != "" {
			opts = append(opts, jwt.WithAudience(audience))
		}
		token, err := jwt.ParseWithClaims(tokenString, claims, jwks.Keyfunc, opts...)
		if err != nil || !token.Valid {
			continue
		}
		identity := Identity{
			Subject:     strings.TrimSpace(shared.FirstStringClaim(claims, "sub")),
			Email:       strings.TrimSpace(shared.FirstStringClaim(claims, "email", "name")),
			DisplayName: strings.TrimSpace(shared.FirstStringClaim(claims, "display_name", "name", "email")),
		}
		if identity.Subject != "" {
			return identity, nil
		}
	}
	return Identity{}, fmt.Errorf("could not extract authenticated subject from oidc tokens")
}
