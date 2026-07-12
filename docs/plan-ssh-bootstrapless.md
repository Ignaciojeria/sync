# Plan de implementación y pruebas: SSH sin `sh exe.dev`

## Objetivo
Permitir que `einarc init/sync/take` funcione en una máquina nueva sin bootstrap externo (`sh exe.dev`), manteniendo conectividad SSH a VMs de forma confiable.

---

## Alcance
- Eliminar dependencia implícita de configuración SSH del sistema.
- Forzar identidad SSH explícita en operaciones internas del CLI.
- Mejorar diagnóstico y mensajes de error.
- Validar en entornos limpios (incluyendo Codespaces).

No incluye:
- Cambios de infraestructura backend.
- Reemplazar Mutagen.

---

## Estado actual (resumen técnico)
- El CLI ya guarda una key privada en `~/.einar/id_ed25519`.
- Varias llamadas a `ssh` no pasan `-i` explícito, por lo que dependen de:
  - `~/.ssh/config`
  - ssh-agent
  - key por defecto del sistema
- En máquinas nuevas eso puede forzar pasos manuales/externos (`sh exe.dev`).

---

## Estrategia de implementación

### 1) Resolver identidad SSH de forma centralizada
Crear helper:
- `resolveSSHIdentityFile() (string, error)`

Orden de resolución:
1. `EINAR_SSH_IDENTITY_FILE` (si existe)
2. fallback a `~/.einar/id_ed25519`

Validaciones:
- archivo existe
- no vacío
- permisos seguros (Unix `0600`; Windows ACL razonable)

---

### 2) Estandarizar argumentos SSH en el CLI
Agregar constructor reusable de args SSH, por ejemplo:
- `buildSSHArgs(target string, extra ...string) []string`

Flags base sugeridas:
- `-i <identityFile>`
- `-o IdentitiesOnly=yes`
- `-o ConnectTimeout=10`
- `-o BatchMode=yes` (en preflight/comandos no interactivos)

En primera confianza de host (trust inicial):
- `-o StrictHostKeyChecking=accept-new`

---

### 3) Aplicar el builder en todos los puntos SSH
Actualizar llamadas directas a `exec.Command("ssh", ...)` para que usen el builder.

Áreas mínimas a tocar:
- `cmd/ssh_helpers.go`
- `cmd/project.go` (`ensureSSHTrustInteractive`, `preflightSSHConnection`, helpers remotos)
- cualquier otro comando con SSH directo (`cmd/dev.go`, etc.)

---

### 4) Mejorar UX de errores SSH
Al fallar, imprimir:
- target SSH
- identity file usado
- comando manual sugerido para validar:
  - `ssh -i <key> -o IdentitiesOnly=yes <target> "echo ok"`

---

### 5) Comando de diagnóstico
Agregar:
- `einarc doctor ssh`

Chequeos:
1. binario `ssh` disponible
2. key resuelta y legible
3. permisos básicos de key
4. conexión de prueba al target (`echo ok`)
5. recomendación accionable por falla

---

### 6) Rollout controlado
Feature flag (temporal):
- `EINAR_SSH_EXPLICIT_IDENTITY=true` (default recomendado: `true`)

Permite rollback rápido ante edge cases.

---

## Plan de pruebas

## A. Unit tests
1. **resolveSSHIdentityFile**
   - usa env var cuando está definida
   - usa fallback cuando no existe env
   - error cuando ruta no existe

2. **buildSSHArgs**
   - incluye `-i` y `IdentitiesOnly=yes`
   - respeta extras
   - no duplica flags críticos

3. **parseo destino SSH**
   - `ssh://user@host/path`
   - `user@host:/path`

---

## B. Integration tests (mock de ejecución)
> Sugerencia: encapsular ejecución de comandos para inspeccionar argumentos invocados.

1. `runSSHScriptWithTimeout` invoca SSH con `-i <key>`
2. `preflightSSHConnection` usa identity explícita
3. trust inicial aplica `StrictHostKeyChecking=accept-new` solo en ese flujo

---

## C. E2E manual

### Entornos
- Windows
- Linux/macOS
- Codespaces (entorno limpio recomendado)

### Escenarios
1. **Máquina limpia, sin `sh exe.dev`**
   - `einarc login`
   - `einarc init <project>`
   - `einarc sync`
   - esperado: éxito end-to-end

2. **`known_hosts` vacío**
   - esperado: trust inicial exitoso y persistente

3. **Key inválida/corrupta**
   - esperado: error claro + acción recomendada

4. **ssh-agent con otras keys cargadas**
   - esperado: no interfiere por `-i` + `IdentitiesOnly=yes`

5. **Proyecto legacy**
   - esperado: sin regresiones en sync/take

---

## Matriz mínima
- OS: Win11 / Ubuntu / macOS
- Estado SSH: limpio / con `~/.ssh/config` existente
- Host key: no registrada / registrada
- Key: correcta / inexistente / inválida

---

## Criterios de aceptación
1. Usuario en máquina nueva ejecuta `login/init/sync` sin `sh exe.dev`.
2. Todas las operaciones SSH del CLI usan identity explícita.
3. Errores SSH entregan pasos accionables.
4. No hay regresiones en proyectos existentes.

---

## Plan de entrega
1. Implementación técnica + tests unitarios
2. Pruebas en Codespaces y 1 Windows real
3. Canary interno
4. Release general + nota de migración breve

---

## Checklist ejecutable
- [ ] Implementar `resolveSSHIdentityFile()`
- [ ] Implementar `buildSSHArgs()` reusable
- [ ] Refactor de llamadas SSH en `cmd/ssh_helpers.go`
- [ ] Refactor de llamadas SSH en `cmd/project.go`
- [ ] Refactor de llamadas SSH restantes (`cmd/dev.go`, etc.)
- [ ] Mejorar errores y mensajes de diagnóstico
- [ ] Crear `einarc doctor ssh`
- [ ] Agregar unit tests
- [ ] Agregar integration tests (mock runner)
- [ ] Ejecutar E2E en Codespaces
- [ ] Ejecutar E2E en Windows
- [ ] Documentar en README/docs
