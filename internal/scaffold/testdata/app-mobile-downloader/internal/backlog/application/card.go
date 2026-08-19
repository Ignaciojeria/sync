// Package application contiene la lógica de negocio del backlog:
// tipos, validación, parseo y serialización de tarjetas. La
// persistencia sobre el FS vive en internal/backlog/infrastructure/fs;
// este paquete NO toca el disco directamente.
package application

import (
	"fmt"
	"strings"
)

// Status representa la columna kanban en la que se encuentra una
// tarjeta. Los valores válidos están fijados por el SPEC §3 y no se
// pueden extender sin bumpear el perfil a v2.
type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
)

// ColumnOrder define el orden visual de las columnas en el tablero.
// Las constantes se mantienen alineadas con el orden de este slice.
var ColumnOrder = []Status{StatusBacklog, StatusTodo, StatusInProgress, StatusDone}

// Valid indica si s es uno de los Status conocidos.
func (s Status) Valid() bool {
	switch s {
	case StatusBacklog, StatusTodo, StatusInProgress, StatusDone:
		return true
	}
	return false
}

// ColumnTitle devuelve la etiqueta legible en español de una columna.
func ColumnTitle(s Status) string {
	switch s {
	case StatusBacklog:
		return "Backlog"
	case StatusTodo:
		return "Por hacer"
	case StatusInProgress:
		return "En curso"
	case StatusDone:
		return "Hecho"
	}
	return string(s)
}

// Priority representa la prioridad de la tarjeta. Mientras más bajo
// el valor, más urgente. P0 es lo más urgente.
//
// El SPEC §4.1 fija el formato como string ("P0".."P3") para
// preservar la conformidad con OKF: cualquier consumidor que lea el
// frontmatter directamente ve el mismo valor.
type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
)

// Valid indica si p está dentro del rango soportado por el SPEC.
func (p Priority) Valid() bool {
	switch p {
	case PriorityP0, PriorityP1, PriorityP2, PriorityP3:
		return true
	}
	return false
}

// DefaultPriority es la prioridad que el SPEC §4.1 asigna cuando el
// campo está ausente en el frontmatter.
const DefaultPriority = PriorityP3

// PriorityLabel devuelve la etiqueta legible. Idéntica al string
// subyacente en este perfil, pero se mantiene como helper para que la
// UI no dependa del tipo.
func PriorityLabel(p Priority) string {
	if p.Valid() {
		return string(p)
	}
	return string(p)
}

// Card es la unidad mínima de trabajo del backlog. Vive en un
// archivo .md con frontmatter YAML; los campos de aquí son los
// declarados en el SPEC §4.1.
//
// Campos derivados (no persistidos en frontmatter):
//   - Path: ruta absoluta al archivo en disco (poblada por el FS layer).
//   - Slug: nombre del archivo sin .md (último segmento del Concept ID).
//
// Campos con default si faltan:
//   - Type: debe ser "backlog/card" en escritura; el parser acepta
//     otros types pero el Service los filtra.
//   - Priority: cae a P3.
//   - Source: cae a "user".
type Card struct {
	Path         string
	Slug         string
	Type         string
	Title        string
	Description  string
	Status       Status
	Priority     Priority
	Timestamp    string
	Source       string
	AgentSession string
	Tags         []string
	Body         string
}

// DefaultType es el discriminador OKF para tarjetas del backlog.
// Forma parte del contrato: cualquier archivo con type distinto es
// preservado por el módulo pero ignorado del listado.
const DefaultType = "backlog/card"

// DefaultSource es el origen por defecto cuando el campo falta.
const DefaultSource = "user"

// Validate revisa los campos obligatorios y semánticos. Devuelve nil
// si la tarjeta es válida.
//
// NO chequea coherencia entre el campo Status y el directorio donde
// vive el archivo: eso es responsabilidad del FS layer porque
// requiere conocer el path.
func (c Card) Validate() error {
	if c.Type != DefaultType {
		return fmt.Errorf("type must be %q, got %q", DefaultType, c.Type)
	}
	if strings.TrimSpace(c.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if !c.Status.Valid() {
		return fmt.Errorf("invalid status %q", c.Status)
	}
	if !c.Priority.Valid() {
		return fmt.Errorf("invalid priority %q", c.Priority)
	}
	if c.Source != "" && c.Source != DefaultSource && c.Source != "agent" {
		return fmt.Errorf("invalid source %q (must be user|agent)", c.Source)
	}
	if c.Source == "agent" && strings.TrimSpace(c.AgentSession) == "" {
		return fmt.Errorf("agent_session is required when source is agent")
	}
	return nil
}

// PriorityBadgeClass devuelve las clases DaisyUI para colorear la
// badge de prioridad según su nivel. P0 se ve más urgente que P3.
func PriorityBadgeClass(p Priority) string {
	switch p {
	case PriorityP0:
		return "badge badge-error badge-sm"
	case PriorityP1:
		return "badge badge-warning badge-sm"
	case PriorityP2:
		return "badge badge-info badge-sm"
	case PriorityP3:
		return "badge badge-ghost badge-sm"
	}
	return "badge badge-ghost badge-sm"
}
