---
description: Extender el render del detalle para reconocer una seccion "# Plan" en el body y mostrarla como bloque destacado (alert-warning) entre el preamble y los Acceptance Criteria. Acompana al workflow card → plan.
priority: P3
source: agent
agent_session: dogfood-plans-card
status: done
tags: [backlog, ui, dogfooding]
timestamp: "2026-07-18T16:44:00Z"
title: 'Mejorar el detalle del backlog agregando seccion de plan'
type: backlog/card
---

# Mejorar el detalle del backlog agregando seccion de plan

Hoy el detalle de una card reconoce solo dos secciones convencionales
del perfil backlog (`# Acceptance Criteria` y `# Links`) y las pinta
como alertas destacadas. Cualquier otra cosa queda como Markdown
inline en el body, incluyendo un eventual `# Plan` que el autor haya
redactado. Queremos que el plan tenga su propio bloque visual para
que la lectura card ↔ plan sea inmediata.

La motivacion es doble: (1) poder dogfoodear — esta misma card tiene
una seccion `# Plan` y queremos verla renderizada como bloque
distinto apenas abramos el modal; (2) sentar la base para un futuro
flujo "anexar plan" en la UI sin tener que retrofitear el render
despues.

# Plan

1. **Extender `detailSectionsParts`** en
   `internal/backlog/ui/detail_sections.go` para incluir un campo
   `plan string`. Mantener `preamble`, `criteria`, `links` y `tail`
   tal como estan.

2. **Detectar la seccion** en `splitSections`: aceptar `# Plan` y
   `# Plan de trabajo` (case-insensitive, mismo criterio que ya se
   usa para criterios y links). Respetar el orden flexible: el plan
   puede estar antes o despues de criterios/links y el split debe
   seguir funcionando.

3. **Render del plan** en `internal/backlog/ui/detail_sections.templ`:
   agregar un `if plan != ""` que invoque `detailHighlightSection`
   con `("Plan", "alert-warning", plan)`. Ubicarlo despues del
   preamble y antes de criterios, para que la lectura vertical sea:
   contexto → plan → como verifico → con quien se relaciona.

4. **Regenerar templ** (`templ generate`) y verificar que
   `internal/backlog/ui/detail_sections_templ.go` se actualiza sin
   warnings.

5. **Tests**: extender `internal/backlog/ui/detail_sections_test.go`
   con un caso donde el body tiene `# Plan` antes de `# Acceptance
   Criteria`, y otro donde esta despues de `# Links`. Verificar que
   `splitSections` devuelve el contenido correcto en `plan` y que
   `preamble`/`tail` no lo duplican.

6. **Verificacion end-to-end**: `go build ./...` y `go test ./...`.
   Si el server esta levantado, abrir el modal de esta misma card y
   confirmar visualmente que el bloque amarillo "Plan" aparece entre
   el contexto y los criterios.

7. **Mover esta card a done** cuando el build y los tests pasen.

# Acceptance Criteria

- [ ] `splitSections` reconoce `# Plan` y `# Plan de trabajo`
      (case-insensitive) y devuelve su contenido en el campo `plan`.
- [ ] El plan se renderiza en el detalle como bloque `alert-warning`
      con icono y titulo "Plan", ubicado entre el preamble y
      `# Acceptance Criteria`.
- [ ] Una card sin seccion `# Plan` sigue renderizando exactamente
      como antes (no hay regresion visual).
- [ ] Cuando el plan aparece antes de `# Acceptance Criteria`, el
      `tail` queda vacio y no hay duplicacion.
- [ ] Cuando el plan aparece despues de `# Links`, no se pierde ni se
      duplica.
- [ ] Hay al menos un test que cubre cada uno de los dos ordenes.
- [ ] `templ generate && go build ./... && go test ./...` pasan
      limpios.
- [ ] Esta card queda en la columna `done/` al terminar.

# Links

- Card semilla usada como referencia visual:
  [Ejemplo: ping endpoint de healthcheck](/backlog/ejemplo-ping-endpoint-de-healthcheck.md).
- Render actual del detalle: `internal/backlog/ui/detail.templ` y
  `internal/backlog/ui/detail_sections.templ`.
- Logica de split: `internal/backlog/ui/detail_sections.go`.
