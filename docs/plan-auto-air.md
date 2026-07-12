# Plan: soporte de `air` automático en proyectos generados por `einarc`

## Objetivo
Hacer que, al ejecutar `einarc init <slug>`, el proyecto quede listo para **hot-reload de Go con `air`** dentro de la VM, usando los archivos sincronizados por Mutagen.

---

## Resultado esperado (UX)
1. `einarc init myapp`
2. CLI crea proyecto + mutagen + workspace remoto.
3. CLI deja configurado `air` automáticamente.
4. Usuario ejecuta un único comando (o ninguno, según modo) y la app recompila al guardar cambios locales.

---

## Decisiones de diseño recomendadas

### 1) Ejecutar `air` en la VM, no en local
- Mutagen sincroniza archivos a VM.
- `air` observa cambios en la VM (destino sync), evitando diferencias de path/FS entre Windows/Linux.
- Mantiene entorno de ejecución homogéneo.

### 2) Configuración por proyecto (`workspace/<slug>`)
- Mantener root remoto en: `/home/exedev/workspace/<slug>`.
- Evita conflictos con archivos del home del usuario.

### 3) Modo habilitable por flag y/o config
Agregar en `init`:
- `--air` (bool): habilita setup de air.
- `--air-auto-start` (bool): además lo levanta en background.

Variables opcionales en config:
- `airEnabled` (default true/false según decisión)
- `airAutoStart`

---

## Alcance funcional (MVP)

### A. Provisión de `air` en VM
Opciones:
1. **Preferida:** instalar vía `go install github.com/air-verse/air@latest` en la VM.
2. Fallback: descargar binario release.

Validación:
- `ssh exedev@<vm> "air -v || ~/go/bin/air -v"`

### B. Generación de `.air.toml` en el proyecto
Crear `.air.toml` estándar para Go API/CLI:
- `root = "."`
- build cmd: `go build -o ./tmp/main .`
- run bin: `./tmp/main`
- excludes: `.git`, `.einar`, `tmp`, `node_modules`

### C. Comando remoto para iniciar `air`
Opciones:
- foreground (debug simple):
  - `ssh exedev@<vm> "cd /home/exedev/workspace/<slug> && ~/go/bin/air"`
- background con log/pid:
  - `nohup ~/go/bin/air > .air.log 2>&1 & echo $! > .air.pid`

### D. Comandos CLI nuevos (sugerido)
- `einarc dev start`  -> inicia air remoto
- `einarc dev stop`   -> mata PID de `.air.pid`
- `einarc dev logs`   -> tail `.air.log`
- `einarc dev status` -> verifica proceso

---

## Cambios técnicos en el CLI

### 1) `cmd/project.go` (`init`)
Después de:
- mutagen start exitoso

Agregar:
- `setupRemoteAir(cfg, slug)` si `--air`.
- opcional `startRemoteAir(cfg, slug)` si `--air-auto-start`.

### 2) Nueva unidad `cmd/dev.go`
- Subcomandos `dev start|stop|logs|status`.
- Reutilizar destino mutagen normalizado para obtener VM host + path.

### 3) Helper remoto SSH
Funciones utilitarias:
- `remoteExec(target string, args ...string)`
- `ensureAirInstalled(target string)`
- `ensureAirConfig(target, remotePath string)`
- `startAirRemote(...)`, `stopAirRemote(...)`

### 4) Template `.air.toml`
Guardar en código (string template) con placeholders mínimos.
Futuro: plantilla por stack/framework.

---

## Flujo detallado (MVP)
1. `init` termina creación y sync Mutagen.
2. CLI ejecuta preflight SSH (ya existe).
3. CLI verifica/instala `air` en VM.
4. CLI crea `.air.toml` si no existe.
5. Si `--air-auto-start`: inicia `air` remoto en background.
6. CLI imprime checklist:
   - comando para ver logs
   - URL app
   - cómo detener `air`

---

## Manejo de errores
- Si falla instalación de `air`, **no romper init** del proyecto.
- Mostrar warning con comando manual de recuperación.
- Si `.air.toml` existe, no sobrescribir (o flag `--air-force`).
- Si `air` ya está corriendo, no duplicar proceso.

---

## Seguridad y robustez
- Usar usuario `exedev` (no root).
- Quoting estricto al ejecutar comandos SSH.
- Guardar PID/log dentro del proyecto remoto (`.air.pid`, `.air.log`).
- Timeouts en comandos remotos.

---

## Plan por fases

### Fase 1 (rápida)
- `--air` y `--air-auto-start` en `init`.
- instalación remota + `.air.toml` + start opcional.
- sin nuevos subcomandos.

### Fase 2
- `einarc dev start|stop|logs|status`.
- mejor UX operativa diaria.

### Fase 3
- perfiles de `air` por tipo de proyecto.
- integración con healthcheck HTTP y mensajes más inteligentes.

---

## Criterios de aceptación
1. Tras `einarc init demo --air --air-auto-start`, cambios en `cmd/api/main.go` local recompilan en VM automáticamente (con fallback legacy a `main.go` raíz).
2. Existe `.air.toml` en el proyecto.
3. `air` corre con usuario `exedev` en `/home/exedev/workspace/demo`.
4. Logs accesibles y proceso controlable (`start/stop`).

---

## Comandos de verificación manual
```bash
# En local
<mutagen-bin> sync list --long

# En VM
ls -la /home/exedev/workspace/<slug>
cat /home/exedev/workspace/<slug>/.air.toml
ps aux | grep air
tail -f /home/exedev/workspace/<slug>/.air.log
```

---

## Nota final
Este enfoque mantiene la arquitectura actual (Mutagen + VM por proyecto), minimiza fricción de onboarding y deja la base lista para un `einarc dev` más completo.
