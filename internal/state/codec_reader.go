package state

import "unicode/utf8"

// One cursor is reused after preflight; only its private second-pass bytes mutate.
type codecReader struct {
	data                []byte
	pos                 int
	failed, materialize bool
}

func (r *codecReader) take(value string) {
	if r.failed {
		return
	}
	if len(value) > len(r.data)-r.pos || string(r.data[r.pos:r.pos+len(value)]) != value {
		r.failed = true
		return
	}
	r.pos += len(value)
}

func (r *codecReader) has(value string) bool {
	return !r.failed && len(value) <= len(r.data)-r.pos && string(r.data[r.pos:r.pos+len(value)]) == value
}

func (r *codecReader) done() bool { return !r.failed && r.pos == len(r.data) }

func (r *codecReader) add(total *uint64, count, size uint64) {
	budget := codecBudget{used: *total}
	if !budget.add(count, size) {
		r.failed = true
		return
	}
	*total = budget.used
}

func (r *codecReader) fixedText(p *codecPlan, value string, count bool) string {
	r.take(`"`)
	start := r.pos
	r.take(value)
	r.take(`"`)
	if r.failed {
		return ""
	}
	if !r.materialize {
		if count {
			r.add(&p.strings, uint64(len(value)), 1)
			r.add(&p.valueStrings, 1, 1)
		}
		return ""
	}
	return string(r.data[start : start+len(value)])
}

func (r *codecReader) text(p *codecPlan, count bool) string {
	r.take(`"`)
	start := r.pos
	decoded := 0
	for !r.failed && r.pos < len(r.data) {
		c := r.data[r.pos]
		if c == '"' {
			end := r.pos
			r.pos++
			if !r.materialize {
				if count {
					r.add(&p.strings, uint64(decoded), 1)
					r.add(&p.valueStrings, 1, 1)
				}
				return ""
			}
			return string(r.unescape(start, end))
		}
		if c == '\\' {
			decoded += r.escape()
			continue
		}
		if c < 0x20 || c == '<' || c == '>' || c == '&' {
			r.failed = true
			break
		}
		if c < utf8.RuneSelf {
			r.pos++
			decoded++
			continue
		}
		value, size := utf8.DecodeRune(r.data[r.pos:])
		if value == utf8.RuneError && size == 1 || value == '\u2028' || value == '\u2029' {
			r.failed = true
			break
		}
		r.pos += size
		decoded += size
	}
	r.failed = true
	return ""
}

func (r *codecReader) escape() int {
	r.pos++
	if r.pos == len(r.data) {
		r.failed = true
		return 0
	}
	c := r.data[r.pos]
	r.pos++
	switch c {
	case '"', '\\', 'b', 'f', 'n', 'r', 't':
		return 1
	case 'u':
		if len(r.data)-r.pos < 4 {
			r.failed = true
			return 0
		}
		value := r.data[r.pos : r.pos+4]
		r.pos += 4
		if string(value) == "2028" || string(value) == "2029" {
			return 3
		}
		if value[0] != '0' || value[1] != '0' {
			r.failed = true
			return 0
		}
		hi, lo := codecHexDigit(value[2]), codecHexDigit(value[3])
		if hi < 0 || lo < 0 {
			r.failed = true
			return 0
		}
		decoded := byte(hi*16 + lo)
		if decoded < 0x20 && decoded != '\b' && decoded != '\f' && decoded != '\n' && decoded != '\r' && decoded != '\t' || decoded == '<' || decoded == '>' || decoded == '&' {
			return 1
		}
	}
	r.failed = true
	return 0
}

func codecHexDigit(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	if c >= 'a' && c <= 'f' {
		return int(c-'a') + 10
	}
	return -1
}

// Escapes were checked before this in-place pass; writes never overtake reads.
func (r *codecReader) unescape(start, end int) []byte {
	write := start
	for read := start; read < end; read++ {
		c := r.data[read]
		if c != '\\' {
			r.data[write] = c
			write++
			continue
		}
		read++
		switch r.data[read] {
		case '"', '\\':
			c = r.data[read]
		case 'b':
			c = '\b'
		case 'f':
			c = '\f'
		case 'n':
			c = '\n'
		case 'r':
			c = '\r'
		case 't':
			c = '\t'
		case 'u':
			if r.data[read+1] == '2' {
				r.data[write], r.data[write+1], r.data[write+2] = 0xe2, 0x80, 0xa0+r.data[read+4]-'0'
				write += 3
				read += 4
				continue
			}
			c = byte(codecHexDigit(r.data[read+3])*16 + codecHexDigit(r.data[read+4]))
			read += 4
		}
		r.data[write] = c
		write++
	}
	return r.data[start:write]
}

func (r *codecReader) unsigned(limit uint64) uint64 {
	if r.failed || r.pos == len(r.data) {
		r.failed = true
		return 0
	}
	start := r.pos
	var value uint64
	for r.pos < len(r.data) && r.data[r.pos] >= '0' && r.data[r.pos] <= '9' {
		digit := uint64(r.data[r.pos] - '0')
		if value > limit/10 || value == limit/10 && digit > limit%10 {
			r.failed = true
			return 0
		}
		value = value*10 + digit
		r.pos++
	}
	if r.pos == start || r.pos-start > 1 && r.data[start] == '0' {
		r.failed = true
	}
	return value
}

func (r *codecReader) boolean() bool {
	if r.has("true") {
		r.take("true")
		return true
	}
	r.take("false")
	return false
}

func (r *codecReader) arrayNext(index int) bool {
	if r.failed || r.has("]") {
		return false
	}
	if index != 0 {
		r.take(",")
	}
	return !r.failed
}

func codecReadArrayStart[T any](r *codecReader, p *codecPlan, slot int) []T {
	r.take("[")
	if !r.materialize {
		return nil
	}
	return make([]T, int(p.counts[slot]))
}

func (r *codecReader) arrayItem(index, maximum, length int) bool {
	if !r.arrayNext(index) {
		return false
	}
	if index >= maximum || r.materialize && index >= length {
		r.failed = true
		return false
	}
	return true
}

func (r *codecReader) arrayEnd(p *codecPlan, slot int, charge uint64, count, expected int) {
	r.take("]")
	if r.materialize {
		if count != expected {
			r.failed = true
		}
	} else {
		p.counts[slot] = uint64(count)
		r.add(&p.arrays, uint64(count), charge)
	}
}
