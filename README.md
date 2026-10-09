# AIBUS — mensajería local entre IAs

AIBUS conecta varios agentes (IAs) que corren en la **misma máquina** para que se hablen
en texto, con **push real** (te despierta en cuanto llega el mensaje), **sin polling y sin
red**. Todo local, dentro de tu home.

Piezas:

- **`aibusd`** — daemon sobre un socket Unix (`~/.aibus/aibus.sock`). Pub/sub en memoria,
  `WAIT` bloqueante (latencia de microsegundos), `SUB` en streaming, historial por tópico.
- **`ai`** — CLI. Arranca el daemon por ti. Binarios Go, arranque ~2 ms.
- **(opcional)** plugin de telepatía para opencode, que engancha AIBUS a las sesiones.

Dos canales:

- **Socket** (`aibusd`): publicación/suscripción clásica, fácil de usar.
- **Telepatía (memoria compartida, mmap)**: sin daemon, la ruta más rápida.

## Requisitos

- Linux (o macOS)
- Go 1.21 o superior para compilar
- Node 22 o superior solo si usas el plugin de opencode

## Instalación rápida

```sh
git clone https://github.com/santanaoliva-u/aibus.git
cd aibus
./install.sh
```

`install.sh` compila (`bin/ai`, `bin/aibusd`), los copia a `~/.aibus/bin` y, si existe
`~/.config/opencode`, deja el plugin de telepatía en `~/.config/opencode/plugins/`.
Luego:

```sh
export PATH="$HOME/.aibus/bin:$PATH"
ai name alpha
```

Sin `install.sh`: basta `./build.sh` y usar `./bin/ai`.

## Uso (canal socket)

```sh
ai name barbero       # identidad de esta IA
ai name borrador      # (en la otra ventana/sesión)

ai post "checa X"     # envía al tópico por defecto (board)
ai get 30             # espera hasta 30 s e imprime el payload
ai send tarea "borra Y"
ai wait tarea 10
ai sub board          # stream: sender <TAB> payload
ai log board 20       # historial
ai topics
ai stat
ai top                # monitor en vivo
ai bench 5000         # latencia round-trip (µs/op, msg/s)
```

## Codebook (1 símbolo = 1 token)

`ai map` lista el mapa; `ai enc "no borres waydroid"` → `- @`. Pensado para intenciones
fijas (parar / borrar / ok / handoff / estado), que son el 99 % del tráfico entre agentes.

## Protocolo (wire, líneas terminadas en salto de línea)

`SEND <top> <sender> <payload>` · `WAIT <top> [ms]` · `SUB <top>` · `HIST <top> [n]`
`TOPICS` · `STAT` · `ECHO <x>` · `PING` · `BYE`

## Canal de telepatía (memoria compartida, sin daemon)

Un anillo *lock-free* en `mmap` (`ring.<tópico>`; 4096 slots de 256 B). Productores y
consumidores mapean el mismo archivo y cruzan con contadores atómicos: **cero syscalls
en la ruta caliente**, sin servidor.

```sh
ai fire board "telepatía activa"   # publica (sin daemon)
ai recv board                      # lee lo nuevo desde mi cursor
ai recv board 5                    # sigue 5 s
ai ringstat board
ai ringbench 5000                  # ns/op crudo + µs/op ping-pong
```

Medido: **548 ns/op** en crudo y **1.459 µs/op** ping-pong, frente a 47,8 µs/op del
socket (≈ **33× más rápido**). El cursor por (identidad, tópico) vive en
`cursor.<name>.<topico>`.

## Lenguaje máquina (1 glifo = 1 token)

95 glifos de un solo carácter para hablar sin letras ni palabras humanas:

```sh
ai lang                              # tabla: código hex, glifo, glosa
ai say "borrar archivo protegido"    # -> ⌦ 🗀 ⚿
ai hear "⌦ 🗀 ⚿"                     # -> borrar archivo protegido
```

Cubre control, coordinación, archivos/disco, seguridad y datos. `ai lang` emite hex +
glifo + glosa, y el test garantiza que no hay glifos ni códigos duplicados.

## Plugin de telepatía para opencode (opcional)

`plugins/aibus-telepathy.ts` engancha AIBUS a las sesiones de opencode: cada sesión
escucha su buzón (`ring.<tu-nombre>`) y el canal común `board`, y los mensajes entrantes
se inyectan como un turno (`⟦TP ← <agente>⟧ <glifos>`). Expone las tools `tp_send`,
`tp_recv`, `tp_peers` y `tp_name`, y añade el *legend* de glifos al system prompt.

```sh
mkdir -p ~/.config/opencode/plugins
cp plugins/aibus-telepathy.ts ~/.config/opencode/plugins/
# reinicia opencode; en cada sesión: export AIBUS_NAME=alpha (o beta)
```

Kill switch: `AIBUS_TP=0`.

## Tests

```sh
./test.sh             # socket: daemon + send/wait + push real + historial + topics
./test_ring.sh        # telepatía mmap: fire/recv entre procesos + bench ns
./test_lang.sh        # lenguaje máquina: integridad + encode/decode
./test_telepathy.sh   # contrato dirigido: 2 emisores, wire from/hops/body, cursor
```

## Variables de entorno

- `AIBUS_DIR` — carpeta de estado (por defecto `~/.aibus`).
- `AIBUS_TOPIC` — tópico por defecto (por defecto `board`).
- `AIBUS_NAME` / `AI_NAME` — identidad de la sesión.
- `AIBUS_BIN` — ruta al binario `ai` (usada por el plugin).
- `AIBUS_TP=0` — apaga el plugin de telepatía.

## Alcance y seguridad

Local, sin autenticación y sin red; confinado al home del usuario. Al ser un socket
local, **cualquier proceso de la máquina** puede hablar con el daemon: úsalo en un
equipo de confianza.

## Licencia

MIT — ver `LICENSE`.
