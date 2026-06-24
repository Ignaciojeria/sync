package design

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type rawDocument struct {
	Version     string                    `yaml:"version"`
	Name        string                    `yaml:"name"`
	Description string                    `yaml:"description"`
	Colors      map[string]any            `yaml:"colors"`
	Typography  map[string]map[string]any `yaml:"typography"`
	Rounded     map[string]any            `yaml:"rounded"`
	Spacing     map[string]any            `yaml:"spacing"`
	Components  map[string]map[string]any `yaml:"components"`
	XPi         rawXPiExtension           `yaml:"x-pi"`
}

type rawXPiExtension struct {
	ThemeID     string         `yaml:"themeId"`
	ColorScheme string         `yaml:"colorScheme"`
	DaisyUI     map[string]any `yaml:"daisyui"`
}

// ParseDocument parsea el frontmatter YAML de un DESIGN.md y conserva el body.
func ParseDocument(content string) (Document, error) {
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return Document{}, err
	}

	var raw rawDocument
	if err := yaml.Unmarshal([]byte(frontmatter), &raw); err != nil {
		return Document{}, fmt.Errorf("parse frontmatter yaml: %w", err)
	}

	return Document{
		Version:     strings.TrimSpace(raw.Version),
		Name:        strings.TrimSpace(raw.Name),
		Description: strings.TrimSpace(raw.Description),
		Colors:      normalizeScalarMap(raw.Colors),
		Typography:  normalizeTypography(raw.Typography),
		Rounded:     normalizeScalarMap(raw.Rounded),
		Spacing:     normalizeScalarMap(raw.Spacing),
		Components:  normalizeNestedScalarMap(raw.Components),
		XPi: XPiExtension{
			ThemeID:     strings.TrimSpace(raw.XPi.ThemeID),
			ColorScheme: strings.TrimSpace(raw.XPi.ColorScheme),
			DaisyUI:     normalizeScalarMap(raw.XPi.DaisyUI),
		},
		Body: strings.TrimSpace(body),
	}, nil
}

func splitFrontmatter(content string) (frontmatter string, body string, err error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(content, "---\n") && content != "---" && !strings.HasPrefix(content, "---\r\n") {
		return "", "", fmt.Errorf("missing frontmatter start fence")
	}

	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", fmt.Errorf("missing frontmatter start fence")
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", "", fmt.Errorf("missing frontmatter end fence")
	}

	frontmatter = strings.Join(lines[1:end], "\n")
	body = strings.Join(lines[end+1:], "\n")
	return frontmatter, body, nil
}

func normalizeScalarMap(input map[string]any) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = scalarString(value)
	}
	return result
}

func normalizeNestedScalarMap(input map[string]map[string]any) map[string]map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]map[string]string, len(input))
	for key, value := range input {
		result[key] = normalizeScalarMap(value)
	}
	return result
}

func normalizeTypography(input map[string]map[string]any) map[string]TypographyToken {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]TypographyToken, len(input))
	for key, value := range input {
		result[key] = TypographyToken{
			FontFamily:    scalarString(value["fontFamily"]),
			FontSize:      scalarString(value["fontSize"]),
			FontWeight:    scalarString(value["fontWeight"]),
			LineHeight:    scalarString(value["lineHeight"]),
			LetterSpacing: scalarString(value["letterSpacing"]),
			FontFeature:   scalarString(value["fontFeature"]),
			FontVariation: scalarString(value["fontVariation"]),
		}
	}
	return result
}

func scalarString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
