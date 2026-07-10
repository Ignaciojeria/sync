# Plan técnico: Session Preview Proxy

## Objetivo

Permitir que cada sesión del agente pueda exponer un **preview HTTP real** de un servicio levantado dentro de su workspace, usando una URL estable del host:

`/agent/sessions/:id/preview/*`

El sistema debe:

- registrar previews por sesión
- proxear sólo a `127.0.0.1:<port>`
- evitar loops
- limpiar el preview al cerrar la sesión
- dejar base lista para conectar con **Preview** y **Preview Guide**

---

## 1. Decisiones del diseño

### MVP

- **1 preview activo por sesión**
- upstream permitido: **localhost/127.0.0.1 únicamente**
- el **agente/orquestador** registra el preview automáticamente
- el host hace **reverse proxy**
- el puerto debe ser **conocido explícitamente**, no descubierto por magia

### No MVP

- múltiples previews por sesión
- autodiscovery por scan de puertos
- parseo de stdout como mecanismo principal
- upstreams remotos
- balanceo, TLS interno, service mesh inventado

---

## 2. Diseño funcional

### Flujo

1. Se crea sesión.
2. La sesión ya tiene `worktree`/workspace listo.
3. El agente levanta un servicio preview en un puerto elegido.
4. El agente espera healthcheck OK.
5. El agente registra el preview en el servicio de sesiones.
6. El host publica `/agent/sessions/:id/preview/*`.
7. El usuario abre esa URL.
8. El host resuelve el upstream y hace proxy.

### Cleanup

Si la sesión se elimina o se cierra:

- se borra el registro preview
- si el proceso fue manejado por el agente, se apaga
- la URL pública deja de responder

---

## 3. Modelo de datos

### Archivo a tocar

- `pkg/agent/application/session.go`

### Cambio propuesto

Extender `Session` con metadata real de preview.

```go
type PreviewStatus string

const (
	PreviewStatusNone     PreviewStatus = ""
	PreviewStatusStarting PreviewStatus = "starting"
	PreviewStatusLive     PreviewStatus = "live"
	PreviewStatusDown     PreviewStatus = "down"
)

type Session struct {
	ID              string        `json:"id"`
	Title           string        `json:"title"`
	CWD             string        `json:"cwd"`
	WorkspacePath   string        `json:"workspacePath,omitempty"`
	Branch          string        `json:"branch,omitempty"`
	PreviewURL      string        `json:"previewURL,omitempty"`
	PreviewPort     int           `json:"previewPort,omitempty"`
	PreviewStatus   PreviewStatus `json:"previewStatus,omitempty"`
	PreviewLabel    string        `json:"previewLabel,omitempty"`
	PreviewHealth   string        `json:"previewHealth,omitempty"`
	Model           string        `json:"model"`
	PiSessionFile   string        `json:"piSessionFile"`
	Status          SessionStatus `json:"status"`
	LastPreview     string        `json:"lastPreview,omitempty"`
	LastPreviewKind string        `json:"lastPreviewKind,omitempty"`
	LastSeq         uint64        `json:"lastSeq,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}
```

### Nota lazy

Si se quiere una versión más corta para MVP, basta con:

- `PreviewPort`
- `PreviewStatus`
- `PreviewURL`

---

## 4. Contrato de aplicación

### Archivos a tocar

- `pkg/agent/application/manager.go`

### Nuevas operaciones

Agregar métodos al `AgentService`:

```go
RegisterPreview(ctx context.Context, sessionID string, input RegisterPreviewInput) (Session, error)
ClearPreview(ctx context.Context, sessionID string) (Session, error)
GetPreview(ctx context.Context, sessionID string) (Session, error)
```

### Input sugerido

```go
type RegisterPreviewInput struct {
	Port       int    `json:"port"`
	Label      string `json:"label"`
	HealthPath string `json:"healthPath"`
}
```

### Reglas de validación

- la sesión debe existir
- `port > 0`
- `HealthPath` default `/`
- `PreviewURL` la construye el host, no el agente externo
- no permitir registrar el puerto del propio host si esa información ya está disponible en el runtime

---

## 5. Lógica interna del preview

### Opción recomendada

No crear un registry separado todavía. Guardar el preview directo en `Session`.

Ventajas:

- menos archivos
- menos estado duplicado
- ya existe `SessionStore`

### Comportamiento sugerido en el manager

1. validar input
2. hacer healthcheck a `http://127.0.0.1:<port><healthPath>`
3. si responde:
   - `PreviewPort = port`
   - `PreviewStatus = live`
   - `PreviewHealth = healthPath`
   - `PreviewURL = /agent/sessions/:id/preview/`
