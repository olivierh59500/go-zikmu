package testfixtures

const modSignatureOffset = 1080

func MOD(signature string) []byte {
	buf := make([]byte, modSignatureOffset+4)
	copy(buf[modSignatureOffset:], []byte(signature))
	return buf
}

func S3M() []byte {
	buf := make([]byte, 0x30)
	copy(buf[0x2c:], []byte("SCRM"))
	return buf
}

func XM() []byte {
	buf := make([]byte, 38)
	copy(buf, []byte("Extended Module: "))
	buf[37] = 0x1a
	return buf
}

func IT() []byte {
	buf := make([]byte, 4)
	copy(buf, []byte("IMPM"))
	return buf
}

func Unknown() []byte {
	return []byte("not a module")
}

type S3MSample struct {
	Name      string
	Type      byte
	Length    uint32
	LoopStart uint32
	LoopEnd   uint32
	Volume    byte
	Pack      byte
	Flags     byte
	C2Speed   uint32
	Data      []byte
	SCRS      bool
}

type XMEnvelopePoint struct {
	Tick  uint16
	Value uint16
}

type ITEnvelopePoint struct {
	Tick  uint16
	Value int16
}

type ITSample struct {
	Name         string
	GlobalVolume byte
	Flags        byte
	Volume       byte
	Convert      byte
	Panning      byte
	Length       uint32
	LoopStart    uint32
	LoopEnd      uint32
	C5Speed      uint32
	SustainStart uint32
	SustainEnd   uint32
	VibratoSpeed byte
	VibratoDepth byte
	VibratoRate  byte
	VibratoWave  byte
	Data         []byte
}

type ITInstrument struct {
	Name                string
	NNA                 byte
	DCT                 byte
	DCA                 byte
	FadeOut             uint16
	PitchPanSeparation  byte
	PitchPanCenter      byte
	GlobalVolume        byte
	ChannelPanning      byte
	RandomVolume        byte
	RandomPanning       byte
	VolumeFlags         byte
	VolumePoints        byte
	VolumeLoopStart     byte
	VolumeLoopEnd       byte
	VolumeSustainStart  byte
	VolumeSustainEnd    byte
	PanningFlags        byte
	PanningPoints       byte
	PanningLoopStart    byte
	PanningLoopEnd      byte
	PanningSustainStart byte
	PanningSustainEnd   byte
	PitchFlags          byte
	PitchPoints         byte
	PitchLoopStart      byte
	PitchLoopEnd        byte
	PitchSustainStart   byte
	PitchSustainEnd     byte
	VolumeEnvelope      []ITEnvelopePoint
	PanningEnvelope     []ITEnvelopePoint
	PitchEnvelope       []ITEnvelopePoint
	SampleMap           []uint16
}

type ITEvent struct {
	Channel    int
	Row        int
	Note       byte
	Instrument byte
	VolPan     byte
	Command    byte
	Info       byte
}

type ITPattern struct {
	Rows   uint16
	Events []ITEvent
}

type ITFixtureOptions struct {
	Title          string
	CreatedWith    uint16
	CompatibleWith uint16
	Flags          uint16
	Special        uint16
	GlobalVolume   byte
	InitialSpeed   byte
	InitialTempo   byte
	Orders         []byte
	ChannelPanning []byte
	ChannelVolume  []byte
	Message        string
	Instruments    []ITInstrument
	Samples        []ITSample
	Patterns       []ITPattern
}

type XMSample struct {
	Name         string
	Length       uint32
	LoopStart    uint32
	LoopLength   uint32
	Volume       byte
	FineTune     int8
	Type         byte
	Panning      byte
	RelativeNote int8
	Reserved     byte
	Data         []byte
}

type XMInstrument struct {
	Name             string
	SampleMap        []byte
	VolumeEnvelope   []XMEnvelopePoint
	PanningEnvelope  []XMEnvelopePoint
	VolumePoints     byte
	PanningPoints    byte
	VolumeSustain    byte
	VolumeLoopStart  byte
	VolumeLoopEnd    byte
	PanningSustain   byte
	PanningLoopStart byte
	PanningLoopEnd   byte
	VolumeFlags      byte
	PanningFlags     byte
	VibratoType      byte
	VibratoSweep     byte
	VibratoDepth     byte
	VibratoRate      byte
	FadeOut          uint16
	Samples          []XMSample
}

type XMEvent struct {
	Channel    int
	Row        int
	Note       uint8
	Instrument uint8
	Volume     uint8
	Effect     uint8
	Data       uint8
}

type XMPattern struct {
	Rows   uint16
	Events []XMEvent
}

type XMFixtureOptions struct {
	Title       string
	Tracker     string
	Version     uint16
	Restart     uint16
	Flags       uint16
	Tempo       uint16
	BPM         uint16
	Orders      []byte
	Channels    int
	Patterns    []XMPattern
	Instruments []XMInstrument
}

type S3MEvent struct {
	Channel    int
	Row        int
	Note       uint8
	Instrument uint8
	Volume     uint8
	Command    uint8
	Info       uint8
	HasNote    bool
	HasVolume  bool
	HasEffect  bool
}

type S3MPattern struct {
	Events []S3MEvent
}

type S3MFixtureOptions struct {
	Title           string
	Tracker         uint16
	Flags           uint16
	FileFormat      uint16
	MasterVolume    byte
	InitialSpeed    byte
	InitialTempo    byte
	MasterMult      byte
	Orders          []byte
	ChannelSettings []byte
	ChannelPanning  []byte
	UsePanTable     bool
	Samples         []S3MSample
	Patterns        []S3MPattern
}

