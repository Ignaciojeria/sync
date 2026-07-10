# Plan: mounted application navigation

## Objetivo

Hacer que una aplicación servida bajo un subpath arbitrario se comporte como una **mounted application** de primera clase.

Ejemplo actual:

```text
/agent/sessions/:id/preview/...
```

Pero el mecanismo no debe quedar acoplado al caso de preview. Debe servir para cualquier contexto futuro donde una aplicación viva bajo un prefijo, por ejemplo:

```text
/workspace/123/...
/tenant/acme/...
```

La UI del agente/host sigue siendo una excepción explícita: no forma parte de la mounted application.

---

## 1. Invariantes del sistema

Este plan se reduce a tres invariantes fundamentales.

### Invariante 1

Toda request conoce su **MountPrefix**.

### Invariante 2

Toda URL de la aplicación se genera mediante el **NavigationContext**.

### Invariante 3

Toda URL del host se genera explícitamente mediante el mismo contexto.

Si estas tres reglas se cumplen, el resto del trabajo pasa a ser una migración mecánica.

---

## 2. Fuente de verdad: NavigationContext

La decisión principal no es `AppPath()` o `HostPath()` por sí solas. El centro del diseño es el contexto de montaje.

Ejemplo conceptual:

```go
type NavigationContext struct {
    MountPrefix string
    IsPreview   bool
}
```

Desde ese contexto se derivan los builders y helpers necesarios.

Ejemplos:

```go
nav.App("/scheduler/jobs")
nav.App("/design")
nav.Host("/agent")
nav.Host("/agent/providers")
nav.RedirectApp(...)
nav.RedirectHost(...)
nav.ReturnURL()
```

### Nota

`CurrentPath` puede existir si hace falta para marcado de navegación o estado activo, pero debe ser tratado como un dato **derivable**, no como el centro conceptual del sistema.

El dato realmente persistente y fundamental es `MountPrefix`.

---

## 3. Regla de clasificación de URLs

Toda URL del sistema debe clasificarse en una de dos categorías.

### A. Application URL

Debe permanecer dentro de la mounted application.

Ejemplos:

- `/`
- `/editor-view`
- `/report/tests`
- `/scheduler/jobs`
- `/design`
- `/auth/login`
- `/auth/logout`
- acciones HTMX
- forms de módulos
- redirects de módulos
- respuestas JSON que devuelvan URLs de app

### B. Host URL

Nunca debe quedar dentro de la mounted application.

Ejemplos:

- `/agent`
- `/agent/providers`
- endpoints internos del host/agente
- navegación del shell del agente
- controles del host

---

## 4. URL Builder único

Para evitar proliferación de helpers ad-hoc, toda generación de rutas debe salir del mismo objeto de navegación.

Ejemplos recomendados:

```go
nav.App("/scheduler")
nav.App("/design/theme")
nav.Host("/agent")
nav.Host("/agent/providers")
```

### Regla

Queda prohibido hardcodear rutas absolutas internas con strings como:

- `href="/algo"`
- `hx-get="/algo"`
- `action="/algo"`
- `http.Redirect(..., "/algo")`

para cualquier navegación que pertenezca a la mounted application.

---

## 5. Orden correcto de implementación

El orden cambia respecto al plan anterior.

## Fase 1 — Infra compartida

### Objetivo

Introducir el `NavigationContext` como fuente de verdad.

### Entregables

- `MountPrefix` resuelto por request
- builder único tipo `nav.App(...)` y `nav.Host(...)`
- helpers de redirect basados en el mismo contexto
- helper para return URL del flujo de auth

### Resultado

Ya existe una infraestructura consistente para generar URLs de app y host.

---

## Fase 2 — Barrido completo del repo

### Objetivo

Conocer el alcance real del trabajo antes de seguir migrando.

### Búsqueda global obligatoria

Buscar todos los matches de:

- `href="/`
- `hx-get="/`
- `hx-post="/`
- `hx-delete="/`
- `hx-put="/`
- `action="/`
- `http.Redirect(..., "/`
- `Location", "/`
- `HX-Redirect`
- `HX-Location`
- `window.location = "/`
- `window.location.href = "/`
- JSON con payloads tipo:

```json
{"url": "/algo"}
```

### Resultado esperado

Obtener inventario real del trabajo, por ejemplo:

- N `href`
- N `hx-*`
- N redirects
- N usos en JS
- N respuestas del backend con URLs

Este paso debe ocurrir **antes** de la migración completa, no al final.

---

## Fase 3 — Clasificación

