package replay

import (
	"testing"

	modmodel "github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/pitch"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

func TestSchedulerHandlesJumpAndBreak(t *testing.T) {
	mod := buildReplayModule(1, 125, false)
	mod.Orders = []uint16{0, 1}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 3, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.PTEffect(0x0b, 1)
				b.PTEffect(0x0d, 1)
			},
			func(b *unitrk.Builder) {},
		)}},
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {},
			func(b *unitrk.Builder) {
				b.Note(60)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	assertPosition(t, engine.Snapshot(), 0, 0, 0)

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	assertPosition(t, engine.Snapshot(), 0, 1, 0)

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	assertPosition(t, engine.Snapshot(), 1, 1, 0)
}

func TestInitialActiveRowUsesModuleTempoForTickLength(t *testing.T) {
	mod := buildReplayModule(3, 130, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	if got := engine.CurrentTickFrames(); got != 848 {
		t.Fatalf("expected active row to use module tempo 130 for tick length, got %d", got)
	}
}

func TestSilentRowsKeepLegacyTickTempoUntilFirstVoice(t *testing.T) {
	mod := buildReplayModule(3, 130, false)
	mod.Orders = []uint16{0, 1}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 4, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {},
			func(b *unitrk.Builder) {},
			func(b *unitrk.Builder) {},
			func(b *unitrk.Builder) {
				b.PTEffect(0x0d, 0x00)
			},
		)}},
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	if got := engine.CurrentTickFrames(); got != 882 {
		t.Fatalf("expected silent intro row to keep legacy 125 BPM tick length, got %d", got)
	}

	for i := 0; i < 12; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}
	assertPosition(t, engine.Snapshot(), 1, 0, 0)
	if got := engine.CurrentTickFrames(); got != 848 {
		t.Fatalf("expected first active row to switch timing to module tempo 130, got %d", got)
	}
}

func TestITNNATriggersDetachedVoiceSnapshots(t *testing.T) {
	mod := buildReplayModule(1, 125, true)
	mod.Flags |= modmodel.FlagUsesNNA
	mod.Voices = 4
	mod.Instruments[0].NewNoteAction = modmodel.NNAContinue
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.Note(52)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	initial := engine.Snapshot()
	if len(initial.Voices) != 4 {
		t.Fatalf("unexpected voice snapshot size: %d", len(initial.Voices))
	}
	if !initial.Voices[0].Active || initial.Voices[0].Trigger != 1 {
		t.Fatalf("expected first IT voice to occupy slot 0: %+v", initial.Voices[0])
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}

	got := engine.Snapshot()
	if !got.Voices[0].Active || got.Voices[0].Trigger != 1 {
		t.Fatalf("expected previous IT note to remain detached in slot 0: %+v", got.Voices[0])
	}
	if !got.Voices[1].Active || got.Voices[1].Trigger != 2 {
		t.Fatalf("expected new IT note to allocate slot 1: %+v", got.Voices[1])
	}
	if got.Channels[0].Trigger != 2 {
		t.Fatalf("expected foreground channel trigger to advance to the new note: %+v", got.Channels[0])
	}
}

func TestITNNACutReusesForegroundVoiceSlot(t *testing.T) {
	mod := buildReplayModule(1, 125, true)
	mod.Flags |= modmodel.FlagUsesNNA
	mod.Voices = 4
	mod.Instruments[0].NewNoteAction = modmodel.NNACut
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.Note(52)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	initial := engine.Snapshot()
	if !initial.Voices[0].Active || initial.Voices[0].Trigger != 1 {
		t.Fatalf("expected first IT voice to occupy slot 0: %+v", initial.Voices[0])
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}

	got := engine.Snapshot()
	if !got.Voices[0].Active || got.Voices[0].Trigger != 2 || got.Voices[0].Note != 52 {
		t.Fatalf("expected NNA cut to reuse slot 0 for the new note: %+v", got.Voices[0])
	}
	if got.Voices[1].Active {
		t.Fatalf("expected no detached voice allocation on NNA cut: %+v", got.Voices[1])
	}
}

func TestSchedulerHandlesPatternLoopAndDelay(t *testing.T) {
	mod := buildReplayModule(1, 125, false)
	mod.Orders = []uint16{0, 1}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 4, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.PTEffect(0x0e, 0x60)
			},
			func(b *unitrk.Builder) {},
			func(b *unitrk.Builder) {
				b.PTEffect(0x0e, 0x62)
			},
			func(b *unitrk.Builder) {},
		)}},
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.PTEffect(0x0e, 0xe2)
			},
			func(b *unitrk.Builder) {},
		)}},
	}

	engine := newEngineForTest(t, mod)
	wantRows := []struct {
		order int
		row   int
	}{
		{0, 1},
		{0, 2},
		{0, 0},
		{0, 1},
		{0, 2},
		{0, 0},
		{0, 1},
		{0, 2},
		{0, 3},
		{1, 0},
		{1, 0},
		{1, 0},
		{1, 1},
	}

	for i, want := range wantRows {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
		assertPosition(t, engine.Snapshot(), want.order, want.row, 0)
	}
}

