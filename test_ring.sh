#!/usr/bin/env bash
# AIBUS telepathy-ring test: cross-process fire/recv over shared memory, no daemon.
set -euo pipefail
cd "$(dirname "$0")"
export AIBUS_DIR="$(mktemp -d)"
export PATH="$PWD/bin:$PATH"
trap 'rm -rf "$AIBUS_DIR"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

echo "[1/5] fire/recv entre procesos (sin daemon)"
AI_NAME=A bin/ai fire ringdemo "telepatia activa" >/dev/null
OUT=$(AI_NAME=B bin/ai recv ringdemo)
echo "$OUT" | grep -q "telepatia activa" || fail "recv: $OUT"

echo "[2/5] cursor: no re-entrega"
OUT2=$(AI_NAME=B bin/ai recv ringdemo)
[ -z "$OUT2" ] || fail "re-entrego: $OUT2"

echo "[3/5] segundo mensaje se entrega"
AI_NAME=A bin/ai fire ringdemo "segundo" >/dev/null
OUT3=$(AI_NAME=B bin/ai recv ringdemo)
echo "$OUT3" | grep -q "segundo" || fail "segundo: $OUT3"

echo "[4/5] ringstat"
bin/ai ringstat ringdemo | grep -q "^head  2" || fail "ringstat: $(bin/ai ringstat ringdemo)"

echo "[5/5] ringbench (ns/op)"
bin/ai ringbench 2000 | tee /tmp/aibus_ringbench.txt
grep -q "ns/op" /tmp/aibus_ringbench.txt || fail "bench sin ns"

echo "PASS: ring en verde"
