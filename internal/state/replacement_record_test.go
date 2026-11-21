package state

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/safefs"
)

func replacementFixture() (replacementRecord, replacementExpectation, replacementObservations) {
	old := safefs.FileFacts{Identity: safefs.Identity{Device: 1, Inode: 3, Owner: 1000, Mode: 0o100600, Links: 1, Size: 7, MountID: 5},
		ModifiedSeconds: -2, ModifiedNanoseconds: 3, ChangedSeconds: 4, ChangedNanoseconds: 5, SHA256: strings.Repeat("a", 64)}
	stage := old
	stage.Identity.Inode, stage.Identity.Links = 4, 0
	stage.SHA256 = strings.Repeat("b", 64)
	expected := replacementExpectation{instance: strings.Repeat("1", 32), id: strings.Repeat("2", 32),
		publisher: processIdentity{123, 456, 789}, authority: replacementAuthority{Kind: "initialization", Token: strings.Repeat("3", 32)},
		files: safefs.ReplacementFacts{Parent: safefs.Identity{Device: 1, Inode: 2, Owner: 1000, Mode: 0o040700, Links: 2, Size: 64, MountID: 5},
			Target: "root.json", Temporary: ".replace-" + strings.Repeat("2", 32), Previous: old, Staged: stage}}
	record := replacementRecord{1, "zerb.replace-witness", expected.instance, expected.id, expected.publisher, expected.authority, replacementWireFiles(expected.files)}
	observed := replacementObservations{parentState: replacementPresent, parent: expected.files.Parent,
		target: replacementFileObservation{replacementPresent, old}, temporary: replacementFileObservation{state: replacementAbsent}}
	return record, expected, observed
}

func TestReplacementCanonicalVector(t *testing.T) {
	record, _, _ := replacementFixture()
	want := `{"schema":1,"domain":"zerb.replace-witness","instance":"11111111111111111111111111111111","id":"22222222222222222222222222222222","publisher":{"pid":123,"start":456,"pid_namespace":789},"authority":{"kind":"initialization","generation":0,"token":"33333333333333333333333333333333","operation_id":"","operation_token":""},"files":{"parent":{"device":1,"inode":2,"owner":1000,"mode":16832,"links":2,"size":64,"mount_id":5},"target":"root.json","temporary":".replace-22222222222222222222222222222222","previous":{"identity":{"device":1,"inode":3,"owner":1000,"mode":33152,"links":1,"size":7,"mount_id":5},"modified_seconds":-2,"modified_nanoseconds":3,"changed_seconds":4,"changed_nanoseconds":5,"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"staged":{"identity":{"device":1,"inode":4,"owner":1000,"mode":33152,"links":0,"size":7,"mount_id":5},"modified_seconds":-2,"modified_nanoseconds":3,"changed_seconds":4,"changed_nanoseconds":5,"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}`
	data, err := encodeReplacementRecord(record)
	if err != nil || string(data) != want {
		t.Fatalf("canonical vector mismatch: %v", err)
	}
	got, err := decodeReplacementRecord([]byte(want))
	if err != nil || got != record {
		t.Fatalf("canonical decode mismatch: %v", err)
	}
}

func TestReplacementRejectsNoncanonicalJSON(t *testing.T) {
	record, _, _ := replacementFixture()
	canonical, err := encodeReplacementRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	s := string(canonical)
	cases := map[string]string{
		"empty": "", "null": "null", "array": "[]", "whitespace": " " + s, "newline": s + "\n",
		"multiple": s + s, "trailing": s + "x", "unknown": strings.Replace(s, `"schema":1`, `"extra":0,"schema":1`, 1),
		"duplicate":        strings.Replace(s, `"schema":1`, `"schema":1,"schema":1`, 1),
		"nested duplicate": strings.Replace(s, `"pid":123`, `"pid":123,"pid":123`, 1),
		"alias":            strings.Replace(s, `"schema"`, `"Schema"`, 1),
		"escaped key":      strings.Replace(s, `"schema"`, `"\u0073chema"`, 1),
		"reordered":        strings.Replace(s, `"schema":1,"domain":"zerb.replace-witness"`, `"domain":"zerb.replace-witness","schema":1`, 1),
		"omitted zero":     strings.Replace(s, `"generation":0,`, "", 1),
		"null scalar":      strings.Replace(s, `"generation":0`, `"generation":null`, 1),
		"null object":      strings.Replace(s, `"publisher":{"pid":123,"start":456,"pid_namespace":789}`, `"publisher":null`, 1),
		"float integer":    strings.Replace(s, `"generation":0`, `"generation":0.0`, 1),
		"negative uint":    strings.Replace(s, `"generation":0`, `"generation":-1`, 1),
		"overflow uint":    strings.Replace(s, `"generation":0`, `"generation":18446744073709551616`, 1),
		"overflow owner":   strings.Replace(s, `"owner":1000`, `"owner":4294967296`, 1),
		"overflow seconds": strings.Replace(s, `"modified_seconds":-2`, `"modified_seconds":9223372036854775808`, 1),
		"overflow pid":     strings.Replace(s, `"pid":123`, `"pid":18446744073709551615`, 1),
		"limit minus one":  strings.Repeat(" ", replacementRecordLimit-1),
		"limit":            strings.Repeat(" ", replacementRecordLimit),
		"over limit":       strings.Repeat(" ", replacementRecordLimit+1),
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			input := []byte(value)
			saved := bytes.Clone(input)
			got, err := decodeReplacementRecord(input)
			if err == nil || got != (replacementRecord{}) {
				t.Fatal("accepted unsupported representation")
			}
			if !bytes.Equal(input, saved) {
				t.Fatal("decoder mutated input")
			}
		})
	}
}