func TestEngineHandlesKeyFadeEnvelopeAndNoteCut(t *testing.T) {
	mod := buildReplayModule(1, 125, true)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 4, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.KeyFade()
			},
			func(b *unitrk.Builder) {},
			func(b *unitrk.Builder) {
				b.PTEffect(0x0e, 0xc0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	initial := engine.Snapshot().Channels[0]
	if !initial.Active || initial.EnvelopeVolume != 0 {
		t.Fatalf("unexpected initial channel state: %+v", initial)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	fading := engine.Snapshot().Channels[0]
	if !fading.KeyFade {
		t.Fatalf("expected key fade to be active: %+v", fading)
	}
	if fading.FadeVolume >= initial.FadeVolume {
		t.Fatalf("expected fade volume to decrease: start=%d current=%d", initial.FadeVolume, fading.FadeVolume)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	progressed := engine.Snapshot().Channels[0]
	if progressed.EnvelopeVolume <= initial.EnvelopeVolume {
		t.Fatalf("expected envelope progression: start=%d current=%d", initial.EnvelopeVolume, progressed.EnvelopeVolume)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	cut := engine.Snapshot().Channels[0]
	if cut.Active || cut.Volume != 0 {
		t.Fatalf("expected note cut to stop the channel: %+v", cut)
	}
}

func TestKeyFadeUsesCurrentFadeVolumeForCurrentTick(t *testing.T) {
	mod := buildReplayModule(1, 125, true)
	mod.Instruments[0].FadeOut = 64
	mod.Instruments[0].VolumeEnvelope = modmodel.Envelope{
		Flags: modmodel.EnvelopeEnabled | modmodel.EnvelopeVolume,
		Points: []modmodel.EnvelopePoint{
			{Tick: 0, Value: 256},
		},
	}
	mod.Instruments[0].PanningEnvelope = modmodel.Envelope{}
	mod.Instruments[0].PitchEnvelope = modmodel.Envelope{}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.KeyFade()
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if start.Volume != 256 || start.FadeVolume != maxFadeVolume {
		t.Fatalf("unexpected initial channel state: %+v", start)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	fading := engine.Snapshot().Channels[0]
	if !fading.KeyFade {
		t.Fatalf("expected key fade to be active: %+v", fading)
	}
	if fading.Volume != 256 {
		t.Fatalf("expected current tick volume to use the pre-fade value, got %+v", fading)
	}
	if fading.FadeVolume != maxFadeVolume-2 {
		t.Fatalf("expected fade volume to be prepared for the next tick, got %+v", fading)
	}
}

func TestEngineAppliesVibratoTremoloPanbrelloAndAutoVibrato(t *testing.T) {
	mod := buildReplayModule(2, 125, true)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.PTEffect(0x04, 0x47)
				b.PTEffect(0x07, 0x34)
				b.Effect(unitrk.OpITEffectY, 0x23)
			},
			func(b *unitrk.Builder) {
				b.PTEffect(0x04, 0x00)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	tickOne := engine.Snapshot().Channels[0]
	if tickOne.VolumeDelta == 0 {
		t.Fatalf("expected tremolo delta on tick 1: %+v", tickOne)
	}
	if tickOne.PanningDelta == 0 {
		t.Fatalf("expected panbrello delta on tick 1: %+v", tickOne)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	memory := engine.Snapshot().Channels[0]
	if memory.PitchDelta == 0 && memory.Frequency == oldFrequencyFromPeriod(engine.notePeriod(0, 48)) {
		t.Fatalf("expected vibrato memory to keep modulation active: %+v", memory)
	}
}

func TestOldPeriodSlideDoesNotPersistOnBlankRow(t *testing.T) {
	mod := buildReplayModule(2, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.PTEffect(0x01, 0x01)
			},
			func(b *unitrk.Builder) {},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if start.Frequency == 0 {
		t.Fatalf("expected exact old-period frequency on tick 0: %+v", start)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	slid := engine.Snapshot().Channels[0]
	if slid.Frequency <= start.Frequency {
		t.Fatalf("expected slide-up to increase frequency: start=%d slid=%d", start.Frequency, slid.Frequency)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowOneTickZero := engine.Snapshot().Channels[0]

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowOneTickOne := engine.Snapshot().Channels[0]
	if rowOneTickOne.Frequency != rowOneTickZero.Frequency {
		t.Fatalf("expected blank row to keep the slid frequency: tick0=%d tick1=%d", rowOneTickZero.Frequency, rowOneTickOne.Frequency)
	}
}

func TestS3MVolumeSlideCanApplyOnTickZeroForScreamTracker(t *testing.T) {
	mod := buildReplayModule(2, 125, false)
	mod.Flags |= modmodel.FlagUsesS3MSlides
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.Effect(unitrk.OpS3MEffectD, 0x01)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	got := engine.Snapshot().Channels[0]
	if got.Volume >= 256 {
		t.Fatalf("expected S3M volume slide to reduce volume on tick 0: %+v", got)
	}
}

func TestXMExtraFinePortamentoDownUsesPeriodFrequency(t *testing.T) {
	mod := buildReplayModule(3, 125, false)
	mod.Metadata.Format = modmodel.FormatXM
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagUsesPanning
	mod.Samples[0].Vibrato = modmodel.AutoVibrato{}
	mod.Patterns = []modmodel.Pattern{{
		Rows: 2,
		Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.Effect(unitrk.OpXMEffectX2, 12)
			},
		)},
	}}

	engine, err := New(mod, Config{SampleRate: 44100})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	start := engine.Snapshot().Channels[0]
	basePeriod := pitch.XMPeriod(start.Note, mod.Samples[0].FineTune, false)

	for i := 0; i < int(mod.InitialSpeed); i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	got := engine.Snapshot().Channels[0]
	wantPeriod := basePeriod + 12
	wantFrequency := pitch.XMFrequencyFromPeriod(wantPeriod, false)
	if got.Frequency != wantFrequency {
		t.Fatalf("expected XM X2 to use period-based frequency: got=%d want=%d", got.Frequency, wantFrequency)
	}
}

func TestXMVibratoUsesLibMikModSpeedAndPeriodDelta(t *testing.T) {
	mod := buildReplayModule(3, 125, false)
	mod.Metadata.Format = modmodel.FormatXM
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagUsesPanning
	mod.Samples[0].Vibrato = modmodel.AutoVibrato{}
	mod.Patterns = []modmodel.Pattern{{
		Rows: 2,
		Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(0)
				b.Effect(unitrk.OpXMEffect4, 0x72)
			},
			func(b *unitrk.Builder) {
				b.Effect(unitrk.OpXMEffect4, 0)
			},
		)},
	}}

	engine, err := New(mod, Config{SampleRate: 44100})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	base := engine.Snapshot().Channels[0]
	basePeriod := pitch.XMPeriod(base.Note, mod.Samples[0].FineTune, false)
	for i := 0; i < 2; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	got := engine.Snapshot().Channels[0]
	wantPeriod := basePeriod + xmVibratoDelta(0, 28, 2)
	wantFrequency := pitch.XMFrequencyFromPeriod(wantPeriod, false)
	if got.Frequency != wantFrequency {
		t.Fatalf("expected XM vibrato to use libmikmod period delta: got=%d want=%d", got.Frequency, wantFrequency)
	}
}

func TestXMEffectE5SetsChannelFineTuneAndResetsOnNextTrigger(t *testing.T) {
	mod := buildReplayModule(3, 125, false)
	mod.Metadata.Format = modmodel.FormatXM
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagLinearPeriods | modmodel.FlagUsesPanning
	mod.Samples[0].FineTune = 31
	mod.Samples[0].Vibrato = modmodel.AutoVibrato{}
	mod.Patterns = []modmodel.Pattern{{
		Rows: 2,
		Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(0)
				b.PTEffect(0x0e, 0x51)
			},
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(0)
			},
		)},
	}}

	engine := newEngineForTest(t, mod)
	rowZero := engine.channels[0]
	wantRowZeroPeriod := pitch.XMPeriod(60, 31, true)
	if rowZero.period != wantRowZeroPeriod || rowZero.xmFineTune != 1 || !rowZero.xmFineTunePending {
		t.Fatalf("expected XM E51 on a fresh trigger to keep tick 0 period and defer the retune: finetune=%d pending=%v period=%d want_period=%d", rowZero.xmFineTune, rowZero.xmFineTunePending, rowZero.period, wantRowZeroPeriod)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}

	tickOne := engine.Snapshot().Channels[0]
	wantTickOne := pitch.XMFrequencyFromPeriod(pitch.XMPeriod(60, 1, true), true)
	if tickOne.Frequency != wantTickOne {
		t.Fatalf("expected XM E51 to retune from tick 1 onward: got=%d want=%d snapshot=%+v", tickOne.Frequency, wantTickOne, tickOne)
	}

	for i := 1; i < int(mod.InitialSpeed); i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	rowOne := engine.channels[0]
	wantRowOnePeriod := pitch.XMPeriod(60, 31, true)
	if rowOne.xmFineTune != 31 || rowOne.period != wantRowOnePeriod {
		t.Fatalf("expected next XM trigger to restore the sample finetune: finetune=%d period=%d want_finetune=%d want_period=%d", rowOne.xmFineTune, rowOne.period, 31, wantRowOnePeriod)
	}
}

