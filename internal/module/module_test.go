package module

import (
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

func TestNewInitializesDefaults(t *testing.T) {
	m := New()

	if m.InitialGlobalVolume != 128 {
		t.Fatalf("unexpected initial global volume: %d", m.InitialGlobalVolume)
	}
	if m.BPMThreshold != 33 {
		t.Fatalf("unexpected bpm threshold: %d", m.BPMThreshold)
	}
	if m.ChannelSettings[0].Volume != 64 || m.ChannelSettings[1].Volume != 64 {
		t.Fatalf("unexpected default channel volume: %+v %+v", m.ChannelSettings[0], m.ChannelSettings[1])
	}
	if m.ChannelSettings[0].Panning != PanLeft {
		t.Fatalf("unexpected channel 0 panning: %d", m.ChannelSettings[0].Panning)
	}
	if m.ChannelSettings[1].Panning != PanRight {
		t.Fatalf("unexpected channel 1 panning: %d", m.ChannelSettings[1].Panning)
	}
}

func TestApplyImplicitPanning(t *testing.T) {
	m := New()
	m.Channels = 4
	m.Flags = 0

	m.ApplyImplicitPanning()

	want := []uint16{PanHalfLeft, PanHalfRight, PanHalfRight, PanHalfLeft}
	for i := 0; i < 4; i++ {
		if m.ChannelSettings[i].Panning != want[i] {
			t.Fatalf("unexpected panning at channel %d: got=%d want=%d", i, m.ChannelSettings[i].Panning, want[i])
		}
	}
}

func TestValidateRejectsTrackCountMismatch(t *testing.T) {
	m := New()
	m.Channels = 4
	m.Patterns = []Pattern{
		{
			Rows:   2,
			Tracks: []unitrk.Track{{Rows: make([]unitrk.Row, 2)}, {Rows: make([]unitrk.Row, 2)}},
		},
	}

	if err := m.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