func TestReplacementScalarAndRoleBoundaries(t *testing.T) {
	base, _, _ := replacementFixture()
	mutations := map[string]func(*replacementRecord){
		"schema":                 func(r *replacementRecord) { r.Schema = 2 },
		"domain":                 func(r *replacementRecord) { r.Domain = "zerb.root" },
		"instance":               func(r *replacementRecord) { r.Instance = "" },
		"uppercase id":           func(r *replacementRecord) { r.ID = strings.Repeat("A", 32) },
		"id instance collision":  func(r *replacementRecord) { r.ID = r.Instance },
		"id token collision":     func(r *replacementRecord) { r.ID = r.Authority.Token },
		"pid":                    func(r *replacementRecord) { r.Publisher.PID = 0 },
		"negative pid":           func(r *replacementRecord) { r.Publisher.PID = -1 },
		"start":                  func(r *replacementRecord) { r.Publisher.Start = 0 },
		"pid namespace":          func(r *replacementRecord) { r.Publisher.PIDNamespace = 0 },
		"kind":                   func(r *replacementRecord) { r.Authority.Kind = "unknown" },
		"empty token":            func(r *replacementRecord) { r.Authority.Token = "" },
		"token length":           func(r *replacementRecord) { r.Authority.Token += "a" },
		"extra operation id":     func(r *replacementRecord) { r.Authority.OperationID = strings.Repeat("4", 32) },
		"extra operation token":  func(r *replacementRecord) { r.Authority.OperationToken = strings.Repeat("5", 32) },
		"temporary":              func(r *replacementRecord) { r.Files.Temporary = ".replace-other" },
		"parent inode":           func(r *replacementRecord) { r.Files.Parent.Inode = 0 },
		"parent links":           func(r *replacementRecord) { r.Files.Parent.Links = 0 },
		"parent size":            func(r *replacementRecord) { r.Files.Parent.Size = -1 },
		"parent mode":            func(r *replacementRecord) { r.Files.Parent.Mode = 0o040750 },
		"parent mount":           func(r *replacementRecord) { r.Files.Parent.MountID = 0 },
		"previous inode":         func(r *replacementRecord) { r.Files.Previous.Identity.Inode = 0 },
		"stage inode":            func(r *replacementRecord) { r.Files.Staged.Identity.Inode = 0 },
		"same inode":             func(r *replacementRecord) { r.Files.Staged.Identity.Inode = r.Files.Previous.Identity.Inode },
		"parent inode collision": func(r *replacementRecord) { r.Files.Staged.Identity.Inode = r.Files.Parent.Inode },
		"previous device":        func(r *replacementRecord) { r.Files.Previous.Identity.Device++ },
		"stage mount":            func(r *replacementRecord) { r.Files.Staged.Identity.MountID++ },
		"previous owner":         func(r *replacementRecord) { r.Files.Previous.Identity.Owner++ },
		"stage mode":             func(r *replacementRecord) { r.Files.Staged.Identity.Mode = 0o100644 },
		"previous links":         func(r *replacementRecord) { r.Files.Previous.Identity.Links = 2 },
		"stage links":            func(r *replacementRecord) { r.Files.Staged.Identity.Links = 1 },
		"negative size":          func(r *replacementRecord) { r.Files.Previous.Identity.Size = -1 },
		"size overflow":          func(r *replacementRecord) { r.Files.Staged.Identity.Size = int64(int(^uint(0) >> 1)) },
		"modified nanos":         func(r *replacementRecord) { r.Files.Previous.ModifiedNanoseconds = 1_000_000_000 },
		"changed nanos":          func(r *replacementRecord) { r.Files.Staged.ChangedNanoseconds = 1_000_000_000 },
		"hash":                   func(r *replacementRecord) { r.Files.Staged.SHA256 = strings.Repeat("A", 64) },
		"oversize string":        func(r *replacementRecord) { r.Files.Staged.SHA256 = strings.Repeat("a", replacementRecordLimit+1) },
	}
	for _, target := range []string{"init.lock", "admission.lock", "writer.lock", "replace-witness.json", "owner.json", "current.json", "candidates", "../root.json", ".replace-x", "root.json/", ""} {
		mutations["forbidden target "+target] = func(r *replacementRecord) { r.Files.Target = target }
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			saved := r
			if data, err := encodeReplacementRecord(r); err == nil || data != nil {
				t.Fatal("encoded invalid record")
			}
			if r != saved {
				t.Fatal("encoder mutated input")
			}
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := decodeReplacementRecord(data); err == nil || got != (replacementRecord{}) {
				t.Fatal("decoded invalid record")
			}
		})
	}
	for _, kind := range []string{"initialization", "writer-acquire", "writer-transfer", "writer-release", "journal-phase", "journal-transfer", "current-commit"} {
		t.Run(kind, func(t *testing.T) {
			r := base
			r.Authority.Kind, r.Authority.Generation = kind, 1
			switch kind {
			case "writer-acquire":
				r.Authority.Token = ""
				r.Files.Target = "writer-owner.json"
			case "writer-transfer", "writer-release":
				r.Files.Target = "writer-owner.json"
			case "journal-phase", "journal-transfer", "current-commit":
				r.Authority.OperationID, r.Authority.OperationToken = strings.Repeat("4", 32), strings.Repeat("5", 32)
				r.Files.Target = r.Authority.OperationID + ".json"
				if kind == "current-commit" {
					r.Files.Target = "current.json"
				}
			}
			for _, generation := range []uint64{0, 1, math.MaxUint64 - 1, math.MaxUint64} {
				r.Authority.Generation = generation
				valid := (generation != 0 || kind == "initialization" || kind == "writer-acquire") &&
					(generation != math.MaxUint64 || kind == "initialization" || kind == "journal-phase" || kind == "current-commit")
				if _, err := encodeReplacementRecord(r); (err == nil) != valid {
					t.Fatalf("wrong generation boundary %d: %v", generation, err)
				}
			}
			r.Authority.Generation = 1
			if kind == "writer-acquire" {
				r.Authority.Token = base.Authority.Token
				if validReplacementRecord(r) {
					t.Fatal("acquire accepted token")
				}
				return
			}
			if r.Authority.OperationID != "" {
				for _, field := range []string{"missing id", "missing token", "same operation", "authority collision", "replacement collision"} {
					v := r
					switch field {
					case "missing id":
						v.Authority.OperationID = ""
					case "missing token":
						v.Authority.OperationToken = ""
					case "same operation":
						v.Authority.OperationToken = v.Authority.OperationID
					case "authority collision":
						v.Authority.OperationToken = v.Authority.Token
					case "replacement collision":
						v.ID = v.Authority.OperationToken
					}
					if validReplacementRecord(v) {
						t.Fatal("accepted operation collision: " + field)
					}
				}
			}
		})
	}
	// Zero device/UID and empty bytes are valid facts, unlike missing inode/mount.
	r := base
	r.Files.Parent.Device, r.Files.Previous.Identity.Device, r.Files.Staged.Identity.Device = 0, 0, 0
	r.Files.Parent.Owner, r.Files.Previous.Identity.Owner, r.Files.Staged.Identity.Owner = 0, 0, 0
	r.Files.Previous.Identity.Size, r.Files.Staged.Identity.Size = 0, 0
	r.Files.Previous.ModifiedSeconds, r.Files.Staged.ChangedSeconds = math.MinInt64, math.MaxInt64
	r.Files.Previous.ModifiedNanoseconds, r.Files.Staged.ChangedNanoseconds = 999_999_999, 999_999_999
	if _, err := encodeReplacementRecord(r); err != nil {
		t.Fatal("rejected valid scalar extremes")
	}
}

