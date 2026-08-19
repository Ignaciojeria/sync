package editor

import (
	editorui "gitinittest5/internal/editor/ui"
	"gitinittest5/internal/shared/server"
	"gitinittest5/internal/ui/layout"
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
