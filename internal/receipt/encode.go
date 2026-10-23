package receipt

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"unicode/utf8"
)

const maxReservationBytes = uint64(8_000_000)

type reservation struct{ used uint64 }

func (b *reservation) add(count, size uint64) bool {
	if b.used > maxReservationBytes || size != 0 && count > (maxReservationBytes-b.used)/size {
		return false
	}
	b.used += count * size
	return true
}

// Sizing uses the same writer without a buffer. Encoding receives exactly the
// reserved capacity and cannot grow it. No input-dependent pooled state survives.
type jsonWriter struct {
	out     []byte
	size    uint64
	strings uint64
	failed  bool
	writing bool
}

func (w *jsonWriter) raw(s string) {
	if w.failed {
		return
	}
	if w.size > maxReservationBytes || uint64(len(s)) > maxReservationBytes-w.size {
		w.failed = true
		return
	}
	w.size += uint64(len(s))
	if w.writing {
		if len(s) > cap(w.out)-len(w.out) {
			w.failed = true
			return
		}
		w.out = append(w.out, s...)
	}
}

func (w *jsonWriter) string(s string) {
	if w.failed {
		return
	}
	w.strings++
	if !utf8.ValidString(s) {
		w.failed = true
		return
	}
	w.raw(`"`)
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
			if c < utf8.RuneSelf {
				i++
				continue
			}
			r, n := utf8.DecodeRuneInString(s[i:])
			if r != '\u2028' && r != '\u2029' {
				i += n
				continue
			}
			w.raw(s[start:i])
			if r == '\u2028' {
				w.raw(`\u2028`)
			} else {
				w.raw(`\u2029`)
			}
			i += n
			start = i
			continue
		}
		w.raw(s[start:i])
		switch c {
		case '"':
			w.raw(`\"`)
		case '\\':
			w.raw(`\\`)
		case '\n':
			w.raw(`\n`)
		case '\r':
			w.raw(`\r`)
		case '\t':
			w.raw(`\t`)
		case '\b':
			w.raw(`\b`)
		case '\f':
			w.raw(`\f`)
		default:
			const digits = "0123456789abcdef"
			escaped := [6]byte{'\\', 'u', '0', '0', digits[c>>4], digits[c&15]}
			w.raw(string(escaped[:]))
		}
		i++
		start = i
	}
	w.raw(s[start:])
	w.raw(`"`)
}
func (w *jsonWriter) digest(v Digest) { w.string(string(v)) }
func (w *jsonWriter) integer(v int) {
	var tmp [20]byte
	w.raw(string(strconv.AppendInt(tmp[:0], int64(v), 10)))
}
func (w *jsonWriter) unsigned(v uint64) {
	var tmp [20]byte
	w.raw(string(strconv.AppendUint(tmp[:0], v, 10)))
}
func (w *jsonWriter) unsigned32(v uint32) { w.unsigned(uint64(v)) }
func (w *jsonWriter) boolean(v bool) {
	if v {
		w.raw(`true`)
	} else {
		w.raw(`false`)
	}
}

func (w *jsonWriter) tree(files []File) {
	w.raw(`{"schema":1,"domain":"zerb.tree.v1","files":[`)
	for i, file := range files {
		if i != 0 {
			w.raw(`,`)
		}
		w.file(file)
	}
	w.raw(`]}`)
}

func hashBytes(encoded []byte) Digest {
	sum := sha256.Sum256(encoded)
	var token [71]byte
	copy(token[:], "sha256:")
	hex.Encode(token[7:], sum[:])
	return Digest(token[:])
}
