package profile

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const validManifestText = "schema=1\n[base]\npath='profiles/base'\n[[profiles]]\nid='personal'\nkind='embedded'\npath='profiles/personal'\n"

func TestParseManifestRepresentations(t *testing.T) {
	cases := []struct {
		name, source string
		count        int
	}{
		{"tables", validManifestText, 1},
		{"quoted-keys", "\"schema\"=+1\n['base']\n\"pa\\u0074h\"='profiles/base'\n[['profiles']]\n'id'='personal'\n\"kind\"='embedded'\npath='profiles/personal'\n", 1},
		{"inline", "schema=0x1\nbase={path='profiles/base'}\nprofiles=[{id='personal',kind='embedded',path='profiles/personal'}]\n", 1},
		{"inline-profiles-table-base", "schema=0o1\nprofiles=[{id='personal',kind='embedded',path='profiles/personal'}]\n[base]\npath='profiles/base'\n", 1},
		{"inline-base-table-profiles", "schema=0b1\nbase={path='profiles/base'}\n[[profiles]]\nid='personal'\nkind='embedded'\npath='profiles/personal'\n", 1},
		{"zero-profiles", "schema=1\n[base]\npath='profiles/base'\n", 0},
		{"empty-profiles-array", "schema=1\nprofiles=[]\n[base]\npath='profiles/base'\n", 0},
		{"multiline-path", "schema=1\nbase={path='''\nprofiles/base'''}\nprofiles=[{id=\"\"\"personal\"\"\",kind='gitlink',path='profiles/personal'}]\n", 1},
		{"crlf", strings.ReplaceAll(validManifestText, "\n", "\r\n"), 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := []byte(test.source)
			before := bytes.Clone(input)
			got, err := ParseManifest(input)
			if err != nil || got.Schema != 1 || got.Base != "profiles/base" || len(got.Profiles) != test.count {
				t.Fatalf("manifest shape: got %#v, error %v", got, err)
			}
			if !bytes.Equal(input, before) {
				t.Fatal("parser changed caller input")
			}
			clear(input)
			if got.Base != "profiles/base" || test.count > 0 && got.Profiles[0].ID != "personal" {
				t.Fatal("result aliases caller input")
			}
		})
	}
}

func TestParseManifestPreservesOrderAndStaticGitlink(t *testing.T) {
	source := "schema=1\nbase={path='base'}\nprofiles=[{id='z',kind='gitlink',path='z'},{id='a',kind='embedded',path='a'}]"
	got, err := ParseManifest([]byte(source))
	if err != nil || len(got.Profiles) != 2 || got.Profiles[0].ID != "z" || got.Profiles[0].Kind != Gitlink || got.Profiles[1].ID != "a" {
		t.Fatalf("static declarations changed: %#v, %v", got, err)
	}
}

func TestParseManifestRejectsMalformedOrUnclosedShapes(t *testing.T) {
	cases := []string{
		"", "schema=2\nbase={path='b'}", "schema=1.0\nbase={path='b'}", "schema='1'\nbase={path='b'}",
		"base={path='b'}", "schema=1", "schema=1\nbase={}", "schema=1\n[base]",
		"schema=1\nbase.path='b'", "schema=1\n'base.path'='b'", "schema=1\n[base.nested]\npath='b'",
		"schema=1\nbase={nested.path='b'}", "schema=1\nbase={path='b',unknown=1}",
		"schema=1\n[base]\npath='b'\n[base]\npath='b'", "schema=1\nbase={path='b'}\n[base]\npath='b'",
		"schema=1\nschema=1\nbase={path='b'}", "schema=1\n'schema'=1\nbase={path='b'}",
		"schema=1\nbase={path='b',path='c'}", "schema=1\nbase={path='b',\"pa\\u0074h\"='c'}",
		"schema=1\nbase={path=1}", "schema=1\nbase='b'", "schema=1\nbase=[{path='b'}]",
		"schema=1\nbase={path='b'}\nprofiles={id='x',kind='embedded',path='x'}",
		"schema=1\nbase={path='b'}\nprofiles=[{}]", "schema=1\nbase={path='b'}\nprofiles=['x']",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',kind='embedded'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',path='x'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{kind='embedded',path='x'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',kind='embedded',path=1}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id=1,kind='embedded',path='x'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',kind=1,path='x'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',kind='private',path='x'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',kind='embedded',path='x',extra=true}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',id='y',kind='embedded',path='x'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id.name='x',kind='embedded',path='x'}]",
		"schema=1\nbase={path='b'}\nprofiles=[]\n[[profiles]]\nid='x'\nkind='embedded'\npath='x'",
		"schema=1\nbase={path='b'}\n[[profiles]]\nid='x'\nkind='embedded'\npath='x'\nprofiles=[]",
		"schema=1\nbase={path='b'}\n[[profiles]]", "schema=1\nbase={path='b'}\n[profiles]",
		"schema=1\nbase={path='b'}\n[unknown]", "schema=1\nbase={path='b'}\n[base.unknown]",
		"Schema=1\nbase={path='b'}", "schema=1\nBase={path='b'}", "schema=1\nbase={Path='b'}",
		"schema=1\nbase={path='b'}\nextra=true", "schema=1\nbase={path='b'}\nnot TOML",
		"schema=1\nbase={path='b'}\nprofiles=[[[[]]]]", "schema=1\nbase={path='b'",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',kind='embedded',path='x'},{id='x',kind='embedded',path='y'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',kind='embedded',path='x'},{id='y',kind='embedded',path='x'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='x',kind='embedded',path='b'}]",
		"schema=1\nbase={path='b'}\nprofiles=[{id='base',kind='embedded',path='x'}]",
	}
	for index, source := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			got, err := ParseManifest([]byte(source))
			if err == nil || !reflect.DeepEqual(got, Manifest{}) {
				t.Fatalf("invalid input returned %#v, %v", got, err)
			}
		})
	}
}