4. si no responde:
   - `PreviewStatus = starting` o `down`
   - guardar igual sólo si se decide soportar arranque progresivo

### Recomendación

Para MVP, aceptar registro sólo si el healthcheck ya responde. Eso simplifica la UX y evita estados ambiguos.

---

## 6. Handler HTTP para registrar preview

### Archivos nuevos sugeridos

- `pkg/agent/http/preview_register.go`

### Rutas

#### Registrar

`POST /agent/sessions/{id}/preview`

Body:

```json
{
  "port": 43123,
  "label": "web",
  "healthPath": "/"
}
```

#### Limpiar

`DELETE /agent/sessions/{id}/preview`

### Uso

Este endpoint puede ser usado por:

- la UI, si en el futuro se quiere registro manual
- el runner del agente
- un flujo interno desde el mismo backend

### Registro de rutas

- `pkg/agent/http/register.go`

---

## 7. Handler HTTP para proxy público

### Archivo nuevo sugerido

- `pkg/agent/http/preview_proxy.go`

### Rutas

- `/agent/sessions/{id}/preview`
- `/agent/sessions/{id}/preview/*`

### Comportamiento

1. cargar sesión
2. validar que tenga `PreviewPort`
3. validar estado
4. construir upstream `http://127.0.0.1:<port>`
5. hacer reverse proxy del path restante

### Implementación

Usar stdlib:

- `net/http/httputil`
- `net/url`

Sin dependencias nuevas.

### Reglas

- copiar `RawQuery`
- preservar método HTTP
- reescribir path
- setear header anti-loop: `X-Agent-Preview-Proxy: 1`

### Protección anti-loop

Si una request ya llega con `X-Agent-Preview-Proxy: 1`, responder error y no re-proxear.

Además:

- rechazar puertos asociados al host principal si aplica
- nunca registrar ni resolver una URL pública proxied como upstream

---

## 8. Healthcheck

### Dónde vive

Puede vivir en `manager.go` al principio. No hace falta abstraer todavía.

### Comportamiento

`GET http://127.0.0.1:<port><healthPath>`

- timeout corto: 1–2 s
- si responde: preview `live`
- si falla: preview `starting` o `down`

### Siguiente paso opcional

Más adelante se puede agregar:

- `HEAD`
- fallback a `/`
- rechecks periódicos

Pero para MVP, un `GET` simple es suficiente.

---

## 9. Auto-registro

### Dónde debería pasar

No en la app clonada. No en el browser. Debe pasar en el **orquestador/runner del agente**.

### Posibles lugares

- `pkg/agent/infrastructure/pirpc/runner.go`
- o una capa nueva de launcher, sólo si más adelante realmente hace falta

### Propuesta simple

Cuando el agente necesite preview:

1. elige puerto libre
2. arranca comando con ese puerto
3. espera healthcheck
4. llama a `RegisterPreview(...)`

### Nota

No hace falta meter esta parte en la primera fase si el launcher todavía no está claro. Primero conviene habilitar:

- modelo
- endpoint
- proxy

Y después conectar el auto-registro.

---

## 10. Descubrimiento del puerto

### Recomendación

**No discovery. Puerto explícito.**

### Cómo obtenerlo

El agente define el puerto al lanzar el servicio.

Ejemplos:

- `PORT=43123 go run ./cmd/api`
- `npm run dev -- --port 43123`
- `vite --port 43123`

### Ventaja

- cero heurísticas
- cero parseo de logs
- fácil de testear

---

## 11. Limpieza al cerrar sesión

### Archivos a tocar

- `pkg/agent/application/manager.go`
- revisar el flujo actual donde se borra la sesión o se destruye el worktree

### Comportamiento

Al eliminar la sesión:

- limpiar metadata preview
- opcionalmente matar el proceso preview si fue manejado por el sistema

### Si aún no se manejan procesos preview

Entonces el mínimo aceptable es:

- limpiar `PreviewPort`
- limpiar `PreviewURL`
- limpiar `PreviewStatus`

---

## 12. UI mínima

### Archivos candidatos

- `pkg/agent/ui/page.templ`
- `pkg/agent/ui/fragments.templ`
- handlers/fragments que ya renderizan estado de sesión

### Mostrar

- badge de estado:
  - `No preview`
  - `Starting`
  - `Live`
  - `Down`
