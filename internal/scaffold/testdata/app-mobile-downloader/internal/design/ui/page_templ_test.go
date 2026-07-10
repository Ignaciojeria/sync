package ui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	designapp "scaffoldxd1/internal/design/application"

	"github.com/a-h/templ"
)

type failingWriter struct{ err error }

func (f failingWriter) Write(p []byte) (int, error) { return 0, f.err }

func sampleTheme() designapp.ResolvedTheme {
	return designapp.ResolvedTheme{
		ID:          "ocean",
		Name:        "Ocean",
		Description: "Cool and premium",
		ColorScheme: "light",
		Colors: map[string]string{
			"primary":      "#1e40af",
			"secondary":    "#0f766e",
			"accent":       "#7c3aed",
			"base-100":     "#ffffff",
			"base-200":     "#f3f4f6",
			"base-content": "#111827",
		},
		Typography: map[string]designapp.TypographyToken{
			"body-md":  {FontFamily: "Inter", FontSize: "16px", LineHeight: "24px", FontWeight: "400"},
			"label-md": {FontFamily: "Public Sans", FontSize: "14px", FontWeight: "600"},
			"code-md":  {FontFamily: "JetBrains Mono", FontSize: "13px"},
		},
		Rounded: map[string]string{"sm": "8px", "md": "16px"},
		Spacing: map[string]string{"xs": "0.5rem", "sm": "0.75rem", "md": "1rem"},
		DaisyUI: map[string]string{
			"primary":          "#1e40af",
			"secondary":        "#0f766e",
			"accent":           "#7c3aed",
			"base-100":         "#ffffff",
			"base-200":         "#f3f4f6",
			"base-content":     "#111827",
			"shadow-card":      "0 10px 20px rgba(0,0,0,.1)",
			"surface-elevated": "#f8fafc",
		},
	}
}

func TestPageRendersThemeShowcase(t *testing.T) {
	themes := []designapp.ResolvedTheme{sampleTheme(), {
		ID: "forest", Name: "Forest",
	}}
	active := sampleTheme()

	var buf bytes.Buffer
	if err := Page(themes, active, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Page().Render() error = %v", err)
	}
	body := buf.String()

	checks := []string{
		"Design system",
		"Ocean",
		"Cool and premium",
		"Select theme",
		"/design/select",
		"value=\"ocean\" selected",
		"Premium component preview",
		"Theme applied",
		"Typography",
		"Token summary",
		"Spacing rhythm",
		"width: clamp(2rem, calc(1rem * 6), 14rem);",
		"SELECT * FROM themes WHERE active = true;",
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Fatalf("expected rendered page to contain %q, got %q", want, body)
		}
	}
}

func TestPageOmitsDescriptionWhenEmpty(t *testing.T) {
	active := sampleTheme()
	active.Description = ""

	var buf bytes.Buffer
	if err := Page([]designapp.ResolvedTheme{active}, active, "").Render(context.Background(), &buf); err != nil {
		t.Fatalf("Page().Render() error = %v", err)
	}
	body := buf.String()
	if strings.Contains(body, "Cool and premium") {
		t.Fatalf("expected empty description to be omitted, got %q", body)
	}
}

func TestPagePrimitivesRenderAndErrorBranches(t *testing.T) {
	components := []struct {
		name string
		comp templ.Component
		want string
	}{
		{name: "ColorSwatch", comp: ColorSwatch("Primary", "#fff"), want: "Primary"},
		{name: "TokenStat", comp: TokenStat("rounded.sm", "8px"), want: "rounded.sm"},
		{name: "SpacingRow", comp: SpacingRow("sm", "1rem"), want: "width: clamp(2rem, calc(1rem * 6), 14rem);"},
	}
	for _, tc := range components {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := tc.comp.Render(context.Background(), &buf); err != nil {
				t.Fatalf("Render() error = %v", err)
			}
			if !strings.Contains(buf.String(), tc.want) {
				t.Fatalf("expected rendered output to contain %q, got %q", tc.want, buf.String())
			}
		})
	}

	w := failingWriter{err: errors.New("flush")}
	if err := Page([]designapp.ResolvedTheme{sampleTheme()}, sampleTheme(), "").Render(context.Background(), w); err == nil {
		t.Fatal("expected Page render error")
	}
	if err := ColorSwatch("Primary", "#fff").Render(context.Background(), w); err == nil {
		t.Fatal("expected ColorSwatch render error")
	}
	if err := TokenStat("rounded.sm", "8px").Render(context.Background(), w); err == nil {
		t.Fatal("expected TokenStat render error")
	}
	if err := SpacingRow("sm", "1rem").Render(context.Background(), w); err == nil {
		t.Fatal("expected SpacingRow render error")
	}
}

func TestPageCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Page([]designapp.ResolvedTheme{sampleTheme()}, sampleTheme(), "").Render(ctx, &bytes.Buffer{}); err == nil {
		t.Fatal("expected Page cancelled context error")
	}
}

func TestPageHelpers(t *testing.T) {
	theme := sampleTheme()
	if got := colorValue(theme, "primary"); got != "#1e40af" {
		t.Fatalf("colorValue() = %q", got)
	}
	delete(theme.DaisyUI, "accent")
	if got := colorValue(theme, "accent"); got != "#7c3aed" {
		t.Fatalf("colorValue fallback = %q", got)
	}
	if got := roundedValue(theme, "md"); got != "16px" {
		t.Fatalf("roundedValue() = %q", got)
	}
	if got := typographyValue(theme, "label-md").FontFamily; got != "Public Sans" {
		t.Fatalf("typographyValue() = %q", got)
	}
}

func TestPageRendersWithChildrenInContext(t *testing.T) {
	ctx := templ.WithChildren(context.Background(), templ.Raw("<child/>"))
	var buf bytes.Buffer
	if err := Page([]designapp.ResolvedTheme{sampleTheme()}, sampleTheme(), "").Render(ctx, &buf); err != nil {
		t.Fatalf("Page().Render() error = %v", err)
	}
	if !strings.Contains(buf.String(), "Design system") {
		t.Fatalf("expected rendered page, got %q", buf.String())
	}
}
