---
description: Card de ejemplo para validar el flujo del backlog. No requiere implementación real, sólo dejar la card sembrada en el backlog.
priority: P3
source: user
status: backlog
tags: [ejemplo, healthcheck, observabilidad]
timestamp: "2026-07-18T16:26:47Z"
title: 'Ejemplo: ping endpoint de healthcheck'
type: backlog/card
---

# Ejemplo: ping endpoint de healthcheck

Esta tarjeta es **meramente ilustrativa**: existe para validar que el
flujo completo del backlog (parseo de frontmatter, render de la card
en el board y del detalle en el modal) funciona end-to-end con un
cuerpo Markdown no trivial.

La idea es exponer un endpoint `GET /healthz` que devuelva `200 OK`
con un payload JSON mínimo, y que el load balancer lo use para
decidir cuándo sacar una instancia del pool.

> Nota: el agente nunca debería escribir esta card "en serio". Si
> la ves en producción, es porque alguien está testeando el módulo.

## Contexto

El servicio ya expone un endpoint `/metrics` para Prometheus, pero
los balanceadores k8s necesitan un check **ligero y sin
dependencias** (sin tocar la base, sin leer configs) que responda
en menos de 10 ms. Hoy, si la DB se cuelga, el balanceador sigue
mandando tráfico porque `/metrics` no falla — sólo tarda más.

# Acceptance Criteria

- [ ] `GET /healthz` responde `200 OK` con `Content-Type: application/json`.
- [ ] El body es exactamente `{"status":"ok","uptime":"<segundos>"}`.
- [ ] Latencia p99 < 10 ms en local con `wrk -t4 -c100 -d30s`.
- [ ] El endpoint **no** toca la base de datos ni lee archivos de
  configuración — sólo memoria.
- [ ] Si el proceso está en shutdown (signal `SIGTERM` recibido),
  el endpoint responde `503` con `{"status":"draining"}` para que
  el balanceador lo saque del pool.
- [ ] Hay un test unitario que cubre los dos paths: estado normal
  y estado draining.
- [ ] Documentado en el README de la API con un ejemplo de
  `curl` y la lista de signals manejados.

# Links

- Bloqueada por [Levantar el servicio base](/todo/levantar-servicio-base.md).
- Ver también [Métricas Prometheus](/todo/metricas-prometheus.md) — la
  fuente de la métrica de uptime que se expone acá.
- Relacionada: [Circuit breaker para DB](/in_progress/circuit-breaker-db.md).

# Examples

```bash
# Estado normal
$ curl -i http://localhost:8001/healthz
HTTP/1.1 200 OK
Content-Type: application/json

{"status":"ok","uptime":1234}

# Durante un deploy
$ curl -i http://localhost:8001/healthz
HTTP/1.1 503 Service Unavailable
Content-Type: application/json

{"status":"draining"}
```

# Schema

```json
{
  "$schema": "https://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["status"],
  "properties": {
    "status": {
      "type": "string",
      "enum": ["ok", "draining"]
    },
    "uptime": {
      "type": "string",
      "description": "Segundos desde el arranque del proceso, como string para evitar overflow de int32."
    }
  }
}
```

# Citations

[1]: [Kubernetes liveness probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/)
[2]: [The Twelve-Factor App — Disposability](https://12factor.net/disposability)
