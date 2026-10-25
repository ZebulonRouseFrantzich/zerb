package state

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func initRecordFixtures() (rootRecord, namespaceOwner, writerOwner) {
	instance, token := strings.Repeat("1", 32), strings.Repeat("2", 32)
	process := processIdentity{123, 456, 789}
	return rootRecord{1, "zerb.root", "initializing", instance, token, process, 0},
		namespaceOwner{1, "zerb.data-owner", instance},
		writerOwner{1, "zerb.writer-owner", "held", instance, 1, token, process, 0, ""}
}

func TestInitRecordCanonicalVectors(t *testing.T) {
	root, owner, writer := initRecordFixtures()
	initialized := root
	initialized.Status, initialized.Token, initialized.Process = "initialized", "", processIdentity{}
	for _, tc := range []struct {
		name   string
		record rootRecord
		want   string
	}{
		{"initializing", root, `{"schema":1,"domain":"zerb.root","status":"initializing","instance":"11111111111111111111111111111111","token":"22222222222222222222222222222222","process":{"pid":123,"start":456,"pid_namespace":789},"generation":0}`},
		{"initialized", initialized, `{"schema":1,"domain":"zerb.root","status":"initialized","instance":"11111111111111111111111111111111","token":"","process":{"pid":0,"start":0,"pid_namespace":0},"generation":0}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := encodeRootRecord(tc.record)
			if err != nil || string(data) != tc.want {
				t.Fatalf("canonical root vector mismatch: %v", err)
			}
			if got, err := decodeRootRecord([]byte(tc.want)); err != nil || got != tc.record {
				t.Fatalf("canonical root decode mismatch: %v", err)
			}
		})
	}
	for _, domain := range []string{"zerb.data-owner", "zerb.cache-owner"} {
		t.Run(domain, func(t *testing.T) {
			r := owner
			r.Domain = domain
			want := `{"schema":1,"domain":"` + domain + `","instance":"11111111111111111111111111111111"}`
			data, err := encodeNamespaceOwner(r)
			if err != nil || string(data) != want {
				t.Fatalf("canonical namespace vector mismatch: %v", err)
			}
			if got, err := decodeNamespaceOwner([]byte(want)); err != nil || got != r {
				t.Fatalf("canonical namespace decode mismatch: %v", err)
			}
		})
	}
	released0 := writer
	released0.Status, released0.Generation, released0.Token, released0.Process = "released", 0, "", processIdentity{}
	released2 := released0
	released2.Generation, released2.PreviousGeneration, released2.PreviousToken = 2, 1, writer.Token
	for _, tc := range []struct {
		name   string
		record writerOwner
		want   string
	}{
		{"released0", released0, `{"schema":1,"domain":"zerb.writer-owner","status":"released","instance":"11111111111111111111111111111111","generation":0,"process":{"pid":0,"start":0,"pid_namespace":0},"previous_generation":0}`},
		{"held1", writer, `{"schema":1,"domain":"zerb.writer-owner","status":"held","instance":"11111111111111111111111111111111","generation":1,"token":"22222222222222222222222222222222","process":{"pid":123,"start":456,"pid_namespace":789},"previous_generation":0}`},
		{"released2", released2, `{"schema":1,"domain":"zerb.writer-owner","status":"released","instance":"11111111111111111111111111111111","generation":2,"process":{"pid":0,"start":0,"pid_namespace":0},"previous_generation":1,"previous_token":"22222222222222222222222222222222"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := encodeWriterOwner(tc.record)
			if err != nil || string(data) != tc.want {
				t.Fatalf("canonical writer vector mismatch: %v", err)
			}
			if got, err := decodeWriterOwner([]byte(tc.want)); err != nil || got != tc.record {
				t.Fatalf("canonical writer decode mismatch: %v", err)
			}
		})
	}
}

