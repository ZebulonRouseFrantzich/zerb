// Package profile owns static declarations and validation under PC-040/042 and
// ARCH-030. Source-tree observations and acquisition authority belong to source.
package profile

import (
	"errors"
	"strings"
	"unicode/utf8"
)

type Kind string

const (
	Embedded Kind = "embedded"
	Gitlink  Kind = "gitlink"
)

type Declaration struct {
	ID   string
	Kind Kind
	Path string
}

type Manifest struct {
	Schema   int
	Base     string
	Profiles []Declaration
}

type Component struct {
	Schema  int
	Label   string
	HasMise bool
	HasLock bool
	HasHome bool
}

const (
	maxProfiles       = 256
	maxProfileIDBytes = 64
	maxPathBytes      = 1024
	maxPathDepth      = 64
	maxPathComponent  = 255
)

var (
	errManifest  = errors.New("invalid profile manifest")
	errSelection = errors.New("invalid profile selection")
)

// Select returns an exact declared profile without acquiring or granting source
// access. source.Inspection.Select separately rejects selected gitlinks (PC-032).
// Validate the whole mutable value so a forged or edited manifest cannot make
// duplicate declarations, unknown kinds, or out-of-bounds fields selectable.
func Select(manifest Manifest, id string) (Declaration, error) {
	if !validManifest(manifest) {
		return Declaration{}, errManifest
	}
	if !validID(id) {
		return Declaration{}, errSelection
	}
	for _, declaration := range manifest.Profiles {
		if declaration.ID == id {
			return declaration, nil
		}
	}
	return Declaration{}, errSelection
}

func validManifest(manifest Manifest) bool {
	if manifest.Schema != 1 || len(manifest.Profiles) > maxProfiles || !validPath(manifest.Base) {
		return false
	}
	ids := make(map[string]struct{}, len(manifest.Profiles))
	paths := make(map[string]struct{}, len(manifest.Profiles)+1)
	paths[manifest.Base] = struct{}{}
	for _, declaration := range manifest.Profiles {
		if !validID(declaration.ID) || declaration.ID == "base" || (declaration.Kind != Embedded && declaration.Kind != Gitlink) ||
			!validPath(declaration.Path) {
			return false
		}
		if _, duplicate := ids[declaration.ID]; duplicate {
			return false
		}
		if _, duplicate := paths[declaration.Path]; duplicate {
			return false
		}
		ids[declaration.ID] = struct{}{}
		paths[declaration.Path] = struct{}{}
	}
	return true
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > maxProfileIDBytes {
		return false
	}
	for index := range len(id) {
		value := id[index]
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' {
			continue
		}
		if index > 0 && (value == '_' || value == '-') {
			continue
		}
		return false
	}
	return true
}

func validPath(value string) bool {
	if len(value) == 0 || len(value) > maxPathBytes || !utf8.ValidString(value) || strings.ContainsAny(value, "\\\x00") {
		return false
	}
	depth := 0
	for component := range strings.SplitSeq(value, "/") {
		depth++
		if component == "" || component == "." || component == ".." || len(component) > maxPathComponent || depth > maxPathDepth {
			return false
		}
	}
	return true
}