func TestPTEffectEFineVolumeSlidesApplyOnTickZero(t *testing.T) {
	mod := buildReplayModule(3, 125, false)
	mod.Metadata.Format = modmodel.FormatXM
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagUsesPanning
	mod.Patterns = []modmodel.Pattern{{
		Rows: 3,
		Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.PTEffect(0x0c, 0x00)
			},
			func(b *unitrk.Builder) {
				b.PTEffect(0x0e, 0xa2)
			},
			func(b *unitrk.Builder) {
				b.PTEffect(0x0e, 0xb1)
			},
		)},
	}}

	engine := newEngineForTest(t, mod)
	if got := engine.channels[0].baseVolume; got != 0 {
		t.Fatalf("expected row 0 C00 to zero the base volume: %d", got)
	}

	for i := 0; i < int(mod.InitialSpeed); i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}
	if got := engine.channels[0].baseVolume; got != 2 {
		t.Fatalf("expected EA2 to raise the base volume on tick 0 of the next row: %d", got)
	}

	for i := 0; i < int(mod.InitialSpeed); i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1+int(mod.InitialSpeed), err)
		}
	}
	if got := engine.channels[0].baseVolume; got != 1 {
		t.Fatalf("expected EB1 to lower the base volume on tick 0 of the following row: %d", got)
	}
}

