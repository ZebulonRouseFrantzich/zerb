package state

import (
	"bytes"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func readyGraphFixture(t *testing.T) readyGraph {
	t.Helper()
	fresh := freshGraphFixture(t, freshReady)
	return readyGraph{state: slices.Clone(readyStateNames), data: []string{"owner.json"}, cache: []string{"owner.json"},
		root: fresh.root, writer: fresh.writer, dataOwner: fresh.dataOwner, cacheOwner: fresh.cacheOwner}
}

func TestReadyGraphGeneralWriterObservations(t *testing.T) {
	for i, writer := range writerRecordFixtures() {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			g := readyGraphFixture(t)
			var err error
			g.writer, err = encodeWriterOwnerRecord(writer)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := readReadyGraph(g)
			if !ok || got.instance != writer.Instance || got.writer != writer {
				t.Fatal("general owner observation changed")
			}
		})
	}
}

func TestReadyGraphExactNarrowInventory(t *testing.T) {
	for i := range readyStateNames {
		g := readyGraphFixture(t)
		g.state = slices.Delete(g.state, i, i+1)
		if got, ok := readReadyGraph(g); ok || got != (readyFacts{}) {
			t.Fatal("missing state entry observed")
		}
	}
	for _, name := range []string{"current.json", "replace-witness.json", ".replace-" + strings.Repeat("a", 32), "extra", "owner.json"} {
		g := readyGraphFixture(t)
		g.state = append(g.state, name)
		slices.Sort(g.state)
		if _, ok := readReadyGraph(g); ok {
			t.Fatal("nonempty state observation accepted")
		}
	}
	for i := range readyStateNames {
		g := readyGraphFixture(t)
		g.state[i] = g.state[(i+1)%len(g.state)]
		if _, ok := readReadyGraph(g); ok {
			t.Fatal("duplicate state entry accepted")
		}
	}
	for _, edit := range []func(*readyGraph){
		func(g *readyGraph) { g.state[0], g.state[1] = g.state[1], g.state[0] },
		func(g *readyGraph) { g.data = nil },
		func(g *readyGraph) { g.cache = nil },
		func(g *readyGraph) { g.data = []string{"owner.json", "sources"} },
		func(g *readyGraph) { g.cache = []string{"downloads", "owner.json"} },
		func(g *readyGraph) { g.controls[0] = []string{"unreferenced"} },
		func(g *readyGraph) { g.controls[1] = []string{"operation.json"} },
		func(g *readyGraph) { g.controls[2] = []string{"orphan.json"} },
	} {
		g := readyGraphFixture(t)
		edit(&g)
		if _, ok := readReadyGraph(g); ok {
			t.Fatal("outside narrow inventory accepted")
		}
	}
}

func TestReadyGraphRecordAndInstanceRejections(t *testing.T) {
	for _, field := range []string{"root", "writer", "data", "cache"} {
		for _, form := range []string{"missing", "null", "oversize", "trailing", "duplicate", "instance", "domain"} {
			t.Run(field+"/"+form, func(t *testing.T) {
				g := readyGraphFixture(t)
				var slot *[]byte
				switch field {
				case "root":
					slot = &g.root
				case "writer":
					slot = &g.writer
				case "data":
					slot = &g.dataOwner
				case "cache":
					slot = &g.cacheOwner
				}
				switch form {
				case "missing":
					*slot = nil
				case "null":
					*slot = []byte("null")
				case "oversize":
					*slot = bytes.Repeat([]byte(" "), initRecordLimit+1)
				case "trailing":
					*slot = append(bytes.Clone(*slot), '\n')
				case "duplicate":
					*slot = bytes.Replace(*slot, []byte(`"schema":1`), []byte(`"schema":1,"schema":1`), 1)
				case "instance":
					*slot = bytes.ReplaceAll(*slot, []byte(strings.Repeat("1", 32)), []byte(strings.Repeat("a", 32)))
				case "domain":
					*slot = bytes.Replace(*slot, []byte(`"domain":"zerb.`), []byte(`"domain":"other.`), 1)
				}
				if got, ok := readReadyGraph(g); ok || got != (readyFacts{}) {
					t.Fatal("invalid record yielded facts")
				}
			})
		}
	}
	g := readyGraphFixture(t)
	r := rootRecord{Schema: 1, Domain: "zerb.root", Status: "initializing", Instance: strings.Repeat("1", 32), Token: strings.Repeat("2", 32), Process: processIdentity{1, 2, 3}}
	g.root, _ = encodeRootRecord(r)
	if _, ok := readReadyGraph(g); ok {
		t.Fatal("initializing root observed as initialized")
	}
	g = readyGraphFixture(t)
	g.dataOwner, g.cacheOwner = g.cacheOwner, g.dataOwner
	if _, ok := readReadyGraph(g); ok {
		t.Fatal("namespace owners interchanged")
	}
}

func TestReadyGraphCallerOwnership(t *testing.T) {
	g := readyGraphFixture(t)
	want, ok := readReadyGraph(g)
	if !ok {
		t.Fatal("fixture")
	}
	root, writer, data, cache := bytes.Clone(g.root), bytes.Clone(g.writer), bytes.Clone(g.dataOwner), bytes.Clone(g.cacheOwner)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				got, ok := readReadyGraph(g)
				if !ok || got != want {
					t.Error("concurrent facts changed")
				}
			}
		}()
	}
	wg.Wait()
	if !bytes.Equal(root, g.root) || !bytes.Equal(writer, g.writer) || !bytes.Equal(data, g.dataOwner) || !bytes.Equal(cache, g.cacheOwner) || !slices.Equal(g.state, readyStateNames) {
		t.Fatal("caller input mutated")
	}
}

func FuzzReadyGraphRecords(f *testing.F) {
	instance := strings.Repeat("1", 32)
	root, _ := encodeRootRecord(rootRecord{Schema: 1, Domain: "zerb.root", Status: "initialized", Instance: instance})
	writer, _ := encodeWriterOwnerRecord(writerRecordFixture(0, "", true))
	data, _ := encodeNamespaceOwner(namespaceOwner{1, "zerb.data-owner", instance})
	cache, _ := encodeNamespaceOwner(namespaceOwner{1, "zerb.cache-owner", instance})
	for i, seed := range [][]byte{root, writer, data, cache} {
		f.Add(uint8(i), seed)
	}
	for _, prefix := range []string{"", "null", `{}`, `{"schema":1}`, strings.Repeat(" ", 4097)} {
		f.Add(uint8(0), []byte(prefix))
	}
	f.Fuzz(func(t *testing.T, field uint8, data []byte) {
		if len(data) > initRecordLimit+1 {
			return
		}
		g := readyGraphFixture(t)
		switch field % 4 {
		case 0:
			g.root = data
		case 1:
			g.writer = data
		case 2:
			g.dataOwner = data
		case 3:
			g.cacheOwner = data
		}
		before := bytes.Clone(data)
		facts, ok := readReadyGraph(g)
		if !ok && facts != (readyFacts{}) {
			t.Fatal("failure yielded partial facts")
		}
		if ok && (!replacementHex(facts.instance, 32) || facts.writer.Instance != facts.instance || !validWriterOwnerRecord(facts.writer)) {
			t.Fatal("invalid success")
		}
		if !bytes.Equal(data, before) {
			t.Fatal("input mutated")
		}
	})
}
