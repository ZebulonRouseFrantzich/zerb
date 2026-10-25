package state

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func freshGraphFixture(t *testing.T, prefix freshPrefix) freshGraph {
	t.Helper()
	g := freshGraph{prefix: prefix, instance: strings.Repeat("1", 32), initToken: strings.Repeat("2", 32), writerToken: strings.Repeat("4", 32), process: processIdentity{12, 34, 56}}
	g.issued = []string{g.instance, g.initToken, strings.Repeat("3", 32), g.writerToken, strings.Repeat("5", 32), strings.Repeat("6", 32)}
	g.state = freshNames(prefix, "", "")
	if prefix >= freshClaim {
		r := rootRecord{Schema: 1, Domain: "zerb.root", Status: "initializing", Instance: g.instance, Token: g.initToken, Process: g.process}
		if prefix >= freshReady {
			r.Status, r.Token, r.Process = "initialized", "", processIdentity{}
		}
		var err error
		g.root, err = encodeRootRecord(r)
		if err != nil {
			t.Fatal(err)
		}
	}
	if prefix >= freshWriterOwner {
		w := writerOwner{Schema: 1, Domain: "zerb.writer-owner", Status: "released", Instance: g.instance}
		if prefix == freshHeld {
			w.Status, w.Generation, w.Token, w.Process = "held", 1, g.writerToken, g.process
		}
		if prefix == freshReleased {
			w.Generation, w.PreviousGeneration, w.PreviousToken = 2, 1, g.writerToken
		}
		var err error
		g.writer, err = encodeWriterOwner(w)
		if err != nil {
			t.Fatal(err)
		}
	}
	if prefix >= freshDataOwner {
		g.data = []string{"owner.json"}
		g.dataOwner, _ = encodeNamespaceOwner(namespaceOwner{1, "zerb.data-owner", g.instance})
	}
	if prefix >= freshCacheOwner {
		g.cache = []string{"owner.json"}
		g.cacheOwner, _ = encodeNamespaceOwner(namespaceOwner{1, "zerb.cache-owner", g.instance})
	}
	return g
}

func TestFreshGraphClosedPrefixes(t *testing.T) {
	for prefix := freshEmpty; prefix <= freshReleased; prefix++ {
		g := freshGraphFixture(t, prefix)
		if !validFreshGraph(g) {
			t.Fatalf("prefix %d rejected", prefix)
		}
		for i := range g.state {
			bad := g
			bad.state = slices.Delete(slices.Clone(g.state), i, i+1)
			if validFreshGraph(bad) {
				t.Fatalf("prefix %d missing entry %d admitted", prefix, i)
			}
		}
		for _, name := range []string{"unknown", "current.json", "replace-witness.json", ".replace-" + strings.Repeat("a", 32), "toolchain"} {
			bad := g
			bad.state = append(slices.Clone(g.state), name)
			slices.Sort(bad.state)
			if validFreshGraph(bad) {
				t.Fatalf("prefix %d extra %q admitted", prefix, name)
			}
		}
		if len(g.state) > 1 {
			bad := g
			bad.state = slices.Clone(g.state)
			bad.state[0], bad.state[1] = bad.state[1], bad.state[0]
			if validFreshGraph(bad) {
				t.Fatal("unsorted observation admitted")
			}
		}
		for _, next := range freshStateEntries {
			if prefix < next.from {
				bad := g
				bad.state = append(slices.Clone(g.state), next.name)
				slices.Sort(bad.state)
				if validFreshGraph(bad) {
					t.Fatal("out of order entry admitted")
				}
			}
		}
		for _, edit := range []func(*freshGraph){
			func(g *freshGraph) { g.data = append(slices.Clone(g.data), "sources") },
			func(g *freshGraph) { g.cache = append(slices.Clone(g.cache), "downloads") },
			func(g *freshGraph) { g.issued = append(slices.Clone(g.issued), g.issued[0]) },
			func(g *freshGraph) { g.root = append(bytes.Clone(g.root), '\n') },
		} {
			bad := g
			edit(&bad)
			if validFreshGraph(bad) {
				t.Fatalf("prefix %d altered graph admitted", prefix)
			}
		}
	}
}

func TestFreshGraphCanonicalRelations(t *testing.T) {
	for _, prefix := range []freshPrefix{freshEmpty, freshInitLock} {
		g := freshGraphFixture(t, prefix)
		g.witness = "replace-witness.json"
		g.state = freshNames(prefix, g.witness, "")
		if validFreshGraph(g) {
			t.Fatal("preclaim witness admitted")
		}
	}
	g := freshGraphFixture(t, freshHeld)
	for _, edit := range []func(*freshGraph){
		func(g *freshGraph) { g.instance = g.initToken },
		func(g *freshGraph) { g.writerToken = g.initToken },
		func(g *freshGraph) { g.writerToken = g.issued[2] },
		func(g *freshGraph) { g.process.Start++ },
		func(g *freshGraph) {
			g.dataOwner, _ = encodeNamespaceOwner(namespaceOwner{1, "zerb.data-owner", strings.Repeat("a", 32)})
		},
		func(g *freshGraph) { g.cacheOwner = bytes.Clone(g.dataOwner) },
		func(g *freshGraph) { g.root = freshGraphFixture(t, freshClaim).root },
		func(g *freshGraph) { g.writer = freshGraphFixture(t, freshReleased).writer },
	} {
		bad := g
		edit(&bad)
		if validFreshGraph(bad) {
			t.Fatal("cross-record substitution admitted")
		}
	}
	for _, mode := range []uint32{0, 0o040750, 0o040755, 0o100700} {
		if freshStateMode(mode) {
			t.Fatal("unusable witness parent admitted")
		}
	}
	if !freshStateMode(0o040700) {
		t.Fatal("0700 state refused")
	}
}

