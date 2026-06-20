package editor

import (
	editorui "app-mobile-downloader/internal/editor/ui"
	"app-mobile-downloader/internal/shared/server"
	"app-mobile-downloader/internal/ui/layout"

	"github.com/Ignaciojeria/ioc"
	"github.com/go-fuego/fuego"
)

var _ = ioc.Register(editorViewHandler)

func editorViewHandler(s *server.Server) {
	fuego.Get(s.Server, "/editor-view", func(c fuego.ContextNoBody) (any, error) {
		c.SetHeader("Content-Type", "text/html; charset=utf-8")
		page, err := layout.RenderPage(c, "Console", editorui.EditorView())
		if err != nil {
			return nil, err
		}
		return nil, page.Render(c.Context(), c.Response())
	})
}
