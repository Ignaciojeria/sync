# Backlog — SPEC (perfil sobre OKF v0.1)

Este documento define el **perfil de backlog** construido sobre el
[**Open Knowledge Format v0.1**](https://google.com/okf) (OKF). NO
redefine OKF: lo asume conforme y agrega las convenciones mínimas
para que el módulo Go pueda parsear, renderizar y mutar tarjetas de
backlog en un bundle OKF.

Donde este SPEC contradice OKF, **gana OKF**. Donde OKF es
permisivo, este SPEC ajusta al caso backlog.

---

## 1. Conformidad con OKF v0.1

Este perfil cumple los requisitos de conformidad de OKF v0.1 (§9):

- ✅ Cada tarjeta es un `.md` con frontmatter YAML parseable.
- ✅ Cada tarjeta tiene `type` no vacío.
- ✅ `index.md` y `log.md` (cuando existen) respetan §6 y §7 de OKF.
- ✅ El módulo Go **preserva keys desconocidas** al reescribir
  frontmatter (§4.1 de OKF: "Consumers SHOULD preserve unknown keys
  when round-tripping").
- ✅ El módulo Go **tolera types desconocidos** sin fallar (§4.1 de
  OKF: "consumers MUST tolerate unknown types gracefully").

### 1.1 Declaración de versión

El bundle puede (opcional) declarar la versión OKF que targetea
incluyendo `okf_version: "0.1"` en el frontmatter del `index.md` de la
raíz del bundle. Este es el **único** `index.md` al que OKF v0.1
permite tener frontmatter (§11 de OKF).

Ejemplo `backlog/index.md`:

```markdown
---
okf_version: "0.1"
---

# Backlog

Bundle OKF que representa el backlog del proyecto …
```

---

## 2. Identidad de una tarjeta

OKF v0.1 §2 define: "**Concept ID — The path of the concept's file
within the bundle, with the `.md` suffix removed.**"

Por lo tanto, **no usamos un campo `id` en el frontmatter**. La
identidad nativa y estable de una tarjeta es su path dentro del
bundle.

Ejemplos:

| Archivo                              | Concept ID                |
|--------------------------------------|---------------------------|
| `backlog/todo/refactorizar.md`       | `backlog/todo/refactorizar` |
| `todo/in_progress/agregar-tests.md`  | `todo/in_progress/agregar-tests` |

### 2.1 ID interno del módulo

Para selectores HTML y HTMX OOB swaps, el módulo deriva un **id
interno** a partir del slug (último segmento del path):

```
backlog-card-<slug>
```

donde `<slug>` es el nombre del archivo sin `.md`. Si dos tarjetas en
el bundle tuvieran el mismo slug, el módulo suffija (`-2`, `-3`, …)
al detectarlo en lectura; la convención de naming del SPEC (§6) ya
previene colisiones, esto es solo defensivo.

Este id interno **no** se persiste en el archivo: se recalcula en
cada lectura.

---

## 3. Layout en disco

```
<BACKLOG_DIR>/                          ← bundle OKF raíz
├── index.md                            ← opcional, único index con frontmatter
├── backlog/                            ← columna 1
│   ├── index.md                        ← opcional, sin frontmatter (§6 OKF)
│   ├── log.md                          ← opcional, append-only (§7 OKF)
│   └── *.md                            ← tarjetas (type=backlog/card)
├── todo/                               ← columna 2
├── in_progress/                        ← columna 3
└── done/                               ← columna 4
```

Reglas:

- `<BACKLOG_DIR>` lo define la variable de entorno `BACKLOG_DIR`
  (default: `<cwd>/backlog`). El módulo Go lo crea si no existe,
  junto con los cuatro subdirectorios.
- El orden de columnas es fijo:
  `backlog → todo → in_progress → done`. Agregar una columna es un
  cambio incompatible del perfil.
- Cada subdirectorio contiene:
  - Archivos `.md` con `type: backlog/card`.
  - Opcionalmente `index.md` (sin frontmatter, salvo en la raíz).
  - Opcionalmente `log.md` (append-only).
  - Opcionalmente `.keep` (placeholder, ignorado).
- Archivos OKF con `type` distinto a `backlog/card` se **preservan**
  en disco y se **ignoran** del listado de tarjetas. Son válidos OKF
  y conviven con el bundle.

---

## 4. Estructura de una tarjeta

```markdown
---
type: backlog/card
title: Refactorizar el parser de frontmatter
description: Migrar el parser regex a un parser YAML real para soportar
  caracteres unicode en valores de frontmatter.
status: in_progress
priority: P1
timestamp: 2025-01-15T10:00:00Z
source: agent
agent_session: sess-abc123
tags: [ui, refactor]
---

# Refactorizar el parser de frontmatter

Cuerpo libre en Markdown. Se recomienda usar las secciones
convencionales de OKF v0.1 §4.2 (`# Schema`, `# Examples`,
`# Citations`) y del perfil backlog (`# Acceptance Criteria`,
`# Links`).

Ver también [agregar tests de frontmatter](/todo/agregar-tests.md).
```

### 4.1 Frontmatter

YAML delimitado por `---` en la línea 1 y `---` de cierre. Los campos
del perfil backlog son **aditivos** sobre OKF v0.1 §4.1: OKF exige
solo `type`, el resto es convención del perfil.

| Campo            | Tipo                | Requerido | Valores válidos                                       | Default si falta       | Notas |
|------------------|---------------------|-----------|-------------------------------------------------------|------------------------|-------|
| `type`           | string              | sí (OKF)  | `backlog/card`                                        | —                      | Discriminador OKF. |
| `title`          | string              | sí (perfil)| 1–140 chars, sin saltos de línea                     | —                      | Resumen humano. |
| `status`         | string              | sí (perfil)| `backlog` \| `todo` \| `in_progress` \| `done`        | —                      | Debe matchear el directorio. |
| `priority`       | string              | sí (perfil)| `P0` \| `P1` \| `P2` \| `P3`                          | `P3`                   | `P0` = más urgente. |
| `description`    | string              | opcional  | una línea, sin saltos                                | derivado del filename  | Recomendado por OKF §4.1. |
| `timestamp`      | string (RFC3339)    | opcional  | timestamp UTC                                         | derivado del mtime     | "Last meaningful change" (§4.1 OKF). Reescrito en cada mutación. |
| `source`         | string              | opcional  | `user` \| `agent`                                     | `user`                 | Quién originó la tarjeta. |
| `agent_session`  | string              | opcional  | id de sesión del agente                               | —                      | Solo si `source == "agent"`. |
| `tags`           | []string            | opcional  | lowercase, kebab-case, únicos                         | `[]`                   | Cross-cutting (§4.1 OKF). |

Notas de alineación con OKF v0.1:

- Usamos `timestamp` (no `updated`/`created`): es el campo canónico
  de OKF para "last meaningful change". OKF no tiene `created`
  separado; si el agente necesita preservar fecha de creación, puede
  agregarlo como key custom (`created`, `created_at`, etc.) — el
  módulo Go lo preserva sin rechazarlo.
- **No usamos un campo `id` propio**: la identidad es el Concept ID
  (= path) de OKF.
- **Cualquier key adicional** que el agente o un humano agregue es
  preservada por el módulo Go al reescribir el frontmatter
  (§4.1 OKF).

### 4.2 Cuerpo

Texto Markdown libre entre el `---` de cierre y el final del archivo.

Reglas:

- Si el cuerpo está vacío, la tarjeta renderiza sin bloque de
  descripción.
- El módulo Go **preserva el cuerpo byte por byte** al reescribir
  frontmatter (clave para `git diff` limpio).
- Se **recomienda** adoptar las secciones convencionales de OKF v0.1
  §4.2 cuando apliquen:
  - `# Schema` — para activos estructurados (no típico en backlog).
  - `# Examples` — uso concreto.
  - `# Citations` — fuentes externas.
- Secciones convencionales del **perfil backlog** (opcional pero
  sugerido):
  - `# Acceptance Criteria` — bullets con `- [ ]`.
  - `# Links` — links a otras tarjetas (cuando no se usan inline).

---

## 5. Cross-linking

Las tarjetas pueden linkear entre sí usando links Markdown estándar,
según OKF v0.1 §5.

### 5.1 Forma recomendada: absolute bundle-relative

Empieza con `/`, interpretado relativo a la raíz del bundle
(`<BACKLOG_DIR>`).

```markdown
Ver [agregar tests de frontmatter](/todo/agregar-tests.md).
```

**Es la forma estable cuando los archivos se mueven dentro de su
columna** (§5.1 OKF).

### 5.2 Forma alternativa: relative

Paths relativos estándar.

```markdown
Ver [otra tarjeta](./otra-tarjeta.md).
```

### 5.3 Links frágiles

OKF v0.1 §5.3: "Consumers MUST tolerate broken links". Cuando una
tarjeta se mueve entre columnas, los links `[/columna/slug.md]` que
apuntaban a ella quedan rotos. El módulo Go los preserva en el body
tal cual (no los reescribe) y la UI los renderiza con estilo
indicador de link roto.

### 5.4 Lo que NO soportamos

- **No usamos `[[id]]` estilo Obsidian**: OKF v0.1 no lo define.
  Cualquier link debe ser Markdown estándar.
- **No mantenemos un índice inverso**: si una tarjeta se mueve, los
  links que la apuntaban quedan rotos. Es OK según §5.3.

---

## 6. Naming

El nombre del archivo se deriva del `title`:

1. Lowercase.
2. Reemplazar cualquier secuencia de caracteres no `[a-z0-9]` por `-`.
3. Colapsar runs de `-` a uno solo.
4. Trim de `-` al inicio y final.
5. Truncar a 60 caracteres.
6. Append `.md`.

En colisión dentro del mismo directorio, suffijear `-2`, `-3`, …
hasta encontrar nombre libre. El slug final lo aplica el módulo Go al
escribir; el agente puede proponer el `title` libremente.

---

## 7. Validaciones

Una tarjeta se considera **válida** si cumple:

- [ ] Empieza con `---\n` en la línea 1.
- [ ] Frontmatter YAML parseable.
- [ ] `type == "backlog/card"`.
- [ ] `title`, `status`, `priority` presentes y válidos.
- [ ] `status` matchea el directorio donde vive el archivo.
- [ ] `priority` ∈ {`P0`,`P1`,`P2`,`P3`}.
- [ ] Si `source == "agent"`, `agent_session` presente.
- [ ] Si `timestamp` está presente, es RFC3339 UTC.
- [ ] El nombre del archivo es coherente con el slug del `title`.

Archivos inválidos:

- Se loguean: `log.Printf("backlog: invalid card at %s: %s", path, motivo)`.
- Se excluyen del listado que ve la UI.
- **No se eliminan ni se mueven**: el agente o un humano debe
  corregirlos.

Archivos OKF con `type` distinto a `backlog/card`:

- Se preservan en disco pero se ignoran del listado de tarjetas.
- Son válidos OKF.

---

## 8. Concurrencia

- Las acciones de UI (mover, ↑/↓, borrar, crear vía form manual)
  pasan por endpoints del servidor que toman un **flock por columna**
  durante la operación. Esto serializa cambios dentro de una columna
  y permite paralelismo entre columnas.
- El agente, al editar con sus file tools, debe respetar el mismo
  flock. La forma más simple: el system prompt del agente le indica
  invocar un wrapper `backlog-edit <archivo>` provisto por el host.
  Si edita a mano, acepta que puede haber race con la UI (raro en la
  práctica con un solo agente y una sola UI).
- El módulo Go invalida su cache en memoria cuando detecta un cambio
  de mtime en disco (siguiente request relee).

---

## 9. Acciones canónicas

Cada acción es **una operación atómica** sobre el FS:

| Acción          | Efecto en disco                                                                 |
|-----------------|---------------------------------------------------------------------------------|
| `Create`        | Escribe `<dir>/<slug>.md` con frontmatter completo + body. Si existe, suffixa. |
| `Move(dir)`     | `mv` del archivo al directorio destino. Actualiza `status` y `timestamp`.        |
| `Update`        | Reescribe solo frontmatter (campos provistos) y deja el cuerpo intacto.        |
| `SetPriority`   | Como `Update` pero solo cambia `priority` y `timestamp`.                       |
| `Delete`        | `rm` del archivo.                                                               |
| `AppendLog`     | Append de una entrada a `<dir>/log.md` (si existe).                           |

Reglas críticas de `Update`:

- **Preservar keys desconocidas** del frontmatter (§4.1 OKF).
- **Preservar el cuerpo byte por byte**.
- **Reescribir `timestamp`** al valor actual.

`Move` debe rechazar el move si origen == destino (no-op silencioso
válido con log debug).

---

## 10. Orden de visualización

Dentro de una columna, el orden de las tarjetas es:

1. `priority` ascendente (`P0` primero).
2. `timestamp` ascendente (más vieja primero — interpretando
   `timestamp` como creation proxy cuando no hay `created` separado).
3. Concept ID ascendente (desempate estable).

Se aplica en lectura; los archivos en disco no necesitan estar
ordenados físicamente.

---

## 11. `index.md` por columna (convención OKF §6, opcional)

`index.md` **NO tiene frontmatter** salvo en la raíz del bundle (§11
OKF). Estructura (§6 OKF):

```markdown
# Backlog

N tarjetas. Última actualización: 2025-01-15T10:00:00Z.

## Tarjetas

* [Refactorizar el parser](refactorizar-el-parser.md) — Migrar el parser regex a YAML.
* [Agregar tests](agregar-tests.md) — Cubrir los casos del SPEC §7.
```

- Si `index.md` está presente, el módulo Go lo lee para mostrar un
  resumen de la columna en la UI.
- Si está ausente, la UI muestra solo el listado de tarjetas.
- El módulo Go puede generar/regenerar `index.md` automáticamente
  desde el listado (futuro).

---

## 12. `log.md` por columna (convención OKF §7, opcional)

Histórico cronológico append-only. Estructura (§7 OKF): "a flat list
of date-grouped entries, newest first".

```markdown
# Directory Update Log

## 2025-01-15
* **Creation**: [Refactorizar el parser](refactorizar-el-parser.md) (agent/sess-abc)
* **Move**: [Refactorizar el parser](refactorizar-el-parser.md) backlog → todo
* **Priority**: [Refactorizar el parser](refactorizar-el-parser.md) P2 → P1
```

- El módulo Go appendea una entrada por cada acción create/move/
  priority/delete.
- Es OKF-conforme: vive como un archivo más en el bundle.
- **Implementación diferida**: el SPEC lo define pero el módulo v1
  puede no escribirlo todavía.

---

## 13. Ejemplo completo

`backlog/todo/refactorizar-el-parser-de-frontmatter.md`:

```markdown
---
type: backlog/card
title: Refactorizar el parser de frontmatter
description: Migrar el parser regex a YAML para soportar unicode y
  valores multi-línea.
status: todo
priority: P1
timestamp: 2025-01-15T10:00:00Z
source: agent
agent_session: sess-abc123
tags: [ui, refactor]
---

# Refactorizar el parser de frontmatter

El parser actual usa regex. Migrar a un parser YAML real para soportar
frontmatter multilínea y caracteres unicode en los valores.

# Acceptance Criteria

- [ ] Soporta valores con `:` y comillas.
- [ ] Preserva el cuerpo byte por byte.
- [ ] Preserva keys desconocidas al reescribir frontmatter.
- [ ] Tests cubren los casos del SPEC §7.

# Links

- Bloqueada por [actualizar dependencias](/todo/actualizar-deps.md).
- Ver también [agregar tests](/todo/agregar-tests.md).

# Citations

[1] [OKF v0.1 spec](https://google.com/okf)
```

---

## 14. Cambios incompatibles del perfil (futuro)

Cualquiera de los siguientes requiere bumpear a `backlog/v2`:

- Agregar o quitar un valor de `status` o `priority`.
- Cambiar el orden o el set de columnas.
- Cambiar el set de campos requeridos del perfil.

Cambios compatibles (perfil v1 se mantiene):

- Adoptar nuevas convenciones aditivas de OKF v0.x.
- Agregar campos opcionales al frontmatter.
- Implementar `index.md` / `log.md` (hoy opcionales).
- Agregar nuevas secciones convencionales del perfil.

---

## Apéndice A — Diferencias entre el perfil y OKF genérico

| Aspecto               | OKF genérico         | Perfil backlog                                  |
|-----------------------|----------------------|-------------------------------------------------|
| `type`                | libre                | fijo: `backlog/card`                            |
| Identidad             | path (Concept ID)    | path (idéntico, sin campo `id` propio)          |
| `status`              | no existe            | columna kanban                                  |
| `priority`            | no existe            | `P0`–`P3`                                       |
| `timestamp`           | opcional (last change)| semántica de "último cambio" preservada        |
| `source`              | no existe            | extension del perfil: `user` \| `agent`         |
| `agent_session`       | no existe            | extension del perfil: trazabilidad              |
| `index.md` por dir    | sin frontmatter      | igual, salvo root que puede tener `okf_version` |
| `log.md` por dir      | date-grouped, newest first | idéntico                                 |
| Layout                | libre                | 4 subdirectorios fijos (uno por columna)        |
| Cross-links           | bundle-relative OK   | igual; **NO** usamos `[[id]]`                   |

---

## Apéndice B — Referencias a OKF v0.1

- §2 Terminology: Concept, Concept ID, Frontmatter, Body, Link.
- §3 Bundle Structure: layout jerárquico.
- §3.1 Reserved filenames: `index.md`, `log.md`.
- §4.1 Frontmatter: campos canónicos + extensiones permitidas.
- §4.2 Body: secciones convencionales.
- §5 Cross-linking: absolute bundle-relative y relative.
- §6 Index Files: estructura sin frontmatter.
- §7 Log Files: estructura date-grouped.
- §9 Conformance: criterios que cumplimos.
- §11 Versioning: `okf_version` en root index frontmatter.