func TestS3MFinePeriodSlideAppliesOnTickZero(t *testing.T) {
	base := buildReplayModule(2, 125, false)
	base.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
		)}},
	}
	baseEngine := newEngineForTest(t, base)
	baseFrequency := baseEngine.Snapshot().Channels[0].Frequency

	mod := buildReplayModule(2, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.Effect(unitrk.OpS3MEffectE, 0xe1)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	got := engine.Snapshot().Channels[0]
	if got.Frequency >= baseFrequency {
		t.Fatalf("expected fine S3M slide-down to lower tick 0 frequency: base=%d got=%d", baseFrequency, got.Frequency)
	}
}

func TestS3MPeriodSlidesShareEffectMemory(t *testing.T) {
	mod := buildReplayModule(3, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.Effect(unitrk.OpS3MEffectE, 0x02)
			},
			func(b *unitrk.Builder) {
				b.Effect(unitrk.OpS3MEffectF, 0x00)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	rowZeroTickZero := engine.Snapshot().Channels[0]

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowZeroTickOne := engine.Snapshot().Channels[0]
	if rowZeroTickOne.Frequency >= rowZeroTickZero.Frequency {
		t.Fatalf("expected S3M E02 to lower frequency on tick 1: start=%d tick1=%d", rowZeroTickZero.Frequency, rowZeroTickOne.Frequency)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowOneTickZero := engine.Snapshot().Channels[0]

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowOneTickOne := engine.Snapshot().Channels[0]
	if rowOneTickOne.Frequency <= rowOneTickZero.Frequency {
		t.Fatalf("expected S3M F00 to reuse E memory and raise frequency: tick0=%d tick1=%d", rowOneTickZero.Frequency, rowOneTickOne.Frequency)
	}
}

func TestSampleOffsetIsExposedOnTriggeredRow(t *testing.T) {
	mod := buildReplayModule(2, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.PTEffect(0x09, 0x02)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	got := engine.Snapshot().Channels[0]
	if got.SampleOffset != 0x0200 {
		t.Fatalf("expected sample offset memory on triggered row: %+v", got)
	}
}

func TestSampleOwnPanningOverridesInstrumentPanning(t *testing.T) {
	mod := buildReplayModule(2, 125, true)
	mod.Samples[0].Flags |= modmodel.SampleOwnPanning
	mod.Samples[0].Panning = 48
	mod.Instruments[0].Flags |= modmodel.InstrumentOwnPanning
	mod.Instruments[0].Panning = 200
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	got := engine.Snapshot().Channels[0]
	if got.Panning != 48 {
		t.Fatalf("expected sample own panning to win over instrument panning: %+v", got)
	}
}

func TestInstrumentPitchPanOffsetsComputedPanning(t *testing.T) {
	mod := buildReplayModule(2, 125, true)
	mod.Flags |= modmodel.FlagUsesPanning
	mod.ChannelSettings[0].Panning = modmodel.PanCenter
	mod.Instruments[0].Flags |= modmodel.InstrumentPitchPan
	mod.Instruments[0].PitchPanCenter = 48
	mod.Instruments[0].PitchPanSeparation = 16
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	got := engine.Snapshot().Channels[0]
	if got.Panning != 152 {
		t.Fatalf("expected instrument pitch-pan separation to bias panning: %+v", got)
	}
}

func TestInstrumentPitchPanUsesRawPatternNote(t *testing.T) {
	mod := buildReplayModule(2, 125, true)
	mod.Flags |= modmodel.FlagUsesPanning
	mod.ChannelSettings[0].Panning = modmodel.PanCenter
	mod.Instruments[0].Flags |= modmodel.InstrumentPitchPan
	mod.Instruments[0].PitchPanCenter = 48
	mod.Instruments[0].PitchPanSeparation = 16
	mod.Instruments[0].NoteMap[48] = modmodel.NoteSample{Note: 60, Sample: 0}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	got := engine.Snapshot().Channels[0]
	if got.Note != 60 {
		t.Fatalf("expected mapped playback note to be preserved: %+v", got)
	}
	if got.Panning != 128 {
		t.Fatalf("expected pitch-pan to use the raw pattern note instead of the mapped note: %+v", got)
	}
}

func TestInstrumentRandomPanningVariationAppliesOnInstrumentChangeOnly(t *testing.T) {
	mod := buildReplayModule(2, 125, true)
	mod.Instruments[0].Flags |= modmodel.InstrumentOwnPanning
	mod.Instruments[0].Panning = 128
	mod.Instruments[0].RandomPanningVar = 64
	mod.Instruments[0].VolumeEnvelope = modmodel.Envelope{}
	mod.Instruments[0].PanningEnvelope = modmodel.Envelope{}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.Note(50)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if start.Panning <= 128 {
		t.Fatalf("expected instrument random panning to offset the triggered note: %+v", start)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	got := engine.Snapshot().Channels[0]
	if got.Panning != start.Panning {
		t.Fatalf("expected note without instrument change to keep the randomized panning: start=%+v got=%+v", start, got)
	}
}

func TestInstrumentRandomVolumeVariationAppliesOnInstrumentChange(t *testing.T) {
	mod := buildReplayModule(2, 125, true)
	mod.Instruments[0].RandomVolumeVar = 64
	mod.Instruments[0].VolumeEnvelope = modmodel.Envelope{}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	got := engine.Snapshot().Channels[0]
	if got.Volume <= 192 {
		t.Fatalf("expected instrument random volume to raise the triggered note volume: %+v", got)
	}
}

func TestITSpecialPanningSetsChannelMemoryAndTriggeredPan(t *testing.T) {
	mod := buildReplayModule(2, 125, false)
	mod.Channels = 2
	mod.Voices = 2
	mod.Flags |= modmodel.FlagUsesPanning
	mod.ChannelSettings[0].Panning = modmodel.PanCenter
	mod.ChannelSettings[1].Panning = modmodel.PanCenter
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{
			buildTrack(func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.Effect(unitrk.OpITEffectS0, 0x86)
			}),
			buildTrack(func(b *unitrk.Builder) {
				b.Effect(unitrk.OpITEffectS0, 0x91)
			}),
		}},
	}

	engine := newEngineForTest(t, mod)
	first := engine.Snapshot()
	if got := first.Channels[0].Panning; got != 96 {
		t.Fatalf("expected IT S86 to set trigger-row panning to 96, got %d", got)
	}
	if got := first.Channels[1].Panning; got != int(modmodel.PanSurround) {
		t.Fatalf("expected IT S91 to set surround panning memory, got %d", got)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	second := engine.Snapshot()
	if got := second.Channels[0].Panning; got != 96 {
		t.Fatalf("expected IT S86 panning memory to persist, got %d", got)
	}
	if got := second.Channels[1].Panning; got != int(modmodel.PanSurround) {
		t.Fatalf("expected IT S91 surround memory to persist, got %d", got)
	}
}

func TestSustainEnvelopeStartsOnInitialPointWithoutInterpolating(t *testing.T) {
	mod := buildReplayModule(3, 125, true)
	mod.Flags |= modmodel.FlagUsesPanning
	mod.ChannelSettings[0].Panning = modmodel.PanCenter
	mod.Instruments[0].PanningEnvelope = modmodel.Envelope{
		Flags:        modmodel.EnvelopeEnabled | modmodel.EnvelopeSustain,
		SustainStart: 0,
		SustainEnd:   0,
		Points: []modmodel.EnvelopePoint{
			{Tick: 0, Value: 128},
			{Tick: 2, Value: 160},
		},
	}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if start.Panning != 128 {
		t.Fatalf("expected trigger tick to use the first sustain point exactly: %+v", start)
	}

	for i := 0; i < 2; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	held := engine.Snapshot().Channels[0]
	if held.Panning != 128 {
		t.Fatalf("expected single-point sustain envelope to hold panning until keyoff: %+v", held)
	}
}

func TestPortamentoWithoutSpeedMemoryTriggersNoteNormally(t *testing.T) {
	mod := buildReplayModule(2, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(0)
				b.PTEffect(0x03, 0x00)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}

	got := engine.Snapshot().Channels[0]
	if got.Trigger == start.Trigger {
		t.Fatalf("expected G00 without speed memory to play the new note normally: start=%d got=%d", start.Trigger, got.Trigger)
	}
}

func TestPortamentoWithInstrumentChangeUpdatesVolumeAndPanning(t *testing.T) {
	mod := buildReplayModule(2, 125, false)
	mod.Samples = append(mod.Samples, modmodel.Sample{
		Name:         "target",
		Volume:       16,
		GlobalVolume: 64,
		C5Speed:      8363,
		Panning:      255,
		Flags:        modmodel.SampleOwnPanning,
		Data:         []int16{0, 1, 0, -1},
	})
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(1)
				b.PTEffect(0x03, 0x10)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	for i := 0; i < 2; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	got := engine.Snapshot().Channels[0]
	if got.Trigger != start.Trigger {
		t.Fatalf("expected tone portamento instrument change to avoid retrigger: start=%d got=%d", start.Trigger, got.Trigger)
	}
	if got.Volume >= start.Volume {
		t.Fatalf("expected instrument change on tone portamento to refresh base volume: start=%d got=%d", start.Volume, got.Volume)
	}
	if got.Panning <= start.Panning {
		t.Fatalf("expected instrument change on tone portamento to refresh panning: start=%d got=%d", start.Panning, got.Panning)
	}
}

func TestPortamentoNoteResetsEnvelopesWithoutRetrigger(t *testing.T) {
	mod := buildReplayModule(3, 125, true)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(0)
				b.PTEffect(0x03, 0x10)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if start.EnvelopeVolume != 0 {
		t.Fatalf("expected initial envelope to start on the first point: %+v", start)
	}

	for i := 0; i < 3; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	rowOne := engine.Snapshot().Channels[0]
	if rowOne.Trigger != start.Trigger {
		t.Fatalf("expected tone portamento note to avoid sample retrigger: start=%d got=%d", start.Trigger, rowOne.Trigger)
	}
	if rowOne.EnvelopeVolume != 0 {
		t.Fatalf("expected tone portamento note to reset the envelope on the new row: %+v", rowOne)
	}
}

func TestXMTonePortamentoSlidesPeriod(t *testing.T) {
	mod := buildReplayModule(3, 125, false)
	mod.Metadata.Format = modmodel.FormatXM
	mod.Flags = modmodel.FlagXMPeriods | modmodel.FlagUsesPanning
	mod.Samples[0].Vibrato = modmodel.AutoVibrato{}
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.Note(60)
				b.Instrument(0)
				b.PTEffect(0x03, 0x01)
			},
		)}},
	}

	engine, err := New(mod, Config{SampleRate: 44100})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	startPeriod := engine.channels[0].period
	for i := 0; i < int(mod.InitialSpeed); i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	if got := engine.channels[0].period; got != startPeriod {
		t.Fatalf("expected XM tone portamento tick 0 to keep the original period: got=%d want=%d", got, startPeriod)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks tick 1 failed: %v", err)
	}

	wantPeriod := startPeriod - 4
	if got := engine.channels[0].period; got != wantPeriod {
		t.Fatalf("expected XM tone portamento to slide in period space: got=%d want=%d", got, wantPeriod)
	}

	got := engine.Snapshot().Channels[0]
	wantFrequency := pitch.XMFrequencyFromPeriod(wantPeriod, false)
	if got.Frequency != wantFrequency {
		t.Fatalf("expected XM tone portamento snapshot frequency to follow the slid period: got=%d want=%d", got.Frequency, wantFrequency)
	}
}