func TestInitRecordRejectsNoncanonicalJSON(t *testing.T) {
	root, owner, writer := initRecordFixtures()
	root.Status, root.Token, root.Process = "initialized", "", processIdentity{}
	writer.Status, writer.Generation, writer.Token, writer.Process = "released", 0, "", processIdentity{}
	rootBytes, err := encodeRootRecord(root)
	if err != nil {
		t.Fatal(err)
	}
	ownerBytes, err := encodeNamespaceOwner(owner)
	if err != nil {
		t.Fatal(err)
	}
	writerBytes, err := encodeWriterOwner(writer)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		data   []byte
		reject func([]byte) bool
	}{
		{"root", rootBytes, func(data []byte) bool {
			r, err := decodeRootRecord(data)
			return err == errInitRecord && r == (rootRecord{})
		}},
		{"namespace", ownerBytes, func(data []byte) bool {
			r, err := decodeNamespaceOwner(data)
			return err == errInitRecord && r == (namespaceOwner{})
		}},
		{"writer", writerBytes, func(data []byte) bool {
			r, err := decodeWriterOwner(data)
			return err == errInitRecord && r == (writerOwner{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := string(tc.data)
			cases := map[string]string{
				"empty": "", "null": "null", "array": "[]", "whitespace": " " + s, "newline": s + "\n",
				"multiple": s + s, "trailing": s + "x", "unknown": strings.Replace(s, `"schema":1`, `"extra":0,"schema":1`, 1),
				"duplicate":       strings.Replace(s, `"schema":1`, `"schema":1,"schema":1`, 1),
				"alias":           strings.Replace(s, `"schema"`, `"Schema"`, 1),
				"escaped key":     strings.Replace(s, `"schema"`, `"\u0073chema"`, 1),
				"escaped string":  strings.Replace(s, `"zerb.`, `"\u007aerb.`, 1),
				"null scalar":     strings.Replace(s, `"schema":1`, `"schema":null`, 1),
				"float integer":   strings.Replace(s, `"schema":1`, `"schema":1.0`, 1),
				"overflow int":    strings.Replace(s, `"schema":1`, `"schema":18446744073709551615`, 1),
				"limit minus one": strings.Repeat(" ", initRecordLimit-1),
				"limit":           strings.Repeat(" ", initRecordLimit),
				"over limit":      strings.Repeat(" ", initRecordLimit+1),
			}
			afterSchema := strings.TrimPrefix(s, `{"schema":1,`)
			domainEnd := strings.IndexByte(afterSchema, ',')
			cases["reordered"] = "{" + afterSchema[:domainEnd] + `,"schema":1` + afterSchema[domainEnd:]
			// Omit each required field while retaining the rest of the exact canonical order.
			fields := []string{`"schema":1,`, `"domain":"` + owner.Domain + `",`, `"instance":"` + owner.Instance + `"`}
			if tc.name != "namespace" {
				domain := "zerb.root"
				status := "initialized"
				if tc.name == "writer" {
					domain, status = "zerb.writer-owner", "released"
				}
				fields[1] = `"domain":"` + domain + `",`
				fields[2] += ","
				fields = append(fields, `"status":"`+status+`",`, `"process":{"pid":0,"start":0,"pid_namespace":0},`, `"pid":0,`, `"start":0,`, `,"pid_namespace":0`)
				cases["null process"] = strings.Replace(s, `{"pid":0,"start":0,"pid_namespace":0}`, `null`, 1)
				cases["nested duplicate"] = strings.Replace(s, `"pid":0`, `"pid":0,"pid":0`, 1)
				cases["nested unknown"] = strings.Replace(s, `"pid":0`, `"extra":0,"pid":0`, 1)
				cases["nested alias"] = strings.Replace(s, `"pid":0`, `"PID":0`, 1)
				cases["overflow uint"] = strings.Replace(s, `"start":0`, `"start":18446744073709551616`, 1)
				cases["negative uint"] = strings.Replace(s, `"start":0`, `"start":-1`, 1)
				cases["null zero scalar"] = strings.Replace(s, `"start":0`, `"start":null`, 1)
				if tc.name == "root" {
					fields = append(fields, `"token":"",`, `,"generation":0`)
					cases["null empty token"] = strings.Replace(s, `"token":""`, `"token":null`, 1)
				} else {
					fields = append(fields, `"generation":0,`, `,"previous_generation":0`)
					cases["explicit empty token"] = strings.Replace(s, `"process":`, `"token":"","process":`, 1)
					cases["null omitted token"] = strings.Replace(s, `"process":`, `"token":null,"process":`, 1)
					cases["explicit empty previous token"] = strings.TrimSuffix(s, "}") + `,"previous_token":""}`
					cases["null omitted previous token"] = strings.TrimSuffix(s, "}") + `,"previous_token":null}`
				}
			} else {
				fields[2] = "," + fields[2]
			}
			for _, field := range fields {
				if !strings.Contains(s, field) {
					t.Fatalf("missing corpus field %s", field)
				}
				cases["missing "+field] = strings.Replace(s, field, "", 1)
			}
			for name, value := range cases {
				t.Run(name, func(t *testing.T) {
					input := []byte(value)
					saved := bytes.Clone(input)
					if !tc.reject(input) {
						t.Fatal("accepted unsupported representation or returned partial record")
					}
					if !bytes.Equal(input, saved) {
						t.Fatal("decoder mutated input")
					}
				})
			}
		})
	}
}

func TestInitRootScalarAndStatusBoundaries(t *testing.T) {
	base, _, _ := initRecordFixtures()
	cases := map[string]func(*rootRecord){
		"schema":                func(r *rootRecord) { r.Schema = 2 },
		"domain":                func(r *rootRecord) { r.Domain = "zerb.data-owner" },
		"status":                func(r *rootRecord) { r.Status = "unknown" },
		"instance":              func(r *rootRecord) { r.Instance = "" },
		"uppercase instance":    func(r *rootRecord) { r.Instance = strings.Repeat("A", 32) },
		"long instance":         func(r *rootRecord) { r.Instance += "a" },
		"oversize instance":     func(r *rootRecord) { r.Instance = strings.Repeat("1", initRecordLimit+1) },
		"empty token":           func(r *rootRecord) { r.Token = "" },
		"short token":           func(r *rootRecord) { r.Token = strings.Repeat("2", 31) },
		"uppercase token":       func(r *rootRecord) { r.Token = strings.Repeat("A", 32) },
		"token collision":       func(r *rootRecord) { r.Token = r.Instance },
		"zero pid":              func(r *rootRecord) { r.Process.PID = 0 },
		"negative pid":          func(r *rootRecord) { r.Process.PID = -1 },
		"zero start":            func(r *rootRecord) { r.Process.Start = 0 },
		"zero namespace":        func(r *rootRecord) { r.Process.PIDNamespace = 0 },
		"generation":            func(r *rootRecord) { r.Generation = 1 },
		"generation overflow":   func(r *rootRecord) { r.Generation = math.MaxUint64 },
		"initialized authority": func(r *rootRecord) { r.Status = "initialized" },
		"initialized token":     func(r *rootRecord) { r.Status, r.Process = "initialized", processIdentity{} },
		"initialized process":   func(r *rootRecord) { r.Status, r.Token = "initialized", "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			if data, err := encodeRootRecord(r); err != errInitRecord || data != nil {
				t.Fatal("encoded invalid root")
			}
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := decodeRootRecord(data); err != errInitRecord || got != (rootRecord{}) {
				t.Fatal("decoded invalid root")
			}
		})
	}
	base.Process = processIdentity{int(^uint(0) >> 1), math.MaxUint64, math.MaxUint64}
	if _, err := encodeRootRecord(base); err != nil {
		t.Fatal("refused positive process scalar boundaries")
	}
}

