// ai - client for the aibus local message bus.
// Fast, text-only, push-based inter-agent comms. Single static binary, ~2ms startup.
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var codebook = []struct{ sym, key, label string }{
	{"?", "ping", "ping / estas ahi"},
	{"!", "stop", "PARA / no toques"},
	{"~", "wait", "esperando"},
	{"v", "ok", "ok / hecho"},
	{".", "done", "completado"},
	{"x", "fail", "fallo"},
	{"-", "no", "no / rechazado"},
	{">", "hand", "te paso la tarea"},
	{"<", "ack", "recibido"},
	{"+", "del", "borra aprobado"},
	{"@", "prot", "protegido / intocable"},
	{"#", "path", "archivo / ruta"},
	{"$", "size", "tamano / espacio"},
	{"^", "up", "inicio / subir"},
	{"_", "down", "fin / bajar"},
	{"%", "loop", "repetir"},
	{"*", "star", "prioridad alta"},
	{"&", "link", "relacion / unir"},
	{"=", "same", "igual"},
	{"/", "diff", "distinto / cambio"},
	{"(", "meta", "meta / contexto"},
	{")", "end", "fin de bloque"},
	{"[", "info", "info / nota"},
	{"]", "exec", "ejecutar"},
	{"{", "sum", "total"},
	{"}", "part", "dividir / parte"},
	{":", "id", "identificador"},
	{";", "note", "nota corta"},
	{"'", "imp", "importante"},
	{"|", "trx", "traducir"},
}

const help = `ai - aibus CLI (bus local entre IAs)

USO:
  ai name <n>              fija mi identidad (se guarda en ~/.aibus/name)
  ai send <top> <msg...>   envia mensaje a un topico
  ai wait <top> [seg]      BLOQUEA hasta recibir (push, sin polling); imprime payload
  ai sub  <top>            stream continuo (sender<TAB>payload)
  ai log  <top> [n]        ultimos n mensajes (default todos)
  ai post <msg...>         envia al topico por defecto (board)
  ai get  [seg]            espera en el topico por defecto
  ai topics                lista topicos
  ai stat                  stats del daemon
  ai bench [n]             latencia round-trip (default 2000)
  ai top                   monitor en vivo
  ai map [simbolo]         codebook de 1 token (1 simbolo = 1 intencion)
  ai enc <texto...>        comprime texto a simbolos del codebook
  ai help

CANAL DE TELEPATIA (memoria compartida, sin daemon, ns):
  ai fire <top> <msg...>   publica en el anillo mmap (sin syscall)
  ai recv <top> [seg] [--all]  lee lo nuevo desde mi cursor; [seg] sigue N seg
  ai ringstat <top>        geometria y head del anillo
  ai ringbench [n]         ping-pong de 2 anillos (latencia en ns)

LENGUAJE MAQUINA (1 glifo = 1 token, sin letras):
  ai lang                  tabla de glifos (hex, glifo, glosa)
  ai say <texto...>        codifica a glifos
  ai hear <glifos...>      decodifica glifos a texto

El daemon arranca solo (solo para el canal socket). Variables: AIBUS_DIR, AIBUS_TOPIC, AI_NAME.`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println(help)
		return
	}
	dir, sock := paths()
	cmd := args[0]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Println(help)
	case "name":
		if len(args) < 2 {
			fmt.Println(currentName(dir))
			return
		}
		os.WriteFile(filepath.Join(dir, "name"), []byte(args[1]), 0o600)
		fmt.Println(args[1])
	case "send", "s":
		if len(args) < 3 {
			die("uso: ai send <top> <msg...>")
		}
		ensure(dir, sock)
		send(sock, args[1], currentName(dir), strings.Join(args[2:], " "))
	case "post":
		if len(args) < 2 {
			die("uso: ai post <msg...>")
		}
		ensure(dir, sock)
		send(sock, defaultTopic(), currentName(dir), strings.Join(args[1:], " "))
	case "wait", "w", "get":
		ensure(dir, sock)
		top := defaultTopic()
		rest := args[1:]
		if cmd != "get" {
			if len(rest) < 1 {
				die("uso: ai wait <top> [seg]")
			}
			top = rest[0]
			rest = rest[1:]
		}
		timeout := 0
		if len(rest) >= 1 {
			timeout, _ = strconv.Atoi(rest[0])
		}
		wait(sock, top, timeout)
	case "sub", "tail":
		if len(args) < 2 {
			die("uso: ai sub <top>")
		}
		ensure(dir, sock)
		sub(sock, args[1])
	case "log":
		if len(args) < 2 {
			die("uso: ai log <top> [n]")
		}
		ensure(dir, sock)
		n := 0
		if len(args) >= 3 {
			n, _ = strconv.Atoi(args[2])
		}
		log(sock, args[1], n)
	case "topics":
		ensure(dir, sock)
		topics(sock)
	case "stat":
		ensure(dir, sock)
		stat(sock)
	case "bench":
		ensure(dir, sock)
		n := 2000
		if len(args) >= 2 {
			n, _ = strconv.Atoi(args[1])
		}
		bench(sock, n)
	case "top":
		ensure(dir, sock)
		top(sock)
	case "map":
		filter := ""
		if len(args) >= 2 {
			filter = args[1]
		}
		for _, e := range codebook {
			if filter != "" && !strings.Contains(e.sym+e.key+e.label, filter) {
				continue
			}
			fmt.Printf("%s\t%-6s\t%s\n", e.sym, e.key, e.label)
		}
	case "enc":
		if len(args) < 2 {
			die("uso: ai enc <texto...>")
		}
		fmt.Println(enc(strings.Join(args[1:], " ")))
	case "fire":
		if len(args) < 3 {
			die("uso: ai fire <top> <msg...>")
		}
		cmdFire(dir, currentName(dir), args[1], strings.Join(args[2:], " "))
	case "recv", "r":
		if len(args) < 2 {
			die("uso: ai recv <top> [seg] [--all]")
		}
		secs := 0
		all := false
		for _, a := range args[2:] {
			if a == "--all" {
				all = true
			} else {
				secs, _ = strconv.Atoi(a)
			}
		}
		cmdRecv(dir, currentName(dir), args[1], time.Duration(secs)*time.Second, all)
	case "ringbench":
		n := 2000
		if len(args) >= 2 {
			n, _ = strconv.Atoi(args[1])
		}
		cmdRingBench(dir, n)
	case "ringstat":
		if len(args) < 2 {
			die("uso: ai ringstat <top>")
		}
		cmdRingStat(dir, args[1])
	case "lang", "glyphs", "gl":
		listGlyphs()
	case "say":
		if len(args) < 2 {
			die("uso: ai say <texto...>")
		}
		fmt.Println(glyphEncode(strings.Join(args[1:], " ")))
	case "hear":
		if len(args) < 2 {
			die("uso: ai hear <glifos...>")
		}
		fmt.Println(glyphDecode(strings.Join(args[1:], " ")))
	default:
		fmt.Fprintln(os.Stderr, "comando desconocido:", cmd)
		fmt.Println(help)
		os.Exit(2)
	}
}

