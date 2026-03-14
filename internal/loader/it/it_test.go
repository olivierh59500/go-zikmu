package it

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

func TestLoadMinimalIT(t *testing.T) {
	data := testfixtures.MinimalIT()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if mod.Metadata.Title != "IT Fixture" {
		t.Fatalf("unexpected title: %q", mod.Metadata.Title)
	}
	if mod.Metadata.Tracker != "Impulse Tracker 2.14p3" {
		t.Fatalf("unexpected tracker: %q", mod.Metadata.Tracker)
	}
	if mod.Metadata.Format != module.FormatIT {
		t.Fatalf("unexpected format: %q", mod.Metadata.Format)
	}
	if mod.Metadata.Message != "hello\nworld" {
		t.Fatalf("unexpected message: %q", mod.Metadata.Message)
	}
	if mod.Flags&(module.FlagBackgroundSlides|module.FlagArpeggioMemory|module.FlagUsesPanning|module.FlagXMPeriods|module.FlagLinearPeriods|module.FlagUsesInstruments|module.FlagUsesNNA) != (module.FlagBackgroundSlides | module.FlagArpeggioMemory | module.FlagUsesPanning | module.FlagXMPeriods | module.FlagLinearPeriods | module.FlagUsesInstruments | module.FlagUsesNNA) {
		t.Fatalf("unexpected module flags: %#x", mod.Flags)
	}
	if mod.Channels != 2 || mod.Voices != 2 {
		t.Fatalf("unexpected channel setup: channels=%d voices=%d", mod.Channels, mod.Voices)
	}
	if len(mod.Patterns) != 1 || mod.Patterns[0].Rows != 2 || len(mod.Patterns[0].Tracks) != 2 {
		t.Fatalf("unexpected pattern layout: %+v", mod.Patterns)
	}
	if mod.ChannelSettings[0].Volume != 64 || mod.ChannelSettings[0].Panning != module.PanHalfLeft {
		t.Fatalf("unexpected channel 0 settings: %+v", mod.ChannelSettings[0])
	}
	if mod.ChannelSettings[1].Volume != 48 || mod.ChannelSettings[1].Panning != module.PanHalfRight {
		t.Fatalf("unexpected channel 1 settings: %+v", mod.ChannelSettings[1])
	}

	instrument := mod.Instruments[0]
	if instrument.NewNoteAction != module.NNAFade || instrument.DuplicateCheck != module.DCTSample || instrument.DuplicateAction != module.DCAFade {
		t.Fatalf("unexpected instrument actions: %+v", instrument)
	}
	if instrument.GlobalVolume != 64 || instrument.FadeOut != 256 {
		t.Fatalf("unexpected instrument gain: glob=%d fade=%d", instrument.GlobalVolume, instrument.FadeOut)
	}
	if instrument.Flags&module.InstrumentPitchPan == 0 || instrument.PitchPanSeparation != 16 || instrument.PitchPanCenter != 60 {
		t.Fatalf("unexpected pitch-pan data: %+v", instrument)
	}
	if instrument.VolumeEnvelope.Points[1].Value != 256 || instrument.PanningEnvelope.Points[1].Value != 255 || instrument.PitchEnvelope.Points[1].Value != 48 {
		t.Fatalf("unexpected envelopes: vol=%+v pan=%+v pit=%+v", instrument.VolumeEnvelope, instrument.PanningEnvelope, instrument.PitchEnvelope)
	}

	sample := mod.Samples[0]
	if sample.Flags&(module.SampleOwnPanning|module.SampleDelta|module.SampleSigned|module.SampleLoop|module.SampleSustainLoop|module.SampleSustainBidiLoop) != (module.SampleOwnPanning | module.SampleDelta | module.SampleSigned | module.SampleLoop | module.SampleSustainLoop | module.SampleSustainBidiLoop) {
		t.Fatalf("unexpected sample flags: %#x", sample.Flags)
	}
	if sample.Panning != 128 || sample.RelativeNote != -12 || sample.FineTune != 0 {
		t.Fatalf("unexpected sample metadata: %+v", sample)
	}
	if sample.Vibrato.Flags != module.AutoVibratoIT || sample.Vibrato.Sweep != 10 || sample.Vibrato.Depth != 4 || sample.Vibrato.Rate != 6 {
		t.Fatalf("unexpected sample vibrato: %+v", sample.Vibrato)
	}
	wantSample := []int16{4096, 8192}
	if !reflect.DeepEqual(sample.Data, wantSample) {
		t.Fatalf("unexpected sample data: got=%v want=%v", sample.Data, wantSample)
	}

	row0 := mod.Patterns[0].Tracks[0].Rows[0]
	if !hasCommand(row0.Commands, unitrk.OpNote, 60) || !hasCommand(row0.Commands, unitrk.OpInstrument, 0) || !hasVolumeEffect(row0.Commands, unitrk.VolPortamento, 96) || !hasCommand(row0.Commands, unitrk.OpS3MEffectT, 0x30) {
		t.Fatalf("unexpected row 0 commands: %+v", row0.Commands)
	}

	row1 := mod.Patterns[0].Tracks[1].Rows[1]
	if !hasCommand(row1.Commands, unitrk.OpKeyOff, 0) || !hasVolumeEffect(row1.Commands, unitrk.VolSetPanning, 0) {
		t.Fatalf("unexpected row 1 commands: %+v", row1.Commands)
	}
}

