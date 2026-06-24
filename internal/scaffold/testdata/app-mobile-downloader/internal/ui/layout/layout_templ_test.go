package layout

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func themedNav(currentPath string, isEditor bool) NavigationContext {
	return NavigationContext{
		CurrentPath:   currentPath,
		IsEditor:      isEditor,
		ActiveThemeID: "ocean",
		ThemeCSSHref:  "/design/theme/ocean",
	}
}

func TestLayoutRendersTitleAndContent(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("My Title", "ocean", "/design/theme/ocean").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Layout().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "My Title") {
		t.Fatalf("expected title to render, body=%q", body)
	}
	if !strings.Contains(body, "data-theme=\"ocean\"") {
		t.Fatalf("expected layout theme attribute, body=%q", body)
	}
	if !strings.Contains(body, "/design/theme/ocean") {
		t.Fatalf("expected theme stylesheet link, body=%q", body)
	}
}

func TestLayoutWithNavRendersNavItems(t *testing.T) {
	var buf bytes.Buffer
	nav := themedNav("/report/tests", true)
	if err := LayoutWithNav("Editor Console", nav).Render(context.Background(), &buf); err != nil {
		t.Fatalf("LayoutWithNav().Render() error = %v", err)
	}
	body := buf.String()

	if !strings.Contains(body, "Editor Console") {
		t.Fatalf("expected title, body=%q", body)
	}
	if !strings.Contains(body, "data-theme=\"ocean\"") {
		t.Fatalf("expected active theme, body=%q", body)
	}
	if !strings.Contains(body, "/design/theme/ocean") {
		t.Fatalf("expected theme stylesheet, body=%q", body)
	}
	if !strings.Contains(body, "/editor-view") {
		t.Fatalf("expected editor link, body=%q", body)
	}
	if !strings.Contains(body, "/report/tests") {
		t.Fatalf("expected report link, body=%q", body)
	}
	if !strings.Contains(body, "/scheduler/jobs") {
		t.Fatalf("expected scheduler link, body=%q", body)
	}
	if !strings.Contains(body, "/auth/logout") {
		t.Fatalf("expected logout link, body=%q", body)
	}
	if !strings.Contains(body, "Cerrar sesion") {
		t.Fatalf("expected logout label, body=%q", body)
	}
	if !strings.Contains(body, "sync") || !strings.Contains(body, ">run</span>") {
		t.Fatalf("expected branded sidebar logo, body=%q", body)
	}
	if !strings.Contains(body, ">S4</div>") {
		t.Fatalf("expected collapsed sidebar badge, body=%q", body)
	}
}

func TestLayoutWithNavNonEditorOmitsDevelopment(t *testing.T) {
	var buf bytes.Buffer
	nav := themedNav("/", false)
	if err := LayoutWithNav("Home", nav).Render(context.Background(), &buf); err != nil {
		t.Fatalf("LayoutWithNav().Render() error = %v", err)
	}
	body := buf.String()

	if strings.Contains(body, "/editor-view") || strings.Contains(body, "Console") {
		t.Fatalf("non-editor layout should not include editor links, body=%q", body)
	}
	if !strings.Contains(body, "Inicio") {
		t.Fatalf("non-editor layout should include home link, body=%q", body)
	}
}

func TestSideNavRendersActiveState(t *testing.T) {
	nav := themedNav("/report/tests", true)
	var buf bytes.Buffer
	if err := SideNav(nav).Render(context.Background(), &buf); err != nil {
		t.Fatalf("SideNav().Render() error = %v", err)
	}
	body := buf.String()

	if !strings.Contains(body, "href=\"/\"") {
		t.Fatalf("expected home link, body=%q", body)
	}
	if !strings.Contains(body, "Inicio") {
		t.Fatalf("expected home label, body=%q", body)
	}
	if !strings.Contains(body, "active") {
		t.Fatalf("expected active state, body=%q", body)
	}
}

func TestSideNavEditorSubMenu(t *testing.T) {
	cases := []struct {
		name    string
		editor  bool
		current string
		want    string
	}{
		{"editor on home", true, "/", "Console"},
		{"editor on sub-path", true, "/editor-view", "Console"},
		{"editor on report", true, "/report/tests", "Quality"},
		{"editor on scheduler", true, "/scheduler/jobs", "Scheduler"},
		{"non-editor no sub", false, "/", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := SideNav(themedNav(c.current, c.editor)).Render(context.Background(), &buf); err != nil {
				t.Fatalf("SideNav render: %v", err)
			}
			body := buf.String()
			if c.want == "" {
				if strings.Contains(body, "Console") {
					t.Fatalf("expected no Console item, body=%q", body)
				}
				return
			}
			if !strings.Contains(body, c.want) {
				t.Fatalf("expected %q item, body=%q", c.want, body)
			}
		})
	}
}

func TestNavItemClasses(t *testing.T) {
	if got := navItemClass("/", "/"); got != "active" {
		t.Fatalf("expected active for exact match, got %q", got)
	}
	if got := navItemClass("/", "/something"); got != "" {
		t.Fatalf("expected empty when current is unrelated, got %q", got)
	}
	if got := navItemClass("/report/tests", "/report/tests/run"); got != "active" {
		t.Fatalf("expected active for sub-path, got %q", got)
	}
	if got := navItemBaseClass(); !strings.Contains(got, "rounded-box") {
		t.Fatalf("expected navItemBaseClass to contain rounded-box, got %q", got)
	}
	if got := navItemToneClass("/", "/"); !strings.Contains(got, "bg-base-100/92") {
		t.Fatalf("expected active tone class, got %q", got)
	}
	if got := navItemToneClass("/", "/other"); !strings.Contains(got, "text-base-content/68") {
		t.Fatalf("expected inactive tone class, got %q", got)
	}
}

func TestSideNavWithChildrenSet(t *testing.T) {
	nav := themedNav("/", false)
	var buf bytes.Buffer
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := SideNav(nav).Render(ctx, &buf); err != nil {
		t.Fatalf("SideNav().Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Inicio") {
		t.Fatalf("expected SideNav to render menus, got %q", buf.String())
	}
}

func TestLayoutWithNavWithChildrenSet(t *testing.T) {
	nav := themedNav("/", false)
	var buf bytes.Buffer
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := LayoutWithNav("T", nav).Render(ctx, &buf); err != nil {
		t.Fatalf("LayoutWithNav().Render() error = %v", err)
	}
}

func TestLayoutWithChildrenSet(t *testing.T) {
	var buf bytes.Buffer
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := Layout("T", "ocean", "/design/theme/ocean").Render(ctx, &buf); err != nil {
		t.Fatalf("Layout().Render() error = %v", err)
	}
}
