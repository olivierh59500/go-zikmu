package mod

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
)

func TestLoadMinimalMOD(t *testing.T) {
	data := testfixtures.MinimalMOD()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if mod.Metadata.Title != "Test Module" {
		t.Fatalf("unexpected title: %q", mod.Metadata.Title)
	}
	if mod.Metadata.Tracker != "Protracker" {
		t.Fatalf("unexpected tracker: %q", mod.Metadata.Tracker)
	}
	if mod.Metadata.Format != module.FormatMOD {
		t.Fatalf("unexpected format: %q", mod.Metadata.Format)
	}
	if mod.Channels != 4 || mod.Voices != 4 {
		t.Fatalf("unexpected channel setup: channels=%d voices=%d", mod.Channels, mod.Voices)
	}
	if len(mod.Orders) != 1 || mod.Orders[0] != 0 {
		t.Fatalf("unexpected orders: %v", mod.Orders)
	}
	if len(mod.Patterns) != 1 || len(mod.Patterns[0].Tracks) != 4 {
		t.Fatalf("unexpected pattern layout: %+v", mod.Patterns)
	}
	if mod.ChannelSettings[0].Panning != module.PanHalfLeft || mod.ChannelSettings[1].Panning != module.PanHalfRight {
		t.Fatalf("unexpected implicit panning: %+v %+v", mod.ChannelSettings[0], mod.ChannelSettings[1])
	}

	wantSample := []int16{0, 32512}
	if !reflect.DeepEqual(mod.Samples[0].Data, wantSample) {
		t.Fatalf("unexpected sample data: got=%v want=%v", mod.Samples[0].Data, wantSample)
	}

	row0 := mod.Patterns[0].Tracks[0].Rows[0]
	if len(row0.Commands) != 3 {
		t.Fatalf("unexpected first row command count: %d", len(row0.Commands))
	}
}

func TestLoadADPCMMODSample(t *testing.T) {
	data := testfixtures.MinimalMODADPCM()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	want := []int16{256, 768, 1024, 1536}
	if !reflect.DeepEqual(mod.Samples[0].Data, want) {
		t.Fatalf("unexpected ADPCM sample data: got=%v want=%v", mod.Samples[0].Data, want)
	}
}

func TestLoadStartrekkerFLT8(t *testing.T) {
	data := testfixtures.MinimalFLT8MOD()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if mod.Metadata.Tracker != "Startrekker" {
		t.Fatalf("unexpected tracker: %q", mod.Metadata.Tracker)
	}
	if mod.Channels != 8 {
		t.Fatalf("unexpected channel count: %d", mod.Channels)
	}
	if len(mod.Patterns) != 1 || len(mod.Patterns[0].Tracks) != 8 {
		t.Fatalf("unexpected pattern count/tracks: patterns=%d tracks=%d", len(mod.Patterns), len(mod.Patterns[0].Tracks))
	}
}
