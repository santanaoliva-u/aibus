// Package shm implements a lock-free shared-memory ring for nanosecond
// inter-agent messaging. There is no daemon and no syscall on the hot path:
// every producer and consumer maps the same file and exchanges through
// atomic counters. This is the telepathy channel.
package shm

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	magic      = uint64(0x4149425553523031) // "AIBUSR01"
	headerSize = 64
	Slots      = 4096
	SlotSize   = 256
	MaxPayload = SlotSize - 16
	ringSize   = headerSize + Slots*SlotSize
	HeadOff    = 8 // uint64 publish counter
)

// Ring is a memory-mapped MPSC broadcast log.
type Ring struct {
	buf  []byte
	f    *os.File
	Path string
}

// Record is one delivered message.
type Record struct {
	Seq     uint64
	Sender  uint32
	Payload string
}

func u64p(b []byte, off int) *uint64 { return (*uint64)(unsafe.Pointer(&b[off])) }
func u32p(b []byte, off int) *uint32 { return (*uint32)(unsafe.Pointer(&b[off])) }

func slotOff(i uint64) int { return headerSize + int(i%Slots)*SlotSize }

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	if len(out) == 0 {
		return "default"
	}
	return string(out)
}

// Open maps (creating if needed) the ring file for a topic.
func Open(dir, topic string) (*Ring, error) {
	path := filepath.Join(dir, "ring."+sanitize(topic))
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Truncate(int64(ringSize)); err != nil {
		f.Close()
		return nil, err
	}
	buf, err := syscall.Mmap(int(f.Fd()), 0, ringSize,
		syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}
	r := &Ring{buf: buf, f: f, Path: path}
	if atomic.LoadUint64(u64p(buf, 0)) != magic {
		for i := 0; i < headerSize; i++ {
			buf[i] = 0
		}
		atomic.StoreUint64(u64p(buf, 16), Slots)
		atomic.StoreUint64(u64p(buf, 24), SlotSize)
		atomic.StoreUint64(u64p(buf, 0), magic)
	}
	return r, nil
}

// Publish reserves a slot, writes it, then commits it with a release store.
func (r *Ring) Publish(sender uint32, payload string) uint64 {
	if len(payload) > MaxPayload {
		payload = payload[:MaxPayload]
	}
	seq := atomic.AddUint64(u64p(r.buf, HeadOff), 1) // 1-based
	off := slotOff(seq - 1)
	atomic.StoreUint64(u64p(r.buf, off), 0)
	atomic.StoreUint32(u32p(r.buf, off+8), sender)
	atomic.StoreUint32(u32p(r.buf, off+12), uint32(len(payload)))
	copy(r.buf[off+16:off+16+len(payload)], payload)
	atomic.StoreUint64(u64p(r.buf, off), seq)
	return seq
}

// ReadFrom returns committed records after cursor and the new cursor.
// It busy-waits briefly for a slot that a producer has reserved but not yet
// committed, which is the only case where the log can have a hole.
func (r *Ring) ReadFrom(cursor uint64) ([]Record, uint64) {
	head := atomic.LoadUint64(u64p(r.buf, HeadOff))
	var out []Record
	for cursor < head {
		off := slotOff(cursor)
		want := cursor + 1
		seq := atomic.LoadUint64(u64p(r.buf, off))
		if seq != want {
			ok := false
			for s := 0; s < 2000000; s++ {
				seq = atomic.LoadUint64(u64p(r.buf, off))
				if seq == want {
					ok = true
					break
				}
				if seq > want {
					break
				}
			}
			if !ok {
				break
			}
		}
		sender := atomic.LoadUint32(u32p(r.buf, off+8))
		n := int(atomic.LoadUint32(u32p(r.buf, off+12)))
		if n > MaxPayload {
			n = MaxPayload
		}
		out = append(out, Record{Seq: want, Sender: sender, Payload: string(r.buf[off+16 : off+16+n])})
		cursor++
	}
	return out, cursor
}

// Head returns the number of messages ever published to the ring.
func (r *Ring) Head() uint64 { return atomic.LoadUint64(u64p(r.buf, HeadOff)) }

// Close unmaps and closes the file.
func (r *Ring) Close() {
	if r.buf != nil {
		syscall.Munmap(r.buf)
		r.buf = nil
	}
	if r.f != nil {
		r.f.Close()
		r.f = nil
	}
}
