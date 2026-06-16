package dev

import (
	"app-mobile-downloader/internal/dev/application/test_report"
	"app-mobile-downloader/internal/shared"
	"app-mobile-downloader/internal/shared/access"
	"app-mobile-downloader/internal/shared/server"
	"app-mobile-downloader/internal/shared/server/middleware"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(testReportRunHandler)

func testReportRunHandler(s *server.Server, runner *testreport.Runner) {
	fuego.Post(s.Server, "/report/tests/run", func(c fuego.ContextNoBody) (string, error) {
		claims, ok := middleware.JWTClaimsFromContext(c.Context())
		if !ok {
			return "", fuego.HTTPError{Status: 401, Detail: "unauthorized"}
		}
		email := shared.FirstStringClaim(claims, "email")
		if !access.IsAllowedEditorEmail(email) {
			return "", fuego.HTTPError{Status: 403, Detail: "forbidden"}
		}

		state, err := runner.Run(email)
		if err != nil {
			if err.Error() == "forbidden" {
				return "", fuego.HTTPError{Status: 403, Detail: "forbidden"}
			}
			return "", fuego.HTTPError{Status: 500, Detail: err.Error()}
		}
		return renderResultAndDashboard(c, state)
	})
}

