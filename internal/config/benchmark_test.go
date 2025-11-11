package config

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// BenchmarkParser_ParseString_Small benchmarks parsing a small config (~10 tools).
func BenchmarkParser_ParseString_Small(b *testing.B) {
	luaCode := `
		zerb = {
			tools = {
				"node@20.11.0",
				"python@3.12.1",
				"cargo:ripgrep",
				"npm:prettier",
				"ubi:sharkdp/bat",
			},
		}
	`

	parser := NewParser(nil)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := parser.ParseString(context.Background(), luaCode)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

// BenchmarkParser_ParseString_Medium benchmarks parsing a medium config (~100 tools).
func BenchmarkParser_ParseString_Medium(b *testing.B) {
	var tools strings.Builder
	tools.WriteString("zerb = {\n  tools = {\n")
	for i := 0; i < 100; i++ {
		tools.WriteString(fmt.Sprintf("    \"tool%d@1.0.0\",\n", i))
	}
	tools.WriteString("  },\n}")

	parser := NewParser(nil)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := parser.ParseString(context.Background(), tools.String())
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

// BenchmarkParser_ParseString_Large benchmarks parsing a large config (~1000 tools).
func BenchmarkParser_ParseString_Large(b *testing.B) {
	var tools strings.Builder
	tools.WriteString("zerb = {\n  tools = {\n")
	for i := 0; i < 1000; i++ {
		tools.WriteString(fmt.Sprintf("    \"tool%d@1.0.0\",\n", i))
	}
	tools.WriteString("  },\n}")

	parser := NewParser(nil)
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := parser.ParseString(context.Background(), tools.String())
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}
	}
}

// BenchmarkGenerator_Generate_Small benchmarks generating a small config.
func BenchmarkGenerator_Generate_Small(b *testing.B) {
	config := &Config{
		Tools: []string{
			"node@20.11.0",
			"python@3.12.1",
			"cargo:ripgrep",
		},
	}

	gen := NewGenerator()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := gen.Generate(context.Background(), config)
		if err != nil {
			b.Fatalf("Generate failed: %v", err)
		}
	}
}

// BenchmarkGenerator_Generate_Medium benchmarks generating a medium config.
func BenchmarkGenerator_Generate_Medium(b *testing.B) {
	config := &Config{
		Tools: make([]string, 100),
	}
	for i := 0; i < 100; i++ {
		config.Tools[i] = fmt.Sprintf("tool%d@1.0.0", i)
	}

	gen := NewGenerator()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := gen.Generate(context.Background(), config)
		if err != nil {
			b.Fatalf("Generate failed: %v", err)
		}
	}
}

// BenchmarkGenerator_Generate_Large benchmarks generating a large config.
func BenchmarkGenerator_Generate_Large(b *testing.B) {
	config := &Config{
		Tools: make([]string, 1000),
	}
	for i := 0; i < 1000; i++ {
		config.Tools[i] = fmt.Sprintf("tool%d@1.0.0", i)
	}

	gen := NewGenerator()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := gen.Generate(context.Background(), config)
		if err != nil {
			b.Fatalf("Generate failed: %v", err)
		}
	}
}

// BenchmarkRoundTrip benchmarks a full round-trip (parse → generate → parse).
func BenchmarkRoundTrip(b *testing.B) {
	luaCode := `
		zerb = {
			tools = {
				"node@20.11.0",
				"python@3.12.1",
				"cargo:ripgrep",
			},
		}
	`

	parser := NewParser(nil)
	gen := NewGenerator()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Parse
		config, err := parser.ParseString(context.Background(), luaCode)
		if err != nil {
			b.Fatalf("Parse failed: %v", err)
		}

		// Generate
		generated, err := gen.Generate(context.Background(), config)
		if err != nil {
			b.Fatalf("Generate failed: %v", err)
		}

		// Parse again
		_, err = parser.ParseString(context.Background(), generated)
		if err != nil {
			b.Fatalf("Second parse failed: %v", err)
		}
	}
}
