package profile_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ZebulonRouseFrantzich/zerb/internal/profile"
)

func manifest() profile.Manifest {
	return profile.Manifest{
		Schema: 1,
		Base:   "profiles/base",
		Profiles: []profile.Declaration{
			{ID: "work", Kind: profile.Embedded, Path: "profiles/work"},
			{ID: "public-gitlink", Kind: profile.Gitlink, Path: "profiles/gitlink"},
		},
	}
}

func TestSelectDeclaredProfilesWithoutChangingManifest(t *testing.T) {
	value := manifest()
	before := manifest()
	for _, expected := range before.Profiles {
		got, err := profile.Select(value, expected.ID)
		if err != nil || got != expected {
			t.Fatalf("select declared %s: got %#v, error %v", expected.Kind, got, err)
		}
		got.ID = "changed-copy"
	}
	if !reflect.DeepEqual(value, before) {
		t.Fatal("selection changed manifest or declaration order")
	}
}

func TestSelectRejectsInvalidManifestBeforeReturningAnyDeclaration(t *testing.T) {
	cases := []struct {
		name   string
		change func(*profile.Manifest)
	}{
		{"missing-schema", func(m *profile.Manifest) { m.Schema = 0 }},
		{"future-schema", func(m *profile.Manifest) { m.Schema = 2 }},
		{"missing-base", func(m *profile.Manifest) { m.Base = "" }},
		{"invalid-base", func(m *profile.Manifest) { m.Base = "../outside" }},
		{"missing-id", func(m *profile.Manifest) { m.Profiles[1].ID = "" }},
		{"invalid-unselected-id", func(m *profile.Manifest) { m.Profiles[1].ID = "UPPER" }},
		{"duplicate-id", func(m *profile.Manifest) { m.Profiles[1].ID = m.Profiles[0].ID }},
		{"base-id-not-selectable", func(m *profile.Manifest) { m.Profiles[1].ID = "base" }},
		{"missing-kind", func(m *profile.Manifest) { m.Profiles[1].Kind = "" }},
		{"unknown-kind", func(m *profile.Manifest) { m.Profiles[1].Kind = "private" }},
		{"missing-path", func(m *profile.Manifest) { m.Profiles[1].Path = "" }},
		{"invalid-unselected-path", func(m *profile.Manifest) { m.Profiles[1].Path = "bad//path" }},
		{"duplicate-path", func(m *profile.Manifest) { m.Profiles[1].Path = m.Profiles[0].Path }},
		{"base-path-reused", func(m *profile.Manifest) { m.Profiles[1].Path = m.Base }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value := manifest()
			test.change(&value)
			got, err := profile.Select(value, "work")
			if err == nil || got != (profile.Declaration{}) {
				t.Fatalf("invalid manifest returned selection %#v, error %v", got, err)
			}
		})
	}
}

func TestSelectProfileCountBound(t *testing.T) {
	for _, count := range []int{255, 256, 257} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			value := profile.Manifest{Schema: 1, Base: "base-source"}
			for index := range count {
				value.Profiles = append(value.Profiles, profile.Declaration{
					ID: fmt.Sprintf("p%d", index), Kind: profile.Embedded, Path: fmt.Sprintf("profiles/%d", index),
				})
			}
			_, err := profile.Select(value, "p0")
			if (err == nil) != (count <= 256) {
				t.Fatalf("profile count %d: %v", count, err)
			}
		})
	}
}

func TestSelectIDGrammarAndByteBound(t *testing.T) {
	cases := []struct {
		id    string
		valid bool
	}{
		{"a", true}, {"0", true}, {"x_y-9", true}, {"base-profile", true},
		{strings.Repeat("a", 63), true}, {strings.Repeat("a", 64), true}, {strings.Repeat("a", 65), false},
		{"", false}, {"_leading", false}, {"-leading", false}, {"Upper", false}, {"a.b", false},
		{"white space", false}, {"\x00", false}, {"é", false}, {string([]byte{0xff}), false},
	}
	for index, test := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			value := manifest()
			value.Profiles[0].ID = test.id
			got, err := profile.Select(value, test.id)
			if (err == nil) != test.valid || (err == nil && got.ID != test.id) {
				t.Fatalf("ID grammar acceptance: got %#v, error %v", got, err)
			}
		})
	}
}

func logicalPath(size int) string {
	parts := []string{}
	for size > 255 {
		parts = append(parts, strings.Repeat("a", 254))
		size -= 255
	}
	parts = append(parts, strings.Repeat("a", size))
	return strings.Join(parts, "/")
}

func TestSelectLogicalPathBoundsAndAliases(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		valid bool
	}{
		{"one-component", "settings", true}, {"unicode", "profiles/日本語", true},
		{"no-speculative-containment-ban", "profiles/base/child", true},
		{"bytes-minus-one", logicalPath(1023), true}, {"bytes-max", logicalPath(1024), true},
		{"bytes-plus-one", logicalPath(1025), false},
		{"depth-minus-one", strings.Repeat("p/", 62) + "p", true},
		{"depth-max", strings.Repeat("p/", 63) + "p", true},
		{"depth-plus-one", strings.Repeat("p/", 64) + "p", false},
		{"component-minus-one", strings.Repeat("a", 254), true},
		{"component-max", strings.Repeat("a", 255), true},
		{"component-plus-one", strings.Repeat("a", 256), false},
		{"unicode-component-max", strings.Repeat("é", 127) + "a", true},
		{"unicode-component-plus-one", strings.Repeat("é", 128), false},
		{"empty", "", false}, {"absolute", "/outside", false}, {"dot", ".", false},
		{"dot-component", "p/./q", false}, {"dot-dot", "..", false}, {"traversal", "p/../q", false},
		{"empty-component", "p//q", false}, {"trailing-slash", "p/", false},
		{"backslash", "p\\q", false}, {"nul", "p\x00q", false}, {"invalid-utf8", string([]byte{0xff}), false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value := manifest()
			value.Profiles[0].Path = test.path
			_, err := profile.Select(value, "work")
			if (err == nil) != test.valid {
				t.Fatalf("logical path acceptance: %v", err)
			}
		})
	}
}

func TestSelectCannotImplicitlySelectBaseOrUnknownID(t *testing.T) {
	for _, id := range []string{"", "base", "unknown", "work/child", strings.Repeat("x", 65)} {
		got, err := profile.Select(manifest(), id)
		if err == nil || got != (profile.Declaration{}) {
			t.Fatalf("undeclared selection returned %#v, error %v", got, err)
		}
	}
	_, err := profile.Select(profile.Manifest{Schema: 1, Base: "base-source"}, "work")
	if err == nil {
		t.Fatal("empty profile list became selectable")
	}
}

func TestSelectErrorsDoNotExposeAuthoredMetadata(t *testing.T) {
	const marker = "AuthoredPrivateMetadata"
	value := manifest()
	value.Profiles[1].Path = "../" + marker
	for _, input := range []struct {
		manifest profile.Manifest
		id       string
	}{{value, "work"}, {manifest(), marker}} {
		_, err := profile.Select(input.manifest, input.id)
		if err == nil || strings.Contains(err.Error(), marker) {
			t.Fatalf("selection error exposed metadata: %v", err)
		}
	}
}
