package backlog

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// specMarkdown es el contenido literal de internal/backlog/SPEC.md,
// embebido en el binario. Lo consume:
//
//   - El system prompt del agente cuando arranca una sesión enfocada
//     en el backlog (para que el LLM lea el contrato antes de
//     redactar).
//   - Tests que quieran verificar formato sin tocar el FS.
//
//go:embed SPEC.md
var specMarkdown string

// Spec devuelve el SPEC del perfil backlog sobre OKF v0.1.
func Spec() string {
	return specMarkdown
}

// indexMarkdown es el cuerpo del index.md raíz del bundle. Lo
// mantenemos en código porque es boilerplate estable: solo cambia
// cuando bumpeamos la versión de OKF o del perfil backlog.
//
// NOTA: este string NO puede contener backticks (delimitarían un
// raw string literal). Usamos comillas simples o reformulamos.
const indexMarkdown = `---
okf_version: "0.1"
---

# Backlog

Este bundle es un directorio de archivos Markdown con frontmatter
YAML conforme a Open Knowledge Format v0.1 (OKF). El **perfil**
aplicado es backlog/v1 (ver internal/backlog/SPEC.md para los
detalles del contrato).

## Estructura

- backlog/, todo/, in_progress/, done/ — columnas del tablero Kanban.
  Cada archivo .md dentro es una tarjeta con type=backlog/card.
- AGENTS.md — system prompt para el agente que opera sobre este
  bundle. Lo lee pi runtime al arrancar en este directorio.
- index.md (este archivo) — punto de entrada OKF para disclosure
  progresiva.

## Cómo agregar una tarjeta

1. Elegí la columna destino (default: backlog/).
2. El nombre del archivo se deriva del title según el algoritmo del
   SPEC §6 (lowercase, non-[a-z0-9] → '-', truncar a 60).
3. El frontmatter mínimo: type, title, status, priority. Recomendado:
   description, timestamp (RFC3339 UTC), source (user|agent).
`

// AgentContextMarkdown devuelve el contenido del archivo AGENTS.md
// que el runtime del agente lee como system prompt al arrancar con
// CWD en este bundle.
//
// Equivale a SystemPrompt() pero expuesto con un nombre que refleja
// el archivo real al que se mapea.
func AgentContextMarkdown() string {
	return SystemPrompt()
}

// SystemPrompt devuelve un system prompt listo para pasar al
// runtime del agente cuando se inicia una sesión de backlog. Incluye
// el SPEC completo más instrucciones operativas.
//
// El caller puede agregar contexto extra (ej: el ID del workspace,
// la ruta del bundle, el ID de la sesión actual) concatenándolo.
func SystemPrompt() string {
	return systemPrompt
}

// systemPrompt se construye una sola vez al iniciar el paquete
// porque Spec() (que embebe SPEC.md) es caro de leer en cada llamada.
// El SPEC cambia solo cuando bumpeamos la versión del perfil.
var systemPrompt = buildSystemPrompt()

func buildSystemPrompt() string {
	var b strings.Builder
	b.WriteString(`Eres un agente que opera sobre un bundle OKF v0.1 cuyo
perfil es "backlog". Tu trabajo es redactar, mover y mantener
tarjetas de backlog como archivos Markdown con frontmatter YAML.

# Lo que NO debes hacer

- NO uses tu API propia de cards (no existe): todas las mutaciones
  pasan por file tools (read/write/edit/rename) sobre los archivos
  del bundle. El bundle vive en el directorio configurado por
  BACKLOG_DIR (típicamente internal/backlog/board).
- NO invoques el server HTTP para crear/editar cards: está reservado
  para la UI.
- NO generes IDs propios: OKF ya provee identidad vía el path del
  archivo (Concept ID de OKF v0.1 §2).

# Cómo redactar

1. Leé el SPEC completo antes de tocar nada. Está embebido en este
   system prompt más abajo (sección "SPEC").
2. Respetá el formato: cada tarjeta es un .md con frontmatter YAML
   (campos type/title/status/priority/description/timestamp/source/...).
3. El slug del filename se deriva del title según el algoritmo del
   SPEC §6; el módulo Go lo aplica al escribir, vos solo proponé el
   title.
4. Al crear, status arranca en "backlog" salvo que el caller pida
   otra columna. priority default P3.
5. source: "agent" en cards que vos redactes, "user" en las que
   mantiene un humano.

# Cómo mantener

- Para mover de columna: rename() del archivo al directorio destino +
  update del frontmatter (status + timestamp).
- Para cambiar prioridad: edit() del frontmatter (priority + timestamp).
- Para borrar: delete() del archivo.
- Después de cada mutación, leé el archivo con read() para
  confirmar el estado final.

# Cuándo commitear vs cuándo pedir confirmación

- Si el caller humano te pide explícitamente "redactá N tarjetas",
  procedé directamente. Las tarjetas quedan source="agent" como
  trazabilidad.
- Si el caller dice "qué harías", NO escribas archivos. Mostrá los
  drafts en el chat (lista de tarjetas propuestas) y esperá
  confirmación.

# Cómo presentar drafts al humano

Cuando muestres un draft en chat (sin escribir todavía), usá este
formato para que sea fácil de copiar/pegar:

---
type: backlog/card
title: <título>
status: <columna>
priority: <P0|P1|P2|P3>
description: <una línea>
timestamp: <RFC3339 UTC>
source: agent
agent_session: <id de esta sesión>
---

<opcional: cuerpo markdown con criterios de aceptación, links, etc.>

# Errores comunes que debes evitar

- Frontmatter sin --- de cierre.
- type distinto a "backlog/card".
- status que no matchea el directorio donde vas a escribir.
- priority fuera de P0..P3.
- description en vez de title como línea principal.
- timestamp que no sea RFC3339 UTC.
- source="agent" sin agent_session poblado.

# ============================================================================
# SPEC (referencia normativa)
# ============================================================================

`)
	b.WriteString(specMarkdown)
	return b.String()
}

// EnsureBundleMetadata escribe index.md y AGENTS.md en el bundle si
// no existen. Es la versión "no destructiva" de WriteBundleMetadata:
// no pisa ediciones manuales del usuario.
//
// Llamar desde NewStore garantiza que un bundle recién creado
// siempre tenga los metadatos raíz necesarios para que el agente lo
// entienda.
func EnsureBundleMetadata(root string) error {
	if _, err := os.Stat(filepath.Join(root, "index.md")); err == nil {
		// Ya existe, no tocamos.
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat index.md: %w", err)
	} else {
		if err := os.WriteFile(filepath.Join(root, "index.md"), []byte(indexMarkdown), 0o644); err != nil {
			return fmt.Errorf("write index.md: %w", err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err == nil {
		// Ya existe, no tocamos.
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat AGENTS.md: %w", err)
	} else {
		if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(AgentContextMarkdown()), 0o644); err != nil {
			return fmt.Errorf("write AGENTS.md: %w", err)
		}
	}
	return nil
}

// WriteBundleMetadata escribe los archivos raíz del bundle SIEMPRE,
// sobreescribiendo los existentes. Útil para "regenerar el bundle
// desde código" (ej: tras bumpear el SPEC). El cuerpo de las
// tarjetas NO se toca.
//
// Para el caso típico de arranque, preferí EnsureBundleMetadata.
func WriteBundleMetadata(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("mkdir root: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.md"), []byte(indexMarkdown), 0o644); err != nil {
		return fmt.Errorf("write index.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(AgentContextMarkdown()), 0o644); err != nil {
		return fmt.Errorf("write AGENTS.md: %w", err)
	}
	return nil
}
