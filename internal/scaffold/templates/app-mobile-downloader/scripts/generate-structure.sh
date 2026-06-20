#!/bin/bash
# scripts/generate-structure.sh
# Regenera STRUCTURE.md con el árbol completo del proyecto (carpetas + archivos).
#
# La lógica vive en scripts/_tree_generator.py y se configura con
# scripts/structure.config.toml; este script solo la invoca.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
OUTPUT="${ROOT_DIR}/STRUCTURE.md"
GENERATOR="${SCRIPT_DIR}/_tree_generator.py"
CONFIG="${SCRIPT_DIR}/structure.config.toml"

if [[ ! -f "${GENERATOR}" ]]; then
  echo "ERROR: no se encontró ${GENERATOR}" >&2
  exit 1
fi

python3 "${GENERATOR}" "${ROOT_DIR}" "${OUTPUT}" "${CONFIG}"

echo "Estructura generada en: ${OUTPUT}"
