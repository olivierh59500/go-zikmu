package zikmu

import (
	"bytes"
	"io"
	"slices"
	"testing"
	"time"

	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
)

func TestFiniteRendererStopsAtNativeEndAndRestartsExactly(t *testing.T) {
	data := testfixtures.MinimalMOD()
	module, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SampleRate = 8000
	p, err := NewPlayer(module, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	finite := p.(FiniteRenderer)
	var song []float32
	buffer := make([]float32, 1026)
	for callbacks := 0; ; callbacks++ {
		if callbacks > 1000 {
			t.Fatal("native song never ended")
		}
		n, err := finite.RenderUntilEnd(buffer)
		song = append(song, buffer[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(song) == 0 {
		t.Fatal("empty soundtrack")
	}
	end := p.Position()
	if end != time.Duration(len(song)/2)*time.Second/8000 {
		t.Fatal("end marker changed PCM position")
	}
	if n, err := finite.RenderUntilEnd(buffer); n != 0 || err != io.EOF {
		t.Fatalf("read after end = %d,%v", n, err)
	}
	if p.Position() != end {
		t.Fatal("read past end advanced the transport")
	}
	if err := p.Reset(); err != nil {
		t.Fatal(err)
	}
	p.Play()
	got := make([]float32, len(song)+32)
	n, err := finite.RenderUntilEnd(got)
	if n != len(song) || err != io.EOF || !slices.Equal(got[:n], song) {
		t.Fatal("reset changed PCM or end boundary")
	}
	if err := p.Seek(time.Second); err != nil {
		t.Fatal(err)
	}
	n, err = finite.RenderUntilEnd(buffer)
	if err != nil || n != len(buffer) || !slices.Equal(buffer, song[16000:16000+n]) {
		t.Fatal("seek replay differs")
	}
	if err := p.Reset(); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(10, func() {
		if _, err := finite.RenderUntilEnd(buffer); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("callback allocated %g times", allocations)
	}
	if err := p.Seek(end + time.Second); err != nil {
		t.Fatal(err)
	}
	if n, err := p.Render(buffer); n != len(buffer) || err != nil {
		t.Fatal("legacy Render contract changed")
	}
}
