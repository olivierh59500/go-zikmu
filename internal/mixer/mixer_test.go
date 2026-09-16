package mixer

import (
	"fmt"
	"math"
	"testing"

	modmodel "github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/replay"
)

func TestMixerRendersStereoAudio(t *testing.T) {
	mod := testModule()
	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 2,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	mix.Reset(singleVoiceSnapshot(1, 48, 64, 128, true))

	dst := make([]float32, 32)
	written, err := mix.Render(dst)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if written != len(dst) {
		t.Fatalf("unexpected write count: got=%d want=%d", written, len(dst))
	}
	if !hasNonZero(dst) {
		t.Fatalf("expected non-zero stereo output: %v", dst)
	}
}

func TestMixerSkipMatchesDiscardedRender(t *testing.T) {
	for _, channels := range []int{1, 2} {
		for _, interpolation := range []bool{false, true} {
			for _, flags := range []modmodel.SampleFlags{
				modmodel.SampleLoop,
				modmodel.SampleLoop | modmodel.SampleBidiLoop,
				modmodel.SampleLoop | modmodel.SampleReverse,
			} {
				name := fmt.Sprintf("channels=%d/interpolation=%t/flags=%d", channels, interpolation, flags)
				t.Run(name, func(t *testing.T) {
					mod := testLoopModule()
					mod.Samples[0].Flags = flags
					cfg := Config{SampleRate: 44100, OutputChannels: channels, Interpolation: interpolation}
					rendered, err := New(mod, cfg)
					if err != nil {
						t.Fatalf("New(rendered) failed: %v", err)
					}
					skipped, err := New(mod, cfg)
					if err != nil {
						t.Fatalf("New(skipped) failed: %v", err)
					}
					snapshot := singleVoiceSnapshot(1, 48, 64, 128, true)
					rendered.Reset(snapshot)
					skipped.Reset(snapshot)

					const frames = 257
					if _, err := rendered.Render(make([]float32, frames*channels)); err != nil {
						t.Fatalf("discarded Render failed: %v", err)
					}
					skipped.Skip(frames)
					if rendered.voices[0] != skipped.voices[0] {
						t.Fatalf("voice state differs after skip\nrendered=%+v\nskipped=%+v", rendered.voices[0], skipped.voices[0])
					}
				})
			}
		}
	}
}

func TestMixerInterpolationChangesOutput(t *testing.T) {
	mod := testModule()
	nearest, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	linear, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  true,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	state := singleVoiceSnapshot(1, 36, 64, 128, false)
	nearest.Reset(state)
	linear.Reset(state)

	a := make([]float32, 16)
	b := make([]float32, 16)
	if _, err := nearest.Render(a); err != nil {
		t.Fatalf("nearest Render failed: %v", err)
	}
	if _, err := linear.Render(b); err != nil {
		t.Fatalf("linear Render failed: %v", err)
	}
	if equalBuffers(a, b) {
		t.Fatalf("expected interpolation to change output\nnearest=%v\nlinear=%v", a, b)
	}
}

func TestMixerInterpolationMatchesLibMikModWeightedFormula(t *testing.T) {
	mix := &Mixer{
		interpolation: true,
	}
	sample := &modmodel.Sample{
		Data: []int16{-3000, 1000},
	}
	current := int64(1 << (fractionBits - 1))

	got := mix.sampleAt(sample, current, false)
	want := int32((int64(sample.Data[0])*(1<<fractionBits) + int64(sample.Data[1])*(1<<fractionBits)) >> (fractionBits + 1))
	if got != want {
		t.Fatalf("unexpected interpolated sample: got=%d want=%d", got, want)
	}
}

func TestMixerRetriggersOnTriggerChange(t *testing.T) {
	mod := testLoopModule()
	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	initial := singleVoiceSnapshot(1, 48, 16, 0, false)
	mix.Reset(initial)
	if _, err := mix.Render(make([]float32, clickBuffer)); err != nil {
		t.Fatalf("warmup Render failed: %v", err)
	}

	first := make([]float32, 5)
	if _, err := mix.Render(first); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	second := make([]float32, 5)
	if _, err := mix.Render(second); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if equalBuffers(first, second) {
		t.Fatalf("expected playback position to advance between blocks")
	}

	mix.ApplySnapshot(singleVoiceSnapshot(2, 48, 16, 0, false))
	afterTrigger := make([]float32, 5)
	if _, err := mix.Render(afterTrigger); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if equalBuffers(afterTrigger, second) {
		t.Fatalf("expected retriggered voice to restart instead of continuing playback")
	}
	if !hasNonZero(afterTrigger) {
		t.Fatalf("expected retriggered voice to keep producing audio: %v", afterTrigger)
	}
}