func TestPortamentoWithoutInstrumentKeepsCurrentVolume(t *testing.T) {
	mod := buildReplayModule(2, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
			},
			func(b *unitrk.Builder) {
				b.PTEffect(0x0c, 0x10)
				b.Note(60)
				b.PTEffect(0x03, 0x10)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	for i := 0; i < 2; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	got := engine.Snapshot().Channels[0]
	if got.Volume <= 0 || got.Volume >= start.Volume {
		t.Fatalf("expected note-only tone portamento to preserve the explicit lower channel volume: start=%d got=%d", start.Volume, got.Volume)
	}
}

func TestS3MTremorMutesOffPhaseOnLaterTicks(t *testing.T) {
	mod := buildReplayModule(4, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.Effect(unitrk.OpS3MEffectI, 0x11)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if start.Volume == 0 {
		t.Fatalf("expected tremor tick 0 to preserve the base volume: %+v", start)
	}

	for i := 0; i < 3; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	off := engine.Snapshot().Channels[0]
	if off.Volume != 0 {
		t.Fatalf("expected tremor off-phase to mute the channel: %+v", off)
	}
}

func TestS3MOldStyleVibratoUsesCurrentPhaseOnTickZero(t *testing.T) {
	mod := buildReplayModule(3, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.Effect(unitrk.OpS3MEffectH, 0x14)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	got := engine.Snapshot().Channels[0]
	want := oldFrequencyFromPeriod(engine.notePeriod(0, 48))
	if got.Frequency != want {
		t.Fatalf("expected S3M vibrato tick 0 to use the current phase and preserve the base period: got=%d want=%d snapshot=%+v", got.Frequency, want, got)
	}
}

func TestS3MRetrigRestartsSampleOnLaterTicks(t *testing.T) {
	mod := buildReplayModule(4, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.Effect(unitrk.OpS3MEffectQ, 0x11)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}

	retrig := engine.Snapshot().Channels[0]
	if retrig.Trigger == start.Trigger {
		t.Fatalf("expected S3M retrig to restart the sample on tick 1: start=%d got=%d", start.Trigger, retrig.Trigger)
	}
	if retrig.Volume >= start.Volume {
		t.Fatalf("expected retrig volume slide to attenuate the sample: start=%d got=%d", start.Volume, retrig.Volume)
	}
}

func TestS3MRetrigStopsOnFollowingBlankRow(t *testing.T) {
	mod := buildReplayModule(3, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.Effect(unitrk.OpS3MEffectQ, 0x11)
			},
			func(b *unitrk.Builder) {},
		)}},
	}

	engine := newEngineForTest(t, mod)
	for i := 0; i < 2; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	retriggered := engine.Snapshot().Channels[0]
	if retriggered.Trigger == 0 {
		t.Fatalf("expected retrig on the first row: %+v", retriggered)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowOneTickZero := engine.Snapshot().Channels[0]
	startTrigger := rowOneTickZero.Trigger

	for i := 0; i < 2; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	blankRow := engine.Snapshot().Channels[0]
	if blankRow.Trigger != startTrigger {
		t.Fatalf("expected S3M retrig to stop on the following blank row: start=%d got=%d", startTrigger, blankRow.Trigger)
	}
}

