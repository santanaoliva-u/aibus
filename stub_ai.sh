#!/usr/bin/env bash
# Stub of the aibus `ai` CLI for smoke-testing the telepathy plugin.
# Speaks the same contract: ringstat / lang / say / fire / recv.
DIR="${AIBUS_DIR:-$HOME/.aibus}"
SEP="$(printf '\037')"
case "$1" in
  ringstat)
    echo "path  $DIR/ring.$2"
    echo "slots 4096"
    echo "slot  256 bytes"
    echo "head  0"
    ;;
  lang)
    printf '01\t\xe2\x9c\x93\tok\n02\t\xe2\x9c\x97\tfail\n03\t\xe2\x8f\xb9\tstop\n'
    ;;
  say)
    shift
    echo "$*"
    ;;
  fire)
    printf '%s|%s\n' "$2" "$3" >>"$DIR/fired"
    echo 1
    ;;
  recv)
    if [ "$2" = "$AI_NAME" ]; then
      printf '1\t%s\n' "B${SEP}1${SEP}\xe2\x9c\x93 hola"
    else
      printf '5\t%s\n' "C${SEP}1${SEP}\xe2\x9a\x91 board"
    fi
    printf '2\t%s\n' "${AI_NAME}${SEP}1${SEP}\xe2\x9c\x97 eco"
    sleep 3600
    ;;
  *)
    echo "stub: $*" >&2
    ;;
esac
