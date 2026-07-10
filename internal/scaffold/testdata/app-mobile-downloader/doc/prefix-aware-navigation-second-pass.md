# Segundo barrido: JS, headers y server-generated URLs

Fecha: 2026-07-10

Objetivo: cubrir huecos que no siempre aparecen en un barrido centrado sólo en `href`, `hx-*` y forms.

---

## 1. Resultado general

### Hallazgo principal

El segundo barrido salió **mucho más chico de lo esperado**.

No aparecieron usos relevantes de:

- `fetch("/...")`
- `fetch(`/...`)`
- `window.location = "/..."` fuera de la UI del agente ya conocida
- `HX-Redirect`
- `HX-Location`
- DTOs JSON evidentes con campos `url`, `href`, `location`, `redirect`, `return_to`

Eso es una muy buena señal.

---

## 2. Matches relevantes encontrados

## A. Redirects / Location ya conocidos

Estos reaparecen y siguen siendo los puntos server-side más importantes.

### App / mounted application

#### `internal/auth/http/logout.go`
- `http.Redirect(..., "/", ...)`
- Sigue siendo **App Redirect**

#### `internal/auth/middleware/middleware.go`
- redirects a `"/auth/login"`
- Siguen siendo **App Redirect**

### Host / shell

#### `pkg/agent/http/sessions.go`
- `Location: /agent?session=...`
- Sigue siendo **Host URL**

#### `pkg/agent/worker/handlers/sessions.go`
- `Location: /agent?session=...`
- Sigue siendo **Host URL**

---

## B. No se encontraron escapes adicionales relevantes en JS/HTMX runtime

En esta pasada no aparecieron nuevos casos de:

- navegación JS al root fuera del shell del agente
- headers HTMX especiales
- respuestas JSON con URLs absolutas obvias

---

## 3. Conclusión técnica

Con este segundo barrido, el riesgo principal ya no está en “huecos ocultos de JS o JSON”, sino en:

1. **templates visibles**
2. **HTMX/forms de módulos**
3. **redirects server-side**
4. **auth flow**

Es decir: el problema ya está suficientemente acotado.

---

## 4. Veredicto

### Sí estamos listos para implementar

Con la información del primer barrido + este segundo barrido, ya hay suficiente visibilidad para avanzar con la migración mounted-app aware.

### Qué cambió gracias al segundo barrido

Ahora sabemos que:

- no parece haber una capa grande de JS runtime escondida
- no parece haber un sistema complejo de URLs server-generated en JSON
- el trabajo real está más concentrado de lo que parecía

Eso hace mucho más razonable avanzar sin seguir postergando la implementación.

---

## 5. Recomendación de ejecución

A partir de aquí, el siguiente paso ya no debería ser otro barrido, sino:

1. consolidar `NavigationContext` con `MountPrefix`
2. introducir builder único:
   - `nav.App(...)`
   - `nav.Host(...)`
3. migrar redirects y auth
4. migrar templates/HTMX de módulos
5. agregar check en CI

---

## 6. Riesgo residual

El único riesgo residual importante es que haya URLs construidas de forma no detectable por regex simple, por ejemplo:

- concatenaciones dispersas en Go
- builders ad-hoc no obvios
- JS generado dinámicamente

Pero con el estado actual del repo, ese riesgo parece bajo comparado con el trabajo ya visible.

---

## 7. Decisión sugerida

**No hacer un tercer barrido ahora.**

El mejor retorno a partir de este punto es pasar a implementación sistemática.
