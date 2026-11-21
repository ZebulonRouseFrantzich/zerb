package profile

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"text/template/parse"
	"unicode/utf8"
)

func TestValidateTemplateStaticGrammar(t *testing.T) {
	cases := []string{
		"", "ordinary text", "Unicode 日本語 é", "text\x00text", "}}", "text { } }}}",
		"{{if true}}yes{{end}}", "{{if false}}no{{else}}yes{{end}}",
		"{{if true}}{{if false}}a{{else}}b{{end}}{{else}}c{{end}}",
		"{{if true}}a{{else if false}}b{{else if true}}c{{else}}d{{end}}",
		"{{if true}}{{else if false}}{{if true}}nested{{end}}{{end}}",
		"before \n{{- if true -}}\n text \n{{- else -}}\n other \n{{- end -}}\n after",
		"{{if true}}{{end}}}", "{{if true}}{{end}}}}",
	}
	for index, source := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			data := []byte(source)
			before := bytes.Clone(data)
			if err := ValidateTemplate("authored-name", data); err != nil {
				t.Fatalf("static template rejected: %v", err)
			}
			if !bytes.Equal(data, before) {
				t.Fatal("validation changed input")
			}
		})
	}
}

func TestValidateTemplateNativeWhitespaceAndTrims(t *testing.T) {
	for _, space := range []string{" ", "\t", "\r", "\n", " \t\r\n"} {
		for _, left := range []string{"", "-"} {
			for _, right := range []string{"", "-"} {
				action := func(text string) string { return "{{" + left + space + text + space + right + "}}" }
				source := action("if"+space+"true") + "yes" + action("else"+space+"if"+space+"false") + "no" + action("else") + "other" + action("end")
				if err := ValidateTemplate("", []byte(source)); err != nil {
					t.Fatalf("native whitespace/trim rejected: %q: %v", space, err)
				}
			}
		}
	}
	for _, source := range []string{
		"{{if\vtrue}}{{end}}", "{{if\ftrue}}{{end}}", "{{if\u00a0true}}{{end}}",
		"{{-if true}}{{end}}", "{{if true-}}{{end}}", "{{if true - }}{{end}}",
		"{{ - if true}}{{end}}", "{{-\vif true}}{{end}}", "{{if true\v-}}{{end}}",
	} {
		if err := ValidateTemplate("", []byte(source)); err == nil {
			t.Fatalf("non-native whitespace/trim accepted: %q", source)
		}
	}
}

func TestValidateTemplateRejectsForbiddenAndMalformedActions(t *testing.T) {
	cases := []string{
		"{{", "{{if true", "{{if true}}", "{{if true}}{{else}}", "{{}}", "{{ }}", "{{true}}", "{{false}}",
		"{{if}}{{end}}", "{{if true false}}{{end}}", "{{if TRUE}}{{end}}", "{{if (true)}}{{end}}",
		"{{if true | true}}{{end}}", "{{if true;}}{{end}}", "{{if $x := true}}{{end}}", "{{if $x = true}}{{end}}",
		"{{else}}", "{{end}}", "{{end true}}", "{{if true}}{{else}}{{else}}{{end}}",
		"{{if true}}{{else}}{{else if false}}{{end}}", "{{if true}}{{else if}}{{end}}",
		"{{if true}}{{else false}}{{end}}", "{{if true}}{{else if false false}}{{end}}", "{{if true}}{{end}}{{end}}",
		"{{if true}}{{if false}}{{else}}{{end}}", "{{if true}}{{else with false}}{{end}}",
		"{{/* comment */}}", "{{- /* comment */ -}}", "{{if true}}{{/* hidden */}}{{end}}",
		"{{.}}", "{{.chezmoi.hostname}}", "{{if .}}{{end}}", "{{if $x}}{{end}}", "{{$x := true}}",
		"{{range .}}{{end}}", "{{with true}}{{end}}", "{{block \"named\" .}}{{end}}",
		"{{define \"named\"}}text{{end}}", "{{template \"named\"}}", "{{break}}", "{{continue}}",
		"{{define \"zerb-static-template\"}}{{end}}", "{{define \"oracle\"}}{{end}}",
		"{{if eq true true}}{{end}}", "{{if not false}}{{end}}", "{{if and true false}}{{end}}",
		"{{if call .Callback}}{{end}}", "{{printf \"called\"}}", "{{output \"marker\"}}", "{{exec \"marker\"}}",
		"{{readFile \"marker\"}}", "{{env \"marker\"}}", "{{lookPath \"marker\"}}",
		"{{secret \"marker\"}}", "{{onepassword \"marker\"}}", "{{vault \"marker\"}}", "{{hostname}}",
		"{{if false}}{{output \"unreachable-call-marker\"}}{{end}}", "{{if true}}{{else}}{{readFile \"unreachable-path-marker\"}}{{end}}",
		"{{\"literal\"}}", "{{`literal`}}", "{{if 1}}{{end}}", "{{if nil}}{{end}}",
	}
	for index, source := range cases {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			data := []byte(source)
			before := bytes.Clone(data)
			if err := ValidateTemplate("private-authored-name", data); err == nil {
				t.Fatal("forbidden or malformed action accepted")
			}
			if !bytes.Equal(data, before) {
				t.Fatal("refusal changed source")
			}
		})
	}
}

