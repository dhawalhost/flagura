#!/usr/bin/env bash
set -euo pipefail

if [ -z "${1:-}" ]; then
  echo "Usage: $0 <feature-slug>"
  echo "Example: $0 dynamic-telemetry-filters"
  exit 1
fi

SLUG="$1"
SPECS_DIR="specs"
mkdir -p "$SPECS_DIR"

COUNT=$(find "$SPECS_DIR" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | wc -l | tr -d ' ')
NEXT_NUM=$(printf "%03d" "$((COUNT + 1))")
FEATURE_DIR="${SPECS_DIR}/${NEXT_NUM}-${SLUG}"

mkdir -p "$FEATURE_DIR"
cp .sdd/templates/spec.md "${FEATURE_DIR}/spec.md"
cp .sdd/templates/plan.md "${FEATURE_DIR}/plan.md"
cp .sdd/templates/tasks.md "${FEATURE_DIR}/tasks.md"

echo "Scaffolded feature spec in: ${FEATURE_DIR}"
