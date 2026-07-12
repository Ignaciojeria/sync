package editor

import (
	editorui "testboi1/internal/editor/ui"
	"testboi1/internal/shared/server"
	"testboi1/internal/ui/layout"

	"github.com/go-fuego/fuego"
)

func editorViewHandler(s *server.Server) {
	fuego.Get(s.Server, "/editor-view", func(c fuego.ContextNoBody) (any, error) {
		nav := layout.FromRequest(c.Request())
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Console", editorui.EditorView(nav.PreviewPrefix))
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	})
}
