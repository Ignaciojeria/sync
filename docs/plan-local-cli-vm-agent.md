# Plan de refactor: `local-cli` y `vm-agent`

## Objetivo

Separar el proyecto actual en dos binarios con responsabilidades distintas:

- `local-cli`: CLI principal que corre en la máquina del usuario
- `vm-agent`: binario remoto que corre dentro de la VM provisionada

Esta separación permitirá que el CLI local compile, suba y configure el agente remoto, y que el agente maneje flujos propios de la VM, incluyendo autenticación remota y recepción de callbacks en el DNS público de la VM.

---

## Motivación

Hoy el proyecto está organizado alrededor de un solo binario y un `cmd/` plano con comandos Cobra.

Eso funciona para un único ejecutable, pero deja de escalar bien cuando aparece un segundo binario real con otro propósito.

En este caso:

- `local-cli` tendrá responsabilidades de UX de consola, provisioning, build, upload y orquestación
- `vm-agent` tendrá responsabilidades de runtime remoto, servidor HTTP, login remoto y callback

Por eso conviene adoptar una estructura multi-binario estándar de Go.

---

## Estructura objetivo

```text
cmd/
  local-cli/
    main.go
  vm-agent/
    main.go

internal/
  localcli/
    root.go
    login.go
    project.go
    dev.go
    db.go
    mutagen.go
    workspace_sync_take.go
    ssh_helpers.go

  vmagent/
    app.go
    config.go
    login.go
    callback.go
    server.go

  build/
    vm_agent.go

  deploy/
    vm.go
```

---

## Responsabilidades por binario

### `local-cli`

Responsable de:

- autenticación local si aplica
- comandos Cobra y UX de CLI
- creación de proyectos
- provisioning de VM
- compilación del `vm-agent`
- subida del `vm-agent` a la VM
- configuración remota del agente
- arranque/reinicio del agente remoto

### `vm-agent`

Responsable de:

- correr dentro de la VM
- exponer endpoints HTTP necesarios para login/callback
- iniciar flujo de autenticación contra `pi` / OpenAI
- recibir el callback en el DNS de la VM
- persistir tokens o estado de sesión de forma segura
- exponer endpoints mínimos de salud o estado si hace falta

---

## Flujo objetivo de alto nivel

1. `local-cli` provisiona la VM
2. `local-cli` compila `vm-agent` para Linux
3. `local-cli` sube el binario a la VM
4. `local-cli` configura el `vm-agent` con el DNS o URL pública de la VM
5. `local-cli` inicia el `vm-agent` en la VM
6. `vm-agent` expone el flujo de login remoto
7. el usuario completa el login de `pi` / OpenAI
8. el callback vuelve al DNS de la VM
9. `vm-agent` guarda la sesión y queda listo para operar

---

## Fase 1: refactor estructural mínimo

Objetivo: separar binarios sin cambiar comportamiento funcional.

### Cambios

1. Crear `cmd/local-cli/main.go`
2. Mover `main.go` actual a `cmd/local-cli/main.go`
3. Mover los archivos actuales de `cmd/` a `internal/localcli/`
4. Cambiar el package de `cmd` a `localcli`
5. Actualizar imports para que el entrypoint use `internal/localcli`
6. Verificar que el binario actual siga compilando

### Resultado esperado

El CLI actual sigue funcionando igual, pero ya queda preparado para coexistir con otros binarios.

---

## Fase 2: crear el esqueleto de `vm-agent`

Objetivo: agregar el segundo binario sin implementar todavía todo el flujo final.

### Cambios

1. Crear `cmd/vm-agent/main.go`
2. Crear `internal/vmagent/`
3. Agregar una función pública tipo `vmagent.Execute()` o `vmagent.Run()`
4. Implementar un arranque mínimo del proceso
5. Validar compilación local del nuevo binario

### Resultado esperado

El repositorio ya compila dos binarios separados:

- `./cmd/local-cli`
- `./cmd/vm-agent`

---

## Fase 3: encapsular build del agente remoto

Objetivo: que `local-cli` pueda construir el binario remoto de forma explícita y mantenible.

### Cambios

