package replay

import (
	"fmt"
	"math"

	modmodel "github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/pitch"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

const (
	defaultVolumeEnvelope = 256
	defaultPanningEnv     = 128
	defaultPitchEnv       = 32
	maxFadeVolume         = 1024
)

var farTempoTable = [...]int{256, 128, 64, 42, 32, 25, 21, 18, 16, 14, 12, 11, 10, 9, 9, 8}

type Config struct {
	SampleRate int
}

type Snapshot struct {
	Order        int
	Pattern      int
	Row          int
	Tick         int
	Speed        int
	Tempo        int
	GlobalVolume int
	Ended        bool
	Channels     []ChannelSnapshot
	Voices       []ChannelSnapshot
}

type ChannelSnapshot struct {
	Active          bool
	Trigger         uint64
	Note            int
	Instrument      int
	Sample          int
	SampleOffset    int
	Frequency       int
	Volume          int
	Panning         int
	KeyOn           bool
	KeyFade         bool
	FadeVolume      int
	EnvelopeVolume  int
	EnvelopePanning int
	EnvelopePitch   int
	PitchDelta      int
	VolumeDelta     int
	PanningDelta    int
}

type Engine struct {
	module            *modmodel.Module
	sampleRate        int
	randState         uint32
	order             int
	row               int
	tick              int
	speed             int
	tempo             int
	timingTempo       int
	globalVolume      int
	rowDelayRemaining int
	currentTickFrames int
	ended             bool
	flow              flowState
	channels          []channelState
	detachedVoices    []detachedVoice
	channelVoices     []int
	voiceCount        int
	snapshot          Snapshot
}

type detachedVoice struct {
	slot  int
	state channelState
}

type flowState struct {
	jumpOrder int
	breakRow  int
	loopRow   int
}

type channelState struct {
	active               bool
	trigger              uint64
	note                 int
	panNote              int
	xmFineTune           int16
	xmFineTunePending    bool
	pitch                int
	targetPitch          int
	period               int
	periodDelta          int
	targetPeriod         int
	volume               int
	baseVolume           int
	channelVolume        int
	panning              int
	basePanning          int
	keyOn                bool
	keyFade              bool
	fadeVolume           int
	instrument           int
	lastInstrument       int
	sample               int
	sampleOffset         int
	sampleOffsetMemory   int
	noteCutTick          int
	loopStartRow         int
	loopCount            int
	volumeEnvelope       int
	panningEnvelope      int
	pitchEnvelope        int
	pitchDelta           int
	volumeDelta          int
	panningDelta         int
	arpeggio             byte
	arpeggioActive       bool
	ptSlideUp            byte
	ptSlideUpActive      bool
	ptSlideDown          byte
	ptSlideDownActive    bool
	s3mPeriodSlide       byte
	s3mSlideUp           byte
	s3mSlideUpActive     bool
	s3mSlideDown         byte
	s3mSlideDownActive   bool
	s3mVolumeSlideActive bool
	s3mTremor            byte
	s3mTremorActive      bool
	s3mTremorCounter     int
	s3mTremorMuted       bool
	s3mRetrigSpeed       int
	s3mRetrigSlide       int
	s3mRetrigActive      bool
	retrigCounter        int
	vibratoSpeed         int
	vibratoDepth         int
	vibratoPos           int
	vibratoWave          byte
	vibratoActive        bool
	fineVibrato          bool
	vibratoTick0Bug      bool
	tremoloSpeed         int
	tremoloDepth         int
	tremoloPos           int
	tremoloWave          byte
	tremoloActive        bool
	panbrelloSpeed       int
	panbrelloDepth       int
	panbrelloPos         int
	panbrelloWave        byte
	panbrelloActive      bool
	portamentoSpeed      int
	tonePortamento       bool
	volumeSlide          byte
	volumeSlideActive    bool
	panningSlide         byte
	panningSlideActive   bool
	channelSlide         byte
	channelSlideActive   bool
	pending              pendingNote
	volEnv               envelopeState
	panEnv               envelopeState
	pitchEnv             envelopeState
	autoVibratoPhase     int
	autoVibratoProgress  int
	justTriggered        bool
}

type pendingNote struct {
	active        bool
	tick          int
	note          int
	instrument    int
	usePortamento bool
	prepared      bool
	state         delayedNoteState
	keepVolume    bool
	keepPanning   bool
}

type delayedNoteState struct {
	note               int
	panNote            int
	xmFineTune         int16
	pitch              int
	targetPitch        int
	period             int
	targetPeriod       int
	baseVolume         int
	basePanning        int
	instrument         int
	lastInstrument     int
	sample             int
	sampleOffset       int
	sampleOffsetMemory int
	keyOn              bool
	keyFade            bool
}

type rowEvent struct {
	note          int
	instrument    int
	keyOff        bool
	keyFade       bool
	noteDelay     int
	noteCut       int
	usePortamento bool
	portaParam    int
}

type envelopeKind uint8

const (
	envelopeVolume envelopeKind = iota
	envelopePanning
	envelopePitch
)

type envelopeState struct {
	env   *modmodel.Envelope
	kind  envelopeKind
	a     int
	b     int
	pos   int
	fresh bool
}

func New(module *modmodel.Module, cfg Config) (*Engine, error) {
	if module == nil {
		return nil, fmt.Errorf("replay: nil module")
	}
	if cfg.SampleRate <= 0 {
		return nil, fmt.Errorf("replay: invalid sample rate %d", cfg.SampleRate)
	}

	engine := &Engine{
		module:     module,
		sampleRate: cfg.SampleRate,
	}
	if err := engine.Reset(); err != nil {
		return nil, err
	}
	return engine, nil
}

func (e *Engine) Reset() error {
	e.order = -1
	e.row = 0
	e.tick = 0
	e.randState = 1
	e.speed = clampInt(int(e.module.InitialSpeed), 1, 255)
	if e.speed == 0 {
		e.speed = 6
	}
	e.tempo = clampInt(int(e.module.InitialTempo), 1, 512)
	if e.tempo == 0 {
		e.tempo = 125
	}
	e.timingTempo = 125
	e.globalVolume = clampInt(int(e.module.InitialGlobalVolume), 0, 128)
	if e.globalVolume == 0 {
		e.globalVolume = 128
	}
	e.rowDelayRemaining = 0
	e.currentTickFrames = 0
	e.ended = false
	e.flow = flowState{jumpOrder: -1, breakRow: -1, loopRow: -1}

	e.channels = resizeZeroed(e.channels, e.module.Channels)
	e.channelVoices = resizeZeroed(e.channelVoices, e.module.Channels)
	for i := range e.channelVoices {
		e.channelVoices[i] = -1
	}
	e.detachedVoices = e.detachedVoices[:0]
	e.voiceCount = maxInt(e.module.Channels, e.module.Voices)
	if e.voiceCount < 0 {
		e.voiceCount = 0
	}
	for i := range e.channels {
		setting := e.module.ChannelSettings[i]
		e.channels[i] = channelState{
			volume:            0,
			baseVolume:        64,
			channelVolume:     clampInt(int(setting.Volume), 0, 64),
			panning:           clampPanning(int(setting.Panning)),
			basePanning:       clampPanning(int(setting.Panning)),
			fadeVolume:        maxFadeVolume,
			instrument:        -1,
			lastInstrument:    -1,
			sample:            -1,
			note:              -1,
			panNote:           -1,
			xmFineTune:        0,
			xmFineTunePending: false,
			targetPitch:       -1,
			targetPeriod:      -1,
			noteCutTick:       -1,
			loopStartRow:      0,
			loopCount:         -1,
			volumeEnvelope:    defaultVolumeEnvelope,
			panningEnvelope:   defaultPanningEnv,
			pitchEnvelope:     defaultPitchEnv,
			volEnv:            envelopeState{kind: envelopeVolume},
			panEnv:            envelopeState{kind: envelopePanning},
			pitchEnv:          envelopeState{kind: envelopePitch},
		}
	}

	if !e.seekOrderRow(0, 0) {
		e.ended = true
		e.captureSnapshot()
		return nil
	}

	e.processTick(true)
	return nil
}

func (e *Engine) Snapshot() Snapshot {
	return cloneSnapshot(e.snapshot)
}

// SnapshotView returns the current snapshot without copying its channel data.
// The returned slices remain valid only until the next Reset or AdvanceTicks.
func (e *Engine) SnapshotView() Snapshot {
	return e.snapshot
}

func (e *Engine) AdvanceTicks(count int) error {
	if count < 0 {
		return fmt.Errorf("replay: invalid tick count %d", count)
	}
	for i := 0; i < count; i++ {
		e.advanceOneTick()
	}
	return nil
}

func (e *Engine) CurrentTickFrames() int {
	return e.currentTickFrames
}

func (e *Engine) advanceOneTick() {
	if e.ended {
		return
	}

	processRow := false
	if e.tick+1 < e.speed {
		e.tick++
	} else {
		e.tick = 0
		if e.rowDelayRemaining > 0 {
			e.rowDelayRemaining--
		} else {
			processRow = true
			e.advancePosition()
			if e.ended {
				e.captureSnapshot()
				return
			}
		}
	}

	e.processTick(processRow)
}

func (e *Engine) advancePosition() {
	defer func() {
		e.flow = flowState{jumpOrder: -1, breakRow: -1, loopRow: -1}
	}()

	if e.flow.loopRow >= 0 {
		e.row = e.flow.loopRow
		return
	}

	if e.flow.jumpOrder >= 0 || e.flow.breakRow >= 0 {
		order := e.order
		row := 0
		if e.flow.jumpOrder >= 0 {
			order = e.flow.jumpOrder
		} else {
			order++
		}
		if e.flow.breakRow >= 0 {
			row = e.flow.breakRow
		}
		if !e.seekOrderRow(order, row) {
			e.ended = true
		}
		return
	}

	pattern := e.currentPattern()
	if pattern != nil && e.row+1 < int(pattern.Rows) {
		e.row++
		return
	}

	if !e.seekOrderRow(e.order+1, 0) {
		e.ended = true
	}
}

func (e *Engine) seekOrderRow(order, row int) bool {
	order = e.nextPlayableOrder(order)
	if order < 0 {
		return false
	}

	patternIndex := int(e.module.Orders[order])
	if patternIndex < 0 || patternIndex >= len(e.module.Patterns) {
		return false
	}

	pattern := e.module.Patterns[patternIndex]
	if row < 0 {
		row = 0
	}
	if row >= int(pattern.Rows) {
		row = 0
	}

	e.order = order
	e.row = row
	return true
}

func (e *Engine) nextPlayableOrder(start int) int {
	for i := start; i < len(e.module.Orders); i++ {
		if e.module.Orders[i] != modmodel.LastPattern {
			return i
		}
	}
	return -1
}

func (e *Engine) currentPattern() *modmodel.Pattern {
	if e.order < 0 || e.order >= len(e.module.Orders) {
		return nil
	}
	patternIndex := int(e.module.Orders[e.order])
	if patternIndex < 0 || patternIndex >= len(e.module.Patterns) {
		return nil
	}
	return &e.module.Patterns[patternIndex]
}

func (e *Engine) processTick(processRow bool) {
	if e.ended {
		e.captureSnapshot()
		return
	}

	if processRow {
		pattern := e.currentPattern()
		if pattern == nil {
			e.ended = true
			e.captureSnapshot()
			return
		}
		for i := range e.channels {
			e.channels[i].resetRowEffects()
			e.channels[i].pending.active = false
			e.channels[i].noteCutTick = -1
		}
		for channel := range e.channels {
			if channel >= len(pattern.Tracks) {
				continue
			}
			rows := pattern.Tracks[channel].Rows
			if e.row >= len(rows) {
				continue
			}
			e.processRowEvent(channel, rows[e.row])
		}
	}

	for channel := range e.channels {
		e.tickChannel(channel)
	}
	e.tickDetachedVoices()

	e.updateTimingTempo()
	e.currentTickFrames = e.tickFrames()
	e.captureSnapshot()
}

func (e *Engine) tickDetachedVoices() {
	if len(e.detachedVoices) == 0 {
		return
	}

	next := e.detachedVoices[:0]
	for _, voice := range e.detachedVoices {
		e.tickDetachedVoice(&voice.state)
		if voice.state.active {
			next = append(next, voice)
		}
	}
	e.detachedVoices = next
}

func (e *Engine) processRowEvent(index int, row unitrk.Row) {
	channel := &e.channels[index]
	event := rowEvent{
		note:       -1,
		instrument: -1,
		noteDelay:  -1,
		noteCut:    -1,
		portaParam: -1,
	}
	rowSetsVolume := false
	rowSetsPanning := false

	for _, cmd := range row.Commands {
		switch cmd.Op {
		case unitrk.OpNote:
			event.note = int(cmd.Param)
		case unitrk.OpInstrument:
			event.instrument = int(cmd.Param)
		case unitrk.OpKeyOff:
			event.keyOff = true
		case unitrk.OpKeyFade:
			event.keyFade = true
		case unitrk.OpPTEffect3, unitrk.OpPTEffect5, unitrk.OpITEffectG:
			event.usePortamento = true
			event.portaParam = int(cmd.Param)
		case unitrk.OpVolumeEffects:
			effect, _ := unitrk.DecodePair(cmd.Param)
			switch unitrk.VolumeEffect(effect) {
			case unitrk.VolPortamento:
				event.usePortamento = true
			case unitrk.VolSetVolume:
				rowSetsVolume = true
			case unitrk.VolSetPanning:
				rowSetsPanning = true
			}
		case unitrk.OpPTEffectC:
			rowSetsVolume = true
		case unitrk.OpPTEffect8:
			rowSetsPanning = true
		case unitrk.OpPTEffectE:
			if cmd.Param>>4 == 0x08 {
				rowSetsPanning = true
			}
		case unitrk.OpITEffectS0:
			switch cmd.Param >> 4 {
			case 0x08, 0x09:
				rowSetsPanning = true
			}
		}

		if tick, ok := noteDelayTick(cmd); ok {
			event.noteDelay = tick
		}
		if tick, ok := noteCutTick(cmd); ok {
			event.noteCut = tick
		}
	}

	if event.keyOff {
		e.keyOff(channel)
	}
	if event.keyFade {
		e.keyFade(channel)
	}
	if event.usePortamento && event.portaParam == 0 && channel.portamentoSpeed == 0 {
		event.usePortamento = false
	}

	if event.noteDelay > 0 && (event.note >= 0 || event.instrument >= 0) {
		liveState := snapshotDelayedNoteState(channel)
		prepared := false
		preparedState := delayedNoteState{}
		if !event.usePortamento {
			preparedState, prepared = e.prepareDelayedNote(index, event.note, event.instrument)
		}
		channel.pending = pendingNote{
			active:        true,
			tick:          event.noteDelay,
			note:          event.note,
			instrument:    event.instrument,
			usePortamento: event.usePortamento,
			prepared:      prepared,
			state:         preparedState,
			keepVolume:    rowSetsVolume,
			keepPanning:   rowSetsPanning,
		}
		defer func() {
			if !channel.pending.active || !channel.pending.prepared {
				return
			}
			channel.pending.state.baseVolume = channel.baseVolume
			channel.pending.state.basePanning = channel.basePanning
			channel.pending.state.sampleOffset = channel.sampleOffset
			channel.pending.state.sampleOffsetMemory = channel.sampleOffsetMemory
			if channel.pending.state.xmFineTune != channel.xmFineTune {
				channel.pending.state.xmFineTune = channel.xmFineTune
				if channel.pending.state.note >= 0 && channel.pending.state.sample >= 0 {
					channel.pending.state.period = e.notePeriodWithFineTune(channel.pending.state.sample, channel.pending.state.note, channel.pending.state.xmFineTune)
					channel.pending.state.targetPeriod = channel.pending.state.period
				}
			}
			restoreDelayedNoteState(channel, liveState)
		}()
	} else if event.note >= 0 || event.instrument >= 0 {
		e.applyNote(index, event.note, event.instrument, event.usePortamento)
	}

	if event.noteCut >= 0 {
		channel.noteCutTick = event.noteCut
	}

	for _, cmd := range row.Commands {
		e.applyRowCommand(index, cmd)
	}
}

func (e *Engine) applyRowCommand(index int, cmd unitrk.Command) {
	channel := &e.channels[index]

	switch cmd.Op {
	case unitrk.OpNote, unitrk.OpInstrument, unitrk.OpKeyOff, unitrk.OpKeyFade:
		return
	case unitrk.OpPTEffect0:
		if cmd.Param != 0 || e.module.Flags&modmodel.FlagArpeggioMemory == 0 {
			channel.arpeggio = byte(cmd.Param)
		}
		channel.arpeggioActive = channel.arpeggio != 0
	case unitrk.OpVolumeEffects:
		effect, data := unitrk.DecodePair(cmd.Param)
		switch unitrk.VolumeEffect(effect) {
		case unitrk.VolSetVolume:
			channel.baseVolume = clampInt(int(data), 0, 64)
		case unitrk.VolSetPanning:
			channel.basePanning = clampPanning(int(data))
		case unitrk.VolSlide:
			if data != 0 {
				channel.volumeSlide = data
			}
			channel.volumeSlideActive = channel.volumeSlide != 0
			channel.s3mVolumeSlideActive = false
		case unitrk.VolPortamento:
			if data != 0 {
				channel.portamentoSpeed = int(data)
			}
			channel.tonePortamento = channel.portamentoSpeed > 0
		case unitrk.VolVibrato:
			if data != 0 {
				channel.vibratoDepth = int(data) * 2
			}
			channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
		}
	case unitrk.OpPTEffect1:
		if cmd.Param != 0 {
			channel.ptSlideUp = byte(cmd.Param)
		}
		channel.ptSlideUpActive = channel.ptSlideUp != 0
	case unitrk.OpPTEffect2:
		if cmd.Param != 0 {
			channel.ptSlideDown = byte(cmd.Param)
		}
		channel.ptSlideDownActive = channel.ptSlideDown != 0
	case unitrk.OpPTEffect4:
		e.updateOldStyleLFO(&channel.vibratoSpeed, &channel.vibratoDepth, cmd.Param)
		channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
		channel.fineVibrato = false
		channel.vibratoTick0Bug = true
	case unitrk.OpS3MEffectH:
		e.updateOldStyleLFO(&channel.vibratoSpeed, &channel.vibratoDepth, cmd.Param)
		channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
		channel.fineVibrato = false
		channel.vibratoTick0Bug = false
	case unitrk.OpXMEffect4, unitrk.OpITEffectH:
		e.updateLFO(&channel.vibratoSpeed, &channel.vibratoDepth, cmd.Param)
		channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
		channel.fineVibrato = false
		channel.vibratoTick0Bug = false
	case unitrk.OpITEffectHOld:
		e.updateOldStyleLFO(&channel.vibratoSpeed, &channel.vibratoDepth, cmd.Param)
		channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
		channel.fineVibrato = false
		channel.vibratoTick0Bug = false
	case unitrk.OpS3MEffectU:
		e.updateOldStyleLFO(&channel.vibratoSpeed, &channel.vibratoDepth, cmd.Param)
		channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
		channel.fineVibrato = true
		channel.vibratoTick0Bug = false
	case unitrk.OpITEffectU:
		e.updateLFO(&channel.vibratoSpeed, &channel.vibratoDepth, cmd.Param)
		channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
		channel.fineVibrato = true
		channel.vibratoTick0Bug = false
	case unitrk.OpITEffectUOld:
		e.updateOldStyleLFO(&channel.vibratoSpeed, &channel.vibratoDepth, cmd.Param)
		channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
		channel.fineVibrato = true
		channel.vibratoTick0Bug = false
	case unitrk.OpPTEffect7, unitrk.OpS3MEffectR:
		e.updateLFO(&channel.tremoloSpeed, &channel.tremoloDepth, cmd.Param)
		channel.tremoloActive = channel.tremoloSpeed > 0 || channel.tremoloDepth > 0
	case unitrk.OpITEffectY:
		e.updateLFO(&channel.panbrelloSpeed, &channel.panbrelloDepth, cmd.Param)
		channel.panbrelloActive = channel.panbrelloSpeed > 0 || channel.panbrelloDepth > 0
	case unitrk.OpPTEffect3, unitrk.OpITEffectG:
		if cmd.Param != 0 {
			channel.portamentoSpeed = int(cmd.Param)
		}
		channel.tonePortamento = channel.portamentoSpeed > 0
	case unitrk.OpPTEffect5:
		if cmd.Param != 0 {
			channel.volumeSlide = byte(cmd.Param)
		}
		channel.volumeSlideActive = channel.volumeSlide != 0
		channel.s3mVolumeSlideActive = false
		channel.tonePortamento = channel.portamentoSpeed > 0
	case unitrk.OpPTEffect6:
		if cmd.Param != 0 {
			channel.volumeSlide = byte(cmd.Param)
		}
		channel.volumeSlideActive = channel.volumeSlide != 0
		channel.s3mVolumeSlideActive = false
		channel.vibratoActive = channel.vibratoSpeed > 0 || channel.vibratoDepth > 0
	case unitrk.OpPTEffect8:
		channel.basePanning = clampPanning(int(cmd.Param))
	case unitrk.OpPTEffectA, unitrk.OpS3MEffectD, unitrk.OpXMEffectA:
		if cmd.Param != 0 {
			channel.volumeSlide = byte(cmd.Param)
		}
		channel.volumeSlideActive = channel.volumeSlide != 0
		channel.s3mVolumeSlideActive = cmd.Op == unitrk.OpS3MEffectD
	case unitrk.OpXMEffectP, unitrk.OpITEffectP:
		if cmd.Param != 0 {
			channel.panningSlide = byte(cmd.Param)
		}
		channel.panningSlideActive = channel.panningSlide != 0
	case unitrk.OpITEffectN:
		if cmd.Param != 0 {
			channel.channelSlide = byte(cmd.Param)
		}
		channel.channelSlideActive = channel.channelSlide != 0
	case unitrk.OpPTEffectC:
		channel.baseVolume = clampInt(int(cmd.Param), 0, 64)
	case unitrk.OpPTEffect9:
		if cmd.Param != 0 {
			channel.sampleOffsetMemory = int(cmd.Param) << 8
		}
		channel.sampleOffset = channel.sampleOffsetMemory
	case unitrk.OpITEffectM:
		channel.channelVolume = clampInt(int(cmd.Param), 0, 64)
	case unitrk.OpPTEffectB:
		e.flow.jumpOrder = int(cmd.Param)
	case unitrk.OpPTEffectD:
		e.flow.breakRow = int(cmd.Param)
	case unitrk.OpPTEffectF:
		e.applySpeedOrTempo(int(cmd.Param))
	case unitrk.OpS3MEffectA:
		e.setSpeed(int(cmd.Param))
	case unitrk.OpS3MEffectE:
		if cmd.Param != 0 {
			channel.s3mPeriodSlide = byte(cmd.Param)
		}
		channel.s3mSlideDown = channel.s3mPeriodSlide
		channel.s3mSlideDownActive = channel.s3mSlideDown != 0
	case unitrk.OpS3MEffectF:
		if cmd.Param != 0 {
			channel.s3mPeriodSlide = byte(cmd.Param)
		}
		channel.s3mSlideUp = channel.s3mPeriodSlide
		channel.s3mSlideUpActive = channel.s3mSlideUp != 0
	case unitrk.OpS3MEffectI:
		if cmd.Param != 0 {
			channel.s3mTremor = byte(cmd.Param)
		}
		channel.s3mTremorActive = channel.s3mTremor != 0
	case unitrk.OpS3MEffectQ:
		if cmd.Param != 0 {
			channel.s3mRetrigSlide = int(cmd.Param >> 4)
			channel.s3mRetrigSpeed = int(cmd.Param & 0x0f)
		}
		channel.s3mRetrigActive = channel.s3mRetrigSpeed > 0
	case unitrk.OpS3MEffectT:
		e.setTempo(int(cmd.Param))
	case unitrk.OpITEffectT:
		e.applyTempoSlide(byte(cmd.Param))
	case unitrk.OpXMEffectG:
		e.globalVolume = clampInt(int(cmd.Param)<<1, 0, 128)
	case unitrk.OpXMEffectH, unitrk.OpITEffectW:
		e.applyGlobalVolumeSlide(byte(cmd.Param), false)
	case unitrk.OpXMEffectL:
		channel.volEnv.setPosition(int(cmd.Param))
		channel.panEnv.setPosition(int(cmd.Param))
	case unitrk.OpXMEffectE1:
		if e.module.Metadata.Format == modmodel.FormatXM && channel.period > 0 {
			channel.period = maxInt(1, channel.period-int(cmd.Param)*4)
		} else {
			channel.pitch -= int(cmd.Param) * 4
		}
	case unitrk.OpXMEffectE2:
		if e.module.Metadata.Format == modmodel.FormatXM && channel.period > 0 {
			channel.period += int(cmd.Param) * 4
		} else {
			channel.pitch += int(cmd.Param) * 4
		}
	case unitrk.OpXMEffectEA:
		channel.baseVolume = clampInt(channel.baseVolume+int(cmd.Param), 0, 64)
	case unitrk.OpXMEffectEB:
		channel.baseVolume = clampInt(channel.baseVolume-int(cmd.Param), 0, 64)
	case unitrk.OpXMEffectX1:
		if e.module.Metadata.Format == modmodel.FormatXM && channel.period > 0 {
			channel.period = maxInt(1, channel.period-int(cmd.Param))
		} else {
			channel.pitch -= int(cmd.Param)
		}
	case unitrk.OpXMEffectX2:
		if e.module.Metadata.Format == modmodel.FormatXM && channel.period > 0 {
			channel.period += int(cmd.Param)
		} else {
			channel.pitch += int(cmd.Param)
		}
	case unitrk.OpPTEffectE:
		e.applySpecial(index, byte(cmd.Param>>4), byte(cmd.Param&0x0f), true)
	case unitrk.OpITEffectS0:
		e.applySpecial(index, byte(cmd.Param>>4), byte(cmd.Param&0x0f), false)
	}
}

func (e *Engine) applySpecial(index int, high, low byte, ptStyle bool) {
	channel := &e.channels[index]

	switch {
	case ptStyle && high == 0x4:
		channel.vibratoWave = low
	case ptStyle && high == 0x5:
		if e.module.Metadata.Format == modmodel.FormatXM && channel.period > 0 && channel.note >= 0 && channel.sample >= 0 {
			channel.xmFineTune = int16(low)
			if channel.justTriggered && e.tick == 0 {
				channel.xmFineTunePending = true
			} else {
				e.applyXMFineTune(channel)
			}
		}
	case ptStyle && high == 0x7:
		channel.tremoloWave = low
	case ptStyle && high == 0x8:
		channel.basePanning = specialPanning(low)
	case ptStyle && high == 0x0a:
		channel.baseVolume = clampInt(channel.baseVolume+int(low), 0, 64)
	case ptStyle && high == 0x0b:
		channel.baseVolume = clampInt(channel.baseVolume-int(low), 0, 64)
	case ptStyle && high == 0x6:
		e.applyPatternLoop(channel, int(low))
	case ptStyle && high == 0x0e:
		e.rowDelayRemaining = maxInt(e.rowDelayRemaining, int(low))
	case !ptStyle && high == 0x3:
		channel.vibratoWave = low
	case !ptStyle && high == 0x4:
		channel.tremoloWave = low
	case !ptStyle && high == 0x5:
		channel.panbrelloWave = low
	case !ptStyle && high == 0x8:
		channel.basePanning = specialPanning(low)
	case !ptStyle && high == 0x9:
		channel.basePanning = int(modmodel.PanSurround)
	case !ptStyle && high == 0x0b:
		e.applyPatternLoop(channel, int(low))
	case !ptStyle && high == 0x0e:
		e.rowDelayRemaining = maxInt(e.rowDelayRemaining, int(low))
	}
}

func (e *Engine) applyPatternLoop(channel *channelState, count int) {
	if count == 0 {
		channel.loopStartRow = e.row
		return
	}
	if channel.loopCount < 0 {
		channel.loopCount = count
	}
	if channel.loopCount > 0 {
		e.flow.loopRow = channel.loopStartRow
		channel.loopCount--
		return
	}
	channel.loopCount = -1
}

func (e *Engine) applyXMFineTune(channel *channelState) {
	if channel == nil || e.module.Metadata.Format != modmodel.FormatXM || channel.period <= 0 || channel.note < 0 || channel.sample < 0 {
		return
	}
	channel.period = e.notePeriodWithFineTune(channel.sample, channel.note, channel.xmFineTune)
	if channel.targetPitch == channel.pitch || channel.targetPeriod <= 0 {
		channel.targetPeriod = channel.period
	}
}

func (e *Engine) tickChannel(index int) {
	channel := &e.channels[index]

	if channel.pending.active && e.tick == channel.pending.tick {
		baseVolume := channel.baseVolume
		basePanning := channel.basePanning
		if channel.pending.prepared {
			e.triggerPreparedNote(index, channel)
		} else {
			e.applyNote(index, channel.pending.note, channel.pending.instrument, channel.pending.usePortamento)
			if channel.pending.keepVolume {
				channel.baseVolume = baseVolume
			}
			if channel.pending.keepPanning {
				channel.basePanning = basePanning
			}
		}
		channel.pending.active = false
	}

	if channel.noteCutTick >= 0 && e.tick == channel.noteCutTick {
		e.cutNote(channel)
	}

	if channel.xmFineTunePending && e.tick > 0 {
		e.applyXMFineTune(channel)
		channel.xmFineTunePending = false
	}

	if channel.s3mTremorActive {
		channel.s3mTremorMuted = false
		if e.tick > 0 {
			on := int(channel.s3mTremor>>4) + 1
			off := int(channel.s3mTremor&0x0f) + 1
			cycle := on + off
			if cycle > 0 {
				channel.s3mTremorCounter %= cycle
				channel.s3mTremorMuted = channel.s3mTremorCounter >= on
				channel.s3mTremorCounter++
			}
		}
	}

	if channel.active && channel.period > 0 && channel.s3mRetrigActive && channel.s3mRetrigSpeed > 0 {
		if channel.retrigCounter <= 0 {
			if !channel.justTriggered {
				channel.trigger++
				channel.sampleOffset = 0
			}
			channel.retrigCounter = channel.s3mRetrigSpeed

			if e.tick > 0 || e.module.Flags&modmodel.FlagUsesS3MSlides != 0 {
				channel.baseVolume = clampInt(applyS3MRetrigSlide(channel.baseVolume, channel.s3mRetrigSlide), 0, 64)
			}
		}
		channel.retrigCounter--
	}

	if channel.s3mSlideUpActive && channel.period > 0 {
		channel.period = maxInt(1, channel.period-applyS3MPeriodSlide(byte(e.tick), channel.s3mSlideUp))
	}
	if channel.s3mSlideDownActive && channel.period > 0 {
		channel.period += applyS3MPeriodSlide(byte(e.tick), channel.s3mSlideDown)
	}
	if channel.volumeSlideActive && channel.s3mVolumeSlideActive {
		channel.baseVolume = clampInt(channel.baseVolume+applyS3MVolumeSlide(byte(e.tick), e.module.Flags, channel.volumeSlide), 0, 64)
	}

	if e.tick > 0 {
		if channel.ptSlideUpActive && channel.period > 0 {
			channel.period = maxInt(1, channel.period-(int(channel.ptSlideUp)<<2))
		}
		if channel.ptSlideDownActive && channel.period > 0 {
			channel.period += int(channel.ptSlideDown) << 2
		}
		if channel.volumeSlideActive {
			if !channel.s3mVolumeSlideActive {
				channel.baseVolume = clampInt(channel.baseVolume+slideAmount(channel.volumeSlide), 0, 64)
			}
		}
		if channel.channelSlideActive {
			channel.channelVolume = clampInt(channel.channelVolume+slideAmount(channel.channelSlide), 0, 64)
		}
		if channel.panningSlideActive {
			channel.basePanning = clampPanning(channel.basePanning + slideAmount(channel.panningSlide)*4)
		}
		if channel.tonePortamento && channel.portamentoSpeed > 0 {
			if (e.usesOldPeriods() || e.module.Metadata.Format == modmodel.FormatXM) && channel.period > 0 && channel.targetPeriod > 0 {
				channel.period = slideToward(channel.period, channel.targetPeriod, channel.portamentoSpeed<<2)
			} else if channel.targetPitch >= 0 {
				channel.pitch = slideToward(channel.pitch, channel.targetPitch, channel.portamentoSpeed)
			}
		}
	}

	channel.pitchDelta = 0
	channel.periodDelta = 0
	channel.volumeDelta = 0
	channel.panningDelta = 0

	if channel.active {
		if channel.arpeggioActive && channel.arpeggio != 0 && !e.usesOldPeriods() {
			channel.pitchDelta += arpeggioOffset(e.tick, channel.arpeggio) * 128
		}
		if channel.vibratoActive {
			if e.usesOldPeriods() {
				if e.tick != 0 || !channel.vibratoTick0Bug {
					channel.periodDelta += oldStyleVibratoDelta(channel.vibratoWave, channel.vibratoPos, channel.vibratoDepth, channel.fineVibrato)
				}
			} else if e.module.Metadata.Format == modmodel.FormatXM {
				if e.tick != 0 || !channel.vibratoTick0Bug {
					channel.periodDelta += xmVibratoDelta(channel.vibratoWave, channel.vibratoPos, channel.vibratoDepth)
				}
			} else if e.tick != 0 || !channel.vibratoTick0Bug {
				channel.pitchDelta += lfo(channel.vibratoWave, channel.vibratoPos) * channel.vibratoDepth / vibratoScale(channel.fineVibrato)
			}
			if e.tick > 0 {
				channel.vibratoPos = (channel.vibratoPos + channel.vibratoSpeed) & 0xff
			}
		}
		if channel.tremoloActive {
			channel.tremoloPos = (channel.tremoloPos + channel.tremoloSpeed) & 0x3f
			channel.volumeDelta = lfo(channel.tremoloWave, channel.tremoloPos) * channel.tremoloDepth / 64
		}
		if channel.panbrelloActive {
			channel.panbrelloPos = (channel.panbrelloPos + channel.panbrelloSpeed) & 0x3f
			channel.panningDelta = lfo(channel.panbrelloWave, channel.panbrelloPos) * channel.panbrelloDepth / 32
		}
		if e.module.Metadata.Format == modmodel.FormatXM {
			channel.periodDelta += e.xmAutoVibratoDelta(channel)
		} else {
			channel.pitchDelta += e.autoVibrato(channel)
		}

		channel.volumeEnvelope = channel.volEnv.tickValue(channel.keyOn)
		channel.panningEnvelope = channel.panEnv.tickValue(channel.keyOn)
		channel.pitchEnvelope = channel.pitchEnv.tickValue(channel.keyOn)
		if e.module.Metadata.Format == modmodel.FormatXM {
			channel.periodDelta += e.xmPitchEnvelopeDelta(channel)
		} else {
			channel.pitchDelta += (channel.pitchEnvelope - defaultPitchEnv) * 64
		}

	}

	e.updateComputedState(channel)
	if channel.active && channel.keyFade {
		channel.fadeVolume -= e.fadeStep(channel)
		if channel.fadeVolume < 0 {
			channel.fadeVolume = 0
		}
	}
	channel.justTriggered = false
}

func (e *Engine) tickDetachedVoice(channel *channelState) {
	if channel == nil {
		return
	}

	channel.pitchDelta = 0
	channel.periodDelta = 0
	channel.volumeDelta = 0
	channel.panningDelta = 0

	if channel.active {
		if e.module.Metadata.Format == modmodel.FormatXM {
			channel.periodDelta += e.xmAutoVibratoDelta(channel)
		} else {
			channel.pitchDelta += e.autoVibrato(channel)
		}

		channel.volumeEnvelope = channel.volEnv.tickValue(channel.keyOn)
		channel.panningEnvelope = channel.panEnv.tickValue(channel.keyOn)
		channel.pitchEnvelope = channel.pitchEnv.tickValue(channel.keyOn)
		if e.module.Metadata.Format == modmodel.FormatXM {
			channel.periodDelta += e.xmPitchEnvelopeDelta(channel)
		} else {
			channel.pitchDelta += (channel.pitchEnvelope - defaultPitchEnv) * 64
		}
	}

	e.updateComputedState(channel)
	if channel.active && channel.keyFade {
		channel.fadeVolume -= e.fadeStep(channel)
		if channel.fadeVolume < 0 {
			channel.fadeVolume = 0
		}
	}
	channel.justTriggered = false
}

func (e *Engine) updateComputedState(channel *channelState) {
	if !channel.active {
		channel.volume = 0
		channel.panning = clampPanning(channel.basePanning)
		return
	}

	channel.volume = e.mixVolume(channel)
	if channel.s3mTremorMuted {
		channel.volume = 0
	}

	panning := channel.basePanning
	panning = applyEnvelopePanning(panning, channel.panningEnvelope)
	panning += channel.panningDelta
	channel.panning = clampPanning(panning)

	if channel.volume == 0 && channel.fadeVolume == 0 {
		channel.active = false
	}
}

func (e *Engine) applyNote(index, note, instrument int, usePortamento bool) {
	channel := &e.channels[index]
	resolvedNote, resolvedInstrument, resolvedSample := e.resolveTarget(channel, note, instrument)

	if resolvedInstrument >= 0 {
		channel.lastInstrument = resolvedInstrument
	}
	if resolvedSample >= 0 {
		channel.sample = resolvedSample
	}
	if resolvedInstrument >= 0 {
		channel.instrument = resolvedInstrument
	}

	if note < 0 && instrument >= 0 {
		channel.baseVolume = e.sampleVolume(channel.sample)
		channel.basePanning = e.defaultPanningFor(channel)
		channel.baseVolume, channel.basePanning = e.applyInstrumentRandomization(channel.instrument, channel.baseVolume, channel.basePanning)
		channel.retrigCounter = 0
		channel.s3mTremorCounter = 0
		return
	}

	if resolvedNote < 0 || resolvedSample < 0 {
		return
	}

	targetPitch := resolvedNote * 128
	targetFineTune := e.sampleFineTune(resolvedSample)
	targetPeriod := e.notePeriodWithFineTune(resolvedSample, resolvedNote, targetFineTune)
	if usePortamento && channel.active {
		channel.targetPitch = targetPitch
		channel.targetPeriod = targetPeriod
		if note >= 0 {
			channel.panNote = note
			channel.keyOn = true
			channel.keyFade = false
			channel.volEnv.reset(e.volumeEnvelope(channel))
			channel.panEnv.reset(e.panningEnvelope(channel))
			channel.pitchEnv.reset(e.pitchEnvelope(channel))
		}
		if instrument >= 0 {
			channel.baseVolume = e.sampleVolume(resolvedSample)
		}
		channel.basePanning = e.defaultPanningFor(channel)
		if instrument >= 0 {
			channel.baseVolume, channel.basePanning = e.applyInstrumentRandomization(channel.instrument, channel.baseVolume, channel.basePanning)
		}
		channel.tonePortamento = channel.portamentoSpeed > 0
		return
	}

	e.prepareVoiceSlot(index, resolvedInstrument)
	channel.active = true
	channel.trigger++
	channel.keyOn = true
	channel.keyFade = false
	channel.note = resolvedNote
	if note >= 0 {
		channel.panNote = note
	}
	channel.xmFineTune = targetFineTune
	channel.xmFineTunePending = false
	channel.pitch = targetPitch
	channel.targetPitch = targetPitch
	channel.period = targetPeriod
	channel.targetPeriod = targetPeriod
	channel.fadeVolume = maxFadeVolume
	channel.baseVolume = e.sampleVolume(resolvedSample)
	channel.basePanning = e.defaultPanningFor(channel)
	if instrument >= 0 {
		channel.baseVolume, channel.basePanning = e.applyInstrumentRandomization(channel.instrument, channel.baseVolume, channel.basePanning)
	}
	channel.vibratoPos = 0
	channel.tremoloPos = 0
	channel.panbrelloPos = 0
	channel.autoVibratoPhase = 0
	channel.autoVibratoProgress = 0
	channel.retrigCounter = 0
	channel.s3mTremorCounter = 0
	channel.s3mTremorMuted = false
	channel.justTriggered = true
	channel.volEnv.reset(e.volumeEnvelope(channel))
	channel.panEnv.reset(e.panningEnvelope(channel))
	channel.pitchEnv.reset(e.pitchEnvelope(channel))
}

func (e *Engine) prepareDelayedNote(index, note, instrument int) (delayedNoteState, bool) {
	channel := &e.channels[index]
	resolvedNote, resolvedInstrument, resolvedSample := e.resolveTarget(channel, note, instrument)
	state := snapshotDelayedNoteState(channel)

	if resolvedInstrument >= 0 {
		state.lastInstrument = resolvedInstrument
		state.instrument = resolvedInstrument
	}
	if resolvedSample >= 0 {
		state.sample = resolvedSample
	}

	if note < 0 && instrument >= 0 {
		tmp := *channel
		restoreDelayedNoteState(&tmp, state)
		state.baseVolume = e.sampleVolume(tmp.sample)
		state.basePanning = e.defaultPanningFor(&tmp)
		state.baseVolume, state.basePanning = e.applyInstrumentRandomization(state.instrument, state.baseVolume, state.basePanning)
		return state, false
	}

	if resolvedNote < 0 || resolvedSample < 0 {
		return state, false
	}

	targetPitch := resolvedNote * 128
	targetFineTune := e.sampleFineTune(resolvedSample)
	targetPeriod := e.notePeriodWithFineTune(resolvedSample, resolvedNote, targetFineTune)
	tmp := *channel
	restoreDelayedNoteState(&tmp, state)
	tmp.lastInstrument = state.lastInstrument
	tmp.instrument = state.instrument
	tmp.sample = state.sample
	state.note = resolvedNote
	if note >= 0 {
		state.panNote = note
	}
	state.xmFineTune = targetFineTune
	state.pitch = targetPitch
	state.targetPitch = targetPitch
	state.period = targetPeriod
	state.targetPeriod = targetPeriod
	state.baseVolume = e.sampleVolume(resolvedSample)
	state.basePanning = e.defaultPanningFor(&tmp)
	if instrument >= 0 {
		state.baseVolume, state.basePanning = e.applyInstrumentRandomization(state.instrument, state.baseVolume, state.basePanning)
	}
	state.keyOn = true
	state.keyFade = false
	return state, true
}

func (e *Engine) triggerPreparedNote(index int, channel *channelState) {
	if channel == nil || channel.pending.state.sample < 0 || channel.pending.state.note < 0 {
		return
	}
	e.prepareVoiceSlot(index, channel.pending.state.instrument)
	restoreDelayedNoteState(channel, channel.pending.state)
	channel.active = true
	channel.xmFineTunePending = false
	channel.trigger++
	channel.keyOn = true
	channel.keyFade = false
	channel.fadeVolume = maxFadeVolume
	channel.vibratoPos = 0
	channel.tremoloPos = 0
	channel.panbrelloPos = 0
	channel.autoVibratoPhase = 0
	channel.autoVibratoProgress = 0
	channel.retrigCounter = 0
	channel.s3mTremorCounter = 0
	channel.s3mTremorMuted = false
	channel.justTriggered = true
	channel.volEnv.reset(e.volumeEnvelope(channel))
	channel.panEnv.reset(e.panningEnvelope(channel))
	channel.pitchEnv.reset(e.pitchEnvelope(channel))
}

func snapshotDelayedNoteState(channel *channelState) delayedNoteState {
	return delayedNoteState{
		note:               channel.note,
		panNote:            channel.panNote,
		xmFineTune:         channel.xmFineTune,
		pitch:              channel.pitch,
		targetPitch:        channel.targetPitch,
		period:             channel.period,
		targetPeriod:       channel.targetPeriod,
		baseVolume:         channel.baseVolume,
		basePanning:        channel.basePanning,
		instrument:         channel.instrument,
		lastInstrument:     channel.lastInstrument,
		sample:             channel.sample,
		sampleOffset:       channel.sampleOffset,
		sampleOffsetMemory: channel.sampleOffsetMemory,
		keyOn:              channel.keyOn,
		keyFade:            channel.keyFade,
	}
}

func restoreDelayedNoteState(channel *channelState, state delayedNoteState) {
	channel.note = state.note
	channel.panNote = state.panNote
	channel.xmFineTune = state.xmFineTune
	channel.xmFineTunePending = false
	channel.pitch = state.pitch
	channel.targetPitch = state.targetPitch
	channel.period = state.period
	channel.targetPeriod = state.targetPeriod
	channel.baseVolume = state.baseVolume
	channel.basePanning = state.basePanning
	channel.instrument = state.instrument
	channel.lastInstrument = state.lastInstrument
	channel.sample = state.sample
	channel.sampleOffset = state.sampleOffset
	channel.sampleOffsetMemory = state.sampleOffsetMemory
	channel.keyOn = state.keyOn
	channel.keyFade = state.keyFade
}

func (e *Engine) noteActionForInstrument(instrumentIndex int) modmodel.NNA {
	if instrumentIndex < 0 || instrumentIndex >= len(e.module.Instruments) {
		return modmodel.NNACut
	}
	return e.module.Instruments[instrumentIndex].NewNoteAction
}

func (e *Engine) voiceSlotInUse(slot, ignoreChannel int) bool {
	if slot < 0 {
		return false
	}
	for index, voiceSlot := range e.channelVoices {
		if index == ignoreChannel || voiceSlot != slot {
			continue
		}
		if index >= 0 && index < len(e.channels) && e.channels[index].active {
			return true
		}
	}
	for _, voice := range e.detachedVoices {
		if voice.slot == slot && voice.state.active {
			return true
		}
	}
	return false
}

func (e *Engine) allocateVoiceSlot(channelIndex int) int {
	if e.voiceCount <= 0 {
		return -1
	}
	if e.module.Flags&modmodel.FlagUsesNNA == 0 {
		if channelIndex >= 0 && channelIndex < e.voiceCount {
			return channelIndex
		}
		return clampInt(channelIndex, 0, e.voiceCount-1)
	}
	for slot := 0; slot < e.voiceCount; slot++ {
		if !e.voiceSlotInUse(slot, channelIndex) {
			return slot
		}
	}
	if channelIndex >= 0 && channelIndex < len(e.channelVoices) && e.channelVoices[channelIndex] >= 0 {
		return e.channelVoices[channelIndex]
	}
	return -1
}

func (e *Engine) detachVoice(index int, action modmodel.NNA) {
	if index < 0 || index >= len(e.channels) || index >= len(e.channelVoices) {
		return
	}
	slot := e.channelVoices[index]
	channel := &e.channels[index]
	if slot < 0 || !channel.active {
		return
	}

	detached := detachedVoice{
		slot:  slot,
		state: *channel,
	}
	detached.state.pending = pendingNote{}
	detached.state.noteCutTick = -1
	detached.state.justTriggered = false

	switch action {
	case modmodel.NNAContinue:
	case modmodel.NNAOff:
		e.keyOff(&detached.state)
	case modmodel.NNAFade:
		e.keyFade(&detached.state)
	default:
		detached.state.active = false
	}

	if detached.state.active {
		e.detachedVoices = append(e.detachedVoices, detached)
	}
	e.channelVoices[index] = -1
}

func (e *Engine) prepareVoiceSlot(index, instrumentIndex int) {
	if index < 0 || index >= len(e.channels) || index >= len(e.channelVoices) {
		return
	}
	if e.module.Flags&modmodel.FlagUsesNNA != 0 && e.channels[index].active {
		action := e.noteActionForInstrument(instrumentIndex)
		if action != modmodel.NNACut {
			e.detachVoice(index, action)
		}
	}
	if e.channelVoices[index] < 0 || e.module.Flags&modmodel.FlagUsesNNA != 0 {
		e.channelVoices[index] = e.allocateVoiceSlot(index)
	}
}

func (e *Engine) resolveTarget(channel *channelState, note, instrument int) (int, int, int) {
	instrumentIndex := channel.lastInstrument
	if instrument >= 0 {
		instrumentIndex = instrument
	}

	if e.module.Flags&modmodel.FlagUsesInstruments == 0 {
		sampleIndex := channel.sample
		if instrumentIndex >= 0 {
			sampleIndex = instrumentIndex
		}
		resolvedNote := channel.note
		if note >= 0 {
			resolvedNote = note
		}
		if sampleIndex < 0 || sampleIndex >= len(e.module.Samples) {
			return resolvedNote, -1, -1
		}
		return resolvedNote, -1, sampleIndex
	}

	if instrumentIndex < 0 || instrumentIndex >= len(e.module.Instruments) {
		return note, -1, -1
	}

	resolvedNote := note
	sampleIndex := channel.sample
	if note >= 0 && note < modmodel.InstrumentNotes {
		mapped := e.module.Instruments[instrumentIndex].NoteMap[note]
		if mapped.Note != 255 {
			resolvedNote = int(mapped.Note)
			sampleIndex = int(mapped.Sample)
		}
	}
	if sampleIndex < 0 || sampleIndex >= len(e.module.Samples) {
		return resolvedNote, instrumentIndex, -1
	}
	return resolvedNote, instrumentIndex, sampleIndex
}

func (e *Engine) keyOff(channel *channelState) {
	if !channel.active {
		return
	}
	channel.keyOn = false
	if !channel.volEnv.enabled() {
		channel.keyFade = true
	}
}

func (e *Engine) keyFade(channel *channelState) {
	if !channel.active {
		return
	}
	channel.keyOn = false
	channel.keyFade = true
}

func (e *Engine) cutNote(channel *channelState) {
	channel.active = false
	channel.keyOn = false
	channel.keyFade = false
	channel.fadeVolume = 0
	channel.volume = 0
}

func (e *Engine) applySpeedOrTempo(value int) {
	if value <= 0 {
		return
	}
	if e.module.Flags&modmodel.FlagFarTempo != 0 && value < len(farTempoTable) {
		e.tempo = farTempoTable[value]
		return
	}
	threshold := int(e.module.BPMThreshold)
	if threshold <= 0 {
		threshold = 33
	}
	if value < threshold {
		e.setSpeed(value)
		return
	}
	e.setTempo(value)
}

func (e *Engine) setSpeed(value int) {
	if value <= 0 {
		return
	}
	e.speed = clampInt(value, 1, 255)
}

func (e *Engine) setTempo(value int) {
	if value <= 0 {
		return
	}
	e.tempo = clampInt(value, 1, 512)
}

func (e *Engine) applyTempoSlide(data byte) {
	switch {
	case data == 0:
		return
	case data < 0x10:
		e.tempo = clampInt(e.tempo-int(data), 1, 512)
	case data > 0x10 && data < 0x20:
		e.tempo = clampInt(e.tempo+int(data&0x0f), 1, 512)
	default:
		e.setTempo(int(data))
	}
}

func (e *Engine) applyGlobalVolumeSlide(data byte, tickDriven bool) {
	if data == 0 {
		return
	}
	if tickDriven && e.tick == 0 {
		return
	}
	e.globalVolume = clampInt(e.globalVolume+slideAmount(data), 0, 128)
}

func (e *Engine) updateLFO(speed, depth *int, value uint16) {
	if hi := int(value & 0xf0); hi != 0 {
		*speed = hi >> 2
	}
	if lo := int(value & 0x0f); lo != 0 {
		*depth = lo
	}
}

func (e *Engine) updateOldStyleLFO(speed, depth *int, value uint16) {
	if hi := int(value & 0xf0); hi != 0 {
		*speed = hi >> 2
	}
	if lo := int(value & 0x0f); lo != 0 {
		*depth = lo
	}
}

func (e *Engine) sampleVolume(sample int) int {
	if sample < 0 || sample >= len(e.module.Samples) {
		return 64
	}
	return clampInt(int(e.module.Samples[sample].Volume), 0, 64)
}

func (e *Engine) sampleGlobalVolume(sample int) int {
	if sample < 0 || sample >= len(e.module.Samples) {
		return 64
	}
	volume := int(e.module.Samples[sample].GlobalVolume)
	if volume <= 0 {
		return 64
	}
	return clampInt(volume, 0, 64)
}

func (e *Engine) defaultPanningFor(channel *channelState) int {
	panning := channel.basePanning
	sampleOwnPan := false
	if channel.sample >= 0 && channel.sample < len(e.module.Samples) {
		sample := e.module.Samples[channel.sample]
		if sample.Flags&modmodel.SampleOwnPanning != 0 {
			panning = clampPanning(int(sample.Panning))
			sampleOwnPan = true
		}
	}
	if !sampleOwnPan && channel.instrument >= 0 && channel.instrument < len(e.module.Instruments) {
		instrument := e.module.Instruments[channel.instrument]
		if instrument.Flags&modmodel.InstrumentOwnPanning != 0 && instrument.Panning >= 0 {
			panning = clampPanning(int(instrument.Panning))
		}
	}
	if channel.instrument >= 0 && channel.instrument < len(e.module.Instruments) {
		instrument := e.module.Instruments[channel.instrument]
		if e.module.Flags&modmodel.FlagUsesPanning != 0 &&
			instrument.Flags&modmodel.InstrumentPitchPan != 0 &&
			panning != int(modmodel.PanSurround) &&
			channel.panNote >= 0 {
			panning += ((channel.panNote - int(instrument.PitchPanCenter)) * int(instrument.PitchPanSeparation)) / 8
			panning = clampPanning(panning)
		}
	}
	return panning
}

func (e *Engine) instrumentGlobalVolume(channel *channelState) int {
	if channel.instrument < 0 || channel.instrument >= len(e.module.Instruments) {
		return 64
	}
	volume := int(e.module.Instruments[channel.instrument].GlobalVolume)
	if volume <= 0 {
		return 64
	}
	return clampInt(volume, 0, 64)
}

func (e *Engine) mixVolume(channel *channelState) int {
	baseVolume := clampInt(channel.baseVolume+channel.volumeDelta, 0, 64)
	outVolume := baseVolume * e.sampleGlobalVolume(channel.sample)
	if channel.instrument >= 0 && channel.instrument < len(e.module.Instruments) {
		outVolume = (outVolume * e.instrumentGlobalVolume(channel)) >> 10
	} else {
		outVolume >>= 4
	}
	outVolume = clampInt(outVolume, 0, 256)

	fadeVolume := int64(clampInt(channel.fadeVolume, 0, maxFadeVolume)) * 32
	mixVolume := fadeVolume * int64(clampInt(channel.channelVolume, 0, 64)) * int64(outVolume)
	mixVolume /= 256 * 64
	mixVolume *= int64(clampInt(channel.volumeEnvelope, 0, 256))
	mixVolume *= int64(clampInt(e.globalVolume, 0, 128))
	mixVolume /= 128 * 256 * 128

	return clampInt(int(mixVolume), 0, 256)
}

func (e *Engine) volumeEnvelope(channel *channelState) *modmodel.Envelope {
	if channel.instrument < 0 || channel.instrument >= len(e.module.Instruments) {
		return nil
	}
	return &e.module.Instruments[channel.instrument].VolumeEnvelope
}

func (e *Engine) panningEnvelope(channel *channelState) *modmodel.Envelope {
	if channel.instrument < 0 || channel.instrument >= len(e.module.Instruments) {
		return nil
	}
	return &e.module.Instruments[channel.instrument].PanningEnvelope
}

func (e *Engine) pitchEnvelope(channel *channelState) *modmodel.Envelope {
	if channel.instrument < 0 || channel.instrument >= len(e.module.Instruments) {
		return nil
	}
	return &e.module.Instruments[channel.instrument].PitchEnvelope
}

func (e *Engine) fadeStep(channel *channelState) int {
	if channel.instrument < 0 || channel.instrument >= len(e.module.Instruments) {
		return 16
	}
	fade := int(e.module.Instruments[channel.instrument].FadeOut)
	if fade <= 0 {
		return 16
	}
	return maxInt(1, fade/32)
}

func (e *Engine) autoVibrato(channel *channelState) int {
	if channel.sample < 0 || channel.sample >= len(e.module.Samples) {
		return 0
	}
	vibrato := e.module.Samples[channel.sample].Vibrato
	if vibrato.Depth == 0 || vibrato.Rate == 0 {
		return 0
	}

	if vibrato.Flags&modmodel.AutoVibratoIT != 0 {
		amp := autoVibratoIT(channel.autoVibratoPhase, vibrato.Waveform)
		maxDepth := int(vibrato.Depth) << 8
		if (channel.autoVibratoProgress >> 8) < int(vibrato.Depth) {
			channel.autoVibratoProgress += int(vibrato.Sweep)
		}
		if channel.autoVibratoProgress > maxDepth {
			channel.autoVibratoProgress = maxDepth
		}
		periodDelta := (amp * channel.autoVibratoProgress) >> 16
		if e.module.Flags&modmodel.FlagLinearPeriods != 0 {
			periodDelta >>= 1
		}
		channel.autoVibratoPhase = (channel.autoVibratoPhase + int(vibrato.Rate)) & 0xff
		return periodDelta * 2
	}

	amp := autoVibratoXM(channel.autoVibratoPhase, vibrato.Waveform) >> 2
	depth := int(vibrato.Depth)
	if channel.keyOn {
		if vibrato.Sweep > 0 && channel.autoVibratoProgress < int(vibrato.Sweep) {
			depth = channel.autoVibratoProgress * depth / int(vibrato.Sweep)
			channel.autoVibratoProgress++
		}
	} else if channel.autoVibratoProgress < int(vibrato.Sweep) {
		depth = 0
	}
	periodDelta := (amp * depth) >> 8
	channel.autoVibratoPhase = (channel.autoVibratoPhase + int(vibrato.Rate)) & 0xff
	if e.module.Flags&modmodel.FlagLinearPeriods != 0 {
		return periodDelta * 2
	}
	return periodDelta
}

func (e *Engine) xmAutoVibratoDelta(channel *channelState) int {
	if channel.sample < 0 || channel.sample >= len(e.module.Samples) {
		return 0
	}
	vibrato := e.module.Samples[channel.sample].Vibrato
	if vibrato.Depth == 0 || vibrato.Rate == 0 || vibrato.Flags&modmodel.AutoVibratoIT != 0 {
		return 0
	}

	amp := autoVibratoXM(channel.autoVibratoPhase, vibrato.Waveform) >> 2
	depth := int(vibrato.Depth)
	if channel.keyOn {
		if vibrato.Sweep > 0 && channel.autoVibratoProgress < int(vibrato.Sweep) {
			depth = channel.autoVibratoProgress * depth / int(vibrato.Sweep)
			channel.autoVibratoProgress++
		}
	} else if channel.autoVibratoProgress < int(vibrato.Sweep) {
		depth = 0
	}

	channel.autoVibratoPhase = (channel.autoVibratoPhase + int(vibrato.Rate)) & 0xff
	return -((amp * depth) >> 8)
}

func (e *Engine) xmPitchEnvelopeDelta(channel *channelState) int {
	if channel == nil || channel.note < 0 || channel.sample < 0 || channel.sample >= len(e.module.Samples) {
		return 0
	}
	envpit := channel.pitchEnvelope - defaultPitchEnv
	if envpit == 0 {
		return 0
	}

	noteParam := channel.note*2 + envpit
	if noteParam <= 0 {
		noteParam = 0
	}
	basePeriod := e.channelNotePeriod(channel, channel.sample, channel.note)
	if basePeriod <= 0 {
		return 0
	}
	fineTune := channel.xmFineTune
	targetPeriod := pitch.XMPeriodFromNoteParam(noteParam, fineTune, e.module.Flags&modmodel.FlagLinearPeriods != 0)
	return targetPeriod - basePeriod
}

func (e *Engine) applyInstrumentRandomization(instrumentIndex, volume, panning int) (int, int) {
	if instrumentIndex < 0 || instrumentIndex >= len(e.module.Instruments) {
		return volume, panning
	}

	instrument := e.module.Instruments[instrumentIndex]
	if instrument.RandomVolumeVar > 0 {
		volume += (volume * int(instrument.RandomVolumeVar) * e.nextRandom(512)) / 25600
		volume = clampInt(volume, 0, 64)
	}
	if instrument.RandomPanningVar > 0 && panning != int(modmodel.PanSurround) {
		panning += (panning * int(instrument.RandomPanningVar) * e.nextRandom(512)) / 25600
		panning = clampPanning(panning)
	}
	return volume, panning
}

func (e *Engine) nextRandom(ceil int) int {
	if ceil <= 1 {
		return 0
	}
	e.randState = e.randState*1103515245 + 12345
	return int((e.randState >> 16) & uint32(ceil-1))
}

func (e *Engine) tickFrames() int {
	tempo := e.timingTempo
	if tempo <= 0 {
		return 0
	}
	frames := (e.sampleRate * 125) / (tempo * 50)
	if frames < 1 {
		return 1
	}
	return frames
}

func (e *Engine) updateTimingTempo() {
	for i := range e.channels {
		if e.channels[i].active {
			e.timingTempo = clampInt(e.tempo, 1, 512)
			return
		}
	}
	for i := range e.detachedVoices {
		if e.detachedVoices[i].state.active {
			e.timingTempo = clampInt(e.tempo, 1, 512)
			return
		}
	}
}

func (e *Engine) captureSnapshot() {
	patternIndex := -1
	if pattern := e.currentPattern(); pattern != nil && e.order >= 0 && e.order < len(e.module.Orders) {
		patternIndex = int(e.module.Orders[e.order])
	}

	channels := resizeZeroed(e.snapshot.Channels, len(e.channels))
	for i := range e.channels {
		channels[i] = e.snapshotChannel(e.channels[i])
	}

	voices := resizeZeroed(e.snapshot.Voices, e.voiceCount)
	for index, channel := range e.channels {
		if index >= len(e.channelVoices) {
			continue
		}
		slot := e.channelVoices[index]
		if slot < 0 || slot >= len(voices) {
			continue
		}
		voices[slot] = e.snapshotChannel(channel)
	}
	for _, voice := range e.detachedVoices {
		if voice.slot < 0 || voice.slot >= len(voices) {
			continue
		}
		voices[voice.slot] = e.snapshotChannel(voice.state)
	}

	e.snapshot = Snapshot{
		Order:        e.order,
		Pattern:      patternIndex,
		Row:          e.row,
		Tick:         e.tick,
		Speed:        e.speed,
		Tempo:        e.tempo,
		GlobalVolume: e.globalVolume,
		Ended:        e.ended,
		Channels:     channels,
		Voices:       voices,
	}
}

func (e *Engine) snapshotChannel(channel channelState) ChannelSnapshot {
	pitchDelta := channel.pitchDelta
	if channel.note >= 0 {
		pitchDelta += channel.pitch - channel.note*128
	}
	frequency := 0
	baseXMPeriod := 0
	if e.module.Metadata.Format == modmodel.FormatXM && channel.note >= 0 {
		baseXMPeriod = e.notePeriod(channel.sample, channel.note)
	}
	if e.module.Metadata.Format == modmodel.FormatXM &&
		channel.active &&
		channel.period > 0 &&
		channel.pitchDelta == 0 &&
		channel.note >= 0 &&
		channel.pitch == channel.note*128 &&
		!channel.arpeggioActive &&
		(channel.periodDelta != 0 || (baseXMPeriod > 0 && channel.period != baseXMPeriod)) {
		period := channel.period
		if channel.periodDelta != 0 {
			period += channel.periodDelta
			if period < 1 {
				period = 1
			}
		}
		frequency = pitch.XMFrequencyFromPeriod(period, e.module.Flags&modmodel.FlagLinearPeriods != 0)
	} else if e.usesOldPeriods() && channel.active && channel.period > 0 && channel.pitchDelta == 0 {
		period := channel.period
		if channel.arpeggioActive && channel.note >= 0 {
			period = e.notePeriod(channel.sample, channel.note+arpeggioOffset(e.tick, channel.arpeggio))
		} else if channel.periodDelta != 0 {
			period += channel.periodDelta
			if period < 1 {
				period = 1
			}
		}
		frequency = oldFrequencyFromPeriod(period)
	}

	return ChannelSnapshot{
		Active:          channel.active,
		Trigger:         channel.trigger,
		Note:            channel.note,
		Instrument:      channel.instrument,
		Sample:          channel.sample,
		SampleOffset:    channel.sampleOffset,
		Frequency:       frequency,
		Volume:          channel.volume,
		Panning:         channel.panning,
		KeyOn:           channel.keyOn,
		KeyFade:         channel.keyFade,
		FadeVolume:      channel.fadeVolume,
		EnvelopeVolume:  channel.volumeEnvelope,
		EnvelopePanning: channel.panningEnvelope,
		EnvelopePitch:   channel.pitchEnvelope,
		PitchDelta:      pitchDelta,
		VolumeDelta:     channel.volumeDelta,
		PanningDelta:    channel.panningDelta,
	}
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	cloned := snapshot
	cloned.Channels = append([]ChannelSnapshot(nil), snapshot.Channels...)
	cloned.Voices = append([]ChannelSnapshot(nil), snapshot.Voices...)
	return cloned
}

func resizeZeroed[T any](values []T, size int) []T {
	if cap(values) < size {
		return make([]T, size)
	}
	values = values[:size]
	clear(values)
	return values
}

func noteDelayTick(cmd unitrk.Command) (int, bool) {
	switch cmd.Op {
	case unitrk.OpPTEffectE:
		high := int(cmd.Param >> 4)
		if high == 0x0d {
			return int(cmd.Param & 0x0f), true
		}
	case unitrk.OpITEffectS0:
		high := int(cmd.Param >> 4)
		if high == 0x0d {
			return int(cmd.Param & 0x0f), true
		}
	}
	return 0, false
}

func noteCutTick(cmd unitrk.Command) (int, bool) {
	switch cmd.Op {
	case unitrk.OpPTEffectE:
		high := int(cmd.Param >> 4)
		if high == 0x0c {
			return int(cmd.Param & 0x0f), true
		}
	case unitrk.OpITEffectS0:
		high := int(cmd.Param >> 4)
		if high == 0x0c {
			tick := int(cmd.Param & 0x0f)
			if tick == 0 {
				tick = 1
			}
			return tick, true
		}
	}
	return 0, false
}

func (e *envelopeState) reset(env *modmodel.Envelope) {
	e.env = env
	e.a = 0
	e.b = 0
	e.pos = 0
	e.fresh = false
	if env == nil || !env.Enabled() || len(env.Points) == 0 {
		return
	}
	if env.Flags&modmodel.EnvelopeSustain == 0 && len(env.Points) > 1 {
		e.b = 1
	}
	if len(env.Points) >= 2 && env.Points[0].Tick == env.Points[1].Tick {
		e.a = 1
		e.b++
	}
	last := len(env.Points) - 1
	if e.a > last {
		e.a = last
	}
	if e.b > last {
		e.b = last
	}
	e.fresh = true
}

func (e *envelopeState) enabled() bool {
	return e.env != nil && e.env.Enabled() && len(e.env.Points) != 0
}

func (e *envelopeState) setPosition(pos int) {
	if !e.enabled() {
		return
	}
	e.fresh = false
	points := e.env.Points
	for i := 0; i < len(points)-1; i++ {
		if pos >= int(points[i].Tick) && pos < int(points[i+1].Tick) {
			e.a = i
			e.b = i + 1
			e.pos = pos
			return
		}
	}
	e.a = len(points) - 1
	e.b = e.a
	e.pos = int(points[e.a].Tick)
}

func (e *envelopeState) tickValue(keyOn bool) int {
	if !e.enabled() {
		switch e.kind {
		case envelopePanning:
			return defaultPanningEnv
		case envelopePitch:
			return defaultPitchEnv
		default:
			return defaultVolumeEnvelope
		}
	}

	points := e.env.Points
	last := len(points) - 1
	if e.a > last {
		e.a = last
	}
	if e.b > last {
		e.b = last
	}
	if e.b < e.a {
		e.b = e.a
	}

	if e.fresh {
		e.fresh = false
		return int(points[e.a].Value)
	}

	if e.env.Flags&modmodel.EnvelopeSustain != 0 && keyOn && int(e.env.SustainStart) == int(e.env.SustainEnd) && e.pos == int(points[e.env.SustainStart].Tick) {
		return int(points[e.env.SustainStart].Value)
	}

	value := 0
	if e.env.Flags&modmodel.EnvelopeSustain != 0 && keyOn && e.a >= int(e.env.SustainEnd) {
		e.a = int(e.env.SustainStart)
		e.b = e.a
		if e.a < last {
			e.b = e.a + 1
		}
		e.pos = int(points[e.a].Tick)
		value = int(points[e.a].Value)
	} else if e.env.Flags&modmodel.EnvelopeLoop != 0 && e.a >= int(e.env.LoopEnd) {
		e.a = int(e.env.LoopStart)
		e.b = e.a
		if e.a < last {
			e.b = e.a + 1
		}
		e.pos = int(points[e.a].Tick)
		value = int(points[e.a].Value)
	} else if e.a != e.b && e.b < len(points) {
		value = interpolate(e.pos, int(points[e.a].Tick), int(points[e.b].Tick), int(points[e.a].Value), int(points[e.b].Value))
	} else {
		value = int(points[e.a].Value)
	}

	lastTick := int(points[last].Tick)
	if e.pos < lastTick {
		e.pos++
		if e.b < len(points) && e.pos >= int(points[e.b].Tick) {
			e.a = e.b
			if e.b < last {
				e.b++
			}
		}
	}

	return value
}

func interpolate(pos, p1, p2, v1, v2 int) int {
	if p1 == p2 || pos == p1 {
		return v1
	}
	return v1 + (pos-p1)*(v2-v1)/(p2-p1)
}

func applyEnvelopePanning(base, env int) int {
	if base == int(modmodel.PanSurround) {
		return base
	}
	pan := base + ((env-defaultPanningEnv)*(128-absInt(base-defaultPanningEnv)))/128
	return clampPanning(pan)
}

func lfo(kind byte, pos int) int {
	position := pos & 0x3f
	switch kind & 0x03 {
	case 0:
		return int(math.Round(math.Sin((2*math.Pi*float64(position))/64.0) * 64.0))
	case 1:
		return 64 - position*2
	case 2:
		if position < 32 {
			return 64
		}
		return -64
	default:
		v := ((position * 1103515245) + 12345) >> 16
		return (v & 0x7f) - 64
	}
}

var oldStyleVibratoTable = [...]int{
	0, 6, 13, 19, 25, 31, 37, 44, 50, 56, 62, 68, 74, 80, 86, 92,
	98, 103, 109, 115, 120, 126, 131, 136, 142, 147, 152, 157, 162, 167, 171, 176,
	180, 185, 189, 193, 197, 201, 205, 208, 212, 215, 219, 222, 225, 228, 231, 233,
	236, 238, 240, 242, 244, 246, 247, 249, 250, 251, 252, 253, 254, 254, 255, 255,
	255, 255, 255, 254, 254, 253, 252, 251, 250, 249, 247, 246, 244, 242, 240, 238,
	236, 233, 231, 228, 225, 222, 219, 215, 212, 208, 205, 201, 197, 193, 189, 185,
	180, 176, 171, 167, 162, 157, 152, 147, 142, 136, 131, 126, 120, 115, 109, 103,
	98, 92, 86, 80, 74, 68, 62, 56, 50, 44, 37, 31, 25, 19, 13, 6,
}

func oldStyleLFOVibrato(kind byte, pos int) int {
	position := int(int8(uint8(pos)))
	switch kind & 0x03 {
	case 0:
		amp := oldStyleVibratoTable[int(uint8(position)&0x7f)]
		if position >= 0 {
			return amp
		}
		return -amp
	case 1:
		return int(uint8(position))<<1 - 255
	case 2:
		if position >= 0 {
			return 255
		}
		return -255
	default:
		v := ((int(uint8(position)) * 1103515245) + 12345) >> 16
		return (v & 0x1ff) - 256
	}
}

func oldStyleVibratoDelta(kind byte, pos, depth int, fine bool) int {
	temp := oldStyleLFOVibrato(kind, pos) * depth
	temp >>= 7
	if !fine {
		temp <<= 2
	}
	return temp
}

func xmVibratoDelta(kind byte, pos, depth int) int {
	temp := oldStyleLFOVibrato(kind, pos) * depth
	temp >>= 7
	temp <<= 2
	return temp
}

func vibratoScale(fine bool) int {
	if fine {
		return 128
	}
	return 64
}

func slideAmount(data byte) int {
	up := int(data >> 4)
	down := int(data & 0x0f)
	if up != 0 && down == 0 {
		return up
	}
	if down != 0 && up == 0 {
		return -down
	}
	if up != 0 {
		return up
	}
	return -down
}

func applyS3MVolumeSlide(tick byte, flags modmodel.Flags, data byte) int {
	if data == 0 {
		return 0
	}
	lo := int(data & 0x0f)
	hi := int(data >> 4)
	switch {
	case lo == 0:
		if tick > 0 || flags&modmodel.FlagUsesS3MSlides != 0 {
			return hi
		}
	case hi == 0:
		if tick > 0 || flags&modmodel.FlagUsesS3MSlides != 0 {
			return -lo
		}
	case lo == 0x0f:
		if tick == 0 {
			if hi != 0 {
				return hi
			}
			return 0x0f
		}
	case hi == 0x0f:
		if tick == 0 {
			if lo != 0 {
				return -lo
			}
			return -0x0f
		}
	}
	return 0
}

func applyS3MPeriodSlide(tick byte, data byte) int {
	if data == 0 {
		return 0
	}
	hi := int(data >> 4)
	lo := int(data & 0x0f)
	switch {
	case hi == 0x0f:
		if tick == 0 {
			return lo << 2
		}
	case hi == 0x0e:
		if tick == 0 {
			return lo
		}
	default:
		if tick > 0 {
			return int(data) << 2
		}
	}
	return 0
}

func applyS3MRetrigSlide(volume int, slide int) int {
	switch slide {
	case 1, 2, 3, 4, 5:
		return volume - (1 << (slide - 1))
	case 6:
		return (2 * volume) / 3
	case 7:
		return volume >> 1
	case 9, 10, 11, 12, 13:
		return volume + (1 << (slide - 9))
	case 14:
		return (3 * volume) >> 1
	case 15:
		return volume << 1
	default:
		return volume
	}
}

func arpeggioOffset(tick int, value byte) int {
	switch tick % 3 {
	case 1:
		return int(value >> 4)
	case 2:
		return int(value & 0x0f)
	default:
		return 0
	}
}

func slideToward(current, target, step int) int {
	if step <= 0 || current == target {
		return current
	}
	if current < target {
		current += step
		if current > target {
			return target
		}
		return current
	}
	current -= step
	if current < target {
		return target
	}
	return current
}

func (e *Engine) usesOldPeriods() bool {
	return e.module.Flags&modmodel.FlagXMPeriods == 0
}

func (e *Engine) notePeriod(sampleIndex, note int) int {
	return e.notePeriodWithFineTune(sampleIndex, note, e.sampleFineTune(sampleIndex))
}

func (e *Engine) channelNotePeriod(channel *channelState, sampleIndex, note int) int {
	if e.module.Metadata.Format == modmodel.FormatXM && channel != nil {
		return e.notePeriodWithFineTune(sampleIndex, note, channel.xmFineTune)
	}
	return e.notePeriod(sampleIndex, note)
}

func (e *Engine) sampleFineTune(sampleIndex int) int16 {
	if sampleIndex >= 0 && sampleIndex < len(e.module.Samples) {
		return e.module.Samples[sampleIndex].FineTune
	}
	return 0
}

func (e *Engine) notePeriodWithFineTune(sampleIndex, note int, fineTune int16) int {
	if note < 0 {
		return 0
	}
	if e.module.Metadata.Format == modmodel.FormatXM {
		return pitch.XMPeriod(note, fineTune, e.module.Flags&modmodel.FlagLinearPeriods != 0)
	}
	if !e.usesOldPeriods() {
		return 0
	}
	speed := uint32(8363)
	if sampleIndex >= 0 && sampleIndex < len(e.module.Samples) && e.module.Samples[sampleIndex].C5Speed != 0 {
		speed = e.module.Samples[sampleIndex].C5Speed
	}
	return oldPeriodForNote(note, speed)
}

func (c *channelState) resetRowEffects() {
	c.sampleOffset = 0
	c.s3mTremorActive = false
	c.s3mTremorMuted = false
	c.s3mRetrigActive = false
	c.arpeggioActive = false
	c.ptSlideUpActive = false
	c.ptSlideDownActive = false
	c.s3mSlideUpActive = false
	c.s3mSlideDownActive = false
	c.s3mVolumeSlideActive = false
	c.vibratoActive = false
	c.tremoloActive = false
	c.panbrelloActive = false
	c.tonePortamento = false
	c.volumeSlideActive = false
	c.panningSlideActive = false
	c.channelSlideActive = false
}

var oldPeriods = [...]uint32{
	0x6b00, 0x6800, 0x6500, 0x6220, 0x5f50, 0x5c80,
	0x5a00, 0x5740, 0x54d0, 0x5260, 0x5010, 0x4dc0,
	0x4b90, 0x4960, 0x4750, 0x4540, 0x4350, 0x4160,
	0x3f90, 0x3dc0, 0x3c10, 0x3a40, 0x38b0, 0x3700,
}

func oldPeriodForNote(note int, speed uint32) int {
	if note < 0 {
		return 0
	}
	if speed == 0 {
		speed = 8363
	}
	note *= 2
	index := note % len(oldPeriods)
	if index < 0 {
		index += len(oldPeriods)
	}
	octave := note / len(oldPeriods)
	return int(((8363 * oldPeriods[index]) >> octave) / speed)
}

func oldFrequencyFromPeriod(period int) int {
	if period <= 0 {
		return 0
	}
	return (8363 * 1712) / period
}

func clampPanning(value int) int {
	switch {
	case value == int(modmodel.PanSurround):
		return value
	case value < 0:
		return 0
	case value > 255:
		return 255
	default:
		return value
	}
}

func clampInt(value, minValue, maxValue int) int {
	switch {
	case value < minValue:
		return minValue
	case value > maxValue:
		return maxValue
	default:
		return value
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func specialPanning(low byte) int {
	pan := int(low)
	if pan <= 8 {
		pan <<= 4
	} else {
		pan *= 17
	}
	return clampPanning(pan)
}

func autoVibratoIT(phase int, waveform byte) int {
	signed := signedPhase(phase)
	switch waveform & 0x03 {
	case 1:
		return 255 - ((phase & 0xff) << 1)
	case 2:
		if signed >= 0 {
			return 255
		}
		return 0
	default:
		return regularAutoVibrato(phase)
	}
}

func autoVibratoXM(phase int, waveform byte) int {
	signed := signedPhase(phase)
	switch waveform & 0x03 {
	case 1:
		if signed >= 0 {
			return 255
		}
		return -255
	case 2:
		return -(signed << 1)
	case 3:
		return signed << 1
	default:
		return regularAutoVibrato(phase)
	}
}

func regularAutoVibrato(phase int) int {
	return int(math.Round(math.Sin((2*math.Pi*float64(phase&0xff))/256.0) * 255.0))
}

func signedPhase(phase int) int {
	return int(int8(byte(phase)))
}
