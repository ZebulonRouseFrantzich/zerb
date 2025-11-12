package config

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParser_ParseString_PlatformDetectionError(t *testing.T) {
	detector := &mockDetector{err: errors.New("platform detection failed")}
	parser := NewParser(detector)
	_, err := parser.ParseString(context.Background(), `zerb = { tools = {} }`)
	if err == nil {
		t.Fatal("expected error from platform detection")
	}
	if !strings.Contains(err.Error(), "platform detection failed") {
		t.Errorf("error = %v, want platform detection error", err)
	}
}

func TestParser_ParseString_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	parser := NewParser(nil)
	_, err := parser.ParseString(ctx, `zerb = { tools = {} }`)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestParser_ParseString_LargeConfig(t *testing.T) {
	var b strings.Builder
	b.WriteString("zerb = { tools = {")
	for i := 0; i < 1000; i++ {
		b.WriteString(fmt.Sprintf(`"tool%[1]d@1.0.0",`, i))
	}
	b.WriteString("} }")

	parser := NewParser(nil)
	config, err := parser.ParseString(context.Background(), b.String())
	if err != nil {
		t.Fatalf("ParseString() error = %v", err)
	}
	if len(config.Tools) != 1000 {
		t.Errorf("Tools length = %d, want 1000", len(config.Tools))
	}
}
