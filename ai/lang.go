package main

import (
	"fmt"
	"strings"
)

// glyphs is the machine alphabet: one code byte, one single-rune glyph, one
// directive. This is the native AI-to-AI language. It is not meant for humans.
// es/en are matching synonyms; the glyph is the 1-token representation.
var glyphs = []struct {
	b   byte
	g   string
	es  string
	en  string
}{
	{0x01, "⟳", "ping", "ping"},
	{0x02, "✓", "ok hecho", "ok done"},
	{0x03, "✗", "fallo", "fail"},
	{0x04, "⏹", "para stop", "stop"},
	{0x05, "⏳", "espera", "wait"},
	{0x06, "⚑", "urgente", "urgent"},
	{0x07, "↻", "reintentar", "retry"},
	{0x08, "⏱", "timeout", "timeout"},
	{0x09, "⛔", "bloqueado", "blocked"},
	{0x0A, "⚠", "aviso", "warn"},
	{0x0B, "⁇", "duda", "unsure"},
	{0x0C, "‼", "importante", "important"},
	{0x0D, "✔", "aprobado", "approved"},
	{0x0E, "✖", "rechazado", "rejected"},
	{0x0F, "⌛", "tiempo", "time"},
	{0x10, "→", "handoff pasotarea", "handoff"},
	{0x11, "←", "ack recibido", "ack"},
	{0x12, "⇄", "sync sincronizar", "sync"},
	{0x13, "⊕", "unir", "merge"},
	{0x14, "⊖", "separar", "split"},
	{0x15, "⇈", "subir", "up"},
	{0x16, "⇊", "bajar", "down"},
	{0x17, "⊙", "esperar-otro", "await-peer"},
	{0x18, "⊘", "detener-otro", "halt-peer"},
	{0x19, "⟲", "repetir", "loop"},
	{0x1A, "⧉", "duplicado", "duplicate"},
	{0x1B, "⌁", "dividir", "divide"},
	{0x1C, "∞", "total", "total"},
	{0x1D, "∅", "vaciar", "empty"},
	{0x1E, "⁂", "varios", "many"},
	{0x1F, "⟶", "siguiente", "next"},
	{0x20, "⌂", "home", "home"},
	{0x21, "🗀", "archivo", "file"},
	{0x22, "🗁", "carpeta", "folder"},
	{0x23, "⌦", "borrar", "delete"},
	{0x24, "⌫", "no-borrar", "keep"},
	{0x25, "⧗", "backup", "backup"},
	{0x26, "⇥", "mover", "move"},
	{0x27, "⇤", "copiar", "copy"},
	{0x28, "⊞", "crear", "create"},
	{0x29, "◫", "tamano", "size"},
	{0x2A, "⛁", "disco", "disk"},
	{0x2B, "⛃", "cache", "cache"},
	{0x2C, "🗝", "permisos", "perms"},
	{0x2D, "⚿", "protegido", "protected"},
	{0x2E, "⌗", "ruta", "path"},
	{0x2F, "⟟", "abrir", "open"},
	{0x30, "☠", "secreto", "secret"},
	{0x31, "⚔", "ataque", "attack"},
	{0x32, "⚒", "exploit", "exploit"},
	{0x33, "⚖", "auditar", "audit"},
	{0x34, "⌬", "inyeccion", "injection"},
	{0x35, "☣", "malware", "malware"},
	{0x36, "⚡", "riesgo", "risk"},
	{0x37, "✇", "red", "network"},
	{0x38, "⚙", "config", "config"},
	{0x39, "⛨", "sandbox", "sandbox"},
	{0x3A, "⚐", "bandera", "flag"},
	{0x3B, "⦸", "bloqueo", "lock"},
	{0x3C, "⚚", "neutral", "neutral"},
	{0x3D, "⚕", "revisar", "review"},
	{0x3E, "⟨", "dato-ini", "data-open"},
	{0x3F, "⟩", "dato-fin", "data-close"},
	{0x40, "≡", "igual", "same"},
	{0x41, "≠", "distinto", "diff"},
	{0x42, "⩽", "limite", "limit"},
	{0x43, "⁺", "anadir", "add"},
	{0x44, "⁻", "quitar", "remove"},
	{0x45, "∑", "suma", "sum"},
	{0x46, "∏", "producto", "product"},
	{0x47, "ℓ", "lista", "list"},
	{0x48, "№", "id", "id"},
	{0x49, "⊹", "destino", "target"},
	{0x4A, "↺", "volver", "revert"},
	{0x4B, "⇉", "paralelo", "parallel"},
	{0x4C, "⇛", "secuencial", "serial"},
	{0x4D, "⌾", "estado", "state"},
	{0x4E, "⊚", "objetivo", "goal"},
	{0x4F, "⧈", "partido", "chunk"},
	{0x50, "⋯", "continuar", "continue"},
	{0x51, "⏎", "fin-linea", "eol"},
	{0x52, "⇧", "priorizar", "prioritize"},
	{0x53, "⊗", "cancelar", "cancel"},
	{0x54, "⨯", "multiplicar", "multiply"},
	{0x55, "⊝", "quitar-fuerte", "purge"},
	{0x56, "⌇", "flujo", "stream"},
	{0x57, "⧃", "fragmento", "fragment"},
	{0x58, "⌸", "bloque", "block"},
	{0x59, "⍟", "destacar", "highlight"},
	{0x5A, "⊛", "combinar", "combine"},
	{0x5B, "⟿", "derivar", "derive"},
	{0x5C, "⇝", "mapear", "map"},
	{0x5D, "⟴", "filtrar", "filter"},
	{0x5E, "⧎", "reducir", "reduce"},
	{0x5F, "⋈", "juntar", "join"},
}

func glyphIndex() map[string]string {
	idx := make(map[string]string, len(glyphs)*3)
	for _, e := range glyphs {
		idx[e.g] = e.es
		if e.es != "" {
			idx[e.es] = e.g
		}
		if e.en != "" {
			idx[e.en] = e.g
		}
	}
	return idx
}

func listGlyphs() {
	for _, e := range glyphs {
		fmt.Printf("%02X\t%s\t%s\n", e.b, e.g, e.es)
	}
}

// glyphEncode greedily replaces contiguous phrases (up to 4 words) with one
// glyph. Unknown words pass through.
func glyphEncode(text string) string {
	words := strings.Fields(strings.ToLower(text))
	idx := glyphIndex()
	out := make([]string, 0, len(words))
	for i := 0; i < len(words); {
		matched := false
		for n := 4; n >= 1; n-- {
			if i+n > len(words) {
				continue
			}
			ph := strings.Join(words[i:i+n], " ")
			if g, ok := idx[ph]; ok {
				out = append(out, g)
				i += n
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, words[i])
			i++
		}
	}
	return strings.Join(out, " ")
}

// glyphDecode reverses glyphEncode; unknown tokens pass through.
func glyphDecode(text string) string {
	rev := make(map[string]string, len(glyphs))
	for _, e := range glyphs {
		rev[e.g] = e.es
	}
	out := make([]string, 0, 16)
	for _, tok := range strings.Fields(text) {
		if es, ok := rev[tok]; ok {
			out = append(out, es)
		} else {
			out = append(out, tok)
		}
	}
	return strings.Join(out, " ")
}