func TestParseComponentRepresentations(t *testing.T) {
	cases := []struct{ source, label string }{
		{"schema=1", ""}, {"'schema'=0x1\n\"label\"='Personal'", "Personal"},
		{"schema=1\nlabel=\"\"\"\n# [unknown]\nkey = 'value.with.dots'\"\"\"", "# [unknown]\nkey = 'value.with.dots'"},
		{"schema=1\nlabel='''\n# [ ] { } = .\n'''", "# [ ] { } = .\n"},
		{"schema=1\nlabel=\"\"\"one\\\n   two\"\"\"", "onetwo"},
		{"schema=1\nlabel=\"\"\"quote\"\"\"\"", "quote\""}, {"schema=1\nlabel=\"\"\"quotes\"\"\"\"\"", "quotes\"\""},
		{"schema=1\nlabel='''quote''''", "quote'"}, {"schema=1\nlabel='''quotes'''''", "quotes''"},
		{"schema=1\nlabel=\"a\\\"#[]{}=b\"", "a\"#[]{}=b"}, {"schema=1\nlabel=\"\\u0000\"", "\x00"},
	}
	for index, test := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			input := []byte(test.source)
			before := bytes.Clone(input)
			got, err := ParseComponent(input)
			if err != nil || got != (Component{Schema: 1, Label: test.label}) {
				t.Fatalf("component: %#v, %v", got, err)
			}
			if !bytes.Equal(input, before) {
				t.Fatal("parser changed input")
			}
			clear(input)
			if got.Label != test.label {
				t.Fatal("component result aliases input")
			}
		})
	}
}

func TestParseComponentRejectsUnknownMalformedAndWrongTypes(t *testing.T) {
	cases := []string{
		"", "label='x'", "schema=0", "schema=2", "schema='1'", "schema=1.0", "schema=true",
		"schema=1\nschema=1", "schema=1\n'schema'=1", "Schema=1", "schema=1\nLabel='x'",
		"schema=1\nlabel=1", "schema=1\nlabel=[]", "schema=1\nlabel={}", "schema=1\nlabel='x'\nlabel='y'",
		"schema=1\nlabel.name='x'", "schema=1\n'label.name'='x'", "schema=1\n[unknown]", "schema=1\n[[unknown]]",
		"schema=1\nhas_mise=true", "schema=1\nhas_lock=true", "schema=1\nhas_home=true", "schema=1\nprovider='x'",
		"schema=1\nlabel='unterminated", "schema=1\nlabel=\"line\nline\"", "schema=1\nlabel=\"bad\\q\"",
		"schema=1\nlabel=\"\"\"too many\"\"\"\"\"\"", "schema=1\nlabel=\"terminal\\", "schema=1\nlabel='raw\x00'",
		"schema=1\nlabel=[" + strings.Repeat("0,", 10000) + "0]", "schema=1\nlabel=" + strings.Repeat("[", 10000) + strings.Repeat("]", 10000),
	}
	for index, source := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			got, err := ParseComponent([]byte(source))
			if err == nil || got != (Component{}) {
				t.Fatalf("invalid component returned %#v, %v", got, err)
			}
		})
	}
}

