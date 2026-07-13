package editor

import (
	editorui "fixtests1/internal/editor/ui"
	"fixtests1/internal/shared/server"
	"fixtests1/internal/ui/layout"
)

func editorViewHandler(s *server.Server) {
	server.Get(s, "/editor-view", func(c server.ContextNoBody) (any, error) {
		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Console", editorui.EditorView(nav.PreviewPrefix))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	})
}