- link o botón:
  - `Open preview`

### No hacer aún

- panel complejo
- logs embebidos
- múltiples tarjetas por preview

---

## 13. Tests

### Archivos sugeridos

- `pkg/agent/application/manager_preview_test.go`
- `pkg/agent/http/preview_proxy_test.go`
- `pkg/agent/http/preview_register_test.go`

### Casos mínimos

#### Manager

- registra preview válido
- rechaza puerto inválido
- rechaza sesión inexistente
- setea `PreviewURL` esperado
- limpia preview

#### Proxy

- devuelve 404/400 si no hay preview
- proxea correctamente cuando hay upstream vivo
- preserva path y query
- corta loop si llega `X-Agent-Preview-Proxy`

#### Seguridad

- rechaza target no local
- rechaza puertos prohibidos si esa regla se implementa

---

## 14. Fases de implementación

### Fase 1 — Base de dominio

Archivos:

- `pkg/agent/application/session.go`
- `pkg/agent/application/manager.go`

Entregables:

- campos preview en sesión
- `RegisterPreview`
- `ClearPreview`
- validación básica
- tests

Salida:

- una sesión puede guardar y limpiar preview

### Fase 2 — HTTP de control

Archivos:

- `pkg/agent/http/register.go`
- `pkg/agent/http/preview_register.go`

Entregables:

- `POST /agent/sessions/{id}/preview`
- `DELETE /agent/sessions/{id}/preview`

Salida:

- preview registrable por API

### Fase 3 — Reverse proxy

Archivos:

- `pkg/agent/http/preview_proxy.go`
- `pkg/agent/http/register.go`

Entregables:

- `/agent/sessions/{id}/preview/*`
- reverse proxy
- anti-loop
- tests

Salida:

- preview navegable desde el host

### Fase 4 — UI mínima

Archivos:

- `pkg/agent/ui/page.templ`
- handlers o fragments relacionados

Entregables:

- badge de estado
- botón o link `Open preview`

Salida:

- el usuario ve si el preview existe y puede abrirlo

### Fase 5 — Auto-registro del agente

Archivos:

- probablemente `pkg/agent/infrastructure/pirpc/*`
- o una nueva pieza de infraestructura si luego se necesita launcher dedicado

Entregables:

- el agente lanza preview con puerto explícito
- healthcheck
- registro automático

Salida:

- UX completa sin intervención manual

---

## 15. Riesgos y mitigaciones

### Riesgo: loop de proxy

Mitigación:

- header anti-loop
- no registrar el propio host
- upstream siempre `127.0.0.1:<port>`

### Riesgo: SSRF local

Mitigación:

- no aceptar host arbitrario
- no aceptar URL completa
- guardar sólo `port`, no `target URL`

### Riesgo: puertos ocupados

Mitigación:

- el orquestador elige puerto libre
- si falla, no registra

### Riesgo: preview cae luego del registro

Mitigación:

- mostrar `down`
- recheck bajo demanda o al primer error

---

## 16. Definición de done

Está done cuando:

1. se puede registrar preview para una sesión
2. el host sirve `/agent/sessions/:id/preview/*`
3. el proxy funciona contra un upstream local vivo
4. no hay loop
5. al borrar sesión se limpia el preview
6. la UI muestra el estado básico

---

## 17. Recomendación final

Implementar en este orden:

1. **Session fields**
2. **Register/Clear preview en manager**
3. **Endpoint POST/DELETE**
4. **Reverse proxy handler**
5. **UI mínima**
6. **Auto-registro del agente**

Ese orden entrega valor rápido y deja para el final la parte más incierta, que es lanzar y administrar procesos preview.

---

## 18. Relación con Preview y Preview Guide

Este plan deja una base directa para el modelo de sesión que se viene discutiendo:

- **Goal**: define qué se está construyendo
- **Preview**: URL real y estado del resultado ejecutándose
- **Preview Guide**: instrucciones para validar qué revisar, en qué ruta y bajo qué flujo

### Siguiente paso sugerido

Una vez que exista `Session Preview Proxy`, el siguiente documento natural es definir un `Preview Guide` mínimo asociado a la sesión, por ejemplo con:

- `Title`
- `URL` o path sugerido dentro del preview
- `User` o contexto de prueba
- `Flow` o pasos
- `WhatToCheck`
- `KnownIssues`

Así, el preview deja de ser sólo una URL y pasa a ser una herramienta real de validación guiada.