func MinimalS3M() []byte {
	return buildS3MFixture(S3MFixtureOptions{
		Title:        "S3M Fixture",
		Tracker:      0x1300,
		Flags:        0x0040,
		FileFormat:   1,
		MasterVolume: 64,
		InitialSpeed: 6,
		InitialTempo: 125,
		Orders:       []byte{0, 254, 0, 255},
		ChannelSettings: []byte{
			0, 255, 8,
		},
		ChannelPanning: []byte{
			0x20 | 0x03, 0, 0x20 | 0x0c,
		},
		UsePanTable: true,
		Samples: []S3MSample{
			{
				Name:    "Lead",
				Type:    1,
				Length:  2,
				Volume:  48,
				C2Speed: 8363,
				Data:    []byte{0x00, 0x7f},
				SCRS:    true,
			},
		},
		Patterns: []S3MPattern{
			{
				Events: []S3MEvent{
					{
						Channel:    0,
						Row:        0,
						Note:       0x11,
						Instrument: 1,
						Volume:     40,
						Command:    0x01,
						Info:       0x03,
						HasNote:    true,
						HasVolume:  true,
						HasEffect:  true,
					},
					{
						Channel:    2,
						Row:        1,
						Note:       0x12,
						Instrument: 1,
						Volume:     32,
						Command:    0x02,
						Info:       0x02,
						HasNote:    true,
						HasVolume:  true,
						HasEffect:  true,
					},
				},
			},
		},
	})
}

