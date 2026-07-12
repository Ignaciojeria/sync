# Colaboración de workspaces (`sync` / `take`)

Este flujo está pensado para trabajo colaborativo entre múltiples entornos (por ejemplo: local + Codespaces) sobre el **mismo proyecto** sin perder estabilidad con Mutagen.

## Comandos

## `einarc sync`
Sincroniza el working tree del workspace actual **sin cambiar ownership**.

- Valida `.einar/config.json`
- Si no existe `.einar/config.json`, intenta rehidratarla desde backend con `GET /api/projects/by-slug/:slug`
  - prioridad de slug: `--slug` -> `workspaces.yaml` (`project.slug`) -> nombre de carpeta actual
  - requiere sesión/login (token válido)
- Valida `workspaceBranch` (lock de rama)
- Valida ownership (debes ser el owner actual)
- Ejecuta sincronización con Mutagen (`project start` + `sync flush`)

Si otro entorno tiene el ownership, el comando falla con warning para evitar cambios concurrentes peligrosos.

## `einarc take`
Toma control del workspace actual de forma segura.

Flujo:
1. Si falta config local, la rehidrata desde backend por slug.
2. Sincroniza cambios pendientes del working tree con Mutagen.
3. Transfiere ownership al entorno actual (`--force` implícito en take).

Úsalo cuando cambias de entorno y quieres continuar trabajando tú.

## Requisitos

- Login/token válido (`einarc login` o `EINAR_TOKEN`).
- Debe existir `mutagenSessionName` y `mutagenDestination` en config (se rehidratan si falta config local).
- La rama actual debe coincidir con `workspaceBranch`.

> Nota: en un `git clone` nuevo, `.einar/` no se versiona por diseño. `sync` y `take` pueden bootstrapear automáticamente la config con el endpoint `by-slug`.

## Buenas prácticas

- `1 workspace = 1 rama fija`
- `1 workspace = 1 owner activo`
- Usar `take` al cambiar de entorno colaborativo
- Usar `sync` para mantenerte al día cuando ya eres owner
