// Package ui contiene los tipos que las plantillas .templ del módulo
// de versiones consumen. Mantener structs dedicados (en lugar de pasar
// los de application directamente) permite que la UI agregue campos
// derivados (ej. "hace 2 horas") sin tocar el dominio.
package ui

import (
	"time"

	versionsapp "gitinittest5/internal/versions/application"
)

// ListState es lo que recibe ListPage: la lista de versiones + un
// mensaje de error opcional (ej. "no hay merges aún").
type ListState struct {
	Versions []versionsapp.Version
	Error    string
}

// DetailState es lo que recibe DetailPage: la versión puntual y el
// diff contra HEAD (lista de archivos cambiados).
type DetailState struct {
	Version versionsapp.Version
	Files   []versionsapp.VersionFile
}

// RelativeTime devuelve "hace X" en español para mostrar en la UI.
// Se calcula acá (no en application) porque es presentación pura.
func RelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "hace un momento"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "hace 1 min"
		}
		return "hace " + itoa(m) + " min"
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "hace 1 h"
		}
		return "hace " + itoa(h) + " h"
	case d < 30*24*time.Hour:
		dd := int(d.Hours() / 24)
		if dd == 1 {
			return "ayer"
		}
		return "hace " + itoa(dd) + " días"
	default:
		return t.Format("02 Jan 2006")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
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