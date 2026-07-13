package dev

import (
	"fixtests1/internal/quality/ui"
	"fixtests1/internal/shared/server"
)

func renderResultAndDashboard(c server.ContextNoBody, state ui.TestRunState) (string, error) {
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	w := c.Response()
	ctx := c.Context()
	if err := ui.RenderResultAndDashboard(w, ctx, state); err != nil {
		return "", err
	}
	return "", nil
}