func TestMixerRetriggerStartsImmediately(t *testing.T) {
	mix := &Mixer{
		outputChannels: 1,
		interpolation:  false,
	}
	sample := &modmodel.Sample{
		Data: []int16{1000, 1000},
	}
	v := &voice{
		active:         true,
		sample:         0,
		increment:      int64(1) << fractionBits,
		leftSel:        64,
		clickRemaining: 0,
		lastLeftValue:  -64000,
	}

	dest := make([]int32, 1)
	gotCurrent := mix.mixVoice(sample, v, dest, 0, 1)
	if gotCurrent != int64(1)<<fractionBits {
		t.Fatalf("unexpected playback position: got=%d", gotCurrent)
	}
	if dest[0] != 64000 {
		t.Fatalf("expected retriggered sample to start immediately: got=%d", dest[0])
	}
	if v.clickRemaining != 0 {
		t.Fatalf("expected click state to stay disabled, got=%d", v.clickRemaining)
	}
	if v.lastLeftValue != 64000 {
		t.Fatalf("expected last mixed value to track the new waveform, got=%d", v.lastLeftValue)
	}
}

func TestMixerUsesSnapshotVoicesWhenProvided(t *testing.T) {
	mod := testModule()
	mod.Channels = 1
	mod.Voices = 2

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	mix.Reset(replay.Snapshot{
		Channels: []replay.ChannelSnapshot{{
			Active: false,
		}},
		Voices: []replay.ChannelSnapshot{
			{
				Active:    true,
				Trigger:   1,
				Note:      48,
				Sample:    0,
				Volume:    64,
				Panning:   128,
				KeyOn:     true,
				Frequency: 8363,
			},
			{},
		},
	})

	dst := make([]float32, 16)
	if _, err := mix.Render(dst); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !hasNonZero(dst) {
		t.Fatalf("expected mixer to render from snapshot voices")
	}
}

func TestMixerRetriggerKeepsVolumeRampWhenVolumeJumps(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed: 44100,
		Volume:  64,
		Data:    []int16{1000, 1000, 1000, 1000},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	mix.Reset(singleVoiceSnapshot(1, 48, 64, 128, false))

	warmup := make([]int16, 1)
	if _, err := mix.RenderPCM16(warmup); err != nil {
		t.Fatalf("warmup RenderPCM16 failed: %v", err)
	}

	mix.ApplySnapshot(singleVoiceSnapshot(2, 48, 16, 128, false))

	got := make([]int16, 1)
	if _, err := mix.RenderPCM16(got); err != nil {
		t.Fatalf("RenderPCM16 failed: %v", err)
	}
	if got[0] != 125 {
		t.Fatalf("expected retriggered voice to start at the previous ramp volume: got=%v", got)
	}
}

func TestMixerIgnoresSampleChangeWithoutNewTrigger(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{
		{Data: make([]int16, 512)},
		{Data: make([]int16, 512)},
	}
	for i := range mod.Samples[0].Data {
		mod.Samples[0].Data[i] = 1000
		mod.Samples[1].Data[i] = -1000
	}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	mix.Reset(replay.Snapshot{
		Channels: []replay.ChannelSnapshot{{
			Active:    true,
			Trigger:   1,
			Sample:    0,
			Frequency: 44100,
			Volume:    64,
			Panning:   128,
		}},
	})

	if _, err := mix.RenderPCM16(make([]int16, clickBuffer)); err != nil {
		t.Fatalf("warmup RenderPCM16 failed: %v", err)
	}

	first := make([]int16, 2)
	if _, err := mix.RenderPCM16(first); err != nil {
		t.Fatalf("first RenderPCM16 failed: %v", err)
	}
	if first[0] <= 0 || first[1] <= 0 {
		t.Fatalf("expected playback to continue on the original positive sample: %v", first)
	}

	mix.ApplySnapshot(replay.Snapshot{
		Channels: []replay.ChannelSnapshot{{
			Active:    true,
			Trigger:   1,
			Sample:    1,
			Frequency: 44100,
			Volume:    64,
			Panning:   128,
		}},
	})

	second := make([]int16, 2)
	if _, err := mix.RenderPCM16(second); err != nil {
		t.Fatalf("second RenderPCM16 failed: %v", err)
	}
	if second[0] <= 0 || second[1] <= 0 {
		t.Fatalf("expected playback to stay on the current sample without retrigger: %v", second)
	}
}

