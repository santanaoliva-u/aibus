#!/usr/bin/env bash
set -u
AI="${AIBUS_AI_BIN:-$(cd "$(dirname "$0")" && pwd)/bin/ai}"
TMP="$(mktemp -d)"
export AIBUS_DIR="$TMP"
SEP="$(printf '\037')"
TAB="$(printf '\t')"
PASS=0
FAIL=0

cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

ok()  { echo "[$1/6] PASS $2"; PASS=$((PASS + 1)); }
bad() { echo "[$1/6] FAIL $2"; FAIL=$((FAIL + 1)); }

GL="$(AI_NAME=A "$AI" say 'borrar archivo protegido')"

# 1) A publica en el ring de B y recibe seq=1
R="$(AI_NAME=A "$AI" fire B "A${SEP}1${SEP}${GL}")"
if [ "$R" = "1" ]; then ok 1 "fire->ring B devuelve seq=1"; else bad 1 "seq='$R' (esperado 1)"; fi

# 2) B recibe los glifos en la primera linea
OUT="$(AI_NAME=B "$AI" recv B 0 --all)"
if printf '%s' "$OUT" | grep -qF "$GL" && printf '%s' "$OUT" | grep -q "^1${TAB}"; then
  ok 2 "B recibe glifos en seq 1"
else
  bad 2 "B no recibio: ${OUT}"
fi

# 3) El wire se parte en from / hops / body
FIRST="$(printf '%s\n' "$OUT" | head -n1)"
PAYLOAD="${FIRST#*${TAB}}"
F0="${PAYLOAD%%${SEP}*}"
REST="${PAYLOAD#*${SEP}}"
F1="${REST%%${SEP}*}"
F2="${REST#*${SEP}}"
if [ "$F0" = "A" ] && [ "$F1" = "1" ] && [ -n "$F2" ]; then
  ok 3 "wire: from=$F0 hops=$F1 body='$F2'"
else
  bad 3 "wire malo: '$F0'|'$F1'|'$F2'"
fi

# 4) El cursor avanza: el segundo recv (sin --all) NO trae seq 1
AI_NAME=A "$AI" fire B "A${SEP}1${SEP}segundo" >/dev/null
OUT2="$(AI_NAME=B "$AI" recv B 0)"
if printf '%s' "$OUT2" | grep -q "^2${TAB}" && ! printf '%s' "$OUT2" | grep -q "^1${TAB}"; then
  ok 4 "cursor avanza (solo seq 2)"
else
  bad 4 "cursor: ${OUT2}"
fi

# 5) head del anillo crece
H="$(AI_NAME=B "$AI" ringstat B | awk '/head/{print $2}')"
if [ "${H:-0}" -ge 2 ]; then ok 5 "head=$H"; else bad 5 "head='${H:-?}'"; fi

# 6) dos emisores distintos llegan al buzon de B
AI_NAME=A "$AI" fire B "A${SEP}1${SEP}tercero" >/dev/null
AI_NAME=C "$AI" fire B "C${SEP}1${SEP}cuarto" >/dev/null
OUT3="$(AI_NAME=B "$AI" recv B 0)"
if printf '%s' "$OUT3" | grep -q "^3${TAB}" && printf '%s' "$OUT3" | grep -q "^4${TAB}"; then
  ok 6 "dos emisores (A y C) en seq 3 y 4"
else
  bad 6 "emisores: ${OUT3}"
fi

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