func TestReplacementClosedPrefixes(t *testing.T) {
	record, expected, prior := replacementFixture()
	linked := prior
	linked.temporary = replacementFileObservation{replacementPresent, expected.files.Staged}
	linked.temporary.facts.Identity.Links = 1
	linked.temporary.facts.ChangedSeconds = 100
	linked.temporary.facts.ChangedNanoseconds = 999_999_999
	successor := prior
	successor.target = linked.temporary
	cases := []struct {
		name         string
		observations replacementObservations
		want         replacementPrefix
	}{
		{"witness only before link or after temp unlink", prior, retireRecord},
		{"linked stage", linked, removeTempThenRecord},
		{"renamed successor", successor, verifyThenRetire},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			savedRecord, savedExpected, savedObservations := record, expected, tc.observations
			if got := recognizeReplacementPrefix(&record, expected, tc.observations); got != tc.want {
				t.Fatalf("prefix %d", got)
			}
			if record != savedRecord || expected != savedExpected || tc.observations != savedObservations {
				t.Fatal("recognizer mutated input")
			}
		})
	}
	if recognizeReplacementPrefix(nil, expected, prior) != replacementBad {
		t.Fatal("absent record inferred history")
	}
	for _, state := range []replacementObservationState{replacementUnobserved, replacementAbsent, replacementObservationFailed, 255} {
		for _, role := range []string{"parent", "target", "temporary"} {
			v := prior
			switch role {
			case "parent":
				v.parentState = state
			case "target":
				v.target.state = state
			case "temporary":
				v.temporary.state = state
			}
			if role == "temporary" && state == replacementAbsent {
				continue
			}
			if recognizeReplacementPrefix(&record, expected, v) != replacementBad {
				t.Fatalf("%s state %d accepted", role, state)
			}
		}
	}
	bad := map[string]func(*replacementObservations){
		"parent device":           func(o *replacementObservations) { o.parent.Device++ },
		"parent inode":            func(o *replacementObservations) { o.parent.Inode++ },
		"parent owner":            func(o *replacementObservations) { o.parent.Owner++ },
		"parent mode":             func(o *replacementObservations) { o.parent.Mode++ },
		"parent mount":            func(o *replacementObservations) { o.parent.MountID++ },
		"parent zero links":       func(o *replacementObservations) { o.parent.Links = 0 },
		"parent negative size":    func(o *replacementObservations) { o.parent.Size = -1 },
		"target inode same bytes": func(o *replacementObservations) { o.target.facts.Identity.Inode++ },
		"target device":           func(o *replacementObservations) { o.target.facts.Identity.Device++ },
		"target owner":            func(o *replacementObservations) { o.target.facts.Identity.Owner++ },
		"target mode":             func(o *replacementObservations) { o.target.facts.Identity.Mode++ },
		"target links":            func(o *replacementObservations) { o.target.facts.Identity.Links++ },
		"target size":             func(o *replacementObservations) { o.target.facts.Identity.Size++ },
		"target mount":            func(o *replacementObservations) { o.target.facts.Identity.MountID++ },
		"target mtime seconds":    func(o *replacementObservations) { o.target.facts.ModifiedSeconds++ },
		"target mtime nanos":      func(o *replacementObservations) { o.target.facts.ModifiedNanoseconds++ },
		"target ctime seconds":    func(o *replacementObservations) { o.target.facts.ChangedSeconds++ },
		"target ctime nanos":      func(o *replacementObservations) { o.target.facts.ChangedNanoseconds++ },
		"target hash":             func(o *replacementObservations) { o.target.facts.SHA256 = strings.Repeat("f", 64) },
	}
	for name, mutate := range bad {
		t.Run(name, func(t *testing.T) {
			v := prior
			mutate(&v)
			if recognizeReplacementPrefix(&record, expected, v) != replacementBad {
				t.Fatal("accepted substitution")
			}
		})
	}
	for _, transition := range []replacementObservations{linked, successor} {
		for _, field := range []string{"inode", "device", "owner", "mode", "links", "size", "mount", "mtime seconds", "mtime nanos", "ctime nanos", "hash"} {
			v := transition
			f := &v.target.facts
			if v.temporary.state == replacementPresent {
				f = &v.temporary.facts
			}
			switch field {
			case "inode":
				f.Identity.Inode++
			case "device":
				f.Identity.Device++
			case "owner":
				f.Identity.Owner++
			case "mode":
				f.Identity.Mode++
			case "links":
				f.Identity.Links++
			case "size":
				f.Identity.Size++
			case "mount":
				f.Identity.MountID++
			case "mtime seconds":
				f.ModifiedSeconds++
			case "mtime nanos":
				f.ModifiedNanoseconds++
			case "ctime nanos":
				f.ChangedNanoseconds = 1_000_000_000
			case "hash":
				f.SHA256 = strings.Repeat("f", 64)
			}
			if recognizeReplacementPrefix(&record, expected, v) != replacementBad {
				t.Fatal("accepted changed stage " + field)
			}
		}
	}
	v := successor
	v.temporary = linked.temporary
	if recognizeReplacementPrefix(&record, expected, v) != replacementBad {
		t.Fatal("accepted successor plus temp")
	}
	v = prior
	v.temporary.facts = expected.files.Staged
	if recognizeReplacementPrefix(&record, expected, v) != replacementBad {
		t.Fatal("accepted contradictory absence")
	}
	v = prior
	v.parent.Links++
	v.parent.Size++
	if recognizeReplacementPrefix(&record, expected, v) != retireRecord {
		t.Fatal("ordinary directory growth rejected")
	}
	for _, field := range []string{"instance", "id", "publisher", "authority", "facts", "zero"} {
		want := expected
		switch field {
		case "instance":
			want.instance = strings.Repeat("9", 32)
		case "id":
			want.id = strings.Repeat("9", 32)
		case "publisher":
			want.publisher.Start++
		case "authority":
			want.authority.Generation++
		case "facts":
			want.files.Previous.Identity.Inode++
		case "zero":
			want = replacementExpectation{}
		}
		if recognizeReplacementPrefix(&record, want, prior) != replacementBad {
			t.Fatal("accepted expected mismatch " + field)
		}
	}
}