func TestNoteDelayPreservesExplicitRowVolumeOnDelayedTrigger(t *testing.T) {
	mod := buildReplayModule(4, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 1, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.PTEffect(0x0c, 0x08)
				b.Effect(unitrk.OpITEffectS0, 0xd2)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if start.Trigger != 0 || start.Volume != 0 {
		t.Fatalf("expected delayed note to remain silent on tick 0: %+v", start)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	tickOne := engine.Snapshot().Channels[0]
	if tickOne.Trigger != 0 || tickOne.Volume != 0 {
		t.Fatalf("expected delayed note to remain silent on tick 1: %+v", tickOne)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	triggered := engine.Snapshot().Channels[0]
	if triggered.Trigger == 0 {
		t.Fatalf("expected delayed note to trigger on tick 2: %+v", triggered)
	}
	if triggered.Volume == 0 || triggered.Volume >= 64 {
		t.Fatalf("expected delayed note to honor explicit row volume 8 instead of sample default: %+v", triggered)
	}
}

func TestNoteDelayKeepsPreviousVoiceUntilTrigger(t *testing.T) {
	mod := buildReplayModule(4, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(45)
				b.Instrument(0)
				b.PTEffect(0x0c, 0x20)
			},
			func(b *unitrk.Builder) {
				b.Note(57)
				b.Instrument(0)
				b.PTEffect(0x0c, 0x08)
				b.Effect(unitrk.OpITEffectS0, 0xd2)
			},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]
	if start.Trigger == 0 || start.Note != 45 || start.Volume == 0 {
		t.Fatalf("expected first row to start the baseline voice: %+v", start)
	}

	for i := 0; i < 4; i++ {
		if err := engine.AdvanceTicks(1); err != nil {
			t.Fatalf("AdvanceTicks(%d) failed: %v", i+1, err)
		}
	}

	rowOneTickZero := engine.Snapshot().Channels[0]
	if rowOneTickZero.Trigger != start.Trigger || rowOneTickZero.Note != start.Note || rowOneTickZero.Volume != start.Volume {
		t.Fatalf("expected delayed row tick 0 to keep the previous voice untouched: start=%+v row=%+v", start, rowOneTickZero)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowOneTickOne := engine.Snapshot().Channels[0]
	if rowOneTickOne.Trigger != start.Trigger || rowOneTickOne.Note != start.Note || rowOneTickOne.Volume != start.Volume {
		t.Fatalf("expected delayed row tick 1 to keep the previous voice untouched: start=%+v row=%+v", start, rowOneTickOne)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	triggered := engine.Snapshot().Channels[0]
	if triggered.Trigger == start.Trigger || triggered.Note != 57 || triggered.Volume == 0 || triggered.Volume >= start.Volume {
		t.Fatalf("expected delayed note to replace the previous voice only on tick 2: start=%+v triggered=%+v", start, triggered)
	}
}

func TestVolumeSlideDoesNotPersistOnBlankRow(t *testing.T) {
	mod := buildReplayModule(2, 125, false)
	mod.Patterns = []modmodel.Pattern{
		{Rows: 2, Tracks: []unitrk.Track{buildTrack(
			func(b *unitrk.Builder) {
				b.Note(48)
				b.Instrument(0)
				b.PTEffect(0x0a, 0x01)
			},
			func(b *unitrk.Builder) {},
		)}},
	}

	engine := newEngineForTest(t, mod)
	start := engine.Snapshot().Channels[0]

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	slid := engine.Snapshot().Channels[0]
	if slid.Volume >= start.Volume {
		t.Fatalf("expected volume slide to reduce playback volume: start=%d slid=%d", start.Volume, slid.Volume)
	}

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowOneTickZero := engine.Snapshot().Channels[0]

	if err := engine.AdvanceTicks(1); err != nil {
		t.Fatalf("AdvanceTicks failed: %v", err)
	}
	rowOneTickOne := engine.Snapshot().Channels[0]
	if rowOneTickOne.Volume != rowOneTickZero.Volume {
		t.Fatalf("expected blank row to preserve the slid volume: tick0=%d tick1=%d", rowOneTickZero.Volume, rowOneTickOne.Volume)
	}
}

func newEngineForTest(t *testing.T, mod *modmodel.Module) *Engine {
	t.Helper()
	if err := mod.Validate(); err != nil {
		t.Fatalf("module validation failed: %v", err)
	}
	engine, err := New(mod, Config{SampleRate: 44100})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return engine
}

func buildReplayModule(speed uint8, tempo uint16, instruments bool) *modmodel.Module {
	mod := modmodel.New()
	mod.Channels = 1
	mod.Voices = 1
	mod.InitialSpeed = speed
	mod.InitialTempo = tempo
	mod.Orders = []uint16{0}
	mod.Samples = []modmodel.Sample{{
		Name:         "sample",
		Volume:       64,
		GlobalVolume: 64,
		C5Speed:      8363,
		Data:         []int16{0, 1, 0, -1},
		Vibrato: modmodel.AutoVibrato{
			Flags:    modmodel.AutoVibratoIT,
			Waveform: 0,
			Depth:    6,
			Rate:     4,
			Sweep:    2,
		},
	}}
	if instruments {
		mod.Flags |= modmodel.FlagUsesInstruments
		instrument := modmodel.Instrument{
			GlobalVolume: 128,
			FadeOut:      256,
			Panning:      -1,
			VolumeEnvelope: modmodel.Envelope{
				Flags:        modmodel.EnvelopeEnabled | modmodel.EnvelopeVolume,
				SustainStart: 1,
				SustainEnd:   1,
				Points: []modmodel.EnvelopePoint{
					{Tick: 0, Value: 0},
					{Tick: 1, Value: 256},
					{Tick: 3, Value: 128},
				},
			},
			PanningEnvelope: modmodel.Envelope{
				Flags: modmodel.EnvelopeEnabled,
				Points: []modmodel.EnvelopePoint{
					{Tick: 0, Value: 128},
					{Tick: 2, Value: 160},
				},
			},
			PitchEnvelope: modmodel.Envelope{
				Flags: modmodel.EnvelopeEnabled,
				Points: []modmodel.EnvelopePoint{
					{Tick: 0, Value: 32},
					{Tick: 2, Value: 40},
				},
			},
		}
		for i := range instrument.NoteMap {
			instrument.NoteMap[i] = modmodel.NoteSample{Note: uint8(i), Sample: 0}
		}
		mod.Instruments = []modmodel.Instrument{instrument}
	}
	return mod
}

func buildTrack(rows ...func(*unitrk.Builder)) unitrk.Track {
	var builder unitrk.Builder
	builder.Reset()
	builder.SetArpeggioMemory(true)
	for _, row := range rows {
		row(&builder)
		builder.NewLine()
	}
	return builder.Track()
}

func assertPosition(t *testing.T, snapshot Snapshot, order, row, tick int) {
	t.Helper()
	if snapshot.Order != order || snapshot.Row != row || snapshot.Tick != tick {
		t.Fatalf("unexpected position: got order=%d row=%d tick=%d want order=%d row=%d tick=%d", snapshot.Order, snapshot.Row, snapshot.Tick, order, row, tick)
	}
}