func paths() (dir, sock string) {
	dir = os.Getenv("AIBUS_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".aibus")
	}
	return dir, filepath.Join(dir, "aibus.sock")
}

func defaultTopic() string {
	if t := os.Getenv("AIBUS_TOPIC"); t != "" {
		return t
	}
	return "board"
}

func currentName(dir string) string {
	if n := os.Getenv("AI_NAME"); n != "" {
		return n
	}
	if b, err := os.ReadFile(filepath.Join(dir, "name")); err == nil {
		if n := strings.TrimSpace(string(b)); n != "" {
			return n
		}
	}
	return "anon"
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
}

// ensure starts the daemon if it is not already listening.
func ensure(dir, sock string) {
	if _, err := os.Stat(sock); err == nil {
		if c, err := net.Dial("unix", sock); err == nil {
			c.Close()
			return
		}
	}
	exe, err := os.Executable()
	if err != nil {
		die("no puedo localizar el ejecutable: " + err.Error())
	}
	daemon := filepath.Join(filepath.Dir(exe), "aibusd")
	if _, err := os.Stat(daemon); err != nil {
		die("daemon no encontrado en " + daemon + " (corre build.sh)")
	}
	os.MkdirAll(dir, 0o700)
	devnull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	cmd := exec.Command(daemon, "serve")
	cmd.Env = append(os.Environ(), "AIBUS_DIR="+dir)
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		die("no pude arrancar el daemon: " + err.Error())
	}
	cmd.Process.Release()
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(sock); err == nil {
			if c, err := net.Dial("unix", sock); err == nil {
				c.Close()
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	die("timeout esperando al daemon")
}

func dial(sock string) net.Conn {
	c, err := net.Dial("unix", sock)
	if err != nil {
		die("dial: " + err.Error())
	}
	return c
}

func sendLine(c net.Conn, line string) string {
	fmt.Fprintf(c, "%s\n", line)
	br := bufio.NewReader(c)
	resp, _ := br.ReadString('\n')
	return strings.TrimRight(resp, "\r\n")
}

func send(sock, topic, sender, payload string) {
	c := dial(sock)
	defer c.Close()
	resp := sendLine(c, "SEND "+topic+" "+sender+" "+payload)
	if resp != "OK" {
		die("send falló: " + resp)
	}
}

func wait(sock, topic string, timeoutSec int) {
	c := dial(sock)
	defer c.Close()
	if timeoutSec > 0 {
		c.SetReadDeadline(time.Now().Add(time.Duration(timeoutSec) * time.Second))
	}
	fmt.Fprintf(c, "WAIT %s %d\n", topic, timeoutSec*1000)
	br := bufio.NewReader(c)
	line, err := br.ReadString('\n')
	if err != nil {
		os.Exit(3)
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "TIMEOUT" {
		os.Exit(3)
	}
	if strings.HasPrefix(line, "MSG ") {
		rest := line[4:]
		if i := strings.IndexByte(rest, ' '); i >= 0 {
			rest = rest[i+1:]
		}
		fmt.Println(rest)
		return
	}
	fmt.Fprintln(os.Stderr, line)
	os.Exit(3)
}

func sub(sock, topic string) {
	c := dial(sock)
	defer c.Close()
	fmt.Fprintf(c, "SUB %s\n", topic)
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "MSG ") {
			rest := line[4:]
			if i := strings.IndexByte(rest, ' '); i >= 0 {
				fmt.Printf("%s\t%s\n", rest[:i], rest[i+1:])
			} else {
				fmt.Println(rest)
			}
		}
	}
}