func TestInitNamespaceScalarBoundaries(t *testing.T) {
	_, base, _ := initRecordFixtures()
	for name, mutate := range map[string]func(*namespaceOwner){
		"schema":             func(r *namespaceOwner) { r.Schema = 2 },
		"domain":             func(r *namespaceOwner) { r.Domain = "zerb.root" },
		"instance":           func(r *namespaceOwner) { r.Instance = "" },
		"uppercase instance": func(r *namespaceOwner) { r.Instance = strings.Repeat("A", 32) },
		"long instance":      func(r *namespaceOwner) { r.Instance += "a" },
	} {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			if data, err := encodeNamespaceOwner(r); err != errInitRecord || data != nil {
				t.Fatal("encoded invalid namespace")
			}
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			if got, err := decodeNamespaceOwner(data); err != errInitRecord || got != (namespaceOwner{}) {
				t.Fatal("decoded invalid namespace")
			}
		})
	}
}

func TestInitWriterScalarAndStatusBoundaries(t *testing.T) {
	_, _, held := initRecordFixtures()
	released0 := held
	released0.Status, released0.Generation, released0.Token, released0.Process = "released", 0, "", processIdentity{}
	released2 := released0
	released2.Generation, released2.PreviousGeneration, released2.PreviousToken = 2, 1, held.Token
	for _, base := range []writerOwner{released0, held, released2} {
		t.Run(base.Status+string(rune('0'+base.Generation)), func(t *testing.T) {
			cases := map[string]func(*writerOwner){
				"schema":                   func(r *writerOwner) { r.Schema = 2 },
				"domain":                   func(r *writerOwner) { r.Domain = "zerb.root" },
				"status":                   func(r *writerOwner) { r.Status = "unknown" },
				"instance":                 func(r *writerOwner) { r.Instance = "" },
				"uppercase instance":       func(r *writerOwner) { r.Instance = strings.Repeat("A", 32) },
				"long instance":            func(r *writerOwner) { r.Instance += "a" },
				"unsupported generation":   func(r *writerOwner) { r.Generation = 3 },
				"generation overflow":      func(r *writerOwner) { r.Generation = math.MaxUint64 },
				"wrong predecessor":        func(r *writerOwner) { r.PreviousGeneration++ },
				"token collision":          func(r *writerOwner) { r.Token = r.Instance },
				"previous token collision": func(r *writerOwner) { r.PreviousToken = r.Instance },
			}
			if base.Status == "held" {
				cases["zero pid"] = func(r *writerOwner) { r.Process.PID = 0 }
				cases["negative pid"] = func(r *writerOwner) { r.Process.PID = -1 }
				cases["zero start"] = func(r *writerOwner) { r.Process.Start = 0 }
				cases["zero namespace"] = func(r *writerOwner) { r.Process.PIDNamespace = 0 }
				cases["empty token"] = func(r *writerOwner) { r.Token = "" }
				cases["uppercase token"] = func(r *writerOwner) { r.Token = strings.Repeat("A", 32) }
				cases["short token"] = func(r *writerOwner) { r.Token = strings.Repeat("2", 31) }
				cases["previous token"] = func(r *writerOwner) { r.PreviousToken = strings.Repeat("3", 32) }
				cases["held0"] = func(r *writerOwner) { r.Generation = 0 }
			} else {
				cases["live process"] = func(r *writerOwner) { r.Process = held.Process }
				cases["partial process"] = func(r *writerOwner) { r.Process.Start = 1 }
				cases["live token"] = func(r *writerOwner) { r.Token = held.Token }
				cases["released1"] = func(r *writerOwner) { r.Generation = 1 }
				if base.Generation == 0 {
					cases["previous token"] = func(r *writerOwner) { r.PreviousToken = held.Token }
				} else {
					cases["empty previous token"] = func(r *writerOwner) { r.PreviousToken = "" }
					cases["uppercase previous token"] = func(r *writerOwner) { r.PreviousToken = strings.Repeat("A", 32) }
					cases["short previous token"] = func(r *writerOwner) { r.PreviousToken = strings.Repeat("2", 31) }
				}
			}
			for name, mutate := range cases {
				t.Run(name, func(t *testing.T) {
					r := base
					mutate(&r)
					if data, err := encodeWriterOwner(r); err != errInitRecord || data != nil {
						t.Fatal("encoded invalid writer")
					}
					data, err := json.Marshal(r)
					if err != nil {
						t.Fatal(err)
					}
					if got, err := decodeWriterOwner(data); err != errInitRecord || got != (writerOwner{}) {
						t.Fatal("decoded invalid writer")
					}
				})
			}
		})
	}
}