func MinimalS3MADPCM() []byte {
	return buildS3MFixture(S3MFixtureOptions{
		Title:        "S3M ADPCM",
		Tracker:      0x1300,
		FileFormat:   1,
		MasterVolume: 64,
		InitialSpeed: 6,
		InitialTempo: 125,
		Orders:       []byte{0, 255},
		ChannelSettings: []byte{
			0,
		},
		Samples: []S3MSample{
			{
				Name:    "Packed",
				Type:    1,
				Length:  2,
				Volume:  64,
				Pack:    4,
				C2Speed: 8363,
				Data: append(
					[]byte{1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
					0x10,
				),
				SCRS: true,
			},
		},
		Patterns: []S3MPattern{
			{
				Events: []S3MEvent{
					{
						Channel:    0,
						Row:        0,
						Instrument: 1,
						HasNote:    true,
					},
				},
			},
		},
	})
}

func MinimalXM() []byte {
	return buildXMFixture(XMFixtureOptions{
		Title:    "XM Fixture",
		Tracker:  "FastTracker v2.00",
		Version:  0x0104,
		Flags:    1,
		Tempo:    6,
		BPM:      125,
		Orders:   []byte{0},
		Channels: 2,
		Patterns: []XMPattern{
			{
				Rows: 2,
				Events: []XMEvent{
					{
						Channel:    0,
						Row:        0,
						Note:       49,
						Instrument: 1,
						Volume:     0x28,
						Effect:     'G' - 55,
						Data:       32,
					},
					{
						Channel: 1,
						Row:     1,
						Note:    97,
						Volume:  0xf2,
						Effect:  0x04,
						Data:    0x34,
					},
				},
			},
		},
		Instruments: []XMInstrument{
			{
				Name:             "Lead",
				FadeOut:          256,
				VolumeEnvelope:   []XMEnvelopePoint{{Tick: 0, Value: 0}, {Tick: 10, Value: 64}},
				PanningEnvelope:  []XMEnvelopePoint{{Tick: 0, Value: 32}, {Tick: 5, Value: 40}},
				VolumePoints:     2,
				PanningPoints:    2,
				VolumeSustain:    1,
				VolumeLoopStart:  0,
				VolumeLoopEnd:    1,
				PanningSustain:   0,
				PanningLoopStart: 0,
				PanningLoopEnd:   1,
				VolumeFlags:      0x07,
				PanningFlags:     0x01,
				VibratoType:      2,
				VibratoSweep:     3,
				VibratoDepth:     4,
				VibratoRate:      5,
				Samples: []XMSample{
					{
						Name:         "Lead Sample",
						Length:       2,
						LoopStart:    0,
						LoopLength:   2,
						Volume:       64,
						FineTune:     1,
						Type:         0x01,
						Panning:      200,
						RelativeNote: 2,
						Data:         []byte{0x10, 0x10},
					},
				},
			},
		},
	})
}

func MinimalXMADPCM() []byte {
	return buildXMFixture(XMFixtureOptions{
		Title:    "XM ADPCM",
		Tracker:  "FastTracker v2.00",
		Version:  0x0104,
		Tempo:    6,
		BPM:      125,
		Orders:   []byte{0},
		Channels: 1,
		Patterns: []XMPattern{
			{
				Rows: 1,
				Events: []XMEvent{
					{
						Channel:    0,
						Row:        0,
						Instrument: 1,
					},
				},
			},
		},
		Instruments: []XMInstrument{
			{
				Name: "Packed",
				Samples: []XMSample{
					{
						Name:     "Packed",
						Length:   2,
						Volume:   64,
						Panning:  128,
						Reserved: 0xad,
						Data:     append([]byte{1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, 0x10),
					},
				},
			},
		},
	})
}

func MinimalXM103() []byte {
	return buildXMFixture(XMFixtureOptions{
		Title:    "Legacy XM",
		Tracker:  "FastTracker v1.04",
		Version:  0x0103,
		Tempo:    6,
		BPM:      125,
		Orders:   []byte{1},
		Channels: 1,
		Patterns: []XMPattern{
			{
				Rows: 1,
				Events: []XMEvent{
					{
						Channel:    0,
						Row:        0,
						Note:       49,
						Instrument: 1,
					},
				},
			},
		},
		Instruments: []XMInstrument{
			{
				Name: "Legacy",
				Samples: []XMSample{
					{
						Name:    "Legacy Sample",
						Length:  1,
						Volume:  64,
						Panning: 128,
						Data:    []byte{0x7f},
					},
				},
			},
		},
	})
}

func MinimalIT() []byte {
	return buildITFixture(ITFixtureOptions{
		Title:          "IT Fixture",
		CreatedWith:    0x0216,
		CompatibleWith: 0x0213,
		Flags:          0x000d,
		GlobalVolume:   128,
		InitialSpeed:   6,
		InitialTempo:   125,
		Orders:         []byte{0},
		ChannelPanning: []byte{0, 32},
		ChannelVolume:  []byte{64, 48},
		Message:        "hello\rworld",
		Samples: []ITSample{
			{
				Name:         "Lead Sample",
				GlobalVolume: 64,
				Flags:        0x10 | 0x20 | 0x80,
				Volume:       64,
				Convert:      0x05,
				Panning:      0x80 | 32,
				Length:       2,
				LoopStart:    0,
				LoopEnd:      2,
				C5Speed:      8363,
				SustainStart: 0,
				SustainEnd:   2,
				VibratoSpeed: 6,
				VibratoDepth: 4,
				VibratoRate:  5,
				VibratoWave:  2,
				Data:         []byte{0x10, 0x10},
			},
		},
		Instruments: []ITInstrument{
			{
				Name:                "Lead",
				NNA:                 3,
				DCT:                 2,
				DCA:                 2,
				FadeOut:             8,
				PitchPanSeparation:  4,
				PitchPanCenter:      60,
				GlobalVolume:        128,
				ChannelPanning:      0x80 | 32,
				RandomVolume:        1,
				RandomPanning:       2,
				VolumeFlags:         0x07,
				VolumePoints:        2,
				VolumeLoopStart:     0,
				VolumeLoopEnd:       1,
				VolumeSustainStart:  1,
				VolumeSustainEnd:    1,
				PanningFlags:        0x01,
				PanningPoints:       2,
				PanningLoopStart:    0,
				PanningLoopEnd:      1,
				PanningSustainStart: 0,
				PanningSustainEnd:   0,
				PitchFlags:          0x01,
				PitchPoints:         2,
				PitchLoopStart:      0,
				PitchLoopEnd:        1,
				PitchSustainStart:   0,
				PitchSustainEnd:     0,
				VolumeEnvelope:      []ITEnvelopePoint{{Tick: 0, Value: 0}, {Tick: 10, Value: 64}},
				PanningEnvelope:     []ITEnvelopePoint{{Tick: 0, Value: 0}, {Tick: 8, Value: 32}},
				PitchEnvelope:       []ITEnvelopePoint{{Tick: 0, Value: -16}, {Tick: 6, Value: 16}},
			},
		},
		Patterns: []ITPattern{
			{
				Rows: 2,
				Events: []ITEvent{
					{
						Channel:    0,
						Row:        0,
						Note:       60,
						Instrument: 1,
						VolPan:     200,
						Command:    0x14,
						Info:       0x30,
					},
					{
						Channel:    1,
						Row:        1,
						Note:       255,
						Instrument: 255,
						VolPan:     128,
					},
				},
			},
		},
	})
}

func MinimalITPacked() []byte {
	return buildITFixture(ITFixtureOptions{
		Title:          "IT Packed",
		CreatedWith:    0x0216,
		CompatibleWith: 0x0213,
		Flags:          0x0004,
		GlobalVolume:   128,
		InitialSpeed:   6,
		InitialTempo:   125,
		Orders:         []byte{0},
		ChannelPanning: []byte{32},
		ChannelVolume:  []byte{64},
		Samples: []ITSample{
			{
				Name:         "Packed",
				GlobalVolume: 64,
				Flags:        0x08,
				Volume:       64,
				Convert:      0x01,
				Panning:      0x80 | 32,
				Length:       8,
				C5Speed:      8363,
				Data:         append([]byte{9, 0}, make([]byte, 9)...),
			},
		},
		Instruments: []ITInstrument{
			{
				Name: "Packed",
			},
		},
		Patterns: []ITPattern{
			{
				Rows: 1,
				Events: []ITEvent{
					{
						Channel:    0,
						Row:        0,
						Instrument: 1,
					},
				},
			},
		},
	})
}

func MinimalITLinearSamplesOnly() []byte {
	return buildITFixture(ITFixtureOptions{
		Title:          "IT Linear Sample",
		CreatedWith:    0x0216,
		CompatibleWith: 0x0213,
		Flags:          0x0009,
		GlobalVolume:   128,
		InitialSpeed:   6,
		InitialTempo:   125,
		Orders:         []byte{0},
		ChannelPanning: []byte{32},
		ChannelVolume:  []byte{64},
		Samples: []ITSample{
			{
				Name:         "Shifted",
				GlobalVolume: 64,
				Flags:        0x10,
				Volume:       64,
				Convert:      0x01,
				Panning:      32,
				Length:       1,
				C5Speed:      33452,
				Data:         []byte{0x40},
			},
		},
		Patterns: []ITPattern{
			{
				Rows: 1,
				Events: []ITEvent{
					{
						Channel:    0,
						Row:        0,
						Note:       48,
						Instrument: 1,
						VolPan:     255,
					},
				},
			},
		},
	})
}

type MODSample struct {
	Name            string
	LengthWords     uint16
	FineTune        byte
	Volume          byte
	LoopStartWords  uint16
	LoopLengthWords uint16
	Data            []byte
	ADPCMTable      []byte
}

type MODFixtureOptions struct {
	Title     string
	Signature string
	Channels  int
	Tracker   string
	Samples   []MODSample
}

func MinimalMOD() []byte {
	return buildMODFixture(MODFixtureOptions{
		Title:     "Test Module",
		Signature: "M.K.",
		Channels:  4,
		Samples: []MODSample{
			{
				Name:        "Kick",
				LengthWords: 1,
				Volume:      64,
				Data:        []byte{0x00, 0x7f},
			},
		},
	})
}

func MinimalMODADPCM() []byte {
	return buildMODFixture(MODFixtureOptions{
		Title:     "ADPCM Module",
		Signature: "M.K.",
		Channels:  4,
		Samples: []MODSample{
			{
				Name:        "Packed",
				LengthWords: 2,
				Volume:      64,
				ADPCMTable:  []byte{1, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
				Data:        []byte{0x10, 0x10},
			},
		},
	})
}

func MinimalFLT8MOD() []byte {
	return buildMODFixture(MODFixtureOptions{
		Title:     "FLT8 Module",
		Signature: "FLT8",
		Channels:  8,
		Samples: []MODSample{
			{
				Name:        "Kick",
				LengthWords: 1,
				Volume:      64,
				Data:        []byte{0x00, 0x7f},
			},
		},
	})
}

func buildMODFixture(opts MODFixtureOptions) []byte {
	if opts.Title == "" {
		opts.Title = "MOD Fixture"
	}
	if opts.Signature == "" {
		opts.Signature = "M.K."
	}
	if opts.Channels == 0 {
		opts.Channels = 4
	}

	buf := make([]byte, modSignatureOffset+4)
	copyPadded(buf[0:20], opts.Title)

	sampleOffset := 20
	for i := 0; i < 31; i++ {
		if i < len(opts.Samples) {
			sample := opts.Samples[i]
			copyPadded(buf[sampleOffset:sampleOffset+22], sample.Name)
			putBE16(buf[sampleOffset+22:sampleOffset+24], sample.LengthWords)
			buf[sampleOffset+24] = sample.FineTune
			buf[sampleOffset+25] = sample.Volume
			putBE16(buf[sampleOffset+26:sampleOffset+28], sample.LoopStartWords)
			putBE16(buf[sampleOffset+28:sampleOffset+30], sample.LoopLengthWords)
		}
		sampleOffset += 30
	}

	buf[950] = 1
	buf[951] = 127
	copy(buf[952:1080], make([]byte, 128))
	copy(buf[1080:1084], []byte(opts.Signature))

	patternData := buildMODPatternData(opts.Signature, opts.Channels)
	buf = append(buf, patternData...)

	for _, sample := range opts.Samples {
		if len(sample.ADPCMTable) != 0 {
			buf = append(buf, []byte("ADPCM")...)
			buf = append(buf, sample.ADPCMTable...)
		}
		buf = append(buf, sample.Data...)
	}

	return buf
}

func buildMODPatternData(signature string, channels int) []byte {
	if signature == "FLT8" || signature == "EXO8" {
		pattern := make([]byte, 64*4*4*2)
		copy(pattern[0:4], []byte{0x03, 0x58, 0x1c, 0x20})
		copy(pattern[64*4*4:64*4*4+4], []byte{0x03, 0x58, 0x1c, 0x10})
		return pattern
	}

	pattern := make([]byte, 64*channels*4)
	copy(pattern[0:4], []byte{0x03, 0x58, 0x1c, 0x20})
	return pattern
}

func copyPadded(dst []byte, value string) {
	copy(dst, []byte(value))
}

func putBE16(dst []byte, value uint16) {
	dst[0] = byte(value >> 8)
	dst[1] = byte(value)
}

func buildS3MFixture(opts S3MFixtureOptions) []byte {
	if opts.Title == "" {
		opts.Title = "S3M Fixture"
	}
	if opts.Tracker == 0 {
		opts.Tracker = 0x1300
	}
	if opts.MasterVolume == 0 {
		opts.MasterVolume = 64
	}
	if opts.InitialSpeed == 0 {
		opts.InitialSpeed = 6
	}
	if opts.InitialTempo == 0 {
		opts.InitialTempo = 125
	}
	if len(opts.Orders) == 0 {
		opts.Orders = []byte{0, 255}
	}
	if len(opts.ChannelSettings) == 0 {
		opts.ChannelSettings = []byte{0}
	}

	headerSize := 0x60
	orderOffset := headerSize
	parapointerOffset := orderOffset + len(opts.Orders)
	parapointerCount := len(opts.Samples) + len(opts.Patterns)
	extraPanning := 0
	if opts.UsePanTable {
		extraPanning = 32
	}

	buf := make([]byte, parapointerOffset+parapointerCount*2+extraPanning)
	copyPadded(buf[0:28], opts.Title)
	buf[28] = 0x1a
	buf[29] = 0x10
	putLE16(buf[32:34], uint16(len(opts.Orders)))
	putLE16(buf[34:36], uint16(len(opts.Samples)))
	putLE16(buf[36:38], uint16(len(opts.Patterns)))
	putLE16(buf[38:40], opts.Flags)
	putLE16(buf[40:42], opts.Tracker)
	putLE16(buf[42:44], opts.FileFormat)
	copy(buf[44:48], []byte("SCRM"))
	buf[48] = opts.MasterVolume
	buf[49] = opts.InitialSpeed
	buf[50] = opts.InitialTempo
	buf[51] = opts.MasterMult
	if opts.UsePanTable {
		buf[53] = 252
	}

	for i := 0; i < 32; i++ {
		buf[64+i] = 255
		if i < len(opts.ChannelSettings) {
			buf[64+i] = opts.ChannelSettings[i]
		}
	}

	copy(buf[orderOffset:orderOffset+len(opts.Orders)], opts.Orders)

	panningOffset := parapointerOffset + parapointerCount*2
	if opts.UsePanTable {
		for i := 0; i < 32; i++ {
			if i < len(opts.ChannelPanning) {
				buf[panningOffset+i] = opts.ChannelPanning[i]
			}
		}
	}

	nextOffset := align16(int64(len(buf)))
	if int(nextOffset) > len(buf) {
		buf = append(buf, make([]byte, int(nextOffset)-len(buf))...)
	}

	instrumentOffsets := make([]int64, len(opts.Samples))
	for i, sample := range opts.Samples {
		var offset int64
		buf, offset = appendAlignedBlock(buf, 80)
		instrumentOffsets[i] = offset
		header := buf[offset : offset+80]
		header[0] = sample.Type
		if header[0] == 0 {
			header[0] = 1
		}
		copyPadded(header[48:76], sample.Name)
		putLE32(header[16:20], sample.Length)
		putLE32(header[20:24], sample.LoopStart)
		putLE32(header[24:28], sample.LoopEnd)
		header[28] = sample.Volume
		header[30] = sample.Pack
		header[31] = sample.Flags
		putLE32(header[32:36], sample.C2Speed)
		if sample.SCRS {
			copy(header[76:80], []byte("SCRS"))
		}
		nextOffset = int64(len(buf))
	}

	patternOffsets := make([]int64, len(opts.Patterns))
	for i, pattern := range opts.Patterns {
		stream := buildS3MPatternStream(pattern)
		var offset int64
		buf, offset = appendAlignedBlock(buf, 2+len(stream))
		patternOffsets[i] = offset
		block := buf[offset : offset+int64(2+len(stream))]
		putLE16(block[0:2], uint16(len(stream)))
		copy(block[2:], stream)
		nextOffset = int64(len(buf))
	}

	for i, sample := range opts.Samples {
		if len(sample.Data) == 0 {
			continue
		}

		var dataOffset int64
		buf, dataOffset = appendAlignedBlock(buf, len(sample.Data))
		copy(buf[dataOffset:dataOffset+int64(len(sample.Data))], sample.Data)

		segment := uint32(dataOffset >> 4)
		header := buf[instrumentOffsets[i] : instrumentOffsets[i]+80]
		header[13] = byte(segment >> 16)
		putLE16(header[14:16], uint16(segment))
		nextOffset = int64(len(buf))
	}

	for i, offset := range instrumentOffsets {
		putLE16(buf[parapointerOffset+i*2:parapointerOffset+i*2+2], uint16(offset>>4))
	}
	for i, offset := range patternOffsets {
		base := parapointerOffset + (len(opts.Samples)+i)*2
		putLE16(buf[base:base+2], uint16(offset>>4))
	}

	return buf
}

func buildS3MPatternStream(pattern S3MPattern) []byte {
	eventsByRow := make([][]S3MEvent, 64)
	for _, event := range pattern.Events {
		if event.Row < 0 || event.Row >= 64 || event.Channel < 0 || event.Channel >= 32 {
			continue
		}
		eventsByRow[event.Row] = append(eventsByRow[event.Row], event)
	}

	stream := make([]byte, 0, 128)
	for row := 0; row < 64; row++ {
		for _, event := range eventsByRow[row] {
			flag := byte(event.Channel & 31)
			if event.HasNote {
				flag |= 0x20
			}
			if event.HasVolume {
				flag |= 0x40
			}
			if event.HasEffect {
				flag |= 0x80
			}
			stream = append(stream, flag)
			if event.HasNote {
				stream = append(stream, event.Note, event.Instrument)
			}
			if event.HasVolume {
				stream = append(stream, event.Volume)
			}
			if event.HasEffect {
				stream = append(stream, event.Command, event.Info)
			}
		}
		stream = append(stream, 0)
	}

	return stream
}

func appendAligned(buf []byte, size int) []byte {
	next := align16(int64(len(buf)))
	if int(next) > len(buf) {
		buf = append(buf, make([]byte, int(next)-len(buf))...)
	}
	return append(buf, make([]byte, size)...)
}

func appendAlignedBlock(buf []byte, size int) ([]byte, int64) {
	offset := align16(int64(len(buf)))
	if int(offset) > len(buf) {
		buf = append(buf, make([]byte, int(offset)-len(buf))...)
	}
	buf = append(buf, make([]byte, size)...)
	return buf, offset
}

func align16(offset int64) int64 {
	return (offset + 15) &^ 15
}

func putLE16(dst []byte, value uint16) {
	dst[0] = byte(value)
	dst[1] = byte(value >> 8)
}

func putLE32(dst []byte, value uint32) {
	dst[0] = byte(value)
	dst[1] = byte(value >> 8)
	dst[2] = byte(value >> 16)
	dst[3] = byte(value >> 24)
}

func buildXMFixture(opts XMFixtureOptions) []byte {
	if opts.Title == "" {
		opts.Title = "XM Fixture"
	}
	if opts.Tracker == "" {
		opts.Tracker = "FastTracker v2.00"
	}
	if opts.Version == 0 {
		opts.Version = 0x0104
	}
	if opts.Tempo == 0 {
		opts.Tempo = 6
	}
	if opts.BPM == 0 {
		opts.BPM = 125
	}
	if len(opts.Orders) == 0 {
		opts.Orders = []byte{0}
	}
	if opts.Channels == 0 {
		opts.Channels = 2
	}

	headerSize := uint32(20 + len(opts.Orders))
	buf := make([]byte, 60+headerSize)
	copy(buf[0:17], []byte("Extended Module: "))
	copyPadded(buf[17:37], opts.Title)
	buf[37] = 0x1a
	copyPadded(buf[38:58], opts.Tracker)
	putLE16(buf[58:60], opts.Version)
	putLE32(buf[60:64], headerSize)
	putLE16(buf[64:66], uint16(len(opts.Orders)))
	putLE16(buf[66:68], opts.Restart)
	putLE16(buf[68:70], uint16(opts.Channels))
	putLE16(buf[70:72], uint16(len(opts.Patterns)))
	putLE16(buf[72:74], uint16(len(opts.Instruments)))
	putLE16(buf[74:76], opts.Flags)
	putLE16(buf[76:78], opts.Tempo)
	putLE16(buf[78:80], opts.BPM)
	copy(buf[80:80+len(opts.Orders)], opts.Orders)

	patternBlocks := make([][]byte, len(opts.Patterns))
	for i, pattern := range opts.Patterns {
		patternBlocks[i] = buildXMPattern(pattern, opts.Channels)
	}

	instrumentBlocks := make([][]byte, len(opts.Instruments))
	sampleDataBlocks := make([][]byte, len(opts.Instruments))
	for i, instrument := range opts.Instruments {
		block, data := buildXMInstrument(instrument)
		instrumentBlocks[i] = block
		sampleDataBlocks[i] = data
	}

	if opts.Version < 0x0104 {
		for _, block := range instrumentBlocks {
			buf = append(buf, block...)
		}
		for _, block := range patternBlocks {
			buf = append(buf, block...)
		}
		for _, block := range sampleDataBlocks {
			buf = append(buf, block...)
		}
		return buf
	}

	for _, block := range patternBlocks {
		buf = append(buf, block...)
	}
	for i, block := range instrumentBlocks {
		buf = append(buf, block...)
		buf = append(buf, sampleDataBlocks[i]...)
	}

	return buf
}

func buildXMPattern(pattern XMPattern, channels int) []byte {
	rows := pattern.Rows
	if rows == 0 {
		rows = 1
	}

	cells := make([]XMEvent, int(rows)*channels)
	for _, event := range pattern.Events {
		if event.Row < 0 || event.Row >= int(rows) || event.Channel < 0 || event.Channel >= channels {
			continue
		}
		cells[event.Row*channels+event.Channel] = event
	}

	stream := make([]byte, 0, len(cells)*2)
	for _, event := range cells {
		mask := byte(0x80)
		if event.Note != 0 {
			mask |= 0x01
		}
		if event.Instrument != 0 {
			mask |= 0x02
		}
		if event.Volume != 0 {
			mask |= 0x04
		}
		if event.Effect != 0 {
			mask |= 0x08
		}
		if event.Data != 0 {
			mask |= 0x10
		}
		stream = append(stream, mask)
		if mask&0x01 != 0 {
			stream = append(stream, event.Note)
		}
		if mask&0x02 != 0 {
			stream = append(stream, event.Instrument)
		}
		if mask&0x04 != 0 {
			stream = append(stream, event.Volume)
		}
		if mask&0x08 != 0 {
			stream = append(stream, event.Effect)
		}
		if mask&0x10 != 0 {
			stream = append(stream, event.Data)
		}
	}

	block := make([]byte, 9+len(stream))
	putLE32(block[0:4], 9)
	block[4] = 0
	putLE16(block[5:7], rows)
	putLE16(block[7:9], uint16(len(stream)))
	copy(block[9:], stream)
	return block
}

func buildXMInstrument(instrument XMInstrument) ([]byte, []byte) {
	if len(instrument.Samples) == 0 {
		block := make([]byte, 29)
		putLE32(block[0:4], 29)
		copyPadded(block[4:26], instrument.Name)
		return block, nil
	}

	size := 29 + 4 + 96 + 48 + 48 + 16
	block := make([]byte, size+len(instrument.Samples)*40)
	putLE32(block[0:4], uint32(size))
	copyPadded(block[4:26], instrument.Name)
	putLE16(block[27:29], uint16(len(instrument.Samples)))
	putLE32(block[29:33], 40)

	sampleMap := instrument.SampleMap
	if len(sampleMap) == 0 {
		sampleMap = make([]byte, 96)
	}
	copy(block[33:33+96], sampleMap)

	envOffset := 129
	writeXMEnvelope(block[envOffset:envOffset+48], instrument.VolumeEnvelope)
	envOffset += 48
	writeXMEnvelope(block[envOffset:envOffset+48], instrument.PanningEnvelope)
	envOffset += 48

	block[envOffset] = instrument.VolumePoints
	block[envOffset+1] = instrument.PanningPoints
	block[envOffset+2] = instrument.VolumeSustain
	block[envOffset+3] = instrument.VolumeLoopStart
	block[envOffset+4] = instrument.VolumeLoopEnd
	block[envOffset+5] = instrument.PanningSustain
	block[envOffset+6] = instrument.PanningLoopStart
	block[envOffset+7] = instrument.PanningLoopEnd
	block[envOffset+8] = instrument.VolumeFlags
	block[envOffset+9] = instrument.PanningFlags
	block[envOffset+10] = instrument.VibratoType
	block[envOffset+11] = instrument.VibratoSweep
	block[envOffset+12] = instrument.VibratoDepth
	block[envOffset+13] = instrument.VibratoRate
	putLE16(block[envOffset+14:envOffset+16], instrument.FadeOut)

	data := make([]byte, 0)
	cursor := size
	for _, sample := range instrument.Samples {
		header := block[cursor : cursor+40]
		putLE32(header[0:4], sample.Length)
		putLE32(header[4:8], sample.LoopStart)
		putLE32(header[8:12], sample.LoopLength)
		header[12] = sample.Volume
		header[13] = byte(sample.FineTune)
		header[14] = sample.Type
		header[15] = sample.Panning
		header[16] = byte(sample.RelativeNote)
		header[17] = sample.Reserved
		copyPadded(header[18:40], sample.Name)
		cursor += 40
		data = append(data, sample.Data...)
	}

	return block, data
}

func writeXMEnvelope(dst []byte, points []XMEnvelopePoint) {
	for i := 0; i < len(points) && i < 12; i++ {
		putLE16(dst[i*4:i*4+2], points[i].Tick)
		putLE16(dst[i*4+2:i*4+4], points[i].Value)
	}
}

func buildITFixture(opts ITFixtureOptions) []byte {
	if opts.Title == "" {
		opts.Title = "IT Fixture"
	}
	if opts.CreatedWith == 0 {
		opts.CreatedWith = 0x0216
	}
	if opts.CompatibleWith == 0 {
		opts.CompatibleWith = 0x0213
	}
	if opts.GlobalVolume == 0 {
		opts.GlobalVolume = 128
	}
	if opts.InitialSpeed == 0 {
		opts.InitialSpeed = 6
	}
	if opts.InitialTempo == 0 {
		opts.InitialTempo = 125
	}
	if len(opts.Orders) == 0 {
		opts.Orders = []byte{0}
	}

	header := make([]byte, 192)
	copy(header[0:4], []byte("IMPM"))
	copyPadded(header[4:30], opts.Title)
	putLE16(header[32:34], uint16(len(opts.Orders)))
	putLE16(header[34:36], uint16(len(opts.Instruments)))
	putLE16(header[36:38], uint16(len(opts.Samples)))
	putLE16(header[38:40], uint16(len(opts.Patterns)))
	putLE16(header[40:42], opts.CreatedWith)
	putLE16(header[42:44], opts.CompatibleWith)
	putLE16(header[44:46], opts.Flags)
	special := opts.Special
	if len(opts.Message) != 0 {
		special |= 0x0001
	}
	putLE16(header[46:48], special)
	header[48] = opts.GlobalVolume
	header[49] = 64
	header[50] = opts.InitialSpeed
	header[51] = opts.InitialTempo
	header[52] = 128

	for i := 0; i < 64; i++ {
		if i < len(opts.ChannelPanning) {
			header[64+i] = opts.ChannelPanning[i]
		} else {
			header[64+i] = 32
		}
		if i < len(opts.ChannelVolume) {
			header[128+i] = opts.ChannelVolume[i]
		} else {
			header[128+i] = 64
		}
	}

	buf := append([]byte(nil), header...)
	buf = append(buf, opts.Orders...)

	parapointerCount := len(opts.Instruments) + len(opts.Samples) + len(opts.Patterns)
	parapointerOffset := len(buf)
	buf = append(buf, make([]byte, parapointerCount*4)...)

	instrumentOffsets := make([]int, len(opts.Instruments))
	for i, instrument := range opts.Instruments {
		instrumentOffsets[i] = len(buf)
		buf = append(buf, buildITInstrument(instrument, len(opts.Samples))...)
	}

	sampleHeaderOffsets := make([]int, len(opts.Samples))
	for i, sample := range opts.Samples {
		sampleHeaderOffsets[i] = len(buf)
		buf = append(buf, buildITSampleHeader(sample)...)
	}

	patternOffsets := make([]int, len(opts.Patterns))
	for i, pattern := range opts.Patterns {
		patternOffsets[i] = len(buf)
		buf = append(buf, buildITPattern(pattern)...)
	}

	for i, sample := range opts.Samples {
		dataOffset := len(buf)
		putLE32(buf[sampleHeaderOffsets[i]+72:sampleHeaderOffsets[i]+76], uint32(dataOffset))
		buf = append(buf, sample.Data...)
	}

	if len(opts.Message) != 0 {
		messageOffset := len(buf)
		putLE16(buf[54:56], uint16(len(opts.Message)))
		putLE32(buf[56:60], uint32(messageOffset))
		buf = append(buf, []byte(opts.Message)...)
	}

	index := parapointerOffset
	for _, offset := range instrumentOffsets {
		putLE32(buf[index:index+4], uint32(offset))
		index += 4
	}
	for _, offset := range sampleHeaderOffsets {
		putLE32(buf[index:index+4], uint32(offset))
		index += 4
	}
	for _, offset := range patternOffsets {
		putLE32(buf[index:index+4], uint32(offset))
		index += 4
	}

	return buf
}

func buildITInstrument(instrument ITInstrument, sampleCount int) []byte {
	block := make([]byte, 550)
	copy(block[0:4], []byte("IMPI"))
	cursor := 17
	block[cursor] = instrument.NNA
	cursor++
	block[cursor] = instrument.DCT
	cursor++
	block[cursor] = instrument.DCA
	cursor++
	putLE16(block[cursor:cursor+2], instrument.FadeOut)
	cursor += 2
	block[cursor] = instrument.PitchPanSeparation
	cursor++
	block[cursor] = instrument.PitchPanCenter
	cursor++
	block[cursor] = instrument.GlobalVolume
	cursor++
	block[cursor] = instrument.ChannelPanning
	cursor++
	block[cursor] = instrument.RandomVolume
	cursor++
	block[cursor] = instrument.RandomPanning
	cursor++
	putLE16(block[cursor:cursor+2], 0x0216)
	cursor += 2
	if sampleCount > 0 {
		block[cursor] = byte(sampleCount)
	}
	cursor += 2
	copyPadded(block[cursor:cursor+26], instrument.Name)
	cursor += 26
	cursor += 6

	sampleMap := instrument.SampleMap
	if len(sampleMap) == 0 {
		sampleMap = make([]uint16, 120)
		for i := 0; i < 120; i++ {
			sampleMap[i] = uint16((1 << 8) | i)
		}
	}
	for i := 0; i < 120; i++ {
		value := uint16(0)
		if i < len(sampleMap) {
			value = sampleMap[i]
		}
		putLE16(block[cursor:cursor+2], value)
		cursor += 2
	}

	cursor = writeITEnvelope(block, cursor, instrument.VolumeFlags, instrument.VolumePoints, instrument.VolumeLoopStart, instrument.VolumeLoopEnd, instrument.VolumeSustainStart, instrument.VolumeSustainEnd, instrument.VolumeEnvelope, false)
	cursor = writeITEnvelope(block, cursor, instrument.PanningFlags, instrument.PanningPoints, instrument.PanningLoopStart, instrument.PanningLoopEnd, instrument.PanningSustainStart, instrument.PanningSustainEnd, instrument.PanningEnvelope, true)
	writeITEnvelope(block, cursor, instrument.PitchFlags, instrument.PitchPoints, instrument.PitchLoopStart, instrument.PitchLoopEnd, instrument.PitchSustainStart, instrument.PitchSustainEnd, instrument.PitchEnvelope, true)

	return block
}

func writeITEnvelope(dst []byte, offset int, flags, points, loopStart, loopEnd, susStart, susEnd byte, env []ITEnvelopePoint, signed bool) int {
	dst[offset] = flags
	dst[offset+1] = points
	dst[offset+2] = loopStart
	dst[offset+3] = loopEnd
	dst[offset+4] = susStart
	dst[offset+5] = susEnd
	offset += 6
	for i := 0; i < 25; i++ {
		var point ITEnvelopePoint
		if i < len(env) {
			point = env[i]
		}
		if signed {
			dst[offset] = byte(int8(point.Value))
		} else {
			dst[offset] = byte(point.Value)
		}
		putLE16(dst[offset+1:offset+3], point.Tick)
		offset += 3
	}
	offset++
	return offset
}

func buildITSampleHeader(sample ITSample) []byte {
	block := make([]byte, 80)
	copy(block[0:4], []byte("IMPS"))
	block[17] = sample.GlobalVolume
	block[18] = sample.Flags
	block[19] = sample.Volume
	copyPadded(block[20:46], sample.Name)
	block[46] = sample.Convert
	block[47] = sample.Panning
	putLE32(block[48:52], sample.Length)
	putLE32(block[52:56], sample.LoopStart)
	putLE32(block[56:60], sample.LoopEnd)
	putLE32(block[60:64], sample.C5Speed)
	putLE32(block[64:68], sample.SustainStart)
	putLE32(block[68:72], sample.SustainEnd)
	block[76] = sample.VibratoSpeed
	block[77] = sample.VibratoDepth
	block[78] = sample.VibratoRate
	block[79] = sample.VibratoWave
	return block
}

func buildITPattern(pattern ITPattern) []byte {
	rows := pattern.Rows
	if rows == 0 {
		rows = 1
	}
	eventsByRow := make([][]ITEvent, rows)
	for _, event := range pattern.Events {
		if event.Row < 0 || event.Row >= int(rows) || event.Channel < 0 || event.Channel >= 64 {
			continue
		}
		eventsByRow[event.Row] = append(eventsByRow[event.Row], event)
	}

	stream := make([]byte, 0, 64)
	for row := 0; row < int(rows); row++ {
		for _, event := range eventsByRow[row] {
			stream = append(stream, byte(event.Channel+1)|0x80, 0x0f)
			stream = append(stream, event.Note, event.Instrument, event.VolPan, event.Command, event.Info)
		}
		stream = append(stream, 0)
	}

	block := make([]byte, 8+len(stream))
	putLE16(block[0:2], uint16(len(stream)))
	putLE16(block[2:4], rows)
	copy(block[8:], stream)
	return block
}