### Objetivo

Clasificar cada URL encontrada.

### Cada ocurrencia debe quedar marcada como una de estas

- `App URL`
- `Host URL`
- pública/no mounted por diseño

### Resultado

La migración deja de ser reactiva y pasa a ser mecánica.

---

## Fase 4 — Migración de templates visibles

### Objetivo

Corregir la navegación más visible primero.

### Tocar

- sidenav
- home cards/buttons
- design panel
- quality links principales
- scheduler links y botones visibles
- login/logout visibles

### Resultado

La navegación principal del usuario deja de escaparse de la mounted application.

---

## Fase 5 — HTMX, forms y acciones parciales

### Objetivo

Evitar que acciones parciales rompan el mount.

### Revisar

- `hx-get`
- `hx-post`
- `hx-delete`
- `hx-put`
- `action`
- fragments/partials

### Regla

Todo endpoint funcional de la mounted application debe construirse con `nav.App(...)`.

---

## Fase 6 — Server-generated URLs

### Objetivo

Cubrir todas las URLs generadas desde backend, no sólo redirects.

### Incluir explícitamente

- `http.Redirect`
- header `Location`
- `HX-Redirect`
- `HX-Location`
- `Link` headers si existen
- payloads JSON con campos URL
- cualquier otro mecanismo server-driven que transporte una URL

### Resultado

Se evita que el frontend o HTMX termine navegando fuera del mount por una URL correcta sintácticamente pero semánticamente host-rooted.

---

## Fase 7 — Auth y return flows

### Objetivo

Evitar que login/logout u otros flujos de retorno rompan el contexto montado.

### Regla

No tratar `return_to` como un hack especial del login. Debe ser una capacidad del contexto de navegación.

Ejemplo conceptual:

```go
nav.ReturnURL()
```

### Flujos que deben consumir esa misma API

- login
- logout
- OAuth
- OIDC callback
- magic links
- cualquier flujo que redirija de vuelta a la app

---

## Fase 8 — Redirect helpers y consolidación

### Objetivo

Eliminar decisiones manuales repetidas.

### Entregables

- builder único para URLs
- redirect helpers únicos
- convenciones estables de uso en handlers y templates

### Resultado

El sistema deja de depender de memoria humana para decidir si una ruta debe respetar o no el mount.

---

## Fase 9 — Guardrail en CI

### Objetivo

Evitar regresiones futuras.

### Agregar un linter/check de CI

El build debe fallar si aparecen nuevos patrones prohibidos sin pasar por el contexto, por ejemplo:

- `href="/`
- `hx-get="/`
- `hx-post="/`
- `action="/`

Se puede extender luego a otros patrones según el inventario real.

### Resultado

Se reemplaza el costo futuro de perseguir bugs por una validación barata y automática.

---

## 6. Qué no hacer como estrategia principal

No usar como base:

- query param propagado manualmente por toda la UI
- JS global que reescriba todos los clicks
- confiar sólo en `<base href>`
- parches módulo por módulo sin inventario completo

### Razón

El problema no es un botón roto; el problema es que la aplicación entera debe comportarse como una aplicación montada bajo un prefijo.

---

## 7. Criterio de done

La implementación se considera correcta cuando:

1. toda request conoce su `MountPrefix`
2. toda URL de la aplicación se genera mediante `NavigationContext`
3. toda URL del host se genera explícitamente mediante el mismo contexto
4. Home, Console, Quality, Scheduler y Design no escapan del mount
5. HTMX sigue funcionando dentro del mount
6. forms no pierden el mount
7. redirects, `Location`, `HX-Redirect`, `HX-Location` y JSON con URLs respetan el contexto
8. login/logout retorna al mount cuando corresponde
9. la UI del agente sigue fuera del mount
10. existe un check en CI que evita reintroducir rutas absolutas internas sin contexto

---

## 8. Decisión arquitectónica

Este plan asume explícitamente una arquitectura de **mounted application** sobre un host estable.

En el caso actual:

- dominio/puerto públicos estables
- reverse proxy interno por sesión
- aplicación montada bajo un subpath dinámico

No usa como base:

- puertos públicos por sesión
- puertos efímeros visibles al usuario
- query params como representación del mount

---

## 9. Resultado esperado

Al finalizar, la aplicación previewed se comporta como una mounted application real, no como una app normal “forzada” a vivir bajo un prefijo.

Eso permite reutilizar la misma infraestructura mañana para cualquier otro escenario donde una app necesite vivir bajo un subpath arbitrario, no sólo previews por sesión.
