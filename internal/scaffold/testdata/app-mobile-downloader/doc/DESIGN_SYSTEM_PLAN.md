# Plan: Design System dirigido por `DESIGN.md`

> Estado: **propuesta** · Autor: equipo · Última edición: 2026-06
>
> Objetivo: que uno o varios archivos `DESIGN.md` sean la **única fuente de
> verdad** del estilo. Un visor en el sidenav muestra paleta, spacing,
> tipografía y radios; el sitio completo se renderiza con esos mismos tokens; y
> el usuario puede seleccionar entre varios `DESIGN.md` y ver el cambio en vivo.

---

## 1. Idea central (la regla de oro)

No se trata de "leer el `DESIGN.md` y pintar el sidenav". La clave es **una sola
fuente de verdad**:

```text
DESIGN.md (tokens + rationale)
   │  parse
   ▼
DesignDocument (Go struct)
   │  validate + resolve + map
   ▼
ResolvedTheme
   │  compile
   ▼
Theme DaisyUI v5 / CSS variables runtime
   │                         │
   ▼                         ▼
Visor del sidenav      Sitio completo (home, quality, scheduler, …)
```

El visor del sidenav **no imita** al `DESIGN.md`: renderiza literalmente las
mismas variables CSS que usa el resto del sitio. Por eso la alineación es
automática y no se puede desincronizar.

**Invariante que hace que todo funcione:** ningún template usa color/spacing
ad-hoc (`bg-[#1d4ed8]`, `text-blue-500`, `p-[13px]`). Solo clases semánticas de
DaisyUI o utilidades aprobadas del sistema (`bg-primary`, `text-base-content`,
`rounded-box`, `bg-base-200`, etc.). Si se respeta esto, cambiar de
`DESIGN.md` muta todo el sitio sin tocar markup.

---

## 2. Alcance y contrato del formato

### 2.1 Decisión explícita

El proyecto **adoptará `DESIGN.md` alineado con el formato de Google como base
canónica**. La regla general será:

- usar primero las claves y estructuras estándar del spec público
- mapear ese modelo estándar al runtime theme del proyecto
- agregar extensiones locales **solo cuando sean realmente necesarias**
- aislar esas extensiones en claves explícitas para no contaminar los tokens
  canónicos

Esto significa:

- se conserva la idea central del formato:
  - Markdown legible por humanos
  - frontmatter YAML con tokens
  - secciones de rationale y guía visual
- el parser del proyecto intentará leer el documento como `DESIGN.md` estándar
  primero
- cualquier necesidad específica de DaisyUI/Tailwind se resolverá mediante:
  - mapeos internos desde tokens estándar, o
  - extensiones namespaced del proyecto cuando no exista equivalente directo

### 2.2 Compatibilidad buscada

La meta es que un `DESIGN.md` del proyecto siga siendo reconocible y utilizable
como un documento alineado al ecosistema de Google. En particular:

- `colors`, `typography`, `rounded`, `spacing` y `components` serán las claves
  canónicas preferidas
- las secciones Markdown seguirán el orden recomendado por el spec cuando
  existan
- el archivo podrá validarse conceptualmente contra el spec, aunque el proyecto
  use un subconjunto práctico en el MVP

Cuando haga falta información específica de runtime, se agregará como extensión
explícita, por ejemplo bajo `x-pi`.

### 2.3 Convención de archivos

Para evitar ambigüedad con el estándar, se fija esta convención:

```text
design/
  ocean/
    DESIGN.md
  sunset/
    DESIGN.md
  forest/
    DESIGN.md
  _schema.md
```

Ventajas:

- se mantiene el nombre canónico `DESIGN.md`
- cada tema puede crecer con assets o notas propias en su carpeta
- evita nombres mixtos como `ocean.design.md` o `design.md`

El `id` del tema se deriva del nombre de la carpeta (`ocean`, `sunset`, etc.),
no del nombre del archivo.

---

## 3. Formato del `DESIGN.md` del proyecto

