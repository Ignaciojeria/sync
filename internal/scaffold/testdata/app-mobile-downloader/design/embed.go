package designdata

import "embed"

// FS expone los documentos DESIGN.md embebidos del proyecto.
//
//go:embed */DESIGN.md _schema.md
var FS embed.FS
