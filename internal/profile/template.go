package profile

import (
	"bytes"
	"errors"
	"text/template/parse"
	"unicode/utf8"
)

const (
	maxTemplateDepth   = 32
	staticTemplateName = "zerb-static-template"
)

var errTemplate = errors.New("invalid static template")

// ValidateTemplate enforces PC-043's static allowlist without evaluating input.
// Authored names are diagnostic metadata and never enter the native parser.
func ValidateTemplate(_ string, source []byte) error {
	if len(source) > maxTemplateBytes || !utf8.Valid(source) {
		return errTemplate
	}
	actions, ok := templateActions(source)
	if !ok || !reserveTemplate(source, actions) {
		return errTemplate
	}
	trees, err := parse.Parse(staticTemplateName, string(source), "{{", "}}")
	if err != nil || len(trees) != 1 {
		return errTemplate
	}
	tree := trees[staticTemplateName]
	if tree == nil || !staticTemplateList(tree.Root, 0) {
		return errTemplate
	}
	return nil
}

type templateFrame struct {
	levels  uint8
	hasElse bool
}

// Else-if chains create nested native IfNodes despite sharing one authored end.
// Count that depth before the native parser can recurse or allocate its AST.
func templateActions(source []byte) (uint64, bool) {
	var frames [maxTemplateDepth]templateFrame
	count, depth := 0, 0
	var actions uint64
	for offset := 0; offset < len(source); {
		left := bytes.Index(source[offset:], []byte("{{"))
		if left < 0 {
			break
		}
		start := offset + left + 2
		right := bytes.Index(source[start:], []byte("}}"))
		if right < 0 {
			return actions, false
		}
		end := start + right
		actions++
		if actions > maxTemplateActions {
			return actions, false
		}
		action := source[start:end]
		if len(action) >= 2 && action[0] == '-' && templateSpace(action[1]) {
			action = action[1:]
		}
		if len(action) >= 2 && action[len(action)-1] == '-' && templateSpace(action[len(action)-2]) {
			action = action[:len(action)-1]
		}
		word, rest := templateWord(action)
		switch string(word) {
		case "if":
			if !literalTemplateBool(rest) || depth == maxTemplateDepth {
				return actions, false
			}
			frames[count] = templateFrame{levels: 1}
			count++
			depth++
		case "else":
			if count == 0 || frames[count-1].hasElse {
				return actions, false
			}
			word, rest = templateWord(rest)
			if len(word) == 0 {
				frames[count-1].hasElse = true
			} else if string(word) == "if" && literalTemplateBool(rest) && depth < maxTemplateDepth {
				frames[count-1].levels++
				depth++
			} else {
				return actions, false
			}
		case "end":
			word, _ = templateWord(rest)
			if count == 0 || len(word) != 0 {
				return actions, false
			}
			count--
			depth -= int(frames[count].levels)
		default:
			return actions, false
		}
		offset = end + 2
	}
	return actions, count == 0
}

func templateSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func templateWord(data []byte) (word, rest []byte) {
	start := 0
	for start < len(data) && templateSpace(data[start]) {
		start++
	}
	end := start
	for end < len(data) && !templateSpace(data[end]) {
		end++
	}
	return data[start:end], data[end:]
}

func literalTemplateBool(data []byte) bool {
	word, rest := templateWord(data)
	if string(word) != "true" && string(word) != "false" {
		return false
	}
	word, _ = templateWord(rest)
	return len(word) == 0
}

func staticTemplateList(list *parse.ListNode, depth int) bool {
	if list == nil || depth > maxTemplateDepth {
		return false
	}
	for _, node := range list.Nodes {
		switch value := node.(type) {
		case *parse.TextNode:
		case *parse.IfNode:
			if depth == maxTemplateDepth || value.Pipe == nil || value.Pipe.IsAssign || len(value.Pipe.Decl) != 0 ||
				len(value.Pipe.Cmds) != 1 || len(value.Pipe.Cmds[0].Args) != 1 {
				return false
			}
			if _, ok := value.Pipe.Cmds[0].Args[0].(*parse.BoolNode); !ok {
				return false
			}
			if !staticTemplateList(value.List, depth+1) || value.ElseList != nil && !staticTemplateList(value.ElseList, depth+1) {
				return false
			}
		default:
			return false
		}
	}
	return true
}
