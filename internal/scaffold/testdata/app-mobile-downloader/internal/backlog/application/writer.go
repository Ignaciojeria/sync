package application

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"
)

// knownFields son las keys del frontmatter que el módulo gestiona.
// Cualquier key fuera de esta lista se preserva del frontmatter
// original (OKF v0.1 §4.1: "Consumers SHOULD preserve unknown keys
// when round-tripping").
var knownFields = map[string]bool{
	"type":          true,
	"title":         true,
	"description":   true,
	"status":        true,
	"priority":      true,
	"timestamp":     true,
	"source":        true,
	"agent_session": true,
	"tags":          true,
}

// WriteCardFile serializa una Card a bytes de archivo .md.
//
// Reglas:
//   - Preserva keys desconocidas del originalFM (OKF §4.1).
//   - Aplica defaults suaves para campos vacíos (type, source).
//   - NO emite campos vacíos como description="" o agent_session="".
//   - Preserva el body byte por byte, agregando \n final si falta
//     para que el archivo sea POSIX-compliant.
//   - El timestamp, si la Card lo trae vacío, se completa con
//     NowRFC3339() para que el campo siempre tenga valor.
//
// Devuelve un error solo si el YAML no se puede serializar (muy
// raro en practice); las validaciones de negocio se hacen en
// Service.Write antes de llegar acá.
func WriteCardFile(card Card, originalFM map[string]any) ([]byte, error) {
	fm := make(map[string]any, len(originalFM)+8)
	for k, v := range originalFM {
		if knownFields[k] {
			// Lo vamos a reescribir desde los campos de Card;
			// ignorar la versión vieja para evitar keys
			// duplicadas o con valor stale.
			continue
		}
		fm[k] = v
	}

	// Forzar type por contrato.
	if card.Type == "" {
		fm["type"] = DefaultType
	} else {
		fm["type"] = card.Type
	}

	fm["title"] = strings.TrimSpace(card.Title)
	if card.Description != "" {
		fm["description"] = strings.TrimSpace(card.Description)
	}
	fm["status"] = string(card.Status)
	fm["priority"] = string(card.Priority)

	ts := strings.TrimSpace(card.Timestamp)
	if ts == "" {
		ts = NowRFC3339()
	}
	fm["timestamp"] = ts

	src := card.Source
	if src == "" {
		src = DefaultSource
	}
	fm["source"] = src

	if card.AgentSession != "" {
		fm["agent_session"] = strings.TrimSpace(card.AgentSession)
	}
	if len(card.Tags) > 0 {
		fm["tags"] = card.Tags
	}

	out, err := yaml.Marshal(fm)
	if err != nil {
		return nil, fmt.Errorf("yaml marshal: %w", err)
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(out)
	if !bytes.HasSuffix(out, []byte("\n")) {
		buf.WriteString("\n")
	}
	buf.WriteString("---\n")
	if card.Body != "" {
		buf.WriteString(card.Body)
		if !strings.HasSuffix(card.Body, "\n") {
			buf.WriteString("\n")
		}
	}
	return buf.Bytes(), nil
}

// NowRFC3339 devuelve el timestamp actual en RFC3339 UTC. Helper
// exportado para que el Service pueda setear timestamp antes de
// llamar a WriteCardFile sin importar time directamente.
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// Slugify aplica el algoritmo del SPEC §6 al título de una tarjeta:
// lowercase, no-[a-z0-9] → '-', colapsar runs, trim, truncar a 60,
// agregar sufijo numérico si el slug ya existe en usedSlugs.
//
// usedSlugs es el conjunto de slugs ya presentes en el directorio
// destino (sin la extensión .md). Si el slug derivado no está en
// usedSlugs, se devuelve tal cual. Si está, se prueba con "-2",
// "-3", … hasta encontrar uno libre.
func Slugify(title string, usedSlugs map[string]bool) string {
	slug := baseSlug(title)
	if !usedSlugs[slug] {
		return slug
	}
	for i := 2; i < 10_000; i++ {
		candidate := slug + "-" + itoa(i)
		if !usedSlugs[candidate] {
			return candidate
		}
	}
	// En la práctica inalcanzable; caemos al slug base con un
	// sufijo aleatorio-ish si llegara a pasar.
	return slug + "-x"
}

var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

func baseSlug(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	// Separar diacríticos (NFD). Insertar un "-" después de cada
	// letra base que tenía combining mark, así "á" se separa en
	// "a" + combining → "a-" y termina como "a-" antes del slugify
	// general, que ya colapsa runs de "-". El resultado final es
	// "a-i-o-u" para "áéíóú", lo que mantiene el slug legible.
	s = norm.NFD.String(s)
	s = diacriticSplitRegex.ReplaceAllString(s, "$1-")
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	const max = 60
	if len(s) > max {
		s = s[:max]
		s = strings.TrimRight(s, "-")
	}
	if s == "" {
		s = "untitled"
	}
	return s
}

var diacriticSplitRegex = regexp.MustCompile(`([a-z])[\x{0300}-\x{036f}]+`)
var combiningRegex = regexp.MustCompile(`[\x{0300}-\x{036f}]`)

func removeCombining(s string) string {
	return combiningRegex.ReplaceAllString(s, "")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// SlugFromPath devuelve el slug (último segmento del Concept ID sin
// la extensión .md) a partir de la ruta absoluta o relativa al
// archivo. Si el path no termina en .md, devuelve el basename tal
// cual.
func SlugFromPath(path string) string {
	base := path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".md")
	return base
}

// ConceptID devuelve el Concept ID de OKF v0.1 §2 a partir del path
// absoluto: el path dentro del bundle (relativo a root) sin el
// sufijo .md. Si root no es prefijo de path, devuelve el basename
// sin .md.
func ConceptID(root, path string) string {
	rel := strings.TrimPrefix(path, root)
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimSuffix(rel, ".md")
	return rel
}
