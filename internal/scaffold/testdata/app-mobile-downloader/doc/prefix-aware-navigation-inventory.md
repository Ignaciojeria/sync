# Inventario inicial de rutas absolutas para mounted application

Fecha: 2026-07-10

Este inventario corresponde al barrido inicial del repo para detectar rutas absolutas que pueden romper una mounted application bajo un `MountPrefix`.

---

## Resumen

### App / mounted-application candidates

- `href="/..."`: 1
- `hx-get="/..."`: 2
- `hx-post="/..."`: 1
- `http.Redirect(..., "/...")`: 3
- `Location` header con ruta absoluta: 1

### Host / agent-shell candidates

- `Location` header a `/agent?...`: 2
- `window.location.href = "/agent"`: 1
- `href="/agent"`: 2
- `href="/"` en UI del agente: 1
- `hx-get="/gateway/balance"` en shell/agente: 2

### Tests afectados por comportamiento actual

- `internal/auth/middleware/middleware_test.go`: 2 assertions sobre `Location: /auth/login`

---

## Clasificación detallada

## A. Mounted application URLs

Estas deben migrar a `nav.App(...)`, `RedirectApp(...)` o equivalente del `NavigationContext`.

### 1. Redirects server-side

#### `internal/auth/http/logout.go`
- Línea: 20
- Actual:
  - `http.Redirect(..., "/", ...)`
- Clasificación:
  - **App Redirect**
- Motivo:
  - Logout de la app previewed debería volver al root montado, no al host root.

#### `internal/auth/middleware/middleware.go`
- Líneas: 77, 88
- Actual:
  - redirect a `"/auth/login"`
- Clasificación:
  - **App Redirect**
- Motivo:
  - Si el usuario cae en auth desde la mounted application, el login debe resolverse dentro del contexto montado o al menos preservar `ReturnURL()`.

### 2. Template links/forms

#### `internal/auth/ui/login.templ`
- Línea: 49
- Actual:
  - `href="/auth/login/google"`
- Clasificación:
  - **App URL**
- Motivo:
  - Debe construirse desde el contexto de navegación y/o incorporar la estrategia de retorno.

#### `internal/design/ui/page.templ`
- Línea: 26
- Actual:
  - `hx-post="/design/select"`
- Clasificación:
  - **App URL**
- Motivo:
  - El selector de design system debe respetar el mount cuando se accede desde preview.

#### `internal/ui/layout/sidenav.templ`
- Línea: 54
- Actual:
  - `hx-get="/gateway/balance"`
- Clasificación:
  - **Pendiente de decisión**
- Motivo:
  - Hoy está en layout shell, pero si el sidenav forma parte de la mounted application, este request debe decidirse explícitamente:
    - `Host URL` si el balance es del host global
    - `App URL` si se espera mounted behavior consistente
- Recomendación actual:
  - tratarlo como **Host URL explícita**, no dejarlo implícito.

---

## B. Host / agent-shell URLs

Estas deben quedarse fuera del mount y generarse como `nav.Host(...)` o equivalente.

### 1. Agent HTTP/session wiring

#### `pkg/agent/http/sessions.go`
- Línea: 45
- Actual:
  - `Location: /agent?session=...`
- Clasificación:
  - **Host URL**
- Motivo:
  - La UI del agente es explícitamente la excepción de la regla mounted-app.

#### `pkg/agent/worker/handlers/sessions.go`
- Línea: 118
- Actual:
  - `Location: /agent?session=...`
- Clasificación:
  - **Host URL**
- Motivo:
  - Mismo caso: navegación de shell del agente.

### 2. Agent UI templates / JS

#### `pkg/agent/ui/page.templ`
- Línea: 610
- Actual:
  - `window.location.href = "/agent"`
- Clasificación:
  - **Host URL**
- Motivo:
  - Navegación del shell del agente, no de la mounted application.

#### `pkg/agent/ui/providers.templ`
- Línea: 13
- Actual:
  - `href="/agent"`
- Clasificación:
  - **Host URL**

#### `pkg/agent/ui/providers.templ`
- Línea: 49
- Actual:
  - `href="/agent"`
- Clasificación:
  - **Host URL**

#### `pkg/agent/ui/providers.templ`
- Línea: 50
- Actual:
  - `href="/"`
- Clasificación:
  - **Host URL**
- Motivo:
  - Dashboard general del host.

#### `pkg/agent/ui/providers.templ`
- Línea: 33
- Actual:
  - `hx-get="/gateway/balance"`
- Clasificación:
  - **Host URL**
- Motivo:
  - UI del agente, no mounted app.

---

## C. Tests a revisar cuando migre auth/navigation

### `internal/auth/middleware/middleware_test.go`
- Líneas: 516, 586
- Actual:
  - assertions esperan `Location == "/auth/login"`
- Impacto:
  - Si se introduce `MountPrefix` + `ReturnURL()` o mounted auth redirect, estas expectativas probablemente cambien.

---

## D. Hallazgos importantes del barrido

1. **El inventario actual no es enorme** en cantidad de matches crudos, pero sí toca zonas sensibles:
   - auth
   - layout
   - design system
   - UI del agente

2. **No aparecieron todavía** en este barrido:
   - `HX-Redirect`
   - `HX-Location`
   - `Link` headers
   - JSON explícito con campos `url` montados

   Eso no significa que no existan flujos equivalentes; sólo que no salieron con este patrón de búsqueda.

3. El principal foco de riesgo sigue siendo:
   - auth redirects
   - links/templates visibles
   - responses del shell que deben quedar como host-aware explícitas

---

## E. Recomendación operativa inmediata

Orden sugerido a partir de este inventario:

1. introducir `NavigationContext` con `MountPrefix`
2. introducir builder único:
   - `nav.App(...)`
   - `nav.Host(...)`
3. migrar primero auth redirects
4. migrar `internal/auth/ui/login.templ`
5. decidir explícitamente `gateway/balance` como host URL
6. revisar luego una segunda búsqueda para:
   - JS dinámico
   - JSON con URLs
   - headers custom

---

## F. Decisiones preliminares de clasificación

### App URLs
- `/`
- `/auth/login`
- `/auth/login/google`
- `/design/select`

### Host URLs
- `/agent`
- `/agent?session=...`
- `/gateway/balance` en la UI del agente
- `/` dentro de `pkg/agent/ui/providers.templ` como dashboard general del host

---

## G. Siguiente barrido recomendado

Además de los patrones ya corridos, conviene buscar luego:

- `fetch("/`
- ``fetch(`/``
- `new URL("/`
- `HX-Redirect`
- `HX-Location`
- `json.NewEncoder(...url...)`
- structs/DTOs con campos `URL`, `Href`, `Location`, `RedirectTo`, `ReturnTo`

Ese segundo barrido cubrirá mejor **server-generated URLs** y JS runtime.
