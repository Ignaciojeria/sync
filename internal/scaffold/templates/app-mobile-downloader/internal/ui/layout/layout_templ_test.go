package layout

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestLayoutRendersTitleAndContent(t *testing.T) {
	var buf bytes.Buffer
	if err := Layout("My Title").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Layout().Render() error = %v", err)
	}
	body := buf.String()
	if !strings.Contains(body, "My Title") {
		t.Fatalf("expected title to render, body=%q", body)
	}
	if !strings.Contains(body, "data-theme") {
		t.Fatalf("expected layout theme attribute, body=%q", body)
	}
}

func TestLayoutWithNavRendersNavItems(t *testing.T) {
	var buf bytes.Buffer
	nav := NavigationContext{CurrentPath: "/report/tests", IsEditor: true}
	if err := LayoutWithNav("Editor Console", nav).Render(context.Background(), &buf); err != nil {
		t.Fatalf("LayoutWithNav().Render() error = %v", err)
	}
	body := buf.String()

	if !strings.Contains(body, "Editor Console") {
		t.Fatalf("expected title, body=%q", body)
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
}

func TestLayoutWithNavNonEditorOmitsDevelopment(t *testing.T) {
	var buf bytes.Buffer
	nav := NavigationContext{CurrentPath: "/", IsEditor: false}
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
	nav := NavigationContext{CurrentPath: "/report/tests", IsEditor: true}
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
			if err := SideNav(NavigationContext{CurrentPath: c.current, IsEditor: c.editor}).Render(context.Background(), &buf); err != nil {
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

func TestNavItemClass(t *testing.T) {
	if got := navItemClass("/", "/"); got != "active" {
		t.Fatalf("expected active for exact match, got %q", got)
	}
	if got := navItemClass("/", "/something"); got != "" {
		t.Fatalf("expected empty when current is unrelated, got %q", got)
	}
	if got := navItemClass("/report/tests", "/report/tests/run"); got != "active" {
		t.Fatalf("expected active for sub-path, got %q", got)
	}
}

// TestSideNavWithChildrenSet covers the branch that runs when the request
// context already has a non-nil children component. The templ-generated
// guard `if templ_…_Var1 == nil` is skipped in that case.
func TestSideNavWithChildrenSet(t *testing.T) {
	nav := NavigationContext{CurrentPath: "/", IsEditor: false}
	var buf bytes.Buffer
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := SideNav(nav).Render(ctx, &buf); err != nil {
		t.Fatalf("SideNav().Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Inicio") {
		t.Fatalf("expected SideNav to render menus, got %q", buf.String())
	}
}

// TestLayoutWithNavWithChildrenSet covers the children branch on the layout
// with-nav component.
func TestLayoutWithNavWithChildrenSet(t *testing.T) {
	nav := NavigationContext{CurrentPath: "/", IsEditor: false}
	var buf bytes.Buffer
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := LayoutWithNav("T", nav).Render(ctx, &buf); err != nil {
		t.Fatalf("LayoutWithNav().Render() error = %v", err)
	}
}

// TestLayoutWithChildrenSet exercises the same branch on the bare Layout.
func TestLayoutWithChildrenSet(t *testing.T) {
	var buf bytes.Buffer
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	if err := Layout("T").Render(ctx, &buf); err != nil {
		t.Fatalf("Layout().Render() error = %v", err)
	}
}
