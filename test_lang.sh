#!/usr/bin/env bash
# AIBUS machine-language test: glyph table integrity + encode/decode roundtrip.
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$PWD/bin:$PATH"
fail() { echo "FAIL: $*" >&2; exit 1; }

echo "[1/4] lang: sin glifos ni codigos duplicados"
bin/ai lang > /tmp/aibus_lang.txt
DUPG=$(awk -F'\t' '{print $2}' /tmp/aibus_lang.txt | sort | uniq -d)
[ -z "$DUPG" ] || fail "glifos duplicados: $DUPG"
DUPC=$(awk -F'\t' '{print $1}' /tmp/aibus_lang.txt | sort | uniq -d)
[ -z "$DUPC" ] || fail "codigos duplicados: $DUPC"

echo "[2/4] say: frase -> glifos"
S=$(bin/ai say "borrar archivo protegido")
[ "$S" = "⌦ 🗀 ⚿" ] || fail "say dio: $S"

echo "[3/4] hear: glifos -> frase"
H=$(bin/ai hear "$S")
echo "$H" | grep -q "borrar" || fail "hear dio: $H"

echo "[4/4] compresion: 3 conceptos -> 3 tokens"
W=$(bin/ai say "detener-otro urgente no-borrar")
[ "$(echo "$W" | wc -w)" -le 3 ] || fail "no comprimio: $W"

echo "PASS: lang en verde ($(awk 'END{print NR}' /tmp/aibus_lang.txt) glifos)"
