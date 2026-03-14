package zikmu_test

import (
	"bytes"
	"testing"

	"github.com/olivierh59500/go-zikmu"
	"github.com/olivierh59500/go-zikmu/internal/validation"
)

func BenchmarkLoadRegressionCorpus(b *testing.B) {
	for _, entry := range validation.RegressionCorpus() {
		entry := entry
		b.Run(entry.Name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				module, err := zikmu.Load(bytes.NewReader(entry.Data), int64(len(entry.Data)))
				if err != nil {
					b.Fatalf("Load failed: %v", err)
				}
				if module == nil {
					b.Fatal("expected a module")
				}
			}
		})
	}
}

func BenchmarkRenderRegressionCorpus(b *testing.B) {
	cfg := validation.DefaultConfig()
	frames := validation.DefaultFrames

	for _, entry := range validation.RegressionCorpus() {
		entry := entry
		module, err := zikmu.Load(bytes.NewReader(entry.Data), int64(len(entry.Data)))
		if err != nil {
			b.Fatalf("Load(%s) failed: %v", entry.Name, err)
		}

		b.Run(entry.Name, func(b *testing.B) {
			player, err := zikmu.NewPlayer(module, cfg)
			if err != nil {
				b.Fatalf("NewPlayer failed: %v", err)
			}
			buffer := make([]float32, frames*cfg.Channels)
			b.ReportAllocs()
			b.SetBytes(int64(len(buffer) * 4))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := player.Reset(); err != nil {
					b.Fatalf("Reset failed: %v", err)
				}
				if _, err := player.Render(buffer); err != nil {
					b.Fatalf("Render failed: %v", err)
				}
			}
		})
	}
}
