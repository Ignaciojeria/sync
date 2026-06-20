#!/bin/bash
# scripts/generate-structure.sh
# Genera STRUCTURE.md con la estructura real del proyecto.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT="${ROOT_DIR}/STRUCTURE.md"

EXCLUDE_DIRS=(
  '.git'
  '.einar'
  'tmp'
  'node_modules'
  '.npm-cache'
  '.agents'
  'doc'
  '.githooks'
)

EXCLUDE_FILES=(
  '*.jpg'
  '*.jpeg'
  '*.png'
  '*.ico'
  '*.log'
  '*.lock'
  '*.csv'
  '.env'
  'go.sum'
  'mutagen.yml'
  'mutagen.yml.lock'
  'skills-lock.json'
  'workspaces.yaml'
  'wede.config.json'
  '*.json'
  'coverage.out'
  'coverage_filtered.out'
  'coverage.html'
  'last_run.json'
)

FIND_ARGS=()

for dir in "${EXCLUDE_DIRS[@]}"; do
  FIND_ARGS+=("-not" "-path" "*/${dir}/*")
done

for pattern in "${EXCLUDE_FILES[@]}"; do
  FIND_ARGS+=("-not" "-name" "${pattern}")
done

{
  echo "# Estructura del Proyecto"
  echo ""
  echo "> **Archivo generado automáticamente.** Ejecutar \`scripts/generate-structure.sh\` para regenerar."
  echo "> **No editar manualmente.**"
  echo ""
  echo "\`\`\`"

  (cd "$ROOT_DIR" && find . \
    -not -path '*/\.*' \
    "${FIND_ARGS[@]}" \
    -type f \
    \( -name "*.go" -o -name "*.templ" -o -name "*.md" -o -name "*.sh" -o -name "*.toml" -o -name "*.yml" -o -name "*.yaml" -o -name "*.mod" -o -name "*.sql" -o -name "*.sum" \) \
    | sort \
    | sed 's|^\./||' \
    | awk -F'/' '
    {
      depth = NF - 1
      indent = ""
      for (i = 0; i < depth; i++) indent = indent "  "
      if (NF == 1) {
        print $NF
      } else {
        print indent $NF
      }
    }
  ')

  echo "\`\`\`"
  echo ""
  echo "---"
  echo ""
  echo "## Convenciones de estructura"
  echo ""
  echo "- Cada módulo de negocio vive en \`internal/<modulo>/\` con sus capas: \`application\`, \`http\`, \`infrastructure\`, \`ui\`."
  echo "- Código compartido: \`internal/shared/\` (config, auth, server, infra)."
  echo "- Punto de entrada: \`cmd/api/main.go\`."
  echo "- Plantillas: \`internal/<modulo>/ui/\` o \`internal/ui/layout/\`."
  echo "- Tests: junto al código (\`*_test.go\`)."
  echo "- Skills: \`.agents/skills/\`."
} > "$OUTPUT"

echo "Estructura generada en: ${OUTPUT}"
