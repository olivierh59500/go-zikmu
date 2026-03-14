package s3m

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

func TestLoadMinimalS3M(t *testing.T) {
	data := testfixtures.MinimalS3M()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if mod.Metadata.Title != "S3M Fixture" {
		t.Fatalf("unexpected title: %q", mod.Metadata.Title)
	}
	if mod.Metadata.Tracker != "Screamtracker 3.00" {
		t.Fatalf("unexpected tracker: %q", mod.Metadata.Tracker)
	}
	if mod.Metadata.Format != module.FormatS3M {
		t.Fatalf("unexpected format: %q", mod.Metadata.Format)
	}
	if mod.Channels != 2 || mod.Voices != 2 {
		t.Fatalf("unexpected channel setup: channels=%d voices=%d", mod.Channels, mod.Voices)
	}
	if mod.Flags&module.FlagArpeggioMemory == 0 || mod.Flags&module.FlagUsesPanning == 0 || mod.Flags&module.FlagUsesS3MSlides == 0 {
		t.Fatalf("unexpected module flags: %#x", mod.Flags)
	}
	if !reflect.DeepEqual(mod.Orders, []uint16{0, 0}) {
		t.Fatalf("unexpected orders: %v", mod.Orders)
	}
	if len(mod.Patterns) != 1 || len(mod.Patterns[0].Tracks) != 2 {
		t.Fatalf("unexpected pattern layout: %+v", mod.Patterns)
	}
	if mod.ChannelSettings[0].Panning != 0x30 || mod.ChannelSettings[1].Panning != 0xc0 {
		t.Fatalf("unexpected channel panning: %d %d", mod.ChannelSettings[0].Panning, mod.ChannelSettings[1].Panning)
	}
	if len(mod.Instruments) != 1 || mod.Instruments[0].NoteMap[0].Sample != 0 {
		t.Fatalf("unexpected instruments: %+v", mod.Instruments)
	}

	wantSample := []int16{0, 32512}
	if !reflect.DeepEqual(mod.Samples[0].Data, wantSample) {
		t.Fatalf("unexpected sample data: got=%v want=%v", mod.Samples[0].Data, wantSample)
	}

	row0 := mod.Patterns[0].Tracks[0].Rows[0]
	if !hasCommand(row0.Commands, unitrk.OpS3MEffectA, 3) {
		t.Fatalf("missing speed effect on first row: %+v", row0.Commands)
	}

	row1 := mod.Patterns[0].Tracks[1].Rows[1]
	if !hasCommand(row1.Commands, unitrk.OpPTEffectB, 1) {
		t.Fatalf("missing resolved jump effect on second track: %+v", row1.Commands)
	}
}

func TestLoadS3MADPCM(t *testing.T) {
	data := testfixtures.MinimalS3MADPCM()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	want := []int16{256, 768}
	if !reflect.DeepEqual(mod.Samples[0].Data, want) {
		t.Fatalf("unexpected ADPCM sample data: got=%v want=%v", mod.Samples[0].Data, want)
	}
}

func hasCommand(commands []unitrk.Command, op unitrk.Opcode, param uint16) bool {
	for _, command := range commands {
		if command.Op == op && command.Param == param {
			return true
		}
	}
	return false
}