func profileDocuments(count int, inline bool) string {
	var out strings.Builder
	if inline {
		out.WriteString("schema=1\nbase={path='base'}\nprofiles=[")
	} else {
		out.WriteString("schema=1\n[base]\npath='base'\n")
	}
	for index := range count {
		if inline {
			if index != 0 {
				out.WriteByte(',')
			}
			fmt.Fprintf(&out, "{id='p%d',kind='embedded',path='p%d'}", index, index)
		} else {
			fmt.Fprintf(&out, "[[profiles]]\nid='p%d'\nkind='embedded'\npath='p%d'\n", index, index)
		}
	}
	if inline {
		out.WriteByte(']')
	}
	return out.String()
}

func TestParseProfileCountAndMaximumShapeTokenCounts(t *testing.T) {
	for _, inline := range []bool{false, true} {
		for _, count := range []int{255, 256, 257} {
			t.Run(fmt.Sprintf("inline-%v-count-%d", inline, count), func(t *testing.T) {
				data := []byte(profileDocuments(count, inline))
				got, err := ParseManifest(data)
				if (err == nil) != (count <= 256) || err == nil && len(got.Profiles) != count {
					t.Fatalf("count acceptance: %d, %v", len(got.Profiles), err)
				}
				if count == 256 {
					tokens, _, ok := documentCounts(data)
					expected := uint64(3593)
					if inline {
						expected = 3597
					}
					if !ok || tokens != expected {
						t.Fatalf("maximum shape tokens: %d, expected %d, lexical success %v", tokens, expected, ok)
					}
				}
			})
		}
	}
}

func TestParseDecodedStringBounds(t *testing.T) {
	for _, size := range []int{255, 256, 257} {
		for _, escaped := range []bool{false, true} {
			label := strings.Repeat("a", size)
			if escaped {
				label = strings.Repeat("\\u0061", size)
			}
			got, err := ParseComponent([]byte("schema=1\nlabel=\"" + label + "\""))
			if (err == nil) != (size <= 256) || err == nil && len(got.Label) != size {
				t.Fatalf("label %d escaped %v: %v", size, escaped, err)
			}
		}
	}
	for _, size := range []int{63, 64, 65} {
		source := "schema=1\nbase={path='base'}\nprofiles=[{id='" + strings.Repeat("a", size) + "',kind='embedded',path='path'}]"
		_, err := ParseManifest([]byte(source))
		if (err == nil) != (size <= 64) {
			t.Fatalf("ID bytes %d: %v", size, err)
		}
	}
	for _, size := range []int{254, 255, 256} {
		_, err := ParseManifest([]byte("schema=1\nbase={path='" + strings.Repeat("a", size) + "'}"))
		if (err == nil) != (size <= 255) {
			t.Fatalf("path component bytes %d: %v", size, err)
		}
	}
	for _, label := range []struct {
		value string
		valid bool
	}{{strings.Repeat("é", 128), true}, {strings.Repeat("é", 128) + "a", false}} {
		_, err := ParseComponent([]byte("schema=1\nlabel='" + label.value + "'"))
		if (err == nil) != label.valid {
			t.Fatalf("Unicode label bytes: %v", err)
		}
	}
}

func TestParseLogicalPathByteDepthAndGrammarBounds(t *testing.T) {
	for _, size := range []int{1023, 1024, 1025} {
		parts := []string{}
		remaining := size
		for remaining > 255 {
			parts = append(parts, strings.Repeat("a", 254))
			remaining -= 255
		}
		parts = append(parts, strings.Repeat("a", remaining))
		_, err := ParseManifest([]byte("schema=1\nbase={path='" + strings.Join(parts, "/") + "'}"))
		if (err == nil) != (size <= 1024) {
			t.Fatalf("logical path bytes %d: %v", size, err)
		}
	}
	for _, depth := range []int{63, 64, 65} {
		path := strings.Repeat("p/", depth-1) + "p"
		_, err := ParseManifest([]byte("schema=1\nbase={path='" + path + "'}"))
		if (err == nil) != (depth <= 64) {
			t.Fatalf("logical path depth %d: %v", depth, err)
		}
	}
	for _, path := range []string{"", "/absolute", ".", "..", "a/./b", "a/../b", "a//b", "a/", `a\b`} {
		if _, err := ParseManifest([]byte("schema=1\nbase={path='" + path + "'}")); err == nil {
			t.Fatalf("invalid logical path accepted: %q", path)
		}
	}
}

func chunkedCommentDocument(prefix string, size int) []byte {
	data := make([]byte, 0, size)
	data = append(data, prefix...)
	for len(data) < size {
		remaining := min(80, size-len(data))
		data = append(data, '#')
		for range remaining - 1 {
			data = append(data, 'x')
		}
		if len(data) < size {
			data[len(data)-1] = '\n'
		}
	}
	return data
}

