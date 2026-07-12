package ui

import designapp "testboi1/internal/design/application"

func colorValue(theme designapp.ResolvedTheme, key string) string {
	if value := theme.DaisyUI[key]; value != "" {
		return value
	}
	return theme.Colors[key]
}

func roundedValue(theme designapp.ResolvedTheme, key string) string {
	return theme.Rounded[key]
}

func typographyValue(theme designapp.ResolvedTheme, key string) designapp.TypographyToken {
	return theme.Typography[key]
}
