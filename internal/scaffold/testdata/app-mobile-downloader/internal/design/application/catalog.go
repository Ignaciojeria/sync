package design

import (
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"

	designdata "scaffoldxd1/design"
)

const ThemeCookieName = "design-theme"

type Catalog struct {
	themes         []ResolvedTheme
	themesByID     map[string]ResolvedTheme
	defaultThemeID string
}

var (
	defaultCatalogOnce sync.Once
	defaultCatalog     Catalog
	defaultCatalogErr  error
)

// LoadCatalog carga todos los DESIGN.md embebidos y resuelve los temas válidos.
func LoadCatalog() ([]ResolvedTheme, error) {
	paths, err := fs.Glob(designdata.FS, "*/DESIGN.md")
	if err != nil {
		return nil, fmt.Errorf("glob design catalog: %w", err)
	}

	themes := make([]ResolvedTheme, 0, len(paths))
	for _, filePath := range paths {
		content, err := fs.ReadFile(designdata.FS, filePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filePath, err)
		}
		doc, err := ParseDocument(string(content))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", filePath, err)
		}
		theme, err := ResolveTheme(doc, themeIDFromPath(filePath))
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", filePath, err)
		}
		themes = append(themes, theme)
	}

	sort.Slice(themes, func(i, j int) bool {
		return themes[i].ID < themes[j].ID
	})
	return themes, nil
}

// DefaultCatalog devuelve el catálogo embebido y lo cachea en memoria.
func DefaultCatalog() (Catalog, error) {
	defaultCatalogOnce.Do(func() {
		themes, err := LoadCatalog()
		if err != nil {
			defaultCatalogErr = err
			return
		}
		defaultCatalog = NewCatalog(themes)
	})
	return defaultCatalog, defaultCatalogErr
}

// NewCatalog indexa una lista de temas ya resueltos.
func NewCatalog(themes []ResolvedTheme) Catalog {
	indexed := make(map[string]ResolvedTheme, len(themes))
	for _, theme := range themes {
		indexed[theme.ID] = theme
	}
	defaultID := ""
	if len(themes) > 0 {
		defaultID = themes[0].ID
	}
	return Catalog{
		themes:         append([]ResolvedTheme(nil), themes...),
		themesByID:     indexed,
		defaultThemeID: defaultID,
	}
}

func (c Catalog) Themes() []ResolvedTheme {
	return append([]ResolvedTheme(nil), c.themes...)
}

func (c Catalog) DefaultThemeID() string {
	return c.defaultThemeID
}

func (c Catalog) ThemeByID(id string) (ResolvedTheme, bool) {
	theme, ok := c.themesByID[strings.TrimSpace(id)]
	return theme, ok
}

func (c Catalog) ResolveThemeID(id string) string {
	id = strings.TrimSpace(id)
	if id != "" {
		if _, ok := c.ThemeByID(id); ok {
			return id
		}
	}
	return c.defaultThemeID
}

func (c Catalog) ActiveThemeIDFromRequest(r *http.Request) string {
	if r == nil {
		return c.defaultThemeID
	}
	cookie, err := r.Cookie(ThemeCookieName)
	if err != nil {
		return c.defaultThemeID
	}
	return c.ResolveThemeID(cookie.Value)
}

func ActiveThemeIDFromRequest(r *http.Request) string {
	catalog, err := DefaultCatalog()
	if err != nil {
		return "light"
	}
	return catalog.ActiveThemeIDFromRequest(r)
}

func themeIDFromPath(filePath string) string {
	return strings.TrimSpace(path.Base(path.Dir(filePath)))
}