func templateWithActions(actions int) []byte {
	var out strings.Builder
	if actions%2 != 0 {
		out.WriteString("{{if true}}{{else}}{{end}}")
		actions -= 3
	}
	for range actions / 2 {
		out.WriteString("{{if true}}{{end}}")
	}
	return []byte(out.String())
}

func TestValidateTemplateActionBounds(t *testing.T) {
	for _, actions := range []int{4095, 4096, 4097} {
		data := templateWithActions(actions)
		got, ok := templateActions(data)
		if got != uint64(actions) || ok != (actions <= 4096) {
			t.Fatalf("preflight actions %d: %d %v", actions, got, ok)
		}
		if err := ValidateTemplate("", data); (err == nil) != (actions <= 4096) {
			t.Fatalf("action bound %d: %v", actions, err)
		}
	}
}

func TestValidateTemplateNestedAndDesugaredDepthBounds(t *testing.T) {
	for _, depth := range []int{31, 32, 33} {
		cases := []string{
			strings.Repeat("{{if true}}", depth) + "literal" + strings.Repeat("{{end}}", depth),
			"{{if true}}" + strings.Repeat("{{else if false}}", depth-1) + "literal{{end}}",
			"{{if true}}" + strings.Repeat("{{else if false}}", depth-2) + "{{if true}}literal{{end}}{{end}}",
		}
		for index, source := range cases {
			if err := ValidateTemplate("", []byte(source)); (err == nil) != (depth <= 32) {
				t.Fatalf("depth %d shape %d: %v", depth, index, err)
			}
		}
	}
	// Closing a desugared chain must restore the enclosing sibling depth.
	source := "{{if true}}" + strings.Repeat("{{else if false}}", 31) + "{{end}}" + strings.Repeat("{{if true}}", 32) + strings.Repeat("{{end}}", 32)
	if err := ValidateTemplate("", []byte(source)); err != nil {
		t.Fatalf("closed chain retained depth: %v", err)
	}
}

func TestValidateTemplateBodyBoundsAndUTF8(t *testing.T) {
	for _, size := range []int{maxTemplateBytes - 1, maxTemplateBytes, maxTemplateBytes + 1} {
		if err := ValidateTemplate("", bytes.Repeat([]byte{'x'}, size)); (err == nil) != (size <= maxTemplateBytes) {
			t.Fatalf("template bytes %d: %v", size, err)
		}
	}
	for _, data := range [][]byte{{0xff}, {'x', 0xc0, 0x80}, []byte("{{if true}}\xff{{end}}"), []byte("{{if tr\xffue}}{{end}}")} {
		if err := ValidateTemplate("", data); err == nil {
			t.Fatal("invalid UTF-8 accepted")
		}
	}
	data := templateWithActions(4096)
	data = append(data, bytes.Repeat([]byte{'x'}, maxTemplateBytes-len(data))...)
	if err := ValidateTemplate("", data); err != nil {
		t.Fatalf("joint byte/action maxima rejected: %v", err)
	}
}

