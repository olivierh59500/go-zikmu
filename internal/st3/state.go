package st3

import "math"

type state struct {
	songName [29]byte

	packedPatterns    bool
	musicPaused       bool
	interpolationFlag bool
	oldstvib          bool
	fastvolslide      bool
	amigalimits       bool

	volslidetype     int8
	patterndelay     int8
	patloopcount     int8
	lastachannelused int8

	order       [256]uint8
	chnsettings [32]uint8
	patdata     [100][]byte
	npPatseg    []byte

	musicmax      uint8
	soundcardtype uint8
	breakpat      uint8
	startrow      uint8
	musiccount    uint8

	jmptoord     int16
	npOrd        int16
	npRow        int16
	npPat        int16
	npPatoff     int16
	patloopstart int16
	jumptorow    int16
	globalvol    int16
	aspdmin      int16
	aspdmax      int16

	useglobalvol uint16
	patmusicrand uint16
	ordNum       uint16
	insNum       uint16
	patNum       uint16

	mastermul       int32
	mastervol       int32
	mixingVol       int32
	samplesLeft     int32
	soundBufferSize int32
	mixBufferL      []int32
	mixBufferR      []int32

	prngStateL int32
	prngStateR int32
	randSeed   int32

	samplesPerTick uint32
	audioRate      uint32
	sampleCounter  uint32
	npZframe       uint32

	chn   [32]channel
	voice [32]voice
	ins   [100]instrument

	dPer2HzDiv float64

	sampleMarkers     [128]sampleMarker
	sampleMarkerIndex uint16
	sampleMarkerCount uint16
	totalSampleCount  uint32
}

func newState(sampleRate int, interpolation bool) *state {
	if sampleRate == 0 {
		sampleRate = 44100
	}

	s := &state{
		musicPaused:       true,
		interpolationFlag: interpolation,
		mastervol:         256,
		audioRate:         uint32(sampleRate),
		soundBufferSize:   mixBufSamples,
		randSeed:          initialDitherSeed,
		soundcardtype:     soundcardGUS,
	}

	s.dPer2HzDiv = (14317056.0 / float64(sampleRate)) * 65536.0
	if math.IsNaN(s.dPer2HzDiv) || math.IsInf(s.dPer2HzDiv, 0) {
		s.dPer2HzDiv = 0
	}

	s.mixBufferL = make([]int32, mixBufSamples)
	s.mixBufferR = make([]int32, mixBufSamples)

	return s
}
