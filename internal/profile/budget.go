package profile

const (
	maxReservationBytes = uint64(8_000_000)
	maxParserTokens     = uint64(4096)
)

// reservation accounts backing capacity, including reuse, rather than Go heap
// allocation events. Each complete invocation retains one checked budget.
type reservation struct{ used uint64 }

func (b *reservation) add(count, size uint64) bool {
	if b.used > maxReservationBytes || size != 0 && count > (maxReservationBytes-b.used)/size {
		return false
	}
	b.used += count * size
	return true
}

// PC-042 schedule for the pinned v2.4.3-zerb.1 decoder on Go 1.24. The local
// patch drops all decoder-owned storage before pooling, including on errors.
// 4D: ReadAll current/prior buffers and error document copy; 3S: two string
// parser passes and decoded strings; 3(W+M): error context/underline growth;
// 32L: error line headers; 1024T: both AST arenas, tracker and model capacity.
// The fixed 64KiB covers initial arenas, framing and small scratch collections.
func reserveDocument(data []byte, tokens, quoted uint64) bool {
	if len(data) > 1<<20 || tokens > maxParserTokens || quoted > uint64(len(data)) {
		return false
	}
	lines, longest, window := documentLines(data)
	var budget reservation
	return budget.add(1, 65_536) && budget.add(uint64(len(data)), 4) &&
		budget.add(quoted, 3) && budget.add(window, 3) && budget.add(longest, 3) &&
		budget.add(lines, 32) && budget.add(tokens, 1024)
}

// The native decoder formats at most seven context lines and one underline.
// A fixed ring measures both without constructing a line or string table.
func documentLines(data []byte) (lines, longest, window uint64) {
	var ring [7]uint64
	var sum uint64
	index, start := 0, 0
	add := func(end, next int) {
		length := uint64(end - start)
		if length > longest {
			longest = length
		}
		sum -= ring[index]
		ring[index] = uint64(next - start)
		sum += ring[index]
		index = (index + 1) % len(ring)
		if sum > window {
			window = sum
		}
		lines++
		start = next
	}
	for index, value := range data {
		if value == '\n' {
			add(index, index+1)
		}
	}
	add(len(data), len(data))
	return lines, longest, window
}
