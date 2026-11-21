package profile

import (
	"bytes"
	"errors"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

const (
	maxDocumentBytes   = 1 << 20
	maxDocumentNesting = 2
	maxLabelBytes      = 256
)

var errComponent = errors.New("invalid profile component")

type manifestDocument struct {
	Schema int `toml:"schema"`
	Base   struct {
		Path string `toml:"path"`
	} `toml:"base"`
	Profiles []struct {
		ID   string `toml:"id"`
		Kind Kind   `toml:"kind"`
		Path string `toml:"path"`
	} `toml:"profiles"`
}

type componentDocument struct {
	Schema int    `toml:"schema"`
	Label  string `toml:"label"`
}

// ParseManifest validates the closed PC-040 shape before strict decoding.
// Shape validation excludes every unknown field/table so Decode can construct
// at most one ordinary error, rather than accumulated strict missing errors.
func ParseManifest(data []byte) (Manifest, error) {
	if !guardDocument(data) || !manifestShape(data) {
		return Manifest{}, errManifest
	}
	var document manifestDocument
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&document); err != nil {
		return Manifest{}, errManifest
	}
	result := Manifest{Schema: document.Schema, Base: document.Base.Path, Profiles: make([]Declaration, len(document.Profiles))}
	for index, declaration := range document.Profiles {
		result.Profiles[index] = Declaration{ID: declaration.ID, Kind: declaration.Kind, Path: declaration.Path}
	}
	if !validManifest(result) {
		return Manifest{}, errManifest
	}
	return result, nil
}

// ParseComponent reads only PC-041 metadata; source owns sibling-file facts.
// Its closed shape likewise excludes all accumulated strict missing errors.
func ParseComponent(data []byte) (Component, error) {
	if !guardDocument(data) || !componentShape(data) {
		return Component{}, errComponent
	}
	var document componentDocument
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&document); err != nil || document.Schema != 1 {
		return Component{}, errComponent
	}
	return Component{Schema: document.Schema, Label: document.Label}, nil
}

// Lexical counting bounds the parser's whole-expression arena before it runs.
// The two container levels cover inline profiles and their inline tables.
func guardDocument(data []byte) bool {
	if len(data) > maxDocumentBytes || !utf8.Valid(data) {
		return false
	}
	tokens, quoted, ok := documentCounts(data)
	return ok && reserveDocument(data, tokens, quoted)
}

func documentCounts(data []byte) (tokens, quoted uint64, ok bool) {
	var containers [maxDocumentNesting]byte
	depth := 0
	for index := 0; index < len(data); {
		value := data[index]
		switch value {
		case ' ', '\t', '\r', '\n':
			index++
		case '#':
			for index < len(data) && data[index] != '\n' {
				index++
			}
		case '\'', '"':
			tokens++
			start := index
			index, ok = skipDocumentString(data, index)
			quoted += uint64(index - start)
			if !ok {
				return tokens, quoted, false
			}
		case '[', '{':
			tokens++
			if depth == len(containers) {
				return tokens, quoted, false
			}
			containers[depth] = value
			depth++
			index++
		case ']', '}':
			tokens++
			if depth == 0 || (value == ']' && containers[depth-1] != '[') || (value == '}' && containers[depth-1] != '{') {
				return tokens, quoted, false
			}
			depth--
			index++
		case '=', ',', '.':
			tokens++
			index++
		default:
			tokens++
			start := index
			for index < len(data) && !documentDelimiter(data[index]) {
				index++
			}
			if start == index {
				return tokens, quoted, false
			}
		}
		if tokens > maxParserTokens {
			return tokens, quoted, false
		}
	}
	return tokens, quoted, depth == 0
}

func documentDelimiter(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '#', '\'', '"', '[', ']', '{', '}', '=', ',', '.':
		return true
	default:
		return false
	}
}

func skipDocumentString(data []byte, start int) (int, bool) {
	quote := data[start]
	multiline := start+2 < len(data) && data[start+1] == quote && data[start+2] == quote
	index := start + 1
	if multiline {
		index = start + 3
	}
	for index < len(data) {
		value := data[index]
		if !multiline && (value == '\n' || value == '\r') {
			return index, false
		}
		if value == '\\' && quote == '"' {
			index++
			if index == len(data) {
				return index, false
			}
			if !multiline && (data[index] == '\n' || data[index] == '\r') {
				return index, false
			}
			index++
			continue
		}
		if value == quote {
			if !multiline {
				return index + 1, true
			}
			end := index
			for end < len(data) && data[end] == quote {
				end++
			}
			if end-index >= 3 {
				return end, end-index <= 5
			}
			index = end
			continue
		}
		index++
	}
	return index, false
}

