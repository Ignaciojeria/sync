package application

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ParseResult es lo que devuelve ParseCardFile.
//
//   - Card: la tarjeta con los campos conocidos extraídos.
//   - Frontmatter: el mapa completo del frontmatter, listo para
//     re-emitirse preservando keys que este módulo no conoce. El
//     contrato de OKF v0.1 §4.1 obliga a preservar unknown keys al
//     round-trip.
//   - err: nil si el archivo cumple el SPEC; error con motivo legible
//     si no. El FS layer loguea y excluye tarjetas inválidas.
type ParseResult struct {
	Card        Card
	Frontmatter map[string]any
}

// ParseCardFile parsea el contenido crudo de un archivo .md y
// devuelve la Card, el frontmatter crudo (para preservación) y un
// error si el archivo no cumple el SPEC.
//
// El path se guarda en card.Path; el slug se deriva del basename.
func ParseCardFile(path string, raw []byte) (ParseResult, error) {
	fm, body, err := splitFrontmatter(raw)
	if err != nil {
		return ParseResult{}, fmt.Errorf("split frontmatter: %w", err)
	}

	// 1) Parse completo a map para preservar keys desconocidas.
	all := map[string]any{}
	if err := yaml.Unmarshal(fm, &all); err != nil {
		return ParseResult{}, fmt.Errorf("yaml unmarshal: %w", err)
	}
	// Normalizar time.Time a string RFC3339 para que el round-trip
	// no las serialice como `2025-01-15 10:00:00 +0000 UTC` (formato
	// feo que yaml.v3 usa por defecto). Esto preserva el formato
	// original legible de fechas en el frontmatter.
	for k, v := range all {
		if t, ok := v.(time.Time); ok {
			all[k] = t.UTC().Format(time.RFC3339)
		}
	}

	// 2) Extraer campos conocidos via struct tipado.
	var known struct {
		Type         string   `yaml:"type"`
		Title        string   `yaml:"title"`
		Description  string   `yaml:"description"`
		Status       string   `yaml:"status"`
		Priority     string   `yaml:"priority"`
		Timestamp    string   `yaml:"timestamp"`
		Source       string   `yaml:"source"`
		AgentSession string   `yaml:"agent_session"`
		Tags         []string `yaml:"tags"`
	}
	if err := yaml.Unmarshal(fm, &known); err != nil {
		return ParseResult{}, fmt.Errorf("extract fields: %w", err)
	}

	card := Card{
		Path:         path,
		Slug:         SlugFromPath(path),
		Type:         strings.TrimSpace(known.Type),
		Title:        strings.TrimSpace(known.Title),
		Description:  strings.TrimSpace(known.Description),
		Status:       Status(strings.TrimSpace(known.Status)),
		Priority:     Priority(strings.TrimSpace(known.Priority)),
		Timestamp:    strings.TrimSpace(known.Timestamp),
		Source:       strings.TrimSpace(known.Source),
		AgentSession: strings.TrimSpace(known.AgentSession),
		Tags:         known.Tags,
		Body:         string(body),
	}

	// Defaults suaves: no sobreescriben lo que el archivo dice
	// explícitamente.
	if card.Type == "" {
		card.Type = DefaultType
	}
	if card.Priority == "" {
		card.Priority = DefaultPriority
	}
	if card.Source == "" {
		card.Source = DefaultSource
	}

	return ParseResult{Card: card, Frontmatter: all}, nil
}

// splitFrontmatter parte un archivo .md en (frontmatter, body).
// El frontmatter NO incluye los delimitadores `---`; el body
// tampoco incluye la línea vacía que típicamente los separa.
//
// Reglas:
//   - El archivo debe empezar exactamente con "---\n".
//   - El cierre es una línea cuyo único contenido es "---".
//   - Asume line endings LF (no CRLF). El SPEC no menciona Windows.
func splitFrontmatter(raw []byte) ([]byte, []byte, error) {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return nil, nil, fmt.Errorf("file does not start with '---\\n'")
	}
	rest := raw[4:] // skip "---\n"
	idx := bytes.Index(rest, []byte("\n---"))
	if idx < 0 {
		return nil, nil, fmt.Errorf("frontmatter not closed with '---'")
	}
	fm := rest[:idx]
	body := rest[idx+4:] // skip "\n---"
	// Trim de los newlines iniciales del body (los que típicamente
	// hay entre el cierre de frontmatter y el primer heading).
	body = bytes.TrimLeft(body, "\n")
	return fm, body, nil
}