func TestParseDocumentByteBoundaryAndCombinedBudget(t *testing.T) {
	for _, size := range []int{maxDocumentBytes - 1, maxDocumentBytes, maxDocumentBytes + 1} {
		_, manifestErr := ParseManifest(chunkedCommentDocument(validManifestText, size))
		_, componentErr := ParseComponent(chunkedCommentDocument("schema=1\n", size))
		if (manifestErr == nil) != (size <= maxDocumentBytes) || (componentErr == nil) != (size <= maxDocumentBytes) {
			t.Fatalf("document bytes %d: %v, %v", size, manifestErr, componentErr)
		}
	}
	// The joint reservation includes diagnostic line context, not just input size.
	data := []byte("schema=1\n#" + strings.Repeat("x", maxDocumentBytes-len("schema=1\n#")))
	if _, err := ParseComponent(data); err == nil {
		t.Fatal("oversized aggregate reservation accepted")
	}
}

func TestDocumentLexicalReservationCounts(t *testing.T) {
	for _, count := range []int{4095, 4096, 4097} {
		tokens, quoted, ok := documentCounts(bytes.Repeat([]byte("= "), count))
		if ok != (count <= 4096) || tokens != uint64(count) || quoted != 0 {
			t.Fatalf("token boundary %d: %d %d %v", count, tokens, quoted, ok)
		}
	}
	data := []byte("label='''# [x]\n''' # ignored []{}\nother=\"a\\\"b\"")
	tokens, quoted, ok := documentCounts(data)
	if !ok || tokens != 6 || quoted != uint64(len("'''# [x]\n'''")+len("\"a\\\"b\"")) {
		t.Fatalf("string reservation counts: %d, %d, %v", tokens, quoted, ok)
	}
	for _, source := range []string{"[{}]", "[[x]]", "{x=[0]}"} {
		if _, _, ok := documentCounts([]byte(source)); !ok {
			t.Fatalf("two-level container rejected: %s", source)
		}
	}
	for _, source := range []string{"[[[]]]", "[}", "{]", "[", "]"} {
		if _, _, ok := documentCounts([]byte(source)); ok {
			t.Fatalf("invalid container accepted: %s", source)
		}
	}
	allocations := testing.AllocsPerRun(10, func() { documentCounts(data) })
	if allocations != 0 {
		t.Fatalf("lexical guard allocated: %v", allocations)
	}
}

func TestParseFailuresAreZeroAndRedacted(t *testing.T) {
	const marker = "PrivateMetadataMarker"
	for _, input := range [][]byte{[]byte("schema=1\nlabel='" + marker + "'\nunknown=true"), []byte("schema=1\nlabel='" + marker), {0xff}, []byte("schema=1\x00")} {
		manifest, manifestErr := ParseManifest(input)
		component, componentErr := ParseComponent(input)
		if manifestErr == nil || componentErr == nil || !reflect.DeepEqual(manifest, Manifest{}) || component != (Component{}) {
			t.Fatal("failed parse returned a model")
		}
		if manifestErr.Error() != "invalid profile manifest" || componentErr.Error() != "invalid profile component" || strings.Contains(manifestErr.Error()+componentErr.Error(), marker) {
			t.Fatal("error exposed authored metadata")
		}
	}
}

func FuzzParseDocuments(f *testing.F) {
	for _, seed := range []string{validManifestText, "schema=1\nlabel='Personal'", "schema=1\nbase={path='b'}\nprofiles=[]", "schema=1\nlabel=\"\"\"# [x]\n\"\"\"", "schema=1\nbase.path='b'", "schema=1\nlabel=[[[[]]]]", "schema=1\nlabel='''quote''''"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, input []byte) {
		before := bytes.Clone(input)
		manifest, err := ParseManifest(input)
		if err != nil && !reflect.DeepEqual(manifest, Manifest{}) {
			t.Fatal("invalid manifest returned data")
		}
		if err == nil && !validManifest(manifest) {
			t.Fatal("parser returned an invalid manifest")
		}
		component, err := ParseComponent(input)
		if err != nil && component != (Component{}) {
			t.Fatal("invalid component returned data")
		}
		if err == nil && (component.Schema != 1 || len(component.Label) > maxLabelBytes || component.HasMise || component.HasLock || component.HasHome) {
			t.Fatal("invalid component facts")
		}
		if !bytes.Equal(input, before) {
			t.Fatal("parser changed caller input")
		}
	})
}