Cada `DESIGN.md` es Markdown legible por humanos **con un frontmatter YAML** que
contiene los tokens. El cuerpo Markdown es documentación (rationale, do/don't),
el frontmatter es lo que la app parsea.

Ejemplo alineado al formato de Google, con extensiones mínimas del proyecto:

```markdown
---
version: alpha
name: "Ocean"
description: "Tema corporativo frío, alto contraste."
colors:
  primary: "#2563eb"
  secondary: "#7c3aed"
  tertiary: "#06b6d4"
  neutral: "#ffffff"
  surface: "#f3f4f6"
  on-surface: "#111827"
  info: "#0ea5e9"
  success: "#16a34a"
  warning: "#d97706"
  error: "#dc2626"
typography:
  body-md:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "16px"
    fontWeight: 400
    lineHeight: 1.5
  label-md:
    fontFamily: "Inter, system-ui, sans-serif"
    fontSize: "14px"
    fontWeight: 500
  code-md:
    fontFamily: "JetBrains Mono, monospace"
    fontSize: "14px"
rounded:
  sm: "0.5rem"
  md: "1rem"
spacing:
  xs: "0.25rem"
  sm: "0.5rem"
  md: "1rem"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "#ffffff"
    rounded: "{rounded.sm}"
    padding: "{spacing.sm}"
x-pi:
  themeId: "ocean"
  colorScheme: "light"
  daisyui:
    primary-content: "#ffffff"
    accent: "{colors.tertiary}"
    base-100: "{colors.neutral}"
    base-200: "{colors.surface}"
    base-300: "#e5e7eb"
    base-content: "{colors.on-surface}"
    neutral: "#1f2937"
    neutral-content: "#ffffff"
    radius-box: "{rounded.md}"
    radius-field: "{rounded.sm}"
    radius-selector: "{rounded.sm}"
---

# Ocean

## Overview

Tema corporativo frío, sobrio y de alto contraste, orientado a productividad.

## Colors

El azul principal comunica confianza y acción. El fondo se mantiene claro para
favorecer lectura y jerarquía de contenido.

## Typography

Inter gobierna la interfaz y JetBrains Mono se reserva para datos técnicos y
fragmentos de código.
```

### 3.1 Campos soportados inicialmente

Canónicos del spec de Google:

- `version`
- `name`
- `description`
- `colors`
- `typography`
- `rounded`
- `spacing`
- `components`

Extensiones mínimas del proyecto:

- `x-pi.themeId`
- `x-pi.colorScheme`
- `x-pi.daisyui.*`

### 3.2 Principio de extensibilidad

Siempre que sea posible, una necesidad del sitio debe resolverse desde tokens
estándar. Solo si el runtime necesita información que no puede inferirse de ahí,
se agrega una extensión `x-pi`.

Ejemplos de uso esperado de `x-pi`:

- elegir el `themeId` runtime sin derivarlo ambiguamente
- declarar `colorScheme` (`light`/`dark`)
- completar aliases específicos de DaisyUI que no surgen 1:1 del spec base

### 3.3 Campos fuera de alcance en MVP

Aunque el ecosistema `DESIGN.md` permita más riqueza, en una primera etapa no se
implementan aún:

- edición desde la UI
- component tokens detallados por variante
- exportadores a otros formatos
- sincronización automática con Stitch/Figma

Eso puede venir después sin invalidar esta base.

---

## 4. Modelo interno y pipeline

Se recomienda separar claramente estas capas:

### 4.1 `DesignDocument`
Representa el documento parseado casi tal como viene del archivo.

Responsabilidad:
- parsear frontmatter
- conservar metadata
- conservar cuerpo Markdown
- reportar errores de formato

### 4.2 `ResolvedTheme`
Representa el tema ya validado, normalizado y listo para usarse.

Responsabilidad:
- aplicar defaults
- validar colores/dimensiones
- resolver faltantes
- derivar `ThemeID` desde `x-pi.themeId` o, en su defecto, desde la carpeta
- mapear tokens estándar de `DESIGN.md` a aliases runtime del proyecto
- exponer tokens ya seguros para UI y CSS

### 4.3 `ThemeCSS`
Representa la compilación final a variables CSS/DaisyUI runtime.

Responsabilidad:
- emitir `[data-theme="..."] { ... }`
- generar hash para ETag/cache
- servir como salida estable para layouts

Esta separación permite que en el futuro exista:

- lint propio
- diff entre temas
- exportadores
- vista de validación
- validación más fuerte contra el spec de Google

---

## 5. Defaults, validación y fallback

Un `DESIGN.md` inválido **no debe tumbar el server**.

### 5.1 Reglas

- Si un tema no parsea, se excluye del catálogo y se registra warning.
- Si un tema parsea pero le faltan tokens no críticos, se completa con defaults.
- Si faltan tokens críticos mínimos, el tema no se publica como seleccionable.
- Siempre existe un tema por defecto válido del proyecto.

### 5.2 Tokens mínimos para que un tema sea utilizable

Mínimos sugeridos:

- `name`
- `colors.primary`
- algún token de superficie legible (`colors.neutral` o equivalente mapeable)
- algún token de texto legible (`colors.on-surface` o equivalente mapeable)
- `rounded.sm` o `rounded.md`
- al menos un token de `spacing`
- al menos un nivel tipográfico base utilizable, por ejemplo `typography.body-md`

### 5.3 Política de fallback

Ejemplos:

- falta `x-pi.daisyui.primary-content` → derivar automáticamente contraste legible sobre `colors.primary`
- falta `x-pi.daisyui.base-200` o `base-300` → derivar desde el color de superficie/base mapeado
- falta `x-pi.daisyui.radius-field` → usar `rounded.sm` o el radio base más cercano
- falta tipografía monoespaciada explícita → usar fallback del sistema o buscar un token como `typography.code-md`

Los fallbacks deben ser explícitos, testeados y documentados en `design/_schema.md`.

---

## 6. Fuentes y tipografía

La tipografía necesita una política explícita para que el cambio de tema sea
real y no solo teórico.

### 6.1 Política inicial recomendada

El sistema soporta dos niveles:

1. **Fuentes seguras del sistema**
2. **Un conjunto acotado de fuentes web aprobadas por el proyecto**

### 6.2 MVP

En el MVP, los temas pueden declarar familias en niveles tipográficos estándar
(por ejemplo `body-md.fontFamily`, `code-md.fontFamily`), pero la app solo
promete compatibilidad visual real con una whitelist conocida. Ejemplos:

- `Inter`
- `Public Sans`
- `JetBrains Mono`
- fallbacks del sistema

Si un tema referencia una fuente no soportada:

- no falla el tema
- se usa fallback seguro
- se registra warning en logs/diagnóstico

### 6.3 Implicancia para layouts

El layout debe cargar las fuentes soportadas necesarias o una combinación estable
que cubra todos los temas aprobados.

---

## 7. Catálogo de temas y decisión sobre `go:embed`

### 7.1 Decisión para MVP

El MVP usará `go:embed` para incluir `design/**/DESIGN.md` en el binario.

Ventajas:

- despliegue simple
- sin dependencia de filesystem externo
- comportamiento estable entre entornos

### 7.2 Aclaración importante

Con `go:embed`, **agregar un tema nuevo sí requiere recompilar el binario**.

Por lo tanto, el contrato correcto es:

> Agregar un tema nuevo no requiere cambiar lógica de aplicación, pero sí requiere
> rebuild/redeploy del binario mientras se use `go:embed`.

### 7.3 Posible evolución futura

Si más adelante se necesita carga dinámica sin rebuild, se puede agregar un
modo filesystem/configurable. No es requisito del MVP.

---

## 8. Arquitectura propuesta (alineada con el proyecto)

Nuevo módulo `internal/design/` siguiendo la convención del proyecto
(`application` / `http` / `ui`, ver `AGENTS.md`):

```text
internal/design/
  application/
    document.go        # DesignDocument, metadata y tipos base
    parser.go          # DESIGN.md (frontmatter YAML) -> DesignDocument
    parser_test.go
    resolver.go        # DesignDocument -> ResolvedTheme
    resolver_test.go
    catalog.go         # descubre y cachea design/**/DESIGN.md (go:embed)
    catalog_test.go
    theme_css.go       # ResolvedTheme -> CSS runtime DaisyUI v5
    theme_css_test.go
  http/
    register.go
    theme_css.go       # GET /design/theme/{id}.css
    switch.go          # POST /design/select
    page.go            # GET /design (opcional)
  ui/
    panel.templ        # visor para el sidenav
    page.templ         # visor full-page (opcional)
```

Responsabilidades:

- **`application/`** no conoce HTTP ni templ. Parsea, valida, resuelve y compila.
- **`http/`** sirve el CSS del theme, gestiona la selección y devuelve
  fragmentos HTMX o refresh.
- **`ui/`** solo render; consume `ResolvedTheme` ya resuelto.

---

## 9. Cómo se inyecta al sitio (el punto crítico)

Hoy los layouts tienen `data-theme="light"` hardcodeado
(`internal/ui/layout/layout.templ` y `layout_with_nav.templ`). Cambios:

1. El layout deja de hardcodear el theme. Recibe el `id` del tema activo (desde
   cookie `design-theme`, con default configurable) y emite:

   ```html
   <html lang="es" data-theme="{activeThemeId}">
     <head>
       …
       <link rel="stylesheet" href="/design/theme/{activeThemeId}.css">
     </head>
   ```

2. `GET /design/theme/{id}.css` devuelve el theme compilado como CSS de DaisyUI
   v5, por ejemplo:

   ```css
   [data-theme="ocean"] {
     --color-primary: #2563eb;
     --color-primary-content: #ffffff;
     --color-secondary: #7c3aed;
     --color-accent: #06b6d4;
     --color-base-100: #ffffff;
     --color-base-200: #f3f4f6;
     --color-base-content: #111827;
     /* … */
     --radius-box: 1rem;
     --radius-field: 0.5rem;
     --radius-selector: 0.5rem;
   }
   ```

3. Como todo el markup usa clases semánticas DaisyUI, el sitio entero adopta el
   theme sin tocar ningún `.templ` adicional, salvo donde hoy existan estilos
   hardcodeados que deban migrarse.

> Nota stack: el proyecto carga Tailwind v4 y DaisyUI v5 por **CDN** en runtime.
> Por eso definir el theme como CSS plano inyectado vía `<link>` es lo más
> simple y robusto: no requiere build step de Tailwind.

---

## 10. Selección entre varios `DESIGN.md` (en vivo)

En el sidebar (`internal/ui/layout/sidenav.templ`), una sección nueva
"Design system" con:

- un selector (dropdown o lista de swatches) con todos los temas del catálogo
- preview breve del tema activo
- acceso al visor de tokens activos

Al elegir uno:

- `hx-post="/design/select"` con el `id`
- el handler setea cookie `design-theme=<id>`
- responde con `HX-Refresh: true` para simplificar el cambio integral

Resultado:
- el sitio completo cambia al instante
- el sidenav cambia con el mismo runtime theme
- no hay doble fuente de verdad

Persistencia: cookie server-rendered, compatible con el enfoque HTMX del
proyecto.

---

## 11. Visor del sidenav

El visor (`panel.templ`) muestra **los tokens efectivos del tema activo**, no
solo los del archivo original.

Contenido sugerido:

- **Paleta**: swatch por color con rol y valor final
- **Spacing**: escala y niveles definidos en `spacing`
- **Radios**: preview del mapeo desde `rounded` a radios runtime
- **Tipografía**: muestra de niveles relevantes (`body-md`, `label-md`, `code-md`, etc.)
- **Metadata**: nombre del tema, esquema (`light`/`dark`), descripción breve

### 11.1 Importante sobre spacing

Mostrar spacing es fácil; hacer que todo el sitio responda al spacing del tema
es más difícil. Por eso el plan distingue:

- **preview de tokens** en el visor
- **adopción real del layout** por etapas

En el MVP, el theme controla de forma fuerte:

- colores
- radios
- tipografía base

Y controla de forma progresiva:

- spacing de componentes compartidos
- separación de secciones principales
- densidad del layout

No se promete que cada `gap-*` o `p-*` existente del proyecto quede
parameterizado desde el día uno.

---

## 12. Garantizar la alineación (lint y guardarraíles)

Para que la regla de oro no se rompa con el tiempo, se agregan checks en
CI/pre-commit.

### 12.1 Política

En `*.templ` se permiten:

- clases semánticas de DaisyUI (`bg-primary`, `text-base-content`, etc.)
- utilidades estructurales/layout aprobadas (`flex`, `grid`, `gap-*`, `p-*`, etc.)
- excepciones explícitas en el visor de diseño

Se prohíben:

- colores arbitrarios: `bg-[#…]`, `text-[#…]`, `border-[#…]`
- utilidades de color crudas de Tailwind: `text-blue-500`, `bg-red-600`, etc.
- radios arbitrarios fuera del sistema cuando exista equivalente semántico

### 12.2 Script

- `scripts/check-design-tokens.sh`
- falla si detecta usos no permitidos
- permite excepciones limitadas en `internal/design/ui/` por ser un visor técnico

Esto convierte la alineación en algo **forzado por tooling**, no por disciplina.

---

## 13. Entregables por fases

### Fase 1 — Núcleo de documentos y themes
- [ ] `DesignDocument` + `parser.go` + tests.
- [ ] `ResolvedTheme` + `resolver.go` + tests.
- [ ] `catalog.go` con `go:embed design/**/DESIGN.md`.
- [ ] `theme_css.go`: theme -> CSS DaisyUI v5 + tests.
- [ ] `design/_schema.md` con contrato del perfil del proyecto.
- [ ] 2-3 temas de ejemplo.

### Fase 2 — Inyección en el sitio
- [ ] `GET /design/theme/{id}.css` con `ETag`/cache.
- [ ] Layouts leen tema activo desde cookie.
- [ ] Quitar `data-theme="light"` fijo.
- [ ] Verificar que home/quality/scheduler cambian de tema sin tocar markup,
      salvo estilos hardcodeados a migrar.

### Fase 3 — Visor + selector en el sidenav
- [ ] `panel.templ`: paleta, spacing, radios, tipografía.
- [ ] `POST /design/select` con cookie + `HX-Refresh`.
- [ ] Integración en `sidenav.templ`.

### Fase 4 — Guardarraíles y cobertura real del sistema
- [ ] `scripts/check-design-tokens.sh` + hook/CI.
- [ ] Auditoría de templates existentes para eliminar colores ad-hoc.
- [ ] Migrar componentes compartidos a tokens semánticos donde aún no lo estén.
- [ ] Página full-page `/design` (opcional).
- [ ] Regenerar `STRUCTURE.md`.

### Fase 5 — Evoluciones opcionales
- [ ] diff entre temas
- [ ] diagnóstico/validación de temas inválidos
- [ ] dark/light paired themes
- [ ] carga desde filesystem en modo no embebido
- [ ] integración con Stitch o import/export futuro

---

## 14. Riesgos y decisiones abiertas

- **CDN vs build**: con DaisyUI por CDN, inyectar CSS por `<link>` es la vía
  simple. Si en el futuro se compila Tailwind, el `DESIGN.md` sigue siendo la
  fuente y solo cambia el backend de compilación.
- **FOUC**: el `<link>` del theme va en `<head>` antes del contenido para evitar
  parpadeo.
- **Validación**: un tema inválido no debe tumbar el server.
- **Tipografías**: sin política de fuentes soportadas, la alineación visual no es
  completa.
- **Spacing**: no todo el layout será plenamente themeable en la primera fase.
- **Extensiones locales**: el proyecto debe mantener `x-pi` pequeño y
  justificable, para no alejarse innecesariamente del formato de Google.

---

## 15. Resumen de una línea

Un conjunto de `DESIGN.md` alineados al formato de Google, con extensiones
mínimas `x-pi` cuando haga falta, se parsea a un modelo interno, se resuelve a
un theme runtime de DaisyUI v5 y se inyecta al sitio y al visor del sidenav vía
variables CSS; el usuario elige entre varios temas y todo cambia en vivo, sin
desincronización, porque existe una sola fuente de verdad.