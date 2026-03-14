package xm

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

func TestLoadMinimalXM(t *testing.T) {
	data := testfixtures.MinimalXM()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if mod.Metadata.Title != "XM Fixture" {
		t.Fatalf("unexpected title: %q", mod.Metadata.Title)
	}
	if mod.Metadata.Tracker != "FastTracker v2.00 (XM format 1.04)" {
		t.Fatalf("unexpected tracker: %q", mod.Metadata.Tracker)
	}
	if mod.Metadata.Format != module.FormatXM {
		t.Fatalf("unexpected format: %q", mod.Metadata.Format)
	}
	if mod.Flags&module.FlagXMPeriods == 0 || mod.Flags&module.FlagUsesInstruments == 0 || mod.Flags&module.FlagNoWrapPatternBreak == 0 || mod.Flags&module.FlagFT2Quirks == 0 || mod.Flags&module.FlagUsesPanning == 0 || mod.Flags&module.FlagLinearPeriods == 0 {
		t.Fatalf("unexpected module flags: %#x", mod.Flags)
	}
	if mod.Channels != 2 || mod.Voices != 2 {
		t.Fatalf("unexpected channel setup: channels=%d voices=%d", mod.Channels, mod.Voices)
	}
	if len(mod.Patterns) != 1 || mod.Patterns[0].Rows != 2 || len(mod.Patterns[0].Tracks) != 2 {
		t.Fatalf("unexpected pattern layout: %+v", mod.Patterns)
	}
	if len(mod.Instruments) != 1 || len(mod.Samples) != 1 {
		t.Fatalf("unexpected instrument/sample counts: instruments=%d samples=%d", len(mod.Instruments), len(mod.Samples))
	}

	instrument := mod.Instruments[0]
	if instrument.FadeOut != 256 {
		t.Fatalf("unexpected fadeout: %d", instrument.FadeOut)
	}
	if instrument.VolumeEnvelope.Flags&(module.EnvelopeEnabled|module.EnvelopeSustain|module.EnvelopeLoop|module.EnvelopeVolume) != (module.EnvelopeEnabled | module.EnvelopeSustain | module.EnvelopeLoop | module.EnvelopeVolume) {
		t.Fatalf("unexpected volume envelope flags: %#x", instrument.VolumeEnvelope.Flags)
	}
	if len(instrument.VolumeEnvelope.Points) != 2 || instrument.VolumeEnvelope.Points[1].Tick != 10 || instrument.VolumeEnvelope.Points[1].Value != 256 {
		t.Fatalf("unexpected volume envelope: %+v", instrument.VolumeEnvelope)
	}
	if instrument.NoteMap[0].Sample != 0 || instrument.NoteMap[0].Note != 2 {
		t.Fatalf("unexpected note map: %+v", instrument.NoteMap[0])
	}

	sample := mod.Samples[0]
	if sample.Flags&(module.SampleOwnPanning|module.SampleDelta|module.SampleSigned|module.SampleLoop) != (module.SampleOwnPanning | module.SampleDelta | module.SampleSigned | module.SampleLoop) {
		t.Fatalf("unexpected sample flags: %#x", sample.Flags)
	}
	if sample.Panning != 200 || sample.RelativeNote != 2 || sample.FineTune != 1 {
		t.Fatalf("unexpected sample metadata: %+v", sample)
	}
	if sample.Vibrato.Depth != 16 || sample.Vibrato.Rate != 5 {
		t.Fatalf("unexpected sample vibrato: %+v", sample.Vibrato)
	}
	wantData := []int16{4096, 8192}
	if !reflect.DeepEqual(sample.Data, wantData) {
		t.Fatalf("unexpected sample data: got=%v want=%v", sample.Data, wantData)
	}

	row0 := mod.Patterns[0].Tracks[0].Rows[0]
	if !hasCommand(row0.Commands, unitrk.OpNote, 48) || !hasCommand(row0.Commands, unitrk.OpInstrument, 0) || !hasCommand(row0.Commands, unitrk.OpPTEffectC, 24) || !hasCommand(row0.Commands, unitrk.OpXMEffectG, 64) {
		t.Fatalf("unexpected row 0 commands: %+v", row0.Commands)
	}

	row1 := mod.Patterns[0].Tracks[1].Rows[1]
	if !hasCommand(row1.Commands, unitrk.OpKeyFade, 0) || !hasCommand(row1.Commands, unitrk.OpPTEffect3, 32) || !hasCommand(row1.Commands, unitrk.OpXMEffect4, 0x34) {
		t.Fatalf("unexpected row 1 commands: %+v", row1.Commands)
	}
}

func TestLoadXMADPCM(t *testing.T) {
	data := testfixtures.MinimalXMADPCM()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	want := []int16{256, 768}
	if !reflect.DeepEqual(mod.Samples[0].Data, want) {
		t.Fatalf("unexpected ADPCM sample data: got=%v want=%v", mod.Samples[0].Data, want)
	}
}

func TestLoadLegacyXM103(t *testing.T) {
	data := testfixtures.MinimalXM103()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if mod.Metadata.Tracker != "FastTracker v1.04 (XM format 1.03)" {
		t.Fatalf("unexpected tracker: %q", mod.Metadata.Tracker)
	}
	if len(mod.Patterns) != 2 {
		t.Fatalf("unexpected pattern count: %d", len(mod.Patterns))
	}
	if len(mod.Orders) != 1 || mod.Orders[0] != 1 {
		t.Fatalf("unexpected orders: %v", mod.Orders)
	}
	want := []int16{32512}
	if !reflect.DeepEqual(mod.Samples[0].Data, want) {
		t.Fatalf("unexpected legacy sample data: got=%v want=%v", mod.Samples[0].Data, want)
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
