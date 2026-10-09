import type { Plugin } from "@opencode-ai/plugin"
import { tool } from "@opencode-ai/plugin"
import { execFileSync, spawn, type ChildProcess } from "node:child_process"
import { homedir } from "node:os"
import { mkdirSync, writeFileSync, rmSync, readdirSync, readFileSync } from "node:fs"
import { join } from "node:path"

const SEP = "\x1f"

type Inbox = { from: string; body: string }

export const AibusTelepathy: Plugin = async ({ client }, options) => {
  const opts = (options ?? {}) as Record<string, unknown>
  if (process.env.AIBUS_TP === "0") return {}

  const name =
    (typeof opts.name === "string" && opts.name) ||
    process.env.AIBUS_NAME ||
    process.env.AI_NAME ||
    "oc-" + process.pid
  const dir = process.env.AIBUS_DIR || join(homedir(), ".aibus")
  const aiBin = process.env.AIBUS_BIN || join(dir, "bin", "ai")
  const glyphs = opts.glyphs !== false
  const maxHops = typeof opts.maxHops === "number" ? opts.maxHops : 6
  const maxPerMin = typeof opts.maxPerMin === "number" ? opts.maxPerMin : 30

  const topics = name === "board" ? ["board"] : [name, "board"]
  const peersDir = join(dir, "peers")

  let boundSessionID: string | undefined
  let started = false
  let langLegend: string | undefined
  const buffer: Inbox[] = []
  const children: ChildProcess[] = []
  const injectionTimes: number[] = []

  const log = (level: string, message: string, extra?: unknown) => {
    try {
      void client.app.log({ body: { service: "aibus-telepathy", level: level as never, message, extra } })
    } catch {
      /* never throw from a hook */
    }
  }

  const run = (args: string[]): string =>
    execFileSync(aiBin, args, { env: { ...process.env, AI_NAME: name }, encoding: "utf8" })

  const announce = () => {
    try {
      writeFileSync(
        join(peersDir, name + ".json"),
        JSON.stringify({ name, pid: process.pid, session: boundSessionID ?? null, ts: Date.now() }),
        { mode: 0o600 },
      )
    } catch {
      /* ignore */
    }
  }

  const inject = (from: string, body: string) => {
    if (!boundSessionID) return
    try {
      void client.session
        .promptAsync({
          path: { id: boundSessionID },
          body: { parts: [{ type: "text", text: `⟦TP ← ${from}⟧ ${body}` }] },
        })
        .catch((e: unknown) => log("warn", "promptAsync failed", String(e)))
    } catch (e) {
      log("warn", "inject failed", String(e))
    }
  }

  const flush = () => {
    if (!boundSessionID) return
    while (buffer.length) {
      const m = buffer.shift() as Inbox
      inject(m.from, m.body)
    }
  }

  const handleLine = (line: string) => {
    const idx = line.indexOf("\t")
    if (idx < 0) return
    const payload = line.slice(idx + 1)
    const parts = payload.split(SEP)
    const from = parts.length >= 3 ? parts[0] : "?"
    const hops = parts.length >= 3 ? Number(parts[1]) : 1
    const body = parts.length >= 3 ? parts.slice(2).join(SEP) : payload
    if (from === name) return
    if (!Number.isFinite(hops) || hops > maxHops) return
    const now = Date.now()
    while (injectionTimes.length && now - injectionTimes[0] > 60000) injectionTimes.shift()
    if (injectionTimes.length >= maxPerMin) return
    injectionTimes.push(now)
    const m: Inbox = { from, body }
    if (boundSessionID) inject(m.from, m.body)
    else buffer.push(m)
  }

  const start = () => {
    if (started) return
    started = true
    try {
      mkdirSync(dir, { recursive: true })
      mkdirSync(peersDir, { recursive: true })
      announce()
      for (const topic of topics) {
        let head = "0"
        try {
          const mm = /head\s+(\d+)/.exec(run(["ringstat", topic]))
          if (mm) head = mm[1]
        } catch {
          /* ring may not exist yet */
        }
        try {
          writeFileSync(join(dir, `cursor.${name}.${topic}`), head, { mode: 0o600 })
        } catch {
          /* ignore */
        }
        const child = spawn(aiBin, ["recv", topic, "31536000"], {
          env: { ...process.env, AI_NAME: name },
          stdio: ["ignore", "pipe", "pipe"],
        })
        child.stdout?.on("data", (d: Buffer) => {
          try {
            for (const line of d.toString().split("\n")) {
              if (line.trim()) handleLine(line)
            }
          } catch (e) {
            log("warn", "recv parse failed", String(e))
          }
        })
        child.stderr?.on("data", (d: Buffer) => log("debug", "recv stderr: " + d.toString().trim().slice(0, 400)))
        child.on("error", (e: Error) => log("warn", "recv spawn failed", String(e)))
        child.on("exit", (code: number | null) => log("info", `recv ${topic} exited ${code}`))
        ;(child as unknown as { unref?: () => void }).unref?.()
        children.push(child)
      }
    } catch (e) {
      log("warn", "start failed", String(e))
    }
  }

  start()

  return {
    event: async ({ event }: { event: { type?: string; properties?: { sessionID?: string } } }) => {
      try {
        start()
        if (typeof event?.type === "string" && event.type.startsWith("session.")) {
          const sid = event.properties?.sessionID
          if (typeof sid === "string" && sid) {
            boundSessionID = sid
            announce()
            flush()
          }
        }
      } catch (e) {
        log("warn", "event hook failed", String(e))
      }
    },

    "chat.message": async (input: { sessionID: string }) => {
      try {
        boundSessionID = input.sessionID
        flush()
      } catch (e) {
        log("warn", "chat.message hook failed", String(e))
      }
    },

    "experimental.chat.system.transform": async (
      input: { sessionID?: string },
      output: { system: string[] },
    ) => {
      try {
        start()
        if (input?.sessionID) boundSessionID = input.sessionID
        if (!glyphs || !Array.isArray(output.system)) return
        if (langLegend === undefined) {
          try {
            langLegend = run(["lang"])
              .split("\n")
              .map((l) => l.split("\t"))
              .filter((c) => c.length >= 3)
              .map((c) => `${c[1]}=${c[2]}`)
              .join(" ")
          } catch {
            langLegend = ""
          }
        }
        if (langLegend) {
          output.system.push(
            `Protocolo TELEPATÍA (AIBUS): recibirás mensajes como "⟦TP ← <agente>⟧ <glifos>". ` +
              `Responde con la tool tp_send (to=<agente>, o vacío para difundir). Lenguaje 1 glifo = 1 token: ` +
              langLegend,
          )
        }
      } catch (e) {
        log("warn", "system.transform hook failed", String(e))
      }
    },

    tool: {
      tp_send: tool({
        description:
          "Envía un mensaje por telepatía a otra sesión opencode (AIBUS). Compacta a glifos cuando puede.",
        args: {
          to: tool.schema.string().optional().describe("agente destino; vacío = difundir a 'board'"),
          text: tool.schema.string().describe("mensaje en texto natural o glifos"),
        },
        async execute(args: { to?: string; text: string }) {
          try {
            const hasSymbol = [...args.text].some((c) => (c.codePointAt(0) ?? 0) > 0x2000)
            let body = args.text
            if (glyphs && !hasSymbol) {
              try {
                body = run(["say", args.text]).trim()
              } catch {
                body = args.text
              }
            }
            const dest = args.to?.trim() || "board"
            run(["fire", dest, `${name}${SEP}1${SEP}${body}`])
            return `TP→${dest}: ${body}`
          } catch (e) {
            return `TP error: ${String(e)}`
          }
        },
      }),
      tp_recv: tool({
        description: "Vacía la bandeja de mensajes de telepatía recibidos.",
        args: {},
        async execute() {
          try {
            const items = buffer.splice(0)
            return items.length ? items.map((m) => `${m.from}\t${m.body}`).join("\n") : "(vacío)"
          } catch (e) {
            return `TP error: ${String(e)}`
          }
        },
      }),
      tp_peers: tool({
        description: "Lista las sesiones opencode conectadas a la telepatía (AIBUS).",
        args: {},
        async execute() {
          try {
            const out: string[] = []
            for (const f of readdirSync(peersDir)) {
              if (!f.endsWith(".json")) continue
              try {
                const p = JSON.parse(readFileSync(join(peersDir, f), "utf8"))
                out.push(`${p.name}\tsession=${p.session ?? "?"}\tpid=${p.pid}`)
              } catch {
                /* ignore corrupt entry */
              }
            }
            return out.length ? out.join("\n") : "(sin peers)"
          } catch {
            return "(sin peers)"
          }
        },
      }),
      tp_name: tool({
        description: "Devuelve mi identidad AIBUS y la sesión enlazada.",
        args: {},
        async execute() {
          return `name=${name} session=${boundSessionID ?? "?"} topics=${topics.join(",")}`
        },
      }),
    },

    dispose: async () => {
      try {
        for (const c of children) c.kill()
        rmSync(join(peersDir, name + ".json"), { force: true })
      } catch {
        /* ignore */
      }
    },
  }
}

export default AibusTelepathy
