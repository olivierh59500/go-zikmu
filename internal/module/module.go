package module

import (
	"fmt"

	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

const (
	MaxChannels     = 64
	InstrumentNotes = 120
	MaxFilterMacros = 0x10
	MaxFilters      = 0x100
	LastPattern     = ^uint16(0)
)

const (
	PanLeft      uint16 = 0
	PanHalfLeft  uint16 = 64
	PanCenter    uint16 = 128
	PanHalfRight uint16 = 192
	PanRight     uint16 = 255
	PanSurround  uint16 = 512
)

type Format string

const (
	FormatUnknown Format = ""
	FormatMOD     Format = "mod"
	FormatS3M     Format = "s3m"
	FormatXM      Format = "xm"
	FormatIT      Format = "it"
)

type Flags uint32

const (
	FlagXMPeriods Flags = 1 << iota
	FlagLinearPeriods
	FlagUsesInstruments
	FlagUsesNNA
	FlagUsesS3MSlides
	FlagBackgroundSlides
	FlagHighBPM
	FlagNoWrapPatternBreak
	FlagArpeggioMemory
	FlagFT2Quirks
	FlagUsesPanning
	FlagFarTempo
)

type SampleFlags uint32

const (
	Sample16Bits SampleFlags = 1 << iota
	SampleStereo
	SampleSigned
	SampleBigEndian
	SampleDelta
	SampleITPacked
	SampleADPCM4
	SampleLoop
	SampleBidiLoop
	SampleReverse
	SampleSustainLoop
	SampleSustainBidiLoop
	SampleOwnPanning
)

type InstrumentFlags uint8

const (
	InstrumentOwnPanning InstrumentFlags = 1 << iota
	InstrumentPitchPan
)

type EnvelopeFlags uint8

const (
	EnvelopeEnabled EnvelopeFlags = 1 << iota
	EnvelopeSustain
	EnvelopeLoop
	EnvelopeVolume
)

type NNA uint8

const (
	NNACut NNA = iota
	NNAContinue
	NNAOff
	NNAFade
)

type DCT uint8

const (
	DCTOff DCT = iota
	DCTNote
	DCTSample
	DCTInstrument
)

type DCA uint8

const (
	DCACut DCA = iota
	DCAOff
	DCAFade
)

type AutoVibratoFlags uint8

const (
	AutoVibratoIT AutoVibratoFlags = 1 << iota
)

type Metadata struct {
	Title   string
	Tracker string
	Format  Format
	Message string
}

type Channel struct {
	Panning uint16
	Volume  uint8
}

type FilterSetting struct {
	Filter uint8
	Info   uint8
}

type EnvelopePoint struct {
	Tick  uint16
	Value int16
}

type Envelope struct {
	Flags        EnvelopeFlags
	SustainStart uint8
	SustainEnd   uint8
	LoopStart    uint8
	LoopEnd      uint8
	Points       []EnvelopePoint
}

func (e Envelope) Enabled() bool {
	return e.Flags&EnvelopeEnabled != 0
}

type AutoVibrato struct {
	Flags    AutoVibratoFlags
	Waveform uint8
	Sweep    uint8
	Depth    uint8
	Rate     uint8
}

type NoteSample struct {
	Note   uint8
	Sample uint16
}

type Instrument struct {
	Name               string
	Flags              InstrumentFlags
	NoteMap            [InstrumentNotes]NoteSample
	NewNoteAction      NNA
	DuplicateAction    DCA
	DuplicateCheck     DCT
	GlobalVolume       uint8
	FadeOut            uint16
	Panning            int16
	PitchPanSeparation uint8
	PitchPanCenter     uint8
	RandomVolumeVar    uint8
	RandomPanningVar   uint8
	VolumeEnvelope     Envelope
	PanningEnvelope    Envelope
	PitchEnvelope      Envelope
}

type Sample struct {
	Name         string
	Panning      int16
	C5Speed      uint32
	Volume       uint8
	GlobalVolume uint8
	RelativeNote int8
	FineTune     int16
	DivFactor    uint8
	Flags        SampleFlags
	Length       uint32
	LoopStart    uint32
	LoopEnd      uint32
	SustainStart uint32
	SustainEnd   uint32
	Data         []int16
	Vibrato      AutoVibrato
}

type Pattern struct {
	Rows   uint16
	Tracks []unitrk.Track
}

func (p Pattern) Channels() int {
	return len(p.Tracks)
}

type Module struct {
	Metadata            Metadata
	Flags               Flags
	Channels            int
	Voices              int
	RestartPosition     uint16
	InitialSpeed        uint8
	InitialTempo        uint16
	InitialGlobalVolume uint8
	BPMThreshold        uint16
	Orders              []uint16
	Patterns            []Pattern
	Instruments         []Instrument
	Samples             []Sample
	ChannelSettings     [MaxChannels]Channel
	FilterMacros        [MaxFilterMacros]uint8
	FilterSettings      [MaxFilters]FilterSetting
	ActiveFilterMacro   uint8
}

func New() *Module {
	m := &Module{}
	m.ResetDefaults()
	return m
}

func (m *Module) ResetDefaults() {
	*m = Module{
		InitialGlobalVolume: 128,
		BPMThreshold:        33,
	}

	for i := range m.ChannelSettings {
		m.ChannelSettings[i] = Channel{
			Panning: DefaultChannelPanning(i),
			Volume:  64,
		}
	}
}

func DefaultChannelPanning(index int) uint16 {
	if ((index + 1) & 2) != 0 {
		return PanRight
	}
	return PanLeft
}

func ImplicitPatternPanning(index int) uint16 {
	if ((index + 1) & 2) != 0 {
		return PanHalfRight
	}
	return PanHalfLeft
}

func (m *Module) ApplyImplicitPanning() {
	if m.Flags&FlagUsesPanning != 0 {
		return
	}

	channels := m.Channels
	if channels < 0 {
		channels = 0
	}
	if channels > MaxChannels {
		channels = MaxChannels
	}

	for i := 0; i < channels; i++ {
		m.ChannelSettings[i].Panning = ImplicitPatternPanning(i)
	}
}

func (m *Module) Validate() error {
	if m.Channels < 0 || m.Channels > MaxChannels {
		return fmt.Errorf("module: invalid channel count %d", m.Channels)
	}
	if m.Voices < 0 {
		return fmt.Errorf("module: invalid voice count %d", m.Voices)
	}

	for i, order := range m.Orders {
		if order == LastPattern {
			continue
		}
		if int(order) >= len(m.Patterns) {
			return fmt.Errorf("module: order %d references missing pattern %d", i, order)
		}
	}

	for pi, pattern := range m.Patterns {
		if pattern.Rows == 0 {
			return fmt.Errorf("module: pattern %d has zero rows", pi)
		}
		if m.Channels > 0 && len(pattern.Tracks) != 0 && len(pattern.Tracks) != m.Channels {
			return fmt.Errorf("module: pattern %d has %d tracks for %d channels", pi, len(pattern.Tracks), m.Channels)
		}
		for ti, track := range pattern.Tracks {
			if len(track.Rows) != int(pattern.Rows) {
				return fmt.Errorf("module: pattern %d track %d has %d rows for declared row count %d", pi, ti, len(track.Rows), pattern.Rows)
			}
		}
	}

	return nil
}