func TestLoadITPackedSample(t *testing.T) {
	data := testfixtures.MinimalITPacked()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	want := make([]int16, 8)
	if !reflect.DeepEqual(mod.Samples[0].Data, want) {
		t.Fatalf("unexpected packed sample data: got=%v want=%v", mod.Samples[0].Data, want)
	}
}

func TestLoadLinearSampleOnlyITBuildsInternalInstruments(t *testing.T) {
	data := testfixtures.MinimalITLinearSamplesOnly()

	mod, err := Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if mod.Flags&module.FlagUsesInstruments == 0 {
		t.Fatalf("expected linear sample-only IT to enable internal instruments: %#x", mod.Flags)
	}
	if len(mod.Instruments) != 1 {
		t.Fatalf("expected one generated instrument, got %d", len(mod.Instruments))
	}
	if got := mod.Instruments[0].NoteMap[48].Note; got != 60 {
		t.Fatalf("expected generated instrument note map to include linear sample shift, got %d", got)
	}
}

func TestDecodeITSampleHalvesC5SpeedForNonLinearModules(t *testing.T) {
	raw := []byte{0x80}
	data := make([]byte, 128)
	copy(data[64:], raw)

	sample, _, err := decodeSample(data, sampleHeader{
		C5Speed:    44100,
		Length:     1,
		DataOffset: 64,
		Convert:    1,
	}, header{}, false)
	if err != nil {
		t.Fatalf("decodeSample failed: %v", err)
	}
	if sample.C5Speed != 22050 {
		t.Fatalf("expected non-linear IT sample speed to be halved, got %d", sample.C5Speed)
	}
}

func TestLinearTuningMatchesLibMikModSearch(t *testing.T) {
	tests := []struct {
		c5speed      uint32
		wantRelative int8
		wantFineTune int16
	}{
		{c5speed: 8363, wantRelative: -12, wantFineTune: 0},
		{c5speed: 6652, wantRelative: -15, wantFineTune: -122},
		{c5speed: 9942, wantRelative: -9, wantFineTune: -2},
	}

	for _, tt := range tests {
		gotRelative, gotFineTune := linearTuning(tt.c5speed)
		if gotRelative != tt.wantRelative || gotFineTune != tt.wantFineTune {
			t.Fatalf("linearTuning(%d) = (%d, %d), want (%d, %d)", tt.c5speed, gotRelative, gotFineTune, tt.wantRelative, tt.wantFineTune)
		}
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

func hasVolumeEffect(commands []unitrk.Command, effect unitrk.VolumeEffect, data uint8) bool {
	for _, command := range commands {
		if command.Op != unitrk.OpVolumeEffects {
			continue
		}
		gotEffect, gotData := unitrk.DecodePair(command.Param)
		if gotEffect == uint8(effect) && gotData == data {
			return true
		}
	}
	return false
}