func freshReplacementGraph(t *testing.T, kind string, published bool) freshGraph {
	t.Helper()
	before, after := freshCacheOwner, freshReady
	id, token, generation := strings.Repeat("3", 32), strings.Repeat("2", 32), uint64(0)
	target := "root.json"
	if kind == "writer-acquire" {
		before, after = freshReady, freshHeld
		id, token = strings.Repeat("5", 32), ""
		target = "writer-owner.json"
	}
	if kind == "writer-release" {
		before, after = freshHeld, freshReleased
		id, token, generation = strings.Repeat("6", 32), strings.Repeat("4", 32), 1
		target = "writer-owner.json"
	}
	g := freshGraphFixture(t, before)
	old, new := g.root, freshGraphFixture(t, after).root
	if target != "root.json" {
		old, new = g.writer, freshGraphFixture(t, after).writer
	}
	if published {
		g = freshGraphFixture(t, after)
	}
	file := func(data []byte, inode, links uint64) replacementFileFacts {
		hash := sha256.Sum256(data)
		return replacementFileFacts{Identity: replacementIdentity{Device: 1, Inode: inode, Owner: 123, Mode: 0o100600, Links: links, Size: int64(len(data)), MountID: 2}, SHA256: hex.EncodeToString(hash[:])}
	}
	r := replacementRecord{Schema: 1, Domain: "zerb.replace-witness", Instance: g.instance, ID: id, Publisher: g.process, Authority: replacementAuthority{Kind: kind, Generation: generation, Token: token}, Files: replacementFiles{Parent: replacementIdentity{Device: 1, Inode: 10, Owner: 123, Mode: 0o040700, Links: 2, MountID: 2}, Target: target, Temporary: ".replace-" + id, Previous: file(old, 11, 1), Staged: file(new, 12, 0)}}
	g.replacement = &r
	g.witness = "replace-witness.json"
	g.state = freshNames(g.prefix, g.witness, "")
	return g
}

func TestFreshReplacementGraphAuthority(t *testing.T) {
	for _, kind := range []string{"initialization", "writer-acquire", "writer-release"} {
		for _, published := range []bool{false, true} {
			g := freshReplacementGraph(t, kind, published)
			if !validFreshGraph(g) {
				t.Fatalf("%s published=%v refused", kind, published)
			}
			for _, edit := range []func(*replacementRecord){
				func(r *replacementRecord) { r.Authority.Token = strings.Repeat("e", 32) },
				func(r *replacementRecord) { r.Authority.Generation++ },
				func(r *replacementRecord) { r.Publisher.Start++ },
				func(r *replacementRecord) { r.ID = g.initToken; r.Files.Temporary = ".replace-" + r.ID },
				func(r *replacementRecord) {
					r.ID = g.issued[2]
					r.Files.Temporary = ".replace-" + r.ID
					if kind == "initialization" {
						r.ID = g.issued[4]
						r.Files.Temporary = ".replace-" + r.ID
					}
				},
				func(r *replacementRecord) { r.Files.Previous.SHA256 = strings.Repeat("a", 64) },
				func(r *replacementRecord) { r.Files.Staged.Identity.Size++ },
				func(r *replacementRecord) { r.Instance = strings.Repeat("a", 32) },
			} {
				bad := g
				record := *g.replacement
				edit(&record)
				bad.replacement = &record
				if validFreshGraph(bad) {
					t.Fatalf("%s altered authority admitted", kind)
				}
			}
			if !published {
				g.temporary = g.replacement.Files.Temporary
				g.state = freshNames(g.prefix, g.witness, g.temporary)
				if !validFreshGraph(g) {
					t.Fatal("linked prefix refused")
				}
			} else {
				bad := g
				bad.witness = ""
				bad.state = freshNames(bad.prefix, "", "")
				if validFreshGraph(bad) {
					t.Fatal("successor without retained witness admitted")
				}
				bad = g
				bad.temporary = g.replacement.Files.Temporary
				bad.state = freshNames(bad.prefix, bad.witness, bad.temporary)
				if validFreshGraph(bad) {
					t.Fatal("successor with temporary admitted")
				}
			}
		}
	}
	for _, temporary := range []string{"x", ".replace-", ".replace-" + strings.Repeat("a", 31)} {
		g := freshGraphFixture(t, freshReady)
		g.temporary = temporary
		g.witness = "replace-witness.json"
		g.state = freshNames(g.prefix, g.witness, g.temporary)
		if validFreshGraph(g) {
			t.Fatal("bad temporary admitted")
		}
	}
}

func TestFreshRandomSessionNoReuse(t *testing.T) {
	s := &freshSession{random: bytes.NewReader(bytes.Repeat([]byte{1}, 32))}
	first, err := s.mint()
	if err != nil || first == "" {
		t.Fatal("complete random read refused")
	}
	if _, err = s.mint(); err == nil {
		t.Fatal("session identity reuse admitted")
	}
	for _, reader := range []io.Reader{bytes.NewReader(make([]byte, 15)), iotestFreshError{}, nil} {
		s := &freshSession{random: reader}
		if _, err = s.mint(); err == nil {
			t.Fatal("short/error random read admitted")
		}
	}
	// A retired replacement ID remains in the same session set.
	s = &freshSession{random: bytes.NewReader(bytes.Repeat([]byte{3}, 16)), graph: freshGraph{issued: []string{strings.Repeat("03", 16)}}}
	if _, err = s.mint(); err == nil {
		t.Fatal("retired replacement identity reused")
	}
}

type iotestFreshError struct{}

func (iotestFreshError) Read([]byte) (int, error) { return 0, errors.New("random unavailable") }
