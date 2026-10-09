#!/usr/bin/env bash
# AIBUS deterministic test: daemon + send/wait/push/history/topics. Exit 0 = green.
set -euo pipefail
cd "$(dirname "$0")"
export AIBUS_DIR="$(mktemp -d)"
export PATH="$PWD/bin:$PATH"
fail() { echo "FAIL: $*" >&2; exit 1; }

echo "[1/5] build"; ./build.sh >/dev/null

echo "[2/5] start daemon"
bin/aibusd serve >/tmp/aibusd-test.log 2>&1 &
DPID=$!
trap 'kill $DPID 2>/dev/null || true; rm -rf "$AIBUS_DIR"' EXIT
for _ in $(seq 1 100); do [ -S "$AIBUS_DIR/aibus.sock" ] && break; sleep 0.02; done
[ -S "$AIBUS_DIR/aibus.sock" ] || fail "socket no aparecio"

echo "[3/5] push: wait bloqueado ANTES del send"
( AI_NAME=B bin/ai wait t2 5 > "$AIBUS_DIR/out" ) &
WPID=$!
sleep 0.1
AI_NAME=A bin/ai send t2 "push ok" >/dev/null
wait "$WPID" || fail "wait devolvio error"
[ "$(cat "$AIBUS_DIR/out")" = "push ok" ] || fail "push: $(cat "$AIBUS_DIR/out")"

echo "[4/5] history + topics"
AI_NAME=A bin/ai send t1 "hola mundo" >/dev/null
AI_NAME=C bin/ai send t1 "segundo" >/dev/null
H=$(bin/ai log t1 1)
echo "$H" | grep -q "segundo" || fail "history: $H"
bin/ai topics | grep -q "^t1 " || fail "topics: $(bin/ai topics)"

echo "[5/5] timeout devuelve exit 3"
RC=0
bin/ai wait vacio 1 >/dev/null 2>&1 || RC=$?
[ "$RC" = "3" ] || fail "timeout rc=$RC (esperaba 3)"

echo "PASS: todos los tests en verde"
