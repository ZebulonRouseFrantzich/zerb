package platform

import (
	"context"
	"testing"
)

func BenchmarkDetect(b *testing.B) {
	detector := NewDetector()
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = detector.Detect(ctx)
	}
}

func BenchmarkNormalizeArch(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = normalizeArch("x86_64")
	}
}

func BenchmarkNormalizePlatform(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = normalizePlatform("  Ubuntu  ")
	}
}

func BenchmarkMapFamily(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mapFamily("ubuntu")
	}
}

func BenchmarkInfo_GetDistro(b *testing.B) {
	info := &Info{
		OS:       "linux",
		Arch:     "amd64",
		Platform: "ubuntu",
		Family:   FamilyDebian,
		Version:  "22.04",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = info.GetDistro()
	}
}

func BenchmarkInfo_BooleanMethods(b *testing.B) {
	info := &Info{
		OS:     "linux",
		Arch:   "amd64",
		Family: FamilyDebian,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = info.IsLinux()
		_ = info.IsAMD64()
		_ = info.IsDebianFamily()
	}
}