func TestMixerAppliesSampleOffsetOnRetrigger(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 1
	data := make([]int16, 512)
	copy(data, []int16{1000, 2000, 3000, 4000})
	for i := 4; i < len(data); i++ {
		data[i] = 4000
	}
	mod.Samples = []modmodel.Sample{{
		Data: data,
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	mix.Reset(replay.Snapshot{
		Channels: []replay.ChannelSnapshot{{
			Active:       true,
			Trigger:      1,
			Sample:       0,
			SampleOffset: 2,
			Frequency:    44100,
			Volume:       64,
			Panning:      128,
		}},
	})

	if _, err := mix.RenderPCM16(make([]int16, clickBuffer)); err != nil {
		t.Fatalf("warmup RenderPCM16 failed: %v", err)
	}

	mix.ApplySnapshot(replay.Snapshot{
		Channels: []replay.ChannelSnapshot{{
			Active:       true,
			Trigger:      2,
			Sample:       0,
			SampleOffset: 2,
			Frequency:    44100,
			Volume:       64,
			Panning:      128,
		}},
	})

	dst := make([]int16, 2)
	if _, err := mix.RenderPCM16(dst); err != nil {
		t.Fatalf("RenderPCM16 failed: %v", err)
	}
	if dst[0] != 375 || dst[1] != 500 {
		t.Fatalf("expected retrigger to start from sample offset: %v", dst)
	}
}

func TestMixerHonorsSampleLoop(t *testing.T) {
	mod := testLoopModule()
	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	mix.Reset(singleVoiceSnapshot(1, 48, 64, 128, false))

	dst := make([]float32, 12)
	if _, err := mix.Render(dst); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if dst[len(dst)-1] == 0 {
		t.Fatalf("expected looped sample to keep producing data: %v", dst)
	}
}

func TestMixerInterpolationUsesLoopGuardAtLoopEnd(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed:   44100,
		Volume:    64,
		Flags:     modmodel.SampleLoop,
		LoopStart: 1,
		LoopEnd:   3,
		Data:      []int16{100, 2000, 4000, -30000},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  true,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	current := (int64(2) << fractionBits) + (1 << (fractionBits - 1))
	got := mix.sampleAt(&mod.Samples[0], current, false)
	want := int32(int64(4000) + ((int64(2000-4000) * (1 << (fractionBits - 1))) >> fractionBits))
	if got != want {
		t.Fatalf("expected interpolation to wrap to loop start at loop end: got=%d want=%d", got, want)
	}
}

func TestMixerInterpolationUsesBidiGuardAtLoopEnd(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed:   44100,
		Volume:    64,
		Flags:     modmodel.SampleLoop | modmodel.SampleBidiLoop,
		LoopStart: 1,
		LoopEnd:   4,
		Data:      []int16{100, 2000, 4000, 6000, -30000},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  true,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	current := (int64(3) << fractionBits) + (1 << (fractionBits - 1))
	got := mix.sampleAt(&mod.Samples[0], current, false)
	want := int32(6000)
	if got != want {
		t.Fatalf("expected bidi interpolation guard to mirror last loop sample: got=%d want=%d", got, want)
	}
}

func TestMixerPlaysLastFrameOfUnloopedSample(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed: 44100,
		Volume:  64,
		Data:    []int16{0, 32767},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	mix.Reset(singleVoiceSnapshot(1, 48, 64, 128, false))

	dst := make([]float32, 3)
	if _, err := mix.Render(dst); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if dst[1] == 0 {
		t.Fatalf("expected the last sample frame to be rendered before stop: %v", dst)
	}
}

func TestMixerDoesNotRestartFinishedUnloopedVoiceOnSnapshotRefresh(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed: 44100,
		Volume:  64,
		Data:    []int16{32767, 16384},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	snapshot := singleVoiceSnapshot(1, 48, 64, 128, false)
	mix.Reset(snapshot)

	first := make([]float32, 3)
	if _, err := mix.Render(first); err != nil {
		t.Fatalf("first Render failed: %v", err)
	}
	if !hasNonZero(first[:2]) {
		t.Fatalf("expected initial sample body before stop: %v", first)
	}

	mix.ApplySnapshot(snapshot)

	second := make([]float32, 3)
	if _, err := mix.Render(second); err != nil {
		t.Fatalf("second Render failed: %v", err)
	}
	if hasNonZero(second) {
		t.Fatalf("expected finished unlooped voice to stay silent without retrigger: %v", second)
	}
}

func TestMixerUsesSampleTuningForXMPeriods(t *testing.T) {
	mod := modmodel.New()
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagLinearPeriods | modmodel.FlagUsesInstruments
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed:      8363,
		Volume:       64,
		RelativeNote: 2,
		FineTune:     64,
		Data:         []int16{32767, -32768},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	got := mix.stepFor(50, 0, 0)
	want := quantizedStep(xmLinearFrequency(50, 64), 44100)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("unexpected XM period step: got=%f want=%f", got, want)
	}
}

func TestMixerXMLinearPeriodsIgnoreOddFinetuneLSB(t *testing.T) {
	mod := modmodel.New()
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagLinearPeriods | modmodel.FlagUsesInstruments
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed:  8363,
		Volume:   64,
		FineTune: 1,
		Data:     []int16{32767, -32768},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	got := mix.stepFor(50, 0, 0)
	want := quantizedStep(9387, 44100)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("unexpected XM linear odd-finetune step: got=%f want=%f", got, want)
	}
}