1. Crear `internal/build/vm_agent.go`
2. Encapsular ahí la lógica de build con `GOOS=linux`
3. Definir arquitectura objetivo (`amd64` o `arm64` según la VM)
4. Definir path de salida consistente
5. Preparar posibilidad de pasar flags o metadata de build

### Resultado esperado

El build del agente deja de estar mezclado con comandos Cobra o scripts ad hoc.

---

## Fase 4: encapsular deploy del agente remoto

Objetivo: separar la lógica de subida/configuración de la lógica del CLI.

### Cambios

1. Crear `internal/deploy/vm.go`
2. Mover o centralizar ahí la lógica de upload a la VM
3. Agregar instalación del binario en una ruta conocida
4. Agregar configuración inicial del agente
5. Agregar arranque remoto del proceso

### Resultado esperado

`local-cli` orquesta el deploy, pero la lógica técnica queda aislada en un paquete reutilizable.

---

## Fase 5: flujo de autenticación remota del `vm-agent`

Objetivo: permitir autenticación remota desde la VM con callback apuntando al DNS público de la VM.

### Cambios

1. Implementar servidor HTTP mínimo en `internal/vmagent/server.go`
2. Implementar endpoint `/login`
3. Implementar endpoint `/callback`
4. Construir la callback URL usando el DNS público de la VM
5. Persistir tokens/sesión localmente en la VM
6. Agregar protección por `state`
7. Evitar loguear secretos o tokens

### Consideraciones

- preferir HTTPS si la VM queda expuesta públicamente
- proteger permisos del archivo de tokens
- no dejar endpoints sensibles más abiertos de lo necesario
- si `pi` no soporta bien callback remoto, evaluar un flujo alternativo como device flow

---

## Fase 6: integración del `local-cli` con `vm-agent`

Objetivo: que el CLI local pueda operar el ciclo completo.

### Posibles comandos

- `local-cli vm build-agent`
- `local-cli vm upload-agent`
- `local-cli vm start-agent`
- `local-cli vm auth-url`
- `local-cli vm status`

### Resultado esperado

El usuario puede provisionar, desplegar e iniciar el flujo remoto desde un único CLI local.

---

## Cambios de código iniciales sugeridos

### Nuevo entrypoint local

`cmd/local-cli/main.go`

```go
package main

import "einarc/internal/localcli"

func main() {
    localcli.Execute()
}
```

### Nuevo entrypoint remoto

`cmd/vm-agent/main.go`

```go
package main

import "einarc/internal/vmagent"

func main() {
    vmagent.Execute()
}
```

---

## Estrategia de migración segura

1. hacer primero el refactor de carpetas sin cambiar lógica
2. validar que `local-cli` siga compilando y funcionando
3. recién después agregar `vm-agent`
4. luego encapsular build y deploy
5. finalmente implementar el flujo de login remoto

Esto reduce riesgo y permite avanzar por etapas verificables.

---

## Riesgos

### 1. Acoplamiento actual del CLI
Parte de la lógica actual puede estar demasiado pegada a Cobra o a paths actuales.

Mitigación:
- mover primero sin reescribir demasiado
- extraer paquetes compartidos después

### 2. Login remoto dependiente de capacidades reales de `pi`
Si `pi` no soporta callback configurable o flujo headless, habrá que adaptar el diseño.

Mitigación:
- validar temprano cómo funciona el login de `pi`
- confirmar si soporta callback remoto, device flow o tokens transferibles

### 3. Exposición pública del callback
Recibir callbacks en la VM introduce requisitos de seguridad y conectividad.

Mitigación:
- usar `state`
- preferir TLS
- restringir logs y permisos

---

## Criterios de éxito

- el proyecto compila dos binarios separados
- `local-cli` mantiene la funcionalidad actual
- `vm-agent` puede compilar para Linux
- `local-cli` puede construir y subir `vm-agent`
- `vm-agent` puede iniciar un flujo de login remoto
- el callback puede volver al DNS público de la VM

---

## Decisión tomada

Se adopta la estructura:

```text
cmd/
  local-cli/
  vm-agent/
```

con implementación en:

```text
internal/
  localcli/
  vmagent/
```

porque refleja correctamente la separación entre el CLI local y el agente remoto que corre dentro de la VM.