func singleKey(node *unstable.Node) ([]byte, bool) {
	keys := node.Key()
	if !keys.Next() {
		return nil, false
	}
	key := keys.Node().Data
	return key, !keys.Next()
}

func boundedString(node *unstable.Node, limit int) bool {
	return node.Kind == unstable.String && len(node.Data) <= limit
}

func componentShape(data []byte) bool {
	var parser unstable.Parser
	parser.Reset(data)
	var seen uint8
	for parser.NextExpression() {
		node := parser.Expression()
		if node.Kind != unstable.KeyValue {
			return false
		}
		key, ok := singleKey(node)
		if !ok {
			return false
		}
		var field uint8
		switch string(key) {
		case "schema":
			field = 1
			if node.Value().Kind != unstable.Integer {
				return false
			}
		case "label":
			field = 2
			if !boundedString(node.Value(), maxLabelBytes) {
				return false
			}
		default:
			return false
		}
		if seen&field != 0 {
			return false
		}
		seen |= field
	}
	return parser.Error() == nil && seen&1 != 0
}

func manifestShape(data []byte) bool {
	var parser unstable.Parser
	parser.Reset(data)
	var rootSeen, fields uint8
	scope, profiles := "root", 0
	inlineProfiles := false
	complete := func() bool {
		return scope == "root" || scope == "base" && fields == 1 || scope == "profile" && fields == 7
	}
	for parser.NextExpression() {
		node := parser.Expression()
		if node.Kind != unstable.Table && node.Kind != unstable.ArrayTable && node.Kind != unstable.KeyValue {
			return false
		}
		key, ok := singleKey(node)
		if !ok {
			return false
		}
		switch node.Kind {
		case unstable.Table:
			if !complete() || string(key) != "base" || rootSeen&2 != 0 {
				return false
			}
			rootSeen |= 2
			scope, fields = "base", 0
		case unstable.ArrayTable:
			if !complete() || string(key) != "profiles" || inlineProfiles {
				return false
			}
			profiles++
			if profiles > maxProfiles {
				return false
			}
			rootSeen |= 4
			scope, fields = "profile", 0
		case unstable.KeyValue:
			if scope == "base" {
				if string(key) != "path" || fields != 0 || !boundedString(node.Value(), maxPathBytes) {
					return false
				}
				fields = 1
				continue
			}
			if scope == "profile" {
				if !profileField(node, &fields) {
					return false
				}
				continue
			}
			var field uint8
			switch string(key) {
			case "schema":
				field = 1
				if node.Value().Kind != unstable.Integer {
					return false
				}
			case "base":
				field = 2
				if !baseInlineShape(node.Value()) {
					return false
				}
			case "profiles":
				field = 4
				if !profilesInlineShape(node.Value()) {
					return false
				}
				inlineProfiles = true
			default:
				return false
			}
			if rootSeen&field != 0 {
				return false
			}
			rootSeen |= field
		default:
			return false
		}
	}
	return parser.Error() == nil && rootSeen&3 == 3 && complete()
}

func baseInlineShape(node *unstable.Node) bool {
	if node.Kind != unstable.InlineTable {
		return false
	}
	fields := node.Children()
	if !fields.Next() {
		return false
	}
	field := fields.Node()
	key, ok := singleKey(field)
	return ok && string(key) == "path" && boundedString(field.Value(), maxPathBytes) && !fields.Next()
}

func profilesInlineShape(node *unstable.Node) bool {
	if node.Kind != unstable.Array {
		return false
	}
	entries, count := node.Children(), 0
	for entries.Next() {
		count++
		if count > maxProfiles || entries.Node().Kind != unstable.InlineTable {
			return false
		}
		fields := entries.Node().Children()
		var seen uint8
		for fields.Next() {
			if !profileField(fields.Node(), &seen) {
				return false
			}
		}
		if seen != 7 {
			return false
		}
	}
	return true
}

func profileField(node *unstable.Node, seen *uint8) bool {
	key, ok := singleKey(node)
	if !ok {
		return false
	}
	var field uint8
	limit := maxPathBytes
	switch string(key) {
	case "id":
		field, limit = 1, maxProfileIDBytes
	case "kind":
		field, limit = 2, len(Embedded)
	case "path":
		field = 4
	default:
		return false
	}
	if *seen&field != 0 || !boundedString(node.Value(), limit) {
		return false
	}
	*seen |= field
	return true
}
