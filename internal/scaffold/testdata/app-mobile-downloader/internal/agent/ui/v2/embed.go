package v2

import "embed"

// AssetsFS embebe todos los archivos estáticos del módulo JS V2.
// El path es relativo a este archivo Go, así que los assets viven
// en static/agent-chat/* desde el punto de vista de Go y se sirven
// en /agent/static/<archivo> desde el navegador.
//
// Por ahora embebemos sólo main.js y sus imports directos. Si más
// adelante agregamos chunks con code-splitting, hay que sumarlos al
// FS (o usar un patrón que matchee por extensión).
//
//go:embed static/agent-chat
var AssetsFS embed.FS
