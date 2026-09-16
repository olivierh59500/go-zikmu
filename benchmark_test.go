package zikmu_test

import (
	"bytes"
	"testing"
	"time"

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

func BenchmarkStreamRead(b *testing.B) {
	cfg := validation.DefaultConfig()
	entry := validation.RegressionCorpus()[5]
	module, err := zikmu.Load(bytes.NewReader(entry.Data), int64(len(entry.Data)))
	if err != nil {
		b.Fatalf("Load failed: %v", err)
	}
	player, err := zikmu.NewPlayer(module, cfg)
	if err != nil {
		b.Fatalf("NewPlayer failed: %v", err)
	}
	stream := player.Stream()
	buffer := make([]byte, cfg.BufferSamples*cfg.Channels*4-3)
	if _, err := stream.Read(buffer); err != nil {
		b.Fatalf("warmup Read failed: %v", err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(buffer)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := stream.Read(buffer); err != nil {
			b.Fatalf("Read failed: %v", err)
		}
	}
}

func BenchmarkSeek(b *testing.B) {
	cfg := validation.DefaultConfig()
	entry := validation.RegressionCorpus()[5]
	module, err := zikmu.Load(bytes.NewReader(entry.Data), int64(len(entry.Data)))
	if err != nil {
		b.Fatalf("Load failed: %v", err)
	}
	player, err := zikmu.NewPlayer(module, cfg)
	if err != nil {
		b.Fatalf("NewPlayer failed: %v", err)
	}
	const target = 30 * time.Second
	if err := player.Seek(target); err != nil {
		b.Fatalf("warmup Seek failed: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := player.Seek(target); err != nil {
			b.Fatalf("Seek failed: %v", err)
		}
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
