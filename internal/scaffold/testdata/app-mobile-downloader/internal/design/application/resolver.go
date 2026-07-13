package design

import (
	"fmt"
	"strings"
)

// ResolveTheme normaliza un Document y lo prepara para compilación runtime.
func ResolveTheme(doc Document, folderID string) (ResolvedTheme, error) {
	folderID = strings.TrimSpace(folderID)
	id := strings.TrimSpace(doc.XPi.ThemeID)
	if id == "" {
		id = folderID
	}
	if id == "" {
		return ResolvedTheme{}, fmt.Errorf("missing theme id")
	}
	if strings.TrimSpace(doc.Name) == "" {
		return ResolvedTheme{}, fmt.Errorf("missing theme name")
	}

	colors := cloneMap(doc.Colors)
	rounded := cloneMap(doc.Rounded)
	spacing := cloneMap(doc.Spacing)
	components := cloneNestedMap(doc.Components)
	typography := cloneTypography(doc.Typography)
	xdaisy := cloneMap(doc.XPi.DaisyUI)

	ctx := buildReferenceContext(colors, rounded, spacing, typography)
	resolvedColors := resolveMap(colors, ctx)
	ctx = buildReferenceContext(resolvedColors, rounded, spacing, typography)
	resolvedRounded := resolveMap(rounded, ctx)
	resolvedSpacing := resolveMap(spacing, ctx)
	resolvedComponents := resolveNestedMap(components, ctx)
	resolvedDaisy := resolveMap(xdaisy, ctx)
	resolvedTypography := resolveTypography(typography, ctx)

	daisy := buildDaisyUIAliases(resolvedColors, resolvedRounded, resolvedDaisy)
	warnings := collectWarnings(doc, daisy)

	return ResolvedTheme{
		ID:          id,
		Name:        strings.TrimSpace(doc.Name),
		Description: strings.TrimSpace(doc.Description),
		ColorScheme: defaultString(strings.TrimSpace(doc.XPi.ColorScheme), "light"),
		Colors:      resolvedColors,
		Typography:  resolvedTypography,
		Rounded:     resolvedRounded,
		Spacing:     resolvedSpacing,
		Components:  resolvedComponents,
		DaisyUI:     daisy,
		Warnings:    warnings,
	}, validateResolvedTheme(daisy, resolvedTypography)
}

func validateResolvedTheme(daisy map[string]string, typography map[string]TypographyToken) error {
	required := []string{"primary", "base-100", "base-content", "radius-box"}
	for _, key := range required {
		if strings.TrimSpace(daisy[key]) == "" {
			return fmt.Errorf("missing required runtime token %q", key)
		}
	}
	if len(typography) == 0 {
		return fmt.Errorf("missing typography tokens")
	}
	return nil
}

func collectWarnings(doc Document, daisy map[string]string) []string {
	warnings := make([]string, 0, 4)
	if strings.TrimSpace(doc.XPi.ThemeID) == "" {
		warnings = append(warnings, "x-pi.themeId missing; derived from folder")
	}
	if strings.TrimSpace(doc.XPi.ColorScheme) == "" {
		warnings = append(warnings, "x-pi.colorScheme missing; defaulting to light")
	}
	if strings.TrimSpace(daisy["primary-content"]) == "" {
		warnings = append(warnings, "primary-content missing; derived automatically")
	}
	return warnings
}