func TestValidateTemplateErrorRedactionAndDiagnosticName(t *testing.T) {
	const marker = "PrivateAuthoredMarker"
	for _, name := range []string{marker, strings.Repeat(marker, 100_000), string([]byte{0xff}), ""} {
		if err := ValidateTemplate(name, []byte("{{if true}}text{{end}}")); err != nil {
			t.Fatal("diagnostic name affected acceptance")
		}
		for _, source := range []string{"{{" + marker + "}}", "{{if true}}{{else " + marker + "}}{{end}}", "{{if true}}" + marker} {
			err := ValidateTemplate(name, []byte(source))
			if err == nil || err.Error() != "invalid static template" || strings.Contains(err.Error(), marker) {
				t.Fatalf("unsanitized template error: %v", err)
			}
		}
	}
}

func TestTemplatePreflightDoesNotAllocate(t *testing.T) {
	data := []byte("text {{- if true -}}a{{else if false}}b{{else}}c{{end}}")
	if actions, ok := templateActions(data); !ok || actions != 4 {
		t.Fatalf("preflight count: %d %v", actions, ok)
	}
	if allocations := testing.AllocsPerRun(20, func() { templateActions(data) }); allocations != 0 {
		t.Fatalf("preflight allocated: %v", allocations)
	}
}

// This independent native-AST oracle deliberately has no lexical allowlist.
// It proves soundness only: parsing can discard authored definitions.
func nativeStaticTemplate(source []byte) bool {
	if !utf8.Valid(source) || bytes.Count(source, []byte("{{")) > 4096 {
		return false
	}
	trees := make(map[string]*parse.Tree)
	tree := parse.New("oracle")
	tree.Mode = parse.ParseComments
	if _, err := tree.Parse(string(source), "{{", "}}", trees); err != nil || len(trees) != 1 {
		return false
	}
	var visit func(parse.Node, int) bool
	visit = func(node parse.Node, depth int) bool {
		switch value := node.(type) {
		case *parse.ListNode:
			for _, child := range value.Nodes {
				if !visit(child, depth) {
					return false
				}
			}
			return true
		case *parse.TextNode:
			return true
		case *parse.IfNode:
			if depth >= 32 || value.Pipe == nil || value.Pipe.IsAssign || len(value.Pipe.Decl) != 0 || len(value.Pipe.Cmds) != 1 || len(value.Pipe.Cmds[0].Args) != 1 {
				return false
			}
			if _, ok := value.Pipe.Cmds[0].Args[0].(*parse.BoolNode); !ok {
				return false
			}
			return visit(value.List, depth+1) && (value.ElseList == nil || visit(value.ElseList, depth+1))
		default:
			return false
		}
	}
	return visit(tree.Root, 0)
}

func FuzzValidateTemplate(f *testing.F) {
	for _, seed := range []string{"literal", "{{if true}}yes{{else}}no{{end}}", "{{-\tif true\n-}}a{{else if false}}b{{end}}", "{{/* comment */}}", "{{if (true)}}{{end}}", "{{if true}}{{else}}{{else if false}}{{end}}", "{{.chezmoi.hostname}}", "{{define \"oracle\"}}{{end}}", "{{"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		// Bound this test's independent native parser, which lacks preflight.
		if len(source) > 8192 {
			t.Skip()
		}
		before := bytes.Clone(source)
		accepted := ValidateTemplate("authored-diagnostic-name", source) == nil
		if accepted && !nativeStaticTemplate(source) {
			t.Fatalf("accepted template has an unapproved native AST: %q", source)
		}
		if !bytes.Equal(source, before) {
			t.Fatal("validator changed input")
		}
	})
}
