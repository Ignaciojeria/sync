package design

import (
	"strings"
	"testing"
)

func TestCompileThemeCSS(t *testing.T) {
	theme := ResolvedTheme{
		ID: "ocean",
		DaisyUI: map[string]string{
			"primary":         "#2563eb",
			"primary-content": "#ffffff",
			"base-100":        "#ffffff",
			"base-content":    "#111827",
			"radius-box":      "1rem",
		},
		Colors: map[string]string{
			"primary": "#2563eb",
		},
		Rounded: map[string]string{
			"md": "1rem",
		},
		Spacing: map[string]string{
			"sm": "0.5rem",
		},
		Typography: map[string]TypographyToken{
			"body-md": {FontFamily: "Inter", FontSize: "16px"},
		},
	}

	css := CompileThemeCSS(theme)
	checks := []string{
		"[data-theme=\"ocean\"]",
		"--color-primary: #2563eb;",
		"--color-primary-content: #ffffff;",
		"--radius-box: 1rem;",
		"--pi-typography-body-md-font-family: Inter;",
		"--pi-font-body-family: Inter;",
		"--pi-shadow-card:",
	}
	for _, check := range checks {
		if !strings.Contains(css.Content, check) {
			t.Fatalf("css.Content missing %q\n%s", check, css.Content)
		}
	}
	if css.ETag == "" {
		t.Fatal("css.ETag is empty")
	}
}
