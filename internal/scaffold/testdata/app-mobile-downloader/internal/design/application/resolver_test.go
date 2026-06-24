package design

import "testing"

func TestResolveTheme(t *testing.T) {
	doc := Document{
		Name:        "Ocean",
		Description: "Tema",
		Colors: map[string]string{
			"primary":    "#2563eb",
			"secondary":  "#7c3aed",
			"tertiary":   "#06b6d4",
			"neutral":    "#ffffff",
			"surface":    "#f3f4f6",
			"on-surface": "#111827",
		},
		Typography: map[string]TypographyToken{
			"body-md": {FontFamily: "Inter", FontSize: "16px"},
		},
		Rounded: map[string]string{
			"sm": "0.5rem",
			"md": "1rem",
		},
		Spacing: map[string]string{
			"sm": "0.5rem",
		},
		XPi: XPiExtension{
			DaisyUI: map[string]string{
				"accent":          "{colors.tertiary}",
				"base-content":    "{colors.on-surface}",
				"radius-box":      "{rounded.md}",
				"radius-field":    "{rounded.sm}",
				"radius-selector": "{rounded.sm}",
			},
		},
	}

	theme, err := ResolveTheme(doc, "ocean")
	if err != nil {
		t.Fatalf("ResolveTheme() error = %v", err)
	}
	if got, want := theme.ID, "ocean"; got != want {
		t.Fatalf("theme.ID = %q, want %q", got, want)
	}
	if got, want := theme.DaisyUI["accent"], "#06b6d4"; got != want {
		t.Fatalf("theme.DaisyUI[accent] = %q, want %q", got, want)
	}
	if got, want := theme.DaisyUI["base-content"], "#111827"; got != want {
		t.Fatalf("theme.DaisyUI[base-content] = %q, want %q", got, want)
	}
	if got, want := theme.DaisyUI["radius-box"], "1rem"; got != want {
		t.Fatalf("theme.DaisyUI[radius-box] = %q, want %q", got, want)
	}
	if got, want := theme.ColorScheme, "light"; got != want {
		t.Fatalf("theme.ColorScheme = %q, want %q", got, want)
	}
}

func TestResolveThemeRequiresMinimumRuntimeTokens(t *testing.T) {
	doc := Document{
		Name:   "Broken",
		Colors: map[string]string{},
		Typography: map[string]TypographyToken{
			"body-md": {FontFamily: "Inter", FontSize: "16px"},
		},
	}

	_, err := ResolveTheme(doc, "broken")
	if err == nil {
		t.Fatal("ResolveTheme() error = nil, want error")
	}
}