func TestMixerXMLinearPeriodsUseDiscretePitchDeltaFrequency(t *testing.T) {
	mod := modmodel.New()
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagLinearPeriods | modmodel.FlagUsesInstruments
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed:  8363,
		Volume:   64,
		FineTune: 0,
		Data:     []int16{32767, -32768},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	got := mix.stepFor(50, 1, 0)
	want := quantizedStep(xmLinearFrequency(50, 1), 44100)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("unexpected XM linear pitch-delta step: got=%f want=%f", got, want)
	}
}

func TestMixerXMLogPeriodsUseDiscretePitchDeltaFrequency(t *testing.T) {
	mod := modmodel.New()
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagUsesInstruments
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed:  8363,
		Volume:   64,
		FineTune: 0,
		Data:     []int16{32767, -32768},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	got := mix.stepFor(50, 1, 0)
	want := quantizedStep(xmLogFrequency(50, 1), 44100)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("unexpected XM log pitch-delta step: got=%f want=%f", got, want)
	}
}

func TestMixerUsesOldPeriodFrequencyForMODStyle(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed: 8363,
		Volume:  64,
		Data:    []int16{32767, -32768},
	}}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 1,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	got := mix.stepFor(36, 0, 0)
	want := quantizedStep(oldPeriodFrequency(36, 8363), 44100)
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("unexpected MOD period step: got=%f want=%f", got, want)
	}
}

func TestXMLogFrequencyMatchesLibMikModTable(t *testing.T) {
	if got := xmLogFrequency(62, 0); got != 18863 {
		t.Fatalf("unexpected XM log frequency for note=62 fine=0: got=%f want=18863", got)
	}
	if got := xmLogFrequency(45, 10); got != 7070 {
		t.Fatalf("unexpected XM log frequency for note=45 fine=10: got=%f want=7070", got)
	}
}

func TestMixerUsesIntegerPanningSelectorsLikeLibmikmod(t *testing.T) {
	left, right := mixSelectors(202, 4, true)
	if left != 0 || right != 3 {
		t.Fatalf("unexpected selectors: left=%d right=%d", left, right)
	}
}

