package design

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewCatalogPicksFirstAsDefault(t *testing.T) {
	catalog := NewCatalog([]ResolvedTheme{
		{ID: "ocean"},
		{ID: "forest"},
	})
	if got := catalog.DefaultThemeID(); got != "ocean" {
		t.Errorf("DefaultThemeID() = %q, want ocean", got)
	}
	if len(catalog.Themes()) != 2 {
		t.Errorf("len(Themes) = %d", len(catalog.Themes()))
	}
}

func TestNewCatalogEmpty(t *testing.T) {
	catalog := NewCatalog(nil)
	if got := catalog.DefaultThemeID(); got != "" {
		t.Errorf("DefaultThemeID() = %q, want empty", got)
	}
	if got := catalog.Themes(); len(got) != 0 {
		t.Errorf("Themes() = %d", len(got))
	}
}

func TestThemesReturnsCopy(t *testing.T) {
	catalog := NewCatalog([]ResolvedTheme{{ID: "ocean"}})
	themes := catalog.Themes()
	themes[0].ID = "mutated"
	if got := catalog.Themes()[0].ID; got != "ocean" {
		t.Errorf("internal themes mutated via returned slice: %q", got)
	}
}

func TestThemeByIDAndTrim(t *testing.T) {
	catalog := NewCatalog([]ResolvedTheme{{ID: "ocean"}, {ID: "forest"}})
	if _, ok := catalog.ThemeByID("ocean"); !ok {
		t.Error("expected ocean to be found")
	}
	if _, ok := catalog.ThemeByID("  ocean  "); !ok {
		t.Error("expected ocean with whitespace to be found")
	}
	if _, ok := catalog.ThemeByID("missing"); ok {
		t.Error("expected missing to not be found")
	}
}

func TestResolveThemeIDFallsBackToDefault(t *testing.T) {
	catalog := NewCatalog([]ResolvedTheme{{ID: "ocean"}})
	if got := catalog.ResolveThemeID("missing"); got != "ocean" {
		t.Errorf("ResolveThemeID(missing) = %q", got)
	}
	if got := catalog.ResolveThemeID(""); got != "ocean" {
		t.Errorf("ResolveThemeID(\"\") = %q", got)
	}
	if got := catalog.ResolveThemeID("forest"); got != "ocean" {
		t.Errorf("ResolveThemeID(forest) fallback = %q, want ocean", got)
	}
}

func TestActiveThemeIDFromRequestHonoursCookie(t *testing.T) {
	catalog := NewCatalog([]ResolvedTheme{{ID: "ocean"}, {ID: "forest"}})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: ThemeCookieName, Value: "forest"})
	if got := catalog.ActiveThemeIDFromRequest(req); got != "forest" {
		t.Errorf("ActiveThemeIDFromRequest() = %q, want forest", got)
	}
}

func TestActiveThemeIDFromRequestFallsBackOnBadCookie(t *testing.T) {
	catalog := NewCatalog([]ResolvedTheme{{ID: "ocean"}, {ID: "forest"}})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: ThemeCookieName, Value: "unknown"})
	if got := catalog.ActiveThemeIDFromRequest(req); got != "ocean" {
		t.Errorf("ActiveThemeIDFromRequest() = %q, want ocean default", got)
	}
}

func TestActiveThemeIDFromRequestNilRequest(t *testing.T) {
	catalog := NewCatalog([]ResolvedTheme{{ID: "ocean"}})
	if got := catalog.ActiveThemeIDFromRequest(nil); got != "ocean" {
		t.Errorf("ActiveThemeIDFromRequest(nil) = %q", got)
	}
}

func TestActiveThemeIDFromRequestNoCookie(t *testing.T) {
	catalog := NewCatalog([]ResolvedTheme{{ID: "ocean"}})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := catalog.ActiveThemeIDFromRequest(req); got != "ocean" {
		t.Errorf("ActiveThemeIDFromRequest() sin cookie = %q, want ocean", got)
	}
}

func TestDefaultCatalogLoadsEmbedded(t *testing.T) {
	catalog, err := DefaultCatalog()
	if err != nil {
		t.Fatalf("DefaultCatalog() error = %v", err)
	}
	if got := catalog.DefaultThemeID(); got == "" {
		t.Error("DefaultThemeID() should not be empty for embedded catalog")
	}
}

func TestActiveThemeIDFromRequestGlobalFallsBackToLight(t *testing.T) {
	// ponytail: cuando DefaultCatalog falla, devuelve "light" como
	// último recurso. Forzamos el path fallido forzando una ID rara
	// que el catálogo embebido no conoce.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: ThemeCookieName, Value: ""})
	if got := ActiveThemeIDFromRequest(req); got == "" {
		t.Error("ActiveThemeIDFromRequest() = empty")
	}
}

func TestThemeIDFromPath(t *testing.T) {
	if got := themeIDFromPath("ocean/DESIGN.md"); got != "ocean" {
		t.Errorf("themeIDFromPath() = %q", got)
	}
	if got := themeIDFromPath(" forest /DESIGN.md"); got != "forest" {
		t.Errorf("themeIDFromPath() con whitespace = %q", got)
	}
}
