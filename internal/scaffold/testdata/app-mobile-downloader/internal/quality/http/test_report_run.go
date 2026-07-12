package dev

import (
	"testboi1/internal/quality/application/test_report"
	authmiddleware "testboi1/internal/auth/middleware"
	"testboi1/internal/shared"
	"testboi1/internal/shared/server"

	"github.com/go-fuego/fuego"
)

func testReportRunHandler(s *server.Server, runner *testreport.Runner) {
	fuego.Post(s.Server, "/report/tests/run", func(c fuego.ContextNoBody) (string, error) {
		claims, _ := authmiddleware.JWTClaimsFromContext(c.Context())
		email := shared.FirstStringClaim(claims, "email")
		state, err := runner.Run(email)
		if err != nil {
			if err.Error() == "forbidden" {
				return "", fuego.HTTPError{Status: 403, Detail: "forbidden"}
			}
			return "", fuego.HTTPError{Status: 500, Detail: err.Error()}
		}
		return renderResultAndDashboard(c, state)
	}, fuego.OptionMiddleware(authmiddleware.RequireEditor()))
}

