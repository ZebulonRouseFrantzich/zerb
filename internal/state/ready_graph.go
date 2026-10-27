package state

import "slices"

// This is one narrow empty initialized observation, not exhaustive classification.
type readyGraph struct {
	state, data, cache                  []string
	controls                            [3][]string
	root, writer, dataOwner, cacheOwner []byte
}

type readyFacts struct {
	instance string
	writer   writerOwner
}

var readyStateNames = []string{
	"admission.lock", "candidates", "init.lock", "journals", "receipts",
	"root.json", "writer-owner.json", "writer.lock",
}

func readReadyGraph(g readyGraph) (readyFacts, bool) {
	if !slices.Equal(g.state, readyStateNames) || !slices.Equal(g.data, []string{"owner.json"}) ||
		!slices.Equal(g.cache, []string{"owner.json"}) {
		return readyFacts{}, false
	}
	for _, names := range g.controls {
		if len(names) != 0 {
			return readyFacts{}, false
		}
	}
	root, err := decodeRootRecord(g.root)
	if err != nil || root.Status != "initialized" {
		return readyFacts{}, false
	}
	writer, err := decodeWriterOwnerRecord(g.writer)
	if err != nil || writer.Instance != root.Instance {
		return readyFacts{}, false
	}
	data, err := decodeNamespaceOwner(g.dataOwner)
	if err != nil || data.Domain != "zerb.data-owner" || data.Instance != root.Instance {
		return readyFacts{}, false
	}
	cache, err := decodeNamespaceOwner(g.cacheOwner)
	if err != nil || cache.Domain != "zerb.cache-owner" || cache.Instance != root.Instance {
		return readyFacts{}, false
	}
	return readyFacts{root.Instance, writer}, true
}
