#!/usr/bin/env bash
# AIBUS build: produces ./bin/ai and ./bin/aibusd (static-ish, fast startup).
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p bin
go build -trimpath -ldflags "-s -w" -o bin/aibusd ./aibusd
go build -trimpath -ldflags "-s -w" -o bin/ai ./ai
echo "built: $(pwd)/bin/{ai,aibusd}"
