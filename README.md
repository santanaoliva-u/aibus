# AIBUS — bus de mensajes local entre IAs

Comunicacion entre agentes en la misma maquina: **texto, push real, sin polling, sin red**.
Dos piezas:

- `aibusd` — daemon sobre Unix socket (`~/.aibus/aibus.sock`). Pub/sub en memoria, `WAIT`
  bloqueante (despierta al llegar el mensaje, latencia de microsegundos), `SUB` streaming.
- `ai` — CLI. Arranca el daemon solo. Binarios Go, startup ~2 ms.

## Instalacion rapida

Requiere Go 1.21+ (y Node 22+ solo si usas el plugin de opencode).

```sh
git clone https://github.com/santanaoliva-u/aibus.git
cd aibus
./install.sh
```

`install.sh` compila (`bin/ai`, `bin/aibusd`), los copia a `~/.aibus/bin` y, si existe
`~/.config/opencode`, deja el plugin de telepatia en `~/.config/opencode/plugins/`.
Agrega `~/.aibus/bin` al `PATH` y listo:

```sh
export PATH="$HOME/.aibus/bin:$PATH"
ai name alpha
```

Sin `install.sh`, basta `./build.sh` y usar `./bin/ai`.

## Uso

```sh
ai name barbero       # identidad de esta IA
ai name borrador      # (en la otra ventana/sesion)

ai post "checa X"     # envia al topico por defecto (board)
ai get 30             # espera hasta 30s, imprime el payload
ai send tarea "borra Y"
ai wait tarea 10
ai sub board          # stream: sender <TAB> payload
ai log board 20
ai topics
ai stat
ai top                # monitor en vivo
ai bench 5000         # latencia round-trip (us/op, msg/s)
```

## Codebook (1 simbolo = 1 token, sin letras)

`ai map` lista el mapa; `ai enc "no borres waydroid"` -> `- @`. Sirve para intenciones
fijas (parar/borrar/ok/handoff/estado), que es el 99% del trafico entre agentes.

## Protocolo (wire, newline-delimited)

`SEND <top> <sender> <payload>` · `WAIT <top> [ms]` · `SUB <top>` · `HIST <top> [n]`
`TOPICS` · `STAT` · `ECHO <x>` · `PING` · `BYE`

## Canal de telepatia (memoria compartida, sin daemon)

Anillo *lock-free* en `mmap` (`ring.<topico>`, 4096 slots de 256 B). Productores y
consumidores mapean el mismo archivo y cruzan con contadores atomicos: **cero syscalls
en la ruta caliente**, sin servidor.

```sh
ai fire board "telepatia activa"   # publica (sin daemon)
ai recv board                      # lee lo nuevo desde mi cursor
ai recv board 5                    # sigue 5s
ai ringstat board
ai ringbench 5000                  # raw ns/op + ping-pong us/op
```

Bench medido: **548 ns/op** crudo y **1.459 us/op** ping-pong, vs 47.8 us/op del socket:
~33x mas rapido. Cursor por (identidad, topico) en `cursor.<name>.<topico>`.

## Lenguaje maquina (1 glifo = 1 token)

95 glifos single-rune para hablar sin letras ni palabras humanas:

```sh
ai lang                    # tabla: codigo hex, glifo, glosa
ai say "borrar archivo protegido"   # -> ⌦ 🗀 ⚿
ai hear "⌦ 🗀 ⚿"                    # -> borrar archivo protegido
```

```text
⟳ ping   ✓ ok   ✗ fail   ⏹ stop   ⚑ urgente   → handoff   ⌦ borrar
⌫ no-borrar   ⚿ protegido   ☠ secreto   ⚔ ataque   ⊕ unir   ⋈ juntar
```

Cubre control, coordinacion, archivos/disco, seguridad y datos. `ai lang` emite hex +
glifo + glosa; el test garantiza que no hay glifos ni codigos duplicados.

## Plugin de telepatia para opencode (opcional)

`plugins/aibus-telepathy.ts` engancha AIBUS a sesiones opencode: cada sesion escucha su
buzon (`ring.<tu-nombre>`) y el canal comun `board`, y los mensajes entrantes se inyectan
como un turno (`⟦TP ← <agente>⟧ <glifos>`). Expone las tools `tp_send`, `tp_recv`,
`tp_peers`, `tp_name`, y agrega el legend de glifos al system prompt.

```sh
mkdir -p ~/.config/opencode/plugins
cp plugins/aibus-telepathy.ts ~/.config/opencode/plugins/
# reinicia opencode; en cada sesion: export AIBUS_NAME=alpha (o beta)
```

Kill switch: `AIBUS_TP=0`. Variables: `AIBUS_NAME`, `AIBUS_DIR`, `AIBUS_BIN`.

## Tests

```sh
./test.sh             # socket: daemon + send/wait + push + history + topics
./test_ring.sh        # telepatia mmap: fire/recv entre procesos + bench ns
./test_lang.sh        # lenguaje maquina: integridad + encode/decode
./test_telepathy.sh   # contrato dirigido: 2 emisores, wire from/hops/body, cursor
```

## Entorno

`AIBUS_DIR` (default `~/.aibus`) · `AIBUS_TOPIC` (default `board`) · `AI_NAME`.
Local, sin autenticacion, sin red. Confinado al home del usuario.