func buildDaisyUIAliases(colors, rounded, overrides map[string]string) map[string]string {
	aliases := cloneMap(overrides)
	if aliases == nil {
		aliases = map[string]string{}
	}
	aliases["primary"] = firstNonEmpty(overrides["primary"], colors["primary"])
	aliases["primary-content"] = firstNonEmpty(overrides["primary-content"], contrastText(firstNonEmpty(colors["primary"], "#000000")))
	aliases["secondary"] = firstNonEmpty(overrides["secondary"], colors["secondary"])
	aliases["accent"] = firstNonEmpty(overrides["accent"], colors["tertiary"])
	aliases["neutral"] = firstNonEmpty(overrides["neutral"], colors["surface"], colors["primary"], colors["neutral"])
	aliases["neutral-content"] = firstNonEmpty(overrides["neutral-content"], contrastText(firstNonEmpty(overrides["neutral"], colors["surface"], colors["primary"], colors["neutral"], "#111827")))
	aliases["base-100"] = firstNonEmpty(overrides["base-100"], colors["neutral"], colors["surface"], "#ffffff")
	aliases["base-200"] = firstNonEmpty(overrides["base-200"], colors["surface"], overrides["base-100"], colors["neutral"], "#f3f4f6")
	aliases["base-300"] = firstNonEmpty(overrides["base-300"], overrides["base-200"], colors["surface"], "#e5e7eb")
	aliases["base-content"] = firstNonEmpty(overrides["base-content"], colors["on-surface"], contrastText(firstNonEmpty(overrides["base-100"], colors["neutral"], colors["surface"], "#ffffff")))
	aliases["info"] = firstNonEmpty(overrides["info"], colors["info"])
	aliases["success"] = firstNonEmpty(overrides["success"], colors["success"])
	aliases["warning"] = firstNonEmpty(overrides["warning"], colors["warning"])
	aliases["error"] = firstNonEmpty(overrides["error"], colors["error"])
	aliases["radius-box"] = firstNonEmpty(overrides["radius-box"], rounded["md"], rounded["sm"], "1rem")
	aliases["radius-field"] = firstNonEmpty(overrides["radius-field"], rounded["sm"], rounded["md"], "0.5rem")
	aliases["radius-selector"] = firstNonEmpty(overrides["radius-selector"], rounded["sm"], rounded["md"], "0.5rem")
	return aliases
}

func buildReferenceContext(colors, rounded, spacing map[string]string, typography map[string]TypographyToken) map[string]string {
	ctx := map[string]string{}
	for key, value := range colors {
		ctx["colors."+key] = value
	}
	for key, value := range rounded {
		ctx["rounded."+key] = value
	}
	for key, value := range spacing {
		ctx["spacing."+key] = value
	}
	for key, value := range typography {
		ctx["typography."+key+".fontFamily"] = value.FontFamily
		ctx["typography."+key+".fontSize"] = value.FontSize
		ctx["typography."+key+".fontWeight"] = value.FontWeight
		ctx["typography."+key+".lineHeight"] = value.LineHeight
		ctx["typography."+key+".letterSpacing"] = value.LetterSpacing
		ctx["typography."+key+".fontFeature"] = value.FontFeature
		ctx["typography."+key+".fontVariation"] = value.FontVariation
	}
	return ctx
}

func resolveMap(input, ctx map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = resolveValue(value, ctx)
	}
	return result
}

func resolveNestedMap(input map[string]map[string]string, ctx map[string]string) map[string]map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]map[string]string, len(input))
	for key, nested := range input {
		result[key] = resolveMap(nested, ctx)
	}
	return result
}

func resolveTypography(input map[string]TypographyToken, ctx map[string]string) map[string]TypographyToken {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]TypographyToken, len(input))
	for key, value := range input {
		result[key] = TypographyToken{
			FontFamily:    resolveValue(value.FontFamily, ctx),
			FontSize:      resolveValue(value.FontSize, ctx),
			FontWeight:    resolveValue(value.FontWeight, ctx),
			LineHeight:    resolveValue(value.LineHeight, ctx),
			LetterSpacing: resolveValue(value.LetterSpacing, ctx),
			FontFeature:   resolveValue(value.FontFeature, ctx),
			FontVariation: resolveValue(value.FontVariation, ctx),
		}
	}
	return result
}

func resolveValue(value string, ctx map[string]string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		path := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}"))
		if resolved, ok := ctx[path]; ok && strings.TrimSpace(resolved) != "" {
			return resolved
		}
	}
	return value
}

func contrastText(hex string) string {
	hex = strings.TrimSpace(strings.TrimPrefix(hex, "#"))
	if len(hex) != 6 {
		return "#111827"
	}
	var r, g, b int
	_, err := fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	if err != nil {
		return "#111827"
	}
	brightness := (r*299 + g*587 + b*114) / 1000
	if brightness >= 140 {
		return "#111827"
	}
	return "#ffffff"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func cloneMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func cloneNestedMap(input map[string]map[string]string) map[string]map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]map[string]string, len(input))
	for key, value := range input {
		result[key] = cloneMap(value)
	}
	return result
}

func cloneTypography(input map[string]TypographyToken) map[string]TypographyToken {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]TypographyToken, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
