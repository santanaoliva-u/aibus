// aibusd - local AI message bus.
// Unix-domain-socket pub/sub with real push (blocking WAIT), zero polling.
// Wire protocol: newline-delimited text frames, ASCII command + space args.
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const histCap = 1024

type msg struct {
	sender  string
	payload string
	at      time.Time
}

type topic struct {
	name string
	hist []msg
	subs map[uint64]chan msg
}

type hub struct {
	mu     sync.Mutex
	topics map[string]*topic
	nextID uint64
	count  uint64
	dropped uint64
	start  time.Time
}

func newHub() *hub { return &hub{topics: map[string]*topic{}, start: time.Now()} }

func (h *hub) get(name string) *topic {
	t := h.topics[name]
	if t == nil {
		t = &topic{name: name, subs: map[uint64]chan msg{}}
		h.topics[name] = t
	}
	return t
}

func (h *hub) publish(tname, sender, payload string) {
	m := msg{sender: sender, payload: payload, at: time.Now()}
	h.mu.Lock()
	t := h.get(tname)
	t.hist = append(t.hist, m)
	if len(t.hist) > histCap {
		t.hist = append([]msg(nil), t.hist[len(t.hist)-histCap:]...)
	}
	subs := make([]chan msg, 0, len(t.subs))
	for _, c := range t.subs {
		subs = append(subs, c)
	}
	h.count++
	h.mu.Unlock()
	for _, c := range subs {
		select {
		case c <- m:
		default:
			h.mu.Lock()
			h.dropped++
			h.mu.Unlock()
		}
	}
}

func (h *hub) subscribe(tname string) (uint64, chan msg) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.get(tname)
	id := h.nextID
	h.nextID++
	ch := make(chan msg, 512)
	t.subs[id] = ch
	return id, ch
}

func (h *hub) unsubscribe(tname string, id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if t := h.topics[tname]; t != nil {
		delete(t.subs, id)
	}
}

func (h *hub) history(tname string, n int) []msg {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.topics[tname]
	if t == nil {
		return nil
	}
	if n <= 0 || n > len(t.hist) {
		n = len(t.hist)
	}
	return append([]msg(nil), t.hist[len(t.hist)-n:]...)
}

type topicStat struct {
	name  string
	msgs  int
	subs  int
}

func (h *hub) stats() (uint64, uint64, []topicStat) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]topicStat, 0, len(h.topics))
	for _, t := range h.topics {
		out = append(out, topicStat{name: t.name, msgs: len(t.hist), subs: len(t.subs)})
	}
	return h.count, h.dropped, out
}

func main() {
	dir := os.Getenv("AIBUS_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".aibus")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "aibusd: mkdir:", err)
		os.Exit(1)
	}
	sock := filepath.Join(dir, "aibus.sock")

	if _, err := os.Stat(sock); err == nil {
		if c, err := net.Dial("unix", sock); err == nil {
			c.Close()
			fmt.Fprintln(os.Stderr, "aibusd: already running on", sock)
			return
		}
		os.Remove(sock)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "aibusd: listen:", err)
		os.Exit(1)
	}
	os.Chmod(sock, 0o600)

	h := newHub()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		ln.Close()
		os.Remove(sock)
		os.Exit(0)
	}()

	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go handle(h, c)
	}
}

func handle(h *hub, c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}
		if !dispatch(h, br, bw, line) {
			return
		}
	}
}

// dispatch returns false to close the connection.
func dispatch(h *hub, br *bufio.Reader, bw *bufio.Writer, line string) bool {
	p := strings.SplitN(line, " ", 4)
	switch p[0] {
	case "PING":
		fmt.Fprintln(bw, "PONG")
		bw.Flush()
		return true
	case "ECHO":
		rest := ""
		if len(line) > 5 {
			rest = line[5:]
		}
		fmt.Fprintf(bw, "ECHO %s\n", rest)
		bw.Flush()
		return true
	case "SEND":
		if len(p) < 4 {
			fmt.Fprintln(bw, "ERR need: topic sender payload")
			bw.Flush()
			return true
		}
		h.publish(p[1], p[2], p[3])
		fmt.Fprintln(bw, "OK")
		bw.Flush()
		return true
	case "WAIT":
		if len(p) < 2 {
			fmt.Fprintln(bw, "ERR need: topic [timeout_ms]")
			bw.Flush()
			return true
		}
		timeout := 0
		if len(p) >= 3 {
			timeout, _ = strconv.Atoi(p[2])
		}
		id, ch := h.subscribe(p[1])
		defer h.unsubscribe(p[1], id)
		if timeout > 0 {
			select {
			case m := <-ch:
				fmt.Fprintf(bw, "MSG %s %s\n", m.sender, m.payload)
			case <-time.After(time.Duration(timeout) * time.Millisecond):
				fmt.Fprintln(bw, "TIMEOUT")
			}
		} else {
			m := <-ch
			fmt.Fprintf(bw, "MSG %s %s\n", m.sender, m.payload)
		}
		bw.Flush()
		return true
	case "SUB":
		if len(p) < 2 {
			fmt.Fprintln(bw, "ERR need: topic")
			bw.Flush()
			return true
		}
		id, ch := h.subscribe(p[1])
		defer h.unsubscribe(p[1], id)
		done := make(chan struct{})
		go func() {
			for {
				if _, err := br.ReadByte(); err != nil {
					close(done)
					return
				}
			}
		}()
		for {
			select {
			case m := <-ch:
				if _, err := fmt.Fprintf(bw, "MSG %s %s\n", m.sender, m.payload); err != nil {
					return false
				}
				if err := bw.Flush(); err != nil {
					return false
				}
			case <-done:
				return false
			}
		}
	case "HIST":
		if len(p) < 2 {
			fmt.Fprintln(bw, "ERR need: topic [n]")
			bw.Flush()
			return true
		}
		n := 0
		if len(p) >= 3 {
			n, _ = strconv.Atoi(p[2])
		}
		for _, m := range h.history(p[1], n) {
			fmt.Fprintf(bw, "MSG %s %s\n", m.sender, m.payload)
		}
		fmt.Fprintln(bw, "END")
		bw.Flush()
		return true
	case "TOPICS":
		_, _, ts := h.stats()
		for _, t := range ts {
			fmt.Fprintf(bw, "T %s %d %d\n", t.name, t.msgs, t.subs)
		}
		fmt.Fprintln(bw, "END")
		bw.Flush()
		return true
	case "STAT":
		count, dropped, ts := h.stats()
		up := int(time.Since(h.start).Seconds())
		fmt.Fprintf(bw, "STAT %d %d %d %d\n", count, dropped, len(ts), up)
		bw.Flush()
		return true
	case "BYE":
		return false
	default:
		fmt.Fprintln(bw, "ERR unknown command")
		bw.Flush()
		return true
	}
}
