package dev

import (
	"gitinittest5/internal/quality/ui"
	"gitinittest5/internal/shared/server"
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
