//go:build go1.18

package config

import (
	"context"
	"testing"
)

func FuzzParser_ParseString(f *testing.F) {
	f.Add(`zerb = { tools = { "node@20.11.0" } }`)
	f.Add(`zerb = { configs = { "~/.zshrc" } }`)
	f.Add(`zerb = { meta = { name = "test" } }`)

	parser := NewParser(nil)

	f.Fuzz(func(t *testing.T, luaCode string) {
		_, _ = parser.ParseString(context.Background(), luaCode)
	})
}

func FuzzGenerator_QuoteLuaString(f *testing.F) {
	f.Add("hello")
	f.Add(`say "hello"`)
	f.Add("line1\nline2")
	f.Add(`C:\\Users\\test`)

	gen := NewGenerator()

	f.Fuzz(func(t *testing.T, input string) {
		quoted := gen.quoteLuaString(input)
		if len(quoted) < 2 || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
			t.Errorf("quoteLuaString(%q) = %q, invalid format", input, quoted)
		}
	})
}