func log(sock, topic string, n int) {
	c := dial(sock)
	defer c.Close()
	fmt.Fprintf(c, "HIST %s %d\n", topic, n)
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "END" {
			return
		}
		if strings.HasPrefix(line, "MSG ") {
			rest := line[4:]
			if i := strings.IndexByte(rest, ' '); i >= 0 {
				fmt.Printf("%s\t%s\n", rest[:i], rest[i+1:])
			}
		}
	}
}

func topics(sock string) {
	c := dial(sock)
	defer c.Close()
	fmt.Fprintln(c, "TOPICS")
	sc := bufio.NewScanner(c)
	var rows []string
	for sc.Scan() {
		line := sc.Text()
		if line == "END" {
			break
		}
		if strings.HasPrefix(line, "T ") {
			rows = append(rows, strings.TrimPrefix(line, "T "))
		}
	}
	sort.Strings(rows)
	for _, r := range rows {
		fmt.Println(r)
	}
}

func stat(sock string) {
	c := dial(sock)
	defer c.Close()
	fmt.Fprintln(c, "STAT")
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	fmt.Print(line)
}

func bench(sock string, n int) {
	c := dial(sock)
	defer c.Close()
	br := bufio.NewReader(c)
	if n < 1 {
		n = 1
	}
	for i := 0; i < 200; i++ { // warmup
		fmt.Fprintln(c, "PING")
		br.ReadString('\n')
	}
	start := time.Now()
	for i := 0; i < n; i++ {
		fmt.Fprintln(c, "PING")
		br.ReadString('\n')
	}
	el := time.Since(start)
	perOp := el / time.Duration(n)
	fmt.Printf("round-trips: %d\n", n)
	fmt.Printf("total:       %s\n", el)
	fmt.Printf("latencia:    %.2f us/op\n", float64(perOp.Nanoseconds())/1000.0)
	fmt.Printf("throughput:  %.0f msg/s\n", float64(n)/el.Seconds())
}

func top(sock string) {
	for {
		c := dial(sock)
		fmt.Fprintln(c, "STAT")
		br := bufio.NewReader(c)
		line, _ := br.ReadString('\n')
		fmt.Fprintln(c, "TOPICS")
		var rows []string
		sc := bufio.NewScanner(c)
		for sc.Scan() {
			l := sc.Text()
			if l == "END" {
				break
			}
			if strings.HasPrefix(l, "T ") {
				rows = append(rows, strings.TrimPrefix(l, "T "))
			}
		}
		c.Close()
		fmt.Print("\033[H\033[2J")
		fmt.Println("AIBUS - monitor en vivo (Ctrl-C para salir)")
		fmt.Println("sock:", sock)
		fmt.Println(strings.Repeat("-", 48))
		fmt.Println(strings.TrimSpace(line))
		fmt.Println(strings.Repeat("-", 48))
		for _, r := range rows {
			fmt.Println(r)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func labelWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 0xE0 && r <= 0xFF || r == 0xF1)
	})
}

func matchSymbol(phrase string) (string, bool) {
	for _, e := range codebook {
		if phrase == e.key {
			return e.sym, true
		}
	}
	pw := strings.Fields(phrase)
	for _, e := range codebook {
		lw := labelWords(e.label)
		for i := 0; i+len(pw) <= len(lw); i++ {
			ok := true
			for j := range pw {
				if lw[i+j] != pw[j] {
					ok = false
					break
				}
			}
			if ok {
				return e.sym, true
			}
		}
	}
	return "", false
}

func enc(text string) string {
	words := strings.Fields(strings.ToLower(text))
	out := make([]string, 0, len(words))
	for i := 0; i < len(words); {
		matched := false
		for n := 5; n >= 1; n-- {
			if i+n > len(words) {
				continue
			}
			if sym, ok := matchSymbol(strings.Join(words[i:i+n], " ")); ok {
				out = append(out, sym)
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