func TestMixerRampKeepsFractionalSelectorPrecision(t *testing.T) {
	v := &voice{
		leftSel:       95,
		rightSel:      32,
		oldLeftSel:    0,
		oldRightSel:   0,
		rampRemaining: clickBuffer - 2,
	}

	left, right, ramping := rampMixSelectors(v, clickShift)
	if !ramping {
		t.Fatal("expected ramping selectors")
	}
	if left != 190 || right != 64 {
		t.Fatalf("unexpected raw ramp selectors: left=%d right=%d", left, right)
	}

	value := int32(3087)
	gotLeft := int32((int64(left) * int64(value)) >> clickShift)
	gotRight := int32((int64(right) * int64(value)) >> clickShift)
	if gotLeft != 9164 || gotRight != 3087 {
		t.Fatalf("unexpected ramped mix contribution: left=%d right=%d", gotLeft, gotRight)
	}
}

func TestMixerResetUsesLibMikModInitialPanningForFirstRamp(t *testing.T) {
	mod := modmodel.New()
	mod.Channels = 2
	mod.Samples = []modmodel.Sample{
		{Data: []int16{1000, 1000, 1000, 1000}},
		{Data: []int16{1000, 1000, 1000, 1000}},
	}

	mix, err := New(mod, Config{
		SampleRate:     44100,
		OutputChannels: 2,
		Interpolation:  false,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	mix.Reset(replay.Snapshot{
		Channels: []replay.ChannelSnapshot{
			{
				Active:    true,
				Trigger:   1,
				Note:      48,
				Sample:    0,
				Frequency: 44100,
				Volume:    21,
				Panning:   0,
			},
			{
				Active:    true,
				Trigger:   1,
				Note:      48,
				Sample:    1,
				Frequency: 44100,
				Volume:    21,
				Panning:   240,
			},
		},
	})

	got := make([]int16, 2)
	if _, err := mix.RenderPCM16(got); err != nil {
		t.Fatalf("RenderPCM16 failed: %v", err)
	}
	if got[0] != 0 || got[1] != 0 {
		t.Fatalf("expected initial pan ramps to mute the first stereo frame, got=%v", got)
	}
}

func TestMixerExperimentalXMHighQualityOnlyAppliesToInterpolatedXM(t *testing.T) {
	xm := modmodel.New()
	xm.Metadata.Format = modmodel.FormatXM
	xm.Channels = 1

	mix, err := New(xm, Config{
		SampleRate:                44100,
		OutputChannels:            2,
		Interpolation:             true,
		ExperimentalXMHighQuality: true,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if !mix.useExperimentalXMHighQuality() {
		t.Fatal("expected XM HQ mixer to activate for interpolated XM")
	}
	if mix.mixFractionBits() != experimentalFractionBits {
		t.Fatalf("unexpected XM HQ fraction bits: got=%d want=%d", mix.mixFractionBits(), experimentalFractionBits)
	}
	if mix.mixSamplingFactor() != 4 {
		t.Fatalf("unexpected XM HQ sampling factor: got=%d want=4", mix.mixSamplingFactor())
	}

	mix.interpolation = false
	if mix.useExperimentalXMHighQuality() {
		t.Fatal("expected XM HQ mixer to stay disabled without interpolation")
	}

	mod := modmodel.New()
	mod.Metadata.Format = modmodel.FormatMOD
	mod.Channels = 1
	stable, err := New(mod, Config{
		SampleRate:                44100,
		OutputChannels:            2,
		Interpolation:             true,
		ExperimentalXMHighQuality: true,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if stable.useExperimentalXMHighQuality() {
		t.Fatal("expected XM HQ mixer to stay disabled for non-XM modules")
	}
}

func testModule() *modmodel.Module {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Samples = []modmodel.Sample{{
		C5Speed: 44100,
		Volume:  64,
		Data:    []int16{32767, -32768, 16384, -16384},
	}}
	return mod
}

func testLoopModule() *modmodel.Module {
	mod := testModule()
	mod.Samples[0].Flags = modmodel.SampleLoop
	mod.Samples[0].LoopStart = 0
	mod.Samples[0].LoopEnd = 4
	return mod
}

func singleVoiceSnapshot(trigger uint64, note, volume, panning int, keyOn bool) replay.Snapshot {
	return replay.Snapshot{
		Channels: []replay.ChannelSnapshot{{
			Active:     true,
			Trigger:    trigger,
			Note:       note,
			Sample:     0,
			Volume:     volume,
			Panning:    panning,
			KeyOn:      keyOn,
			PitchDelta: 0,
		}},
	}
}

func hasNonZero(values []float32) bool {
	for _, value := range values {
		if value != 0 {
			return true
		}
	}
	return false
}

func equalBuffers(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
