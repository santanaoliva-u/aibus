import { mkdtempSync, writeFileSync, readFileSync, chmodSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"

const assert = (cond, msg) => {
  if (!cond) {
    console.error("SMOKE FAIL:", msg)
    process.exit(1)
  }
}

const dir = mkdtempSync(join(tmpdir(), "tpsmoke-"))
const stub = new URL("./stub_ai.sh", import.meta.url).pathname
chmodSync(stub, 0o755)

process.env.AIBUS_DIR = dir
process.env.AIBUS_BIN = stub
process.env.AIBUS_NAME = "n1"

const calls = []
const client = {
  app: { log: async () => {} },
  session: {
    promptAsync: async (o) => {
      calls.push(o)
      return {}
    },
  },
}

const ctx = {
  client,
  project: {},
  directory: process.cwd(),
  worktree: process.cwd(),
  serverUrl: new URL("http://localhost"),
  $: () => {},
  experimental_workspace: { register() {} },
}

const mod = await import(new URL("./plugins/aibus-telepathy.ts", import.meta.url).href)
const hooks = await mod.default(ctx)

// give the two spawned recv stubs time to emit their lines
await new Promise((r) => setTimeout(r, 500))
await hooks.event({ event: { type: "session.idle", properties: { sessionID: "ses_test" } } })
await new Promise((r) => setTimeout(r, 250))

const texts = calls.map((c) => c.body.parts[0].text)
assert(texts.some((t) => t.includes("⟦TP ← B⟧") && t.includes("hola")), "inyecta mensaje directo de B")
assert(texts.some((t) => t.includes("⟦TP ← C⟧") && t.includes("board")), "inyecta mensaje de board de C")
assert(!texts.some((t) => t.includes("eco")), "ignora el eco propio (from===name)")

const out = { system: [] }
await hooks["experimental.chat.system.transform"]({ sessionID: "ses_test" }, out)
const legend = out.system.join(" ")
assert(legend.includes("AIBUS") && legend.includes("ok"), "inyecta el legend de glifos en el system prompt")

const send = await hooks.tool.tp_send.execute({ text: "borrar archivo protegido" })
assert(String(send).startsWith("TP→board:"), "tp_send devuelve acuse")
const fired = readFileSync(join(dir, "fired"), "utf8")
assert(fired.includes("board|n1"), "tp_send publica en ring board con el formato from<SEP>hops<SEP>body")

const peers = await hooks.tool.tp_peers.execute({})
assert(String(peers).includes("n1"), "tp_peers lista la sesion propia")

const who = await hooks.tool.tp_name.execute({})
assert(String(who).includes("name=n1") && String(who).includes("ses_test"), "tp_name refleja identidad y sesion")

await hooks.dispose()
await new Promise((r) => setTimeout(r, 150))
console.log("SMOKE PASS: inyeccion, board, eco, legend, tools OK")
process.exit(0)
