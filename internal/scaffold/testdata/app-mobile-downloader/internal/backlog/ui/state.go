package ui

import (
	"strings"
	"sync"
	"time"

	"gitinittest5/internal/backlog/application"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
)

// CardView es la proyección de una application.Card para renderizar.
// Mantener la separación ui ↔ application permite cambiar el modelo
// sin tocar los templates (y viceversa).
type CardView struct {
	Slug         string
	Title        string
	Description  string
	Status       application.Status
	Priority     application.Priority
	Timestamp    string
	Source       string
	AgentSession string
	Tags         []string
	Body         string
}

// ToCardView convierte application.Card → CardView para templates.
func ToCardView(c application.Card) CardView {
	return CardView{
		Slug:         c.Slug,
		Title:        c.Title,
		Description:  c.Description,
		Status:       c.Status,
		Priority:     c.Priority,
		Timestamp:    c.Timestamp,
		Source:       c.Source,
		AgentSession: c.AgentSession,
		Tags:         c.Tags,
		Body:         c.Body,
	}
}

// hasExpandable devuelve true si la tarjeta tiene contenido que vale
// la pena expandir (body, tags o agent_session). Cuando devuelve
// false, el Card no se renderiza como <details> para no insinuar
// un toggle inerte.
func hasExpandable(card CardView) bool {
	return card.Body != "" || len(card.Tags) > 0 || card.AgentSession != ""
}

// Markdown del body de la tarjeta. Renderiza el Markdown del cuerpo
// con goldmark y lo sanea con bluemonday contra un allowlist
// estricto, pensado para incrustar el HTML resultante en la página
// con templ.Raw. La política es la misma que usa el módulo agent
// para previews (allowlist conservadora: nada de <script>, <img>,
// inline styles, etc.).
var (
	bodyMarkdown = goldmark.New(goldmark.WithExtensions())
	bodyPolicy   = bluemonday.NewPolicy()
	bodyOnce     sync.Once
)

func init() {
	bodyOnce.Do(func() {
		bodyPolicy.AllowElements(
			"p", "br", "hr",
			"strong", "em", "del", "s", "u",
			"code", "pre",
			"a", "ul", "ol", "li",
			"h1", "h2", "h3", "h4", "h5", "h6",
			"blockquote",
			"table", "thead", "tbody", "tr", "th", "td",
		)
		bodyPolicy.AllowAttrs("href").OnElements("a")
		bodyPolicy.AllowURLSchemes("http", "https", "mailto")
		bodyPolicy.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("code", "pre")
	})
}

// RenderMarkdown devuelve el body de la tarjeta como HTML seguro,
// listo para inyectar con templ.Raw. Cadena vacía si body es vacío.
func RenderMarkdown(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	var buf strings.Builder
	if err := bodyMarkdown.Convert([]byte(body), &buf); err != nil {
		// Fallback: escapar el body como texto plano para no perder
		// la tarjeta entera si goldmark fallara.
		return bluemonday.NewPolicy().Sanitize(body)
	}
	return bodyPolicy.Sanitize(buf.String())
}

// ColumnView es una columna del tablero lista para renderizar.
type ColumnView struct {
	Status application.Status
	Title  string
	Cards  []CardView
}

// BoardView es el tablero completo listo para renderizar.
type BoardView struct {
	Columns []ColumnView
	Count   int
}

// ToBoardView convierte application.Board → BoardView para templates.
func ToBoardView(b application.Board) BoardView {
	cols := make([]ColumnView, 0, len(b.Columns))
	for _, col := range b.Columns {
		cards := make([]CardView, 0, len(col.Cards))
		for _, c := range col.Cards {
			cards = append(cards, ToCardView(c))
		}
		cols = append(cols, ColumnView{
			Status: col.Status,
			Title:  col.Title,
			Cards:  cards,
		})
	}
	return BoardView{Columns: cols, Count: b.Count}
}

// ColumnID genera un id HTML estable para el contenedor de una
// columna. Es el target de los OOB-swap al mover tarjetas.
func ColumnID(status application.Status) string {
	return "backlog-column-" + string(status)
}

// ColumnListID genera un id HTML estable para el <ul> interno de una
// columna. Es el target de OOB-swap "beforeend" al insertar tarjetas
// vía HTMX (crear, mover).
func ColumnListID(status application.Status) string {
	return "backlog-column-list-" + string(status)
}

// CardID genera un id HTML estable para el <li> de una tarjeta a
// partir del slug. Es el target de los OOB-swap al
// actualizar/borrar.
func CardID(slug string) string {
	return "backlog-card-" + slug
}

// TruncateDescription corta un texto a max caracteres para que no
// rompa el layout. Devuelve el original si ya era corto.
func TruncateDescription(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "…"
}

// FormatTimestamp devuelve una fecha corta. Vacío si s es vacío.
func FormatTimestamp(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Format("02/01 15:04")
}

// appPath compone una ruta de la app respetando el prefijo de mount
// (preview, etc.). Replica el helper que usan los otros módulos.
func appPath(prefix, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return path
	}
	prefix = strings.TrimRight(prefix, "/")
	if path == "/" {
		return prefix + "/"
	}
	return prefix + path
}
