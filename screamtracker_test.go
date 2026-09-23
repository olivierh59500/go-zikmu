package zikmu

import (
	"encoding/binary"
	"slices"
	"sync"
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
)

func packedS3M(data []byte) []byte {
	out := slices.Clone(data)
	orders := int(binary.LittleEndian.Uint16(out[32:]))
	instruments := int(binary.LittleEndian.Uint16(out[34:]))
	patterns := int(binary.LittleEndian.Uint16(out[36:]))
	for i := 0; i < patterns; i++ {
		offset := int(binary.LittleEndian.Uint16(out[96+orders+2*(instruments+i):])) * 16
		if offset == 0 {
			continue
		}
		length := int(binary.LittleEndian.Uint16(out[offset:]))
		for j := 0; j < length; j++ {
			out[offset+2+j] ^= byte((j + 2) ^ ((j + 2) * 4))
		}
	}
	return out
}

func TestScreamTrackerPackedAndOrdinaryPatternsMatch(t *testing.T) {
	data := testfixtures.MinimalS3M()
	for _, order := range []int{0, 2} {
		a, err := NewScreamTracker3(data, ScreamTracker3Options{StartOrder: order, Interpolation: true})
		if err != nil {
			t.Fatal(err)
		}
		defer a.Close()
		b, err := NewScreamTracker3(packedS3M(data), ScreamTracker3Options{StartOrder: order, Interpolation: true, PackedPatterns: true})
		if err != nil {
			t.Fatal(err)
		}
		defer b.Close()
		want, got := make([]int16, 48000*4), make([]int16, 48000*4)
		if a.Fill(want) != len(want)/2 {
			t.Fatal("short render")
		}
		for start := 0; start < len(got); {
			n := min(514, len(got)-start)
			if b.Fill(got[start:start+n]) != n/2 {
				t.Fatal("short fragmented render")
			}
			start += n
		}
		if !slices.Equal(want, got) {
			t.Fatal("pattern encoding or callback size changed PCM")
		}
		nonzero := false
		for _, sample := range got {
			nonzero = nonzero || sample != 0
		}
		if !nonzero {
			t.Fatal("fixture produced no audio")
		}
		for _, sample := range []uint64{0, 960, 4800, 48000, 95999} {
			if a.PositionAt(sample) != b.PositionAt(sample) {
				t.Fatal("position markers differ")
			}
		}
		buffer := make([]int16, 2048)
		if allocations := testing.AllocsPerRun(20, func() { a.Fill(buffer) }); allocations != 0 {
			t.Fatalf("callback allocated %g times", allocations)
		}
	}
}

func TestScreamTrackerValidationAndConcurrentClose(t *testing.T) {
	data := testfixtures.MinimalS3M()
	for _, options := range []ScreamTracker3Options{{SampleRate: 1}, {StartOrder: -1}, {StartOrder: 256}, {StartOrder: 250}} {
		if p, err := NewScreamTracker3(data, options); err == nil {
			p.Close()
			t.Fatal("accepted invalid replay options")
		}
	}
	for _, length := range []int{0, 47, 96, 111} {
		if p, err := NewScreamTracker3(data[:length], ScreamTracker3Options{}); err == nil {
			p.Close()
			t.Fatal("accepted truncated module")
		}
	}
	p, err := NewScreamTracker3(data, ScreamTracker3Options{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		buffer := make([]int16, 2048)
		for i := 0; i < 100; i++ {
			p.Fill(buffer)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			p.PositionAt(uint64(i * 100))
		}
		p.Close()
	}()
	wg.Wait()
	p.Close()
	if n := p.Fill(make([]int16, 3)); n != 0 {
		t.Fatal("closed player still rendered audio")
	}
}
