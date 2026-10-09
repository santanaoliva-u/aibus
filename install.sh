#!/usr/bin/env bash
# AIBUS installer — build and drop the binaries in ~/.aibus/bin.
set -euo pipefail
cd "$(dirname "$0")"

command -v go >/dev/null 2>&1 || { echo "se requiere Go 1.26+"; exit 1; }

echo "[1/2] compilando..."
./build.sh

DEST="${AIBUS_DIR:-$HOME/.aibus}/bin"
mkdir -p "$DEST"
cp bin/ai bin/aibusd "$DEST/"
echo "[2/2] instalado en $DEST"
echo
echo "  export PATH=\"$DEST:\$PATH\""
echo "  ai name alpha"
