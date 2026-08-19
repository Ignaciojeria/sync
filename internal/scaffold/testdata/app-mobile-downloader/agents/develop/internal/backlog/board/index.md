---
okf_version: "0.1"
---

# Backlog

Este bundle es un directorio de archivos Markdown con frontmatter
YAML conforme a Open Knowledge Format v0.1 (OKF). El **perfil**
aplicado es backlog/v1 (ver internal/backlog/SPEC.md para los
detalles del contrato).

## Estructura

- backlog/, todo/, in_progress/, done/ — columnas del tablero Kanban.
  Cada archivo .md dentro es una tarjeta con type=backlog/card.
- AGENTS.md — system prompt para el agente que opera sobre este
  bundle. Lo lee pi runtime al arrancar en este directorio.
- index.md (este archivo) — punto de entrada OKF para disclosure
  progresiva.

## Cómo agregar una tarjeta

1. Elegí la columna destino (default: backlog/).
2. El nombre del archivo se deriva del title según el algoritmo del
   SPEC §6 (lowercase, non-[a-z0-9] → '-', truncar a 60).
3. El frontmatter mínimo: type, title, status, priority. Recomendado:
   description, timestamp (RFC3339 UTC), source (user|agent).
