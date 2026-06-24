package design

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// CompileThemeCSS compila un ResolvedTheme a CSS runtime estable y cacheable.
func CompileThemeCSS(theme ResolvedTheme) ThemeCSS {
	vars := make(map[string]string)

	for key, value := range theme.DaisyUI {
		if strings.TrimSpace(value) == "" {
			continue
		}
		vars["--"+cssVarNameForDaisy(key)] = value
	}
	for key, value := range theme.Colors {
		if strings.TrimSpace(value) == "" {
			continue
		}
		vars["--pi-color-"+sanitizeTokenName(key)] = value
	}
	for key, value := range theme.Rounded {
		if strings.TrimSpace(value) == "" {
			continue
		}
		vars["--pi-rounded-"+sanitizeTokenName(key)] = value
	}
	for key, value := range theme.Spacing {
		if strings.TrimSpace(value) == "" {
			continue
		}
		vars["--pi-spacing-"+sanitizeTokenName(key)] = value
	}
	for key, value := range theme.Typography {
		prefix := "--pi-typography-" + sanitizeTokenName(key) + "-"
		appendIfSet(vars, prefix+"font-family", value.FontFamily)
		appendIfSet(vars, prefix+"font-size", value.FontSize)
		appendIfSet(vars, prefix+"font-weight", value.FontWeight)
		appendIfSet(vars, prefix+"line-height", value.LineHeight)
		appendIfSet(vars, prefix+"letter-spacing", value.LetterSpacing)
		appendIfSet(vars, prefix+"font-feature", value.FontFeature)
		appendIfSet(vars, prefix+"font-variation", value.FontVariation)
	}

	body := theme.Typography["body-md"]
	label := theme.Typography["label-md"]
	code := theme.Typography["code-md"]
	appendIfSet(vars, "--pi-font-body-family", body.FontFamily)
	appendIfSet(vars, "--pi-font-body-size", body.FontSize)
	appendIfSet(vars, "--pi-font-body-weight", body.FontWeight)
	appendIfSet(vars, "--pi-font-body-line-height", body.LineHeight)
	appendIfSet(vars, "--pi-font-label-family", firstNonEmpty(label.FontFamily, body.FontFamily))
	appendIfSet(vars, "--pi-font-label-size", firstNonEmpty(label.FontSize, body.FontSize))
	appendIfSet(vars, "--pi-font-label-weight", firstNonEmpty(label.FontWeight, body.FontWeight, "600"))
	appendIfSet(vars, "--pi-font-code-family", firstNonEmpty(code.FontFamily, body.FontFamily))
	appendIfSet(vars, "--pi-font-code-size", firstNonEmpty(code.FontSize, body.FontSize))
	appendIfSet(vars, "--pi-shadow-card", firstNonEmpty(theme.DaisyUI["shadow-card"], "0 18px 48px -28px color-mix(in srgb, var(--color-base-content) 22%, transparent)"))
	appendIfSet(vars, "--pi-shadow-card-soft", firstNonEmpty(theme.DaisyUI["shadow-card-soft"], "0 10px 28px -18px color-mix(in srgb, var(--color-base-content) 18%, transparent)"))
	appendIfSet(vars, "--pi-shadow-sidebar", firstNonEmpty(theme.DaisyUI["shadow-sidebar"], "0 20px 45px -30px color-mix(in srgb, var(--color-base-content) 28%, transparent)"))
	appendIfSet(vars, "--pi-border-subtle", firstNonEmpty(theme.DaisyUI["border-subtle"], "color-mix(in srgb, var(--color-base-content) 10%, transparent)"))
	appendIfSet(vars, "--pi-surface-muted", firstNonEmpty(theme.DaisyUI["surface-muted"], "color-mix(in srgb, var(--color-base-200) 78%, white)"))
	appendIfSet(vars, "--pi-surface-elevated", firstNonEmpty(theme.DaisyUI["surface-elevated"], "color-mix(in srgb, var(--color-base-100) 92%, white)"))

	lines := []string{fmt.Sprintf("[data-theme=\"%s\"] {", theme.ID)}
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, fmt.Sprintf("  %s: %s;", key, vars[key]))
	}
	lines = append(lines, "}")

	content := strings.Join(lines, "\n")
	hash := sha256.Sum256([]byte(content))

	return ThemeCSS{
		ThemeID: theme.ID,
		Content: content,
		ETag:    hex.EncodeToString(hash[:]),
	}
}

func cssVarNameForDaisy(token string) string {
	token = sanitizeTokenName(token)
	switch {
	case strings.HasPrefix(token, "radius-"):
		return token
	case strings.HasPrefix(token, "shadow-"):
		return "pi-" + token
	case strings.HasPrefix(token, "border-"):
		return "pi-" + token
	case strings.HasPrefix(token, "surface-"):
		return "pi-" + token
	default:
		return "color-" + token
	}
}

func sanitizeTokenName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer("_", "-", " ", "-", ".", "-")
	return replacer.Replace(value)
}

func appendIfSet(dst map[string]string, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	dst[key] = strings.TrimSpace(value)
}
