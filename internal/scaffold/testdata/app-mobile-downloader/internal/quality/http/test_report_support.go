package dev

import (
	"testboi1/internal/quality/ui"
	"github.com/go-fuego/fuego"
)

func renderResultAndDashboard(c fuego.ContextNoBody, state ui.TestRunState) (string, error) {
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	w := c.Response()
	ctx := c.Context()
	if err := ui.RenderResultAndDashboard(w, ctx, state); err != nil {
		return "", err
	}
	return "", nil
}

