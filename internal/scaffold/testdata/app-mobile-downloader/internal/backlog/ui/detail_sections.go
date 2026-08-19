package ui

import (
	"strings"
)

// detailSectionsParts contiene el body partido en tramos según las
// secciones convencionales del perfil. Cada campo es markdown crudo
// (sin los headings) listo para pasar por goldmark + bluemonday.
type detailSectionsParts struct {
	preamble string
	plan     string
	criteria string
	links    string
	tail     string
}

// splitSections parte el cuerpo por las secciones "# Plan",
// "# Acceptance Criteria" y "# Links" en cualquier orden, todas
// case-insensitive sobre el nombre exacto (con aliases en español
// para criterios y links, según el SPEC §4.2). Lo que esté antes de
// la primera sección reconocida va a preamble, lo que quede después
// de la última va a tail. Las secciones del medio se extraen en su
// campo correspondiente; si una no aparece, su campo queda vacío.
//
// Las tres secciones son simétricas: el `# Plan` se renderiza como
// bloque destacado al igual que criterios y links, para que la
// lectura card ↔ plan sea inmediata en el detalle.
func splitSections(body string) detailSectionsParts {
	body = strings.TrimSpace(body)
	if body == "" {
		return detailSectionsParts{}
	}
	lines := strings.Split(body, "\n")

	planStart := -1
	criteriaStart := -1
	linksStart := -1

	for i, line := range lines {
		h := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		h = strings.TrimLeft(h, "# ")
		h = strings.TrimSpace(h)
		lower := strings.ToLower(h)
		switch lower {
		case "plan", "plan de trabajo":
			if planStart == -1 {
				planStart = i
			}
		case "acceptance criteria",
			"criterios de aceptación",
			"criterios de aceptacion":
			if criteriaStart == -1 {
				criteriaStart = i
			}
		case "links", "enlaces":
			if linksStart == -1 {
				linksStart = i
			}
		}
	}

	planEnd := -1
	criteriaEnd := -1
	linksEnd := -1
	if planStart != -1 {
		planEnd = sectionEnd(lines, planStart+1)
	}
	if criteriaStart != -1 {
		criteriaEnd = sectionEnd(lines, criteriaStart+1)
	}
	if linksStart != -1 {
		linksEnd = sectionEnd(lines, linksStart+1)
	}

	// preamble termina donde arranca la primera sección reconocida
	// (en cualquier orden).
	preambleEnd := minPos3(planStart, criteriaStart, linksStart)
	if preambleEnd == -1 {
		preambleEnd = len(lines)
	}

	join := func(from, to int) string {
		if from < 0 || to <= from {
			return ""
		}
		return strings.TrimSpace(strings.Join(lines[from:to], "\n"))
	}

	return detailSectionsParts{
		preamble: join(0, preambleEnd),
		plan:     join(planStart+1, planEnd),
		criteria: join(criteriaStart+1, criteriaEnd),
		links:    join(linksStart+1, linksEnd),
		tail:     tailAfter(lines, planEnd, criteriaEnd, linksEnd),
	}
}

// sectionEnd devuelve el índice de la primera línea a partir de `from`
// que es un heading (#, ## o ###). Si no encuentra ninguno, devuelve
// len(lines).
func sectionEnd(lines []string, from int) int {
	for i := from; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "#") {
			return i
		}
	}
	return len(lines)
}

// tailAfter devuelve lo que queda del cuerpo después de las secciones
// extraídas. Como plan, criterios y links están en orden flexible,
// devolvemos las líneas que están estrictamente después de la última
// sección reconocida, evitando duplicar las que ya forman parte de
// los tramos extraídos.
func tailAfter(lines []string, planEnd, criteriaEnd, linksEnd int) string {
	maxEnd := max3(planEnd, criteriaEnd, linksEnd)
	if maxEnd <= 0 || maxEnd >= len(lines) {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[maxEnd:], "\n"))
}

func minPos3(a, b, c int) int {
	m := a
	if b != -1 && (m == -1 || b < m) {
		m = b
	}
	if c != -1 && (m == -1 || c < m) {
		m = c
	}
	return m
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
