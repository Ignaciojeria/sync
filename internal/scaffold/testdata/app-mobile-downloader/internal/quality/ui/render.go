package ui

import (
	"context"
	"io"
)

func RenderResultAndDashboard(w io.Writer, ctx context.Context, state TestRunState) error {
	if err := DashboardStats(state).Render(ctx, w); err != nil {
		return err
	}
	if err := TestResult(state.Success, state.Output, state.CoverPath, state.CoverPercent).Render(ctx, w); err != nil {
		return err
	}
	return nil
}