func TestReplacementShapeDoesNotEstablishAuthority(t *testing.T) {
	record, expected, observed := replacementFixture()
	data, err := encodeReplacementRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeReplacementRecord(data)
	if err != nil {
		t.Fatal(err)
	}
	identity := func(id replacementIdentity) safefs.Identity {
		return safefs.Identity{Device: id.Device, Inode: id.Inode, Owner: id.Owner, Mode: id.Mode, Links: id.Links, Size: id.Size, MountID: id.MountID}
	}
	file := func(f replacementFileFacts) safefs.FileFacts {
		return safefs.FileFacts{Identity: identity(f.Identity), ModifiedSeconds: f.ModifiedSeconds, ModifiedNanoseconds: f.ModifiedNanoseconds,
			ChangedSeconds: f.ChangedSeconds, ChangedNanoseconds: f.ChangedNanoseconds, SHA256: f.SHA256}
	}
	selfDescribed := replacementExpectation{instance: decoded.Instance, id: decoded.ID, publisher: decoded.Publisher, authority: decoded.Authority,
		files: safefs.ReplacementFacts{Parent: identity(decoded.Files.Parent), Target: decoded.Files.Target, Temporary: decoded.Files.Temporary,
			Previous: file(decoded.Files.Previous), Staged: file(decoded.Files.Staged)}}
	// Copying the witness into its expectation can describe a shape. This is
	// deliberately not an admission API: no graph, exclusion or death is proved.
	// A future state owner must never use this self-described comparison as proof.
	if recognizeReplacementPrefix(&decoded, selfDescribed, observed) != retireRecord {
		t.Fatal("shape mismatch")
	}
	wrong := expected
	wrong.authority.Token = strings.Repeat("f", 32)
	if recognizeReplacementPrefix(&decoded, wrong, observed) != replacementBad {
		t.Fatal("witness overrode independent expectation")
	}
}

func FuzzReplacementRecordDecode(f *testing.F) {
	record, _, _ := replacementFixture()
	data, err := encodeReplacementRecord(record)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(data)
	f.Add([]byte("null"))
	f.Add(bytes.Repeat([]byte("x"), replacementRecordLimit+1))
	f.Fuzz(func(t *testing.T, input []byte) {
		saved := bytes.Clone(input)
		got, err := decodeReplacementRecord(input)
		if !bytes.Equal(input, saved) {
			t.Fatal("decoder mutated input")
		}
		if err != nil {
			if got != (replacementRecord{}) {
				t.Fatal("partial result on error")
			}
			return
		}
		encoded, err := encodeReplacementRecord(got)
		if err != nil || !bytes.Equal(encoded, input) || len(encoded) > replacementRecordLimit {
			t.Fatal("noncanonical success")
		}
	})
}
