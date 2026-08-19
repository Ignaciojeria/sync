package dev

import (
	authmiddleware "gitinittest5/internal/auth/middleware"
	"gitinittest5/internal/quality/application/test_report"
	"gitinittest5/internal/shared"
	"gitinittest5/internal/shared/server"
)

func testReportRunHandler(s *server.Server, runner *testreport.Runner) {
	server.Post(s, "/report/tests/run", func(c server.ContextNoBody) (string, error) {
		claims, _ := authmiddleware.JWTClaimsFromContext(c.Context())
		email := shared.FirstStringClaim(claims, "email")
		state, err := runner.Run(email)
		if err != nil {
			if err.Error() == "forbidden" {
				return "", server.HTTPError{Status: 403, Detail: "forbidden"}
			}
			return "", server.HTTPError{Status: 500, Detail: err.Error()}
		}
		return renderResultAndDashboard(c, state)
	}, server.OptionMiddleware(authmiddleware.RequireEditor()))
}
