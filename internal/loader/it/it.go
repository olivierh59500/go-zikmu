package it

import (
	"fmt"
	"io"
	"strings"

	"github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/pitch"
	"github.com/olivierh59500/go-zikmu/internal/sampledecode"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

const (
	itHeaderSize       = 192
	itFilterCut        = 0x80
	itFilterResonant   = 0x81
	itInstrumentNotes  = 120
	itEnvelopePoints   = 25
	itMaxChannels      = 64
	itPatternHeaderLen = 8
)

type header struct {
	Title           string
	VoiceLimit      byte
	OrderCount      uint16
	InstrumentCount uint16
	SampleCount     uint16
	PatternCount    uint16
	CreatedWith     uint16
	CompatibleWith  uint16
	Flags           uint16
	Special         uint16
	GlobalVolume    byte
	InitialSpeed    byte
	InitialTempo    byte
	MessageLength   uint16
	MessageOffset   uint32
	PanTable        [itMaxChannels]byte
	VolTable        [itMaxChannels]byte
}

type sampleHeader struct {
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
	DataOffset   uint32
	VibratoSpeed byte
	VibratoDepth byte
	VibratoRate  byte
	VibratoWave  byte
}

type instrumentHeader struct {
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
	SampleTable         [itInstrumentNotes]uint16
	VolumeNodes         [itEnvelopePoints]byte
	VolumeTicks         [itEnvelopePoints]uint16
	PanningNodes        [itEnvelopePoints]int8
	PanningTicks        [itEnvelopePoints]uint16
	PitchNodes          [itEnvelopePoints]int8
	PitchTicks          [itEnvelopePoints]uint16
	OldVolumeFlags      byte
	OldVolumeLoopStart  byte
	OldVolumeLoopEnd    byte
	OldVolumeSusStart   byte
	OldVolumeSusEnd     byte
	OldFadeOut          uint16
	OldDNC              byte
	OldVolumeTicks      [itEnvelopePoints]byte
	OldVolumeNodes      [itEnvelopePoints]byte
}

type note struct {
	Note       uint8
	Instrument uint8
	VolPan     uint8
	Command    uint8
	Info       uint8
}

func Load(r io.ReaderAt, size int64) (*module.Module, error) {
	if r == nil {
		return nil, fmt.Errorf("it: nil reader")
	}
	if size < itHeaderSize {
		return nil, fmt.Errorf("it: file too short: %d", size)
	}

	data := make([]byte, size)
	section := io.NewSectionReader(r, 0, size)
	if _, err := io.ReadFull(section, data); err != nil {
		return nil, fmt.Errorf("it: read module: %w", err)
	}

	hdr, err := parseHeader(data)
	if err != nil {
		return nil, err
	}

	orderOffset := itHeaderSize
	if orderOffset+int(hdr.OrderCount) > len(data) {
		return nil, fmt.Errorf("it: truncated order table")
	}

	origOrders := append([]byte(nil), data[orderOffset:orderOffset+int(hdr.OrderCount)]...)
	for i := range origOrders {
		if origOrders[i] > byte(hdr.PatternCount) && origOrders[i] < 254 {
			origOrders[i] = 255
		}
	}
	orders, poslookup := createOrders(origOrders, false)
	_, curiousLookup := createOrders(origOrders, true)
	resolver := func(order uint8) (uint8, bool) {
		index := int(order)
		if index >= len(poslookup) {
			return 0, false
		}
		if poslookup[index] >= 0 {
			return uint8(poslookup[index]), true
		}
		if index < len(origOrders) && origOrders[index] != 255 && curiousLookup[index] >= 0 {
			return uint8(curiousLookup[index]), true
		}
		return 0, false
	}

	parapointerOffset := orderOffset + int(hdr.OrderCount)
	parapointerCount := int(hdr.InstrumentCount + hdr.SampleCount + hdr.PatternCount)
	if parapointerOffset+parapointerCount*4 > len(data) {
		return nil, fmt.Errorf("it: truncated parapointer table")
	}

	parapointers := make([]uint32, parapointerCount)
	for i := range parapointers {
		base := parapointerOffset + i*4
		parapointers[i] = le32(data[base : base+4])
	}

	filterMacros, filterSettings, activeMacro, filterEnabled, midiErr := parseFilterConfiguration(data, parapointerOffset+parapointerCount*4, hdr)
	if midiErr != nil {
		return nil, midiErr
	}

	message := ""
	if hdr.Special&1 != 0 && hdr.CreatedWith >= 0x0104 && hdr.MessageLength != 0 {
		comment, commentErr := readMessage(data, hdr.MessageOffset, hdr.MessageLength)
		if commentErr != nil {
			return nil, commentErr
		}
		message = comment
	}

	linear := hdr.Flags&8 != 0
	samples, sampleAdjust, err := loadSamples(data, hdr, parapointers[int(hdr.InstrumentCount):int(hdr.InstrumentCount+hdr.SampleCount)], linear)
	if err != nil {
		return nil, err
	}

	var instruments []module.Instrument
	if hdr.Flags&4 != 0 {
		instruments, err = loadInstruments(data, hdr, parapointers[:int(hdr.InstrumentCount)], samples, sampleAdjust)
		if err != nil {
			return nil, err
		}
	} else {
		instruments = buildSampleInstruments(samples, sampleAdjust)
	}

	patternPointers := parapointers[int(hdr.InstrumentCount+hdr.SampleCount):]
	rawUsed, patternRows, err := scanUsedChannels(data, patternPointers)
	if err != nil {
		return nil, err
	}
	remap, channelCount := buildRemap(rawUsed)
	if channelCount == 0 {
		channelCount = 1
	}

	patterns, err := loadPatterns(data, patternPointers, patternRows, remap, channelCount, resolver, hdr, filterMacros, filterSettings, activeMacro, filterEnabled)
	if err != nil {
		return nil, err
	}

	mod := module.New()
	mod.Metadata = module.Metadata{
		Title:   hdr.Title,
		Tracker: trackerDescription(hdr.CreatedWith, hdr.CompatibleWith),
		Format:  module.FormatIT,
		Message: message,
	}
	mod.Flags = module.FlagBackgroundSlides | module.FlagArpeggioMemory | module.FlagUsesPanning
	if linear {
		mod.Flags |= module.FlagXMPeriods | module.FlagLinearPeriods
	}
	if hdr.Flags&4 != 0 {
		mod.Flags |= module.FlagUsesInstruments | module.FlagUsesNNA
	} else if linear {
		mod.Flags |= module.FlagUsesInstruments
	}
	mod.Channels = channelCount
	mod.Voices = channelCount
	if hdr.VoiceLimit != 0 {
		limit := int(hdr.VoiceLimit) + 1
		if limit > mod.Voices {
			mod.Voices = limit
		}
	}
	mod.InitialSpeed = hdr.InitialSpeed
	mod.InitialTempo = uint16(hdr.InitialTempo)
	mod.InitialGlobalVolume = hdr.GlobalVolume
	mod.BPMThreshold = 32
	mod.Orders = orders
	mod.Patterns = patterns
	mod.Instruments = instruments
	mod.Samples = samples
	copy(mod.FilterMacros[:], filterMacros[:])
	copyFilterSettings(&mod.FilterSettings, filterSettings)
	mod.ActiveFilterMacro = activeMacro

	applyChannelSettings(mod, hdr, remap)

	if err := mod.Validate(); err != nil {
		return nil, fmt.Errorf("it: validate module: %w", err)
	}

	return mod, nil
}

func parseHeader(data []byte) (header, error) {
	if len(data) < itHeaderSize {
		return header{}, fmt.Errorf("it: truncated header")
	}
	if string(data[:4]) != "IMPM" {
		return header{}, fmt.Errorf("it: invalid header signature")
	}

	rawTitle := append([]byte(nil), data[4:30]...)
	hdr := header{
		Title:           cleanText(string(rawTitle)),
		VoiceLimit:      rawTitle[25],
		OrderCount:      le16(data[32:34]),
		InstrumentCount: le16(data[34:36]),
		SampleCount:     le16(data[36:38]),
		PatternCount:    le16(data[38:40]),
		CreatedWith:     le16(data[40:42]),
		CompatibleWith:  le16(data[42:44]),
		Flags:           le16(data[44:46]),
		Special:         le16(data[46:48]),
		GlobalVolume:    data[48],
		InitialSpeed:    data[50],
		InitialTempo:    data[51],
		MessageLength:   le16(data[54:56]),
		MessageOffset:   le32(data[56:60]),
	}
	copy(hdr.PanTable[:], data[64:128])
	copy(hdr.VolTable[:], data[128:192])

	if hdr.OrderCount > 256 || hdr.InstrumentCount > 255 || hdr.SampleCount > 255 || hdr.PatternCount > 255 {
		return header{}, fmt.Errorf("it: unsupported counts orders=%d instruments=%d samples=%d patterns=%d", hdr.OrderCount, hdr.InstrumentCount, hdr.SampleCount, hdr.PatternCount)
	}

	return hdr, nil
}

func parseFilterConfiguration(data []byte, offset int, hdr header) ([module.MaxFilterMacros]byte, [module.MaxFilters]unitrk.FilterSetting, uint8, bool, error) {
	var macros [module.MaxFilterMacros]byte
	var settings [module.MaxFilters]unitrk.FilterSetting
	active := uint8(0)
	enabled := false

	if hdr.CompatibleWith < 0x0216 {
		return macros, settings, active, enabled, nil
	}

	enabled = true
	if hdr.Special&8 == 0 {
		macros[0] = itFilterCut
		for i := 0x80; i < 0x90; i++ {
			settings[i] = unitrk.FilterSetting{Filter: itFilterResonant, Info: uint8((i & 0x7f) << 3)}
		}
		for i := 0; i < 0x80; i++ {
			settings[i] = unitrk.FilterSetting{Filter: macros[0], Info: uint8(i)}
		}
		return macros, settings, active, enabled, nil
	}

	if offset+2 > len(data) {
		return macros, settings, active, enabled, fmt.Errorf("it: truncated MIDI configuration")
	}
	skip := int(le16(data[offset : offset+2]))
	offset += 2 + 8*skip + 0x120
	if offset+32*16+32*128 > len(data) {
		return macros, settings, active, enabled, fmt.Errorf("it: truncated MIDI macros")
	}

	for i := 0; i < module.MaxFilterMacros; i++ {
		line := normalizeMidiMacro(data[offset : offset+32])
		offset += 32
		if strings.HasPrefix(line, "F0F00") && len(line) > 5 && (line[5] == '0' || line[5] == '1') {
			macros[i] = (line[5] - '0') | 0x80
		}
	}
	for i := 0x80; i < 0x100; i++ {
		line := normalizeMidiMacro(data[offset : offset+32])
		offset += 32
		if strings.HasPrefix(line, "F0F00") && len(line) > 5 && (line[5] == '0' || line[5] == '1') {
			settings[i].Filter = (line[5] - '0') | 0x80
			info := byte(0)
			if len(line) > 6 {
				info = byte(line[6]-'0') << 4
			}
			if len(line) > 7 {
				info |= byte(line[7] - '0')
			}
			settings[i].Info = info
		}
	}
	for i := 0; i < 0x80; i++ {
		settings[i].Filter = macros[0]
		settings[i].Info = uint8(i)
	}

	return macros, settings, active, enabled, nil
}

func normalizeMidiMacro(data []byte) string {
	buf := make([]byte, 0, len(data))
	for _, b := range data {
		switch {
		case b >= '0' && b <= '9':
			buf = append(buf, b)
		case b >= 'A' && b <= 'Z':
			buf = append(buf, b)
		case b >= 'a' && b <= 'z':
			buf = append(buf, b-('a'-'A'))
		}
	}
	return string(buf)
}

func readMessage(data []byte, offset uint32, length uint16) (string, error) {
	start := int(offset)
	end := start + int(length)
	if start < 0 || end > len(data) {
		return "", fmt.Errorf("it: message out of range")
	}
	message := strings.ReplaceAll(string(data[start:end]), "\r", "\n")
	return strings.TrimRight(message, "\x00"), nil
}

func loadSamples(data []byte, hdr header, pointers []uint32, linear bool) ([]module.Sample, []int, error) {
	samples := make([]module.Sample, hdr.SampleCount)
	adjust := make([]int, hdr.SampleCount)

	for i, ptr := range pointers {
		if ptr == 0 {
			continue
		}
		if int(ptr)+80 > len(data) {
			return nil, nil, fmt.Errorf("it: sample %d header out of range", i)
		}
		if string(data[ptr:ptr+4]) != "IMPS" {
			continue
		}

		sh, err := parseSampleHeader(data[ptr:])
		if err != nil {
			return nil, nil, fmt.Errorf("it: sample %d: %w", i, err)
		}
		sample, delta, loadErr := decodeSample(data, sh, hdr, linear)
		if loadErr != nil {
			return nil, nil, fmt.Errorf("it: sample %d: %w", i, loadErr)
		}
		samples[i] = sample
		adjust[i] = delta
	}

	return samples, adjust, nil
}

func parseSampleHeader(data []byte) (sampleHeader, error) {
	cursor := 4
	cursor += 12
	cursor++
	if cursor+3+26+2+4*7+4 > len(data) {
		return sampleHeader{}, fmt.Errorf("truncated sample header")
	}

	sh := sampleHeader{
		GlobalVolume: data[cursor],
	}
	cursor++
	sh.Flags = data[cursor]
	cursor++
	sh.Volume = data[cursor]
	cursor++
	sh.Name = cleanText(string(data[cursor : cursor+26]))
	cursor += 26
	sh.Convert = data[cursor]
	cursor++
	sh.Panning = data[cursor]
	cursor++
	sh.Length = le32(data[cursor : cursor+4])
	cursor += 4
	sh.LoopStart = le32(data[cursor : cursor+4])
	cursor += 4
	sh.LoopEnd = le32(data[cursor : cursor+4])
	cursor += 4
	sh.C5Speed = le32(data[cursor : cursor+4])
	cursor += 4
	sh.SustainStart = le32(data[cursor : cursor+4])
	cursor += 4
	sh.SustainEnd = le32(data[cursor : cursor+4])
	cursor += 4
	sh.DataOffset = le32(data[cursor : cursor+4])
	cursor += 4
	sh.VibratoSpeed = data[cursor]
	sh.VibratoDepth = data[cursor+1]
	sh.VibratoRate = data[cursor+2]
	sh.VibratoWave = data[cursor+3]

	return sh, nil
}

func decodeSample(data []byte, sh sampleHeader, hdr header, linear bool) (module.Sample, int, error) {
	c5speed := sh.C5Speed
	if !linear && c5speed > 0 {
		c5speed /= 2
	}
	sample := module.Sample{
		Name:         sh.Name,
		Panning:      scaleITSamplePanning(sh.Panning),
		C5Speed:      c5speed,
		Volume:       sh.Volume,
		GlobalVolume: sh.GlobalVolume,
		Flags:        0,
		Length:       sh.Length,
		LoopStart:    sh.LoopStart,
		LoopEnd:      sh.LoopEnd,
		SustainStart: sh.SustainStart,
		SustainEnd:   sh.SustainEnd,
		Vibrato: module.AutoVibrato{
			Flags:    module.AutoVibratoIT,
			Waveform: sh.VibratoWave,
			Sweep:    sh.VibratoRate * 2,
			Depth:    sh.VibratoDepth,
			Rate:     sh.VibratoSpeed,
		},
	}

	if sh.Panning&0x80 != 0 {
		sample.Flags |= module.SampleOwnPanning
	}
	if sh.Flags&0x02 != 0 {
		sample.Flags |= module.Sample16Bits
	}
	if sh.Flags&0x08 != 0 && hdr.CreatedWith >= 0x0214 {
		sample.Flags |= module.SampleITPacked
	}
	if sh.Flags&0x10 != 0 {
		sample.Flags |= module.SampleLoop
	}
	if sh.Flags&0x20 != 0 {
		sample.Flags |= module.SampleSustainLoop
	}
	if sh.Flags&0x40 != 0 {
		sample.Flags |= module.SampleBidiLoop
	}
	if sh.Flags&0x80 != 0 {
		sample.Flags |= module.SampleSustainLoop | module.SampleSustainBidiLoop
	}

	if sh.Convert == 0xff {
		sample.Flags |= module.SampleADPCM4 | module.SampleSigned
	} else if hdr.CreatedWith >= 0x0200 {
		if sh.Convert&0x01 != 0 {
			sample.Flags |= module.SampleSigned
		}
		if sh.Convert&0x04 != 0 {
			sample.Flags |= module.SampleDelta
		}
	}

	noteShift := 0
	if linear {
		rel, fine := linearTuning(sh.C5Speed)
		sample.RelativeNote = rel
		sample.FineTune = fine
		noteShift = int(rel)
	}

	if sample.Length == 0 {
		return sample, noteShift, nil
	}
	if int(sh.DataOffset) >= len(data) {
		return sample, noteShift, fmt.Errorf("sample data out of range")
	}

	rawStart := int(sh.DataOffset)
	rawEnd := len(data)
	switch {
	case sample.Flags&module.SampleITPacked != 0:
		rawEnd = len(data)
	case sample.Flags&module.SampleADPCM4 != 0:
		rawEnd = rawStart + 16 + int((sample.Length+1)/2)
	case sample.Flags&module.Sample16Bits != 0:
		rawEnd = rawStart + int(sample.Length)*2
	default:
		rawEnd = rawStart + int(sample.Length)
	}
	if rawEnd > len(data) {
		return sample, noteShift, fmt.Errorf("sample data truncated")
	}

	if err := sampledecode.DecodeIntoSample(&sample, data[rawStart:rawEnd], sampledecode.Options{}); err != nil {
		return sample, noteShift, err
	}

	return sample, noteShift, nil
}

func loadInstruments(data []byte, hdr header, pointers []uint32, samples []module.Sample, sampleAdjust []int) ([]module.Instrument, error) {
	instruments := make([]module.Instrument, hdr.InstrumentCount)
	for i, ptr := range pointers {
		if ptr == 0 || int(ptr)+4 > len(data) {
			return nil, fmt.Errorf("it: instrument %d header out of range", i)
		}
		if string(data[ptr:ptr+4]) != "IMPI" {
			return nil, fmt.Errorf("it: instrument %d missing IMPI signature", i)
		}

		instrument, err := parseInstrument(data[int(ptr):], hdr, samples, sampleAdjust)
		if err != nil {
			return nil, fmt.Errorf("it: instrument %d: %w", i, err)
		}
		instruments[i] = instrument
	}
	return instruments, nil
}

func parseInstrument(data []byte, hdr header, samples []module.Sample, sampleAdjust []int) (module.Instrument, error) {
	if len(data) < 550 {
		return module.Instrument{}, fmt.Errorf("truncated instrument header")
	}

	if hdr.CreatedWith < 0x0200 {
		return parseOldInstrument(data, samples, sampleAdjust)
	}
	return parseModernInstrument(data, hdr, samples, sampleAdjust)
}

func parseModernInstrument(data []byte, hdr header, samples []module.Sample, sampleAdjust []int) (module.Instrument, error) {
	cursor := 4 + 12 + 1
	ih := instrumentHeader{}
	ih.NNA = data[cursor]
	cursor++
	ih.DCT = data[cursor]
	cursor++
	ih.DCA = data[cursor]
	cursor++
	ih.FadeOut = le16(data[cursor : cursor+2])
	cursor += 2
	ih.PitchPanSeparation = data[cursor]
	cursor++
	ih.PitchPanCenter = data[cursor]
	cursor++
	ih.GlobalVolume = data[cursor]
	cursor++
	ih.ChannelPanning = data[cursor]
	cursor++
	ih.RandomVolume = data[cursor]
	cursor++
	ih.RandomPanning = data[cursor]
	cursor++
	cursor += 2 // tracker version
	cursor++    // numsmp
	cursor++    // reserved
	ih.Name = cleanText(string(data[cursor : cursor+26]))
	cursor += 26
	cursor += 6
	for i := 0; i < itInstrumentNotes; i++ {
		ih.SampleTable[i] = le16(data[cursor : cursor+2])
		cursor += 2
	}
	readEnvelopeHeader := func(flags *byte, points *byte, loopStart *byte, loopEnd *byte, susStart *byte, susEnd *byte, nodes []int8, ticks []uint16, signed bool) {
		*flags = data[cursor]
		*points = data[cursor+1]
		if *points > itEnvelopePoints {
			*points = itEnvelopePoints
		}
		*loopStart = data[cursor+2]
		*loopEnd = data[cursor+3]
		*susStart = data[cursor+4]
		*susEnd = data[cursor+5]
		cursor += 6
		for i := 0; i < itEnvelopePoints; i++ {
			if signed {
				nodes[i] = int8(data[cursor])
			} else {
				nodes[i] = int8(data[cursor])
			}
			ticks[i] = le16(data[cursor+1 : cursor+3])
			cursor += 3
		}
		cursor++
	}
	var volNodes [itEnvelopePoints]int8
	readEnvelopeHeader(&ih.VolumeFlags, &ih.VolumePoints, &ih.VolumeLoopStart, &ih.VolumeLoopEnd, &ih.VolumeSustainStart, &ih.VolumeSustainEnd, volNodes[:], ih.VolumeTicks[:], false)
	for i := range volNodes {
		ih.VolumeNodes[i] = byte(volNodes[i])
	}
	readEnvelopeHeader(&ih.PanningFlags, &ih.PanningPoints, &ih.PanningLoopStart, &ih.PanningLoopEnd, &ih.PanningSustainStart, &ih.PanningSustainEnd, ih.PanningNodes[:], ih.PanningTicks[:], true)
	readEnvelopeHeader(&ih.PitchFlags, &ih.PitchPoints, &ih.PitchLoopStart, &ih.PitchLoopEnd, &ih.PitchSustainStart, &ih.PitchSustainEnd, ih.PitchNodes[:], ih.PitchTicks[:], true)

	instrument := defaultInstrument()
	instrument.Name = ih.Name
	instrument.NewNoteAction = module.NNA(ih.NNA & 0x03)
	instrument.DuplicateCheck = module.DCT(ih.DCT)
	instrument.DuplicateAction = module.DCA(ih.DCA)
	instrument.GlobalVolume = ih.GlobalVolume >> 1
	instrument.FadeOut = ih.FadeOut << 5
	instrument.RandomVolumeVar = ih.RandomVolume
	instrument.RandomPanningVar = ih.RandomPanning
	if ih.ChannelPanning&0x80 == 0 {
		instrument.Flags |= module.InstrumentOwnPanning
		instrument.Panning = scaleITSamplePanning(ih.ChannelPanning)
	}
	if ih.PitchPanSeparation&0x80 == 0 {
		instrument.Flags |= module.InstrumentPitchPan
		instrument.PitchPanSeparation = clampByte(int(ih.PitchPanSeparation) * 4)
		instrument.PitchPanCenter = ih.PitchPanCenter
	}
	instrument.VolumeEnvelope = buildITEnvelopeU8(ih.VolumeNodes[:], ih.VolumeTicks[:], ih.VolumePoints, ih.VolumeSustainStart, ih.VolumeSustainEnd, ih.VolumeLoopStart, ih.VolumeLoopEnd, ih.VolumeFlags, true)
	instrument.PanningEnvelope = buildITEnvelopeS8(ih.PanningNodes[:], ih.PanningTicks[:], ih.PanningPoints, ih.PanningSustainStart, ih.PanningSustainEnd, ih.PanningLoopStart, ih.PanningLoopEnd, ih.PanningFlags, false, true)
	if ih.PitchFlags&0x80 == 0 {
		instrument.PitchEnvelope = buildITEnvelopeS8(ih.PitchNodes[:], ih.PitchTicks[:], ih.PitchPoints, ih.PitchSustainStart, ih.PitchSustainEnd, ih.PitchLoopStart, ih.PitchLoopEnd, ih.PitchFlags, false, false)
	}

	buildInstrumentMap(&instrument, ih.SampleTable[:], samples, sampleAdjust, hdr.Flags&8 != 0)
	return instrument, nil
}

func parseOldInstrument(data []byte, samples []module.Sample, sampleAdjust []int) (module.Instrument, error) {
	cursor := 4 + 12 + 1
	ih := instrumentHeader{}
	ih.OldVolumeFlags = data[cursor]
	cursor++
	ih.OldVolumeLoopStart = data[cursor]
	cursor++
	ih.OldVolumeLoopEnd = data[cursor]
	cursor++
	ih.OldVolumeSusStart = data[cursor]
	cursor++
	ih.OldVolumeSusEnd = data[cursor]
	cursor++
	cursor += 2
	ih.OldFadeOut = le16(data[cursor : cursor+2])
	cursor += 2
	ih.NNA = data[cursor]
	cursor++
	ih.OldDNC = data[cursor]
	cursor++
	cursor += 2 // tracker version
	cursor++    // numsmp
	cursor++    // reserved
	ih.Name = cleanText(string(data[cursor : cursor+26]))
	cursor += 26
	cursor += 6
	for i := 0; i < itInstrumentNotes; i++ {
		ih.SampleTable[i] = le16(data[cursor : cursor+2])
		cursor += 2
	}
	cursor += 200
	for i := 0; i < itEnvelopePoints; i++ {
		ih.OldVolumeTicks[i] = data[cursor]
		cursor++
		ih.OldVolumeNodes[i] = data[cursor]
		cursor++
	}

	instrument := defaultInstrument()
	instrument.Name = ih.Name
	instrument.NewNoteAction = module.NNA(ih.NNA & 0x03)
	if ih.OldDNC != 0 {
		instrument.DuplicateCheck = module.DCTNote
		instrument.DuplicateAction = module.DCACut
	}
	instrument.FadeOut = ih.OldFadeOut << 6
	flags := byte(0)
	if ih.OldVolumeFlags&1 != 0 {
		flags |= 1
	}
	if ih.OldVolumeFlags&2 != 0 {
		flags |= 2
	}
	if ih.OldVolumeFlags&4 != 0 {
		flags |= 4
	}
	count := byte(0)
	for count < itEnvelopePoints && ih.OldVolumeTicks[count] != 0xff {
		count++
	}
	nodes := make([]byte, itEnvelopePoints)
	ticks := make([]uint16, itEnvelopePoints)
	for i := 0; i < int(count); i++ {
		nodes[i] = ih.OldVolumeNodes[i]
		ticks[i] = uint16(ih.OldVolumeTicks[i])
	}
	instrument.VolumeEnvelope = buildITEnvelopeU8(nodes, ticks, count, ih.OldVolumeSusStart, ih.OldVolumeSusEnd, ih.OldVolumeLoopStart, ih.OldVolumeLoopEnd, flags, true)
	buildInstrumentMap(&instrument, ih.SampleTable[:], samples, sampleAdjust, false)
	return instrument, nil
}

func buildInstrumentMap(instrument *module.Instrument, table []uint16, samples []module.Sample, sampleAdjust []int, linear bool) {
	if instrument == nil {
		return
	}
	fillInvalidMap(instrument)
	for i := 0; i < len(table) && i < module.InstrumentNotes; i++ {
		raw := table[i]
		note := int(raw & 0xff)
		sampleIndex := int(raw>>8) - 1
		if sampleIndex < 0 || sampleIndex >= len(samples) {
			continue
		}
		if linear && sampleIndex < len(sampleAdjust) {
			note += sampleAdjust[sampleIndex]
		}
		if note < 0 {
			note = 0
		}
		if note > 255 {
			note = 255
		}
		instrument.NoteMap[i] = module.NoteSample{
			Note:   uint8(note),
			Sample: uint16(sampleIndex),
		}
	}
}

func buildSampleInstruments(samples []module.Sample, sampleAdjust []int) []module.Instrument {
	instruments := make([]module.Instrument, len(samples))
	for i, sample := range samples {
		instrument := defaultInstrument()
		instrument.Name = sample.Name
		for note := 0; note < module.InstrumentNotes; note++ {
			shifted := note
			if i < len(sampleAdjust) {
				shifted += sampleAdjust[i]
			}
			if shifted < 0 {
				shifted = 0
			}
			if shifted > 255 {
				shifted = 255
			}
			instrument.NoteMap[note] = module.NoteSample{
				Note:   uint8(shifted),
				Sample: uint16(i),
			}
		}
		instruments[i] = instrument
	}
	return instruments
}

func scanUsedChannels(data []byte, pointers []uint32) ([itMaxChannels]bool, []uint16, error) {
	var used [itMaxChannels]bool
	rows := make([]uint16, len(pointers))
	for i, ptr := range pointers {
		if ptr == 0 {
			rows[i] = 64
			continue
		}
		if int(ptr)+itPatternHeaderLen > len(data) {
			return used, nil, fmt.Errorf("it: pattern %d header out of range", i)
		}

		rowCount := le16(data[ptr+2 : ptr+4])
		if rowCount == 0 || rowCount > 256 {
			return used, nil, fmt.Errorf("it: invalid row count %d in pattern %d", rowCount, i)
		}
		rows[i] = rowCount

		stream := data[ptr+8:]
		mask := [itMaxChannels]byte{}
		last := [itMaxChannels]note{}
		for ch := range last {
			last[ch] = note{Note: 255, Instrument: 255, VolPan: 255, Command: 255}
		}

		row := 0
		cursor := 0
		for row < int(rowCount) {
			if cursor >= len(stream) {
				return used, nil, fmt.Errorf("it: truncated pattern %d data", i)
			}
			flag := stream[cursor]
			cursor++
			if flag == 0 {
				row++
				continue
			}
			channel := int((flag - 1) & 63)
			used[channel] = true
			if flag&0x80 != 0 {
				if cursor >= len(stream) {
					return used, nil, fmt.Errorf("it: truncated pattern %d mask", i)
				}
				mask[channel] = stream[cursor]
				cursor++
			}
			m := mask[channel]
			if m&0x01 != 0 {
				if cursor >= len(stream) {
					return used, nil, fmt.Errorf("it: truncated pattern %d note", i)
				}
				last[channel].Note = decodePatternNote(stream[cursor])
				cursor++
			}
			if m&0x02 != 0 {
				cursor++
			}
			if m&0x04 != 0 {
				cursor++
			}
			if m&0x08 != 0 {
				cursor += 2
			}
			if cursor > len(stream) {
				return used, nil, fmt.Errorf("it: truncated pattern %d event", i)
			}
		}
	}
	return used, rows, nil
}

func buildRemap(used [itMaxChannels]bool) ([itMaxChannels]int, int) {
	remap := [itMaxChannels]int{}
	for i := range remap {
		remap[i] = -1
	}
	count := 0
	for i, ok := range used {
		if ok {
			remap[i] = count
			count++
		}
	}
	return remap, count
}

func loadPatterns(data []byte, pointers []uint32, rows []uint16, remap [itMaxChannels]int, channelCount int, resolver unitrk.OrderResolver, hdr header, filterMacros [module.MaxFilterMacros]byte, filterSettings [module.MaxFilters]unitrk.FilterSetting, activeMacro uint8, filterEnabled bool) ([]module.Pattern, error) {
	patterns := make([]module.Pattern, len(pointers))
	oldStyle := hdr.CreatedWith >= 0x0106 && hdr.Flags&16 != 0

	for i, ptr := range pointers {
		if ptr == 0 {
			patterns[i] = emptyPattern(64, channelCount)
			continue
		}
		rowCount := rows[i]
		stream := data[ptr+8:]
		decoded, err := decodePattern(stream, int(rowCount), remap, channelCount, resolver, oldStyle, filterMacros, filterSettings, activeMacro, filterEnabled)
		if err != nil {
			return nil, fmt.Errorf("it: pattern %d: %w", i, err)
		}
		patterns[i] = module.Pattern{
			Rows:   rowCount,
			Tracks: decoded,
		}
	}

	return patterns, nil
}

func decodePattern(stream []byte, rows int, remap [itMaxChannels]int, channelCount int, resolver unitrk.OrderResolver, oldStyle bool, filterMacros [module.MaxFilterMacros]byte, filterSettings [module.MaxFilters]unitrk.FilterSetting, activeMacro uint8, filterEnabled bool) ([]unitrk.Track, error) {
	events := make([][]note, channelCount)
	for ch := 0; ch < channelCount; ch++ {
		events[ch] = make([]note, rows)
		for row := 0; row < rows; row++ {
			events[ch][row] = note{Note: 255, Instrument: 255, VolPan: 255, Command: 255}
		}
	}

	mask := [itMaxChannels]byte{}
	last := [itMaxChannels]note{}
	for ch := range last {
		last[ch] = note{Note: 255, Instrument: 255, VolPan: 255, Command: 255}
	}

	row := 0
	cursor := 0
	for row < rows {
		if cursor >= len(stream) {
			return nil, fmt.Errorf("truncated pattern data")
		}
		flag := stream[cursor]
		cursor++
		if flag == 0 {
			row++
			continue
		}

		rawChannel := int((flag - 1) & 63)
		if flag&0x80 != 0 {
			if cursor >= len(stream) {
				return nil, fmt.Errorf("truncated pattern mask")
			}
			mask[rawChannel] = stream[cursor]
			cursor++
		}
		m := mask[rawChannel]
		current := note{Note: 255, Instrument: 255, VolPan: 255, Command: 255}

		if m&0x01 != 0 {
			if cursor >= len(stream) {
				return nil, fmt.Errorf("truncated pattern note")
			}
			value := decodePatternNote(stream[cursor])
			last[rawChannel].Note = value
			current.Note = value
			cursor++
		}
		if m&0x02 != 0 {
			if cursor >= len(stream) {
				return nil, fmt.Errorf("truncated pattern instrument")
			}
			last[rawChannel].Instrument = stream[cursor]
			current.Instrument = stream[cursor]
			cursor++
		}
		if m&0x04 != 0 {
			if cursor >= len(stream) {
				return nil, fmt.Errorf("truncated pattern volume column")
			}
			last[rawChannel].VolPan = stream[cursor]
			current.VolPan = stream[cursor]
			cursor++
		}
		if m&0x08 != 0 {
			if cursor+1 >= len(stream) {
				return nil, fmt.Errorf("truncated pattern effect")
			}
			last[rawChannel].Command = stream[cursor]
			last[rawChannel].Info = stream[cursor+1]
			current.Command = stream[cursor]
			current.Info = stream[cursor+1]
			cursor += 2
		}
		if m&0x10 != 0 {
			current.Note = last[rawChannel].Note
		}
		if m&0x20 != 0 {
			current.Instrument = last[rawChannel].Instrument
		}
		if m&0x40 != 0 {
			current.VolPan = last[rawChannel].VolPan
		}
		if m&0x80 != 0 {
			current.Command = last[rawChannel].Command
			current.Info = last[rawChannel].Info
		}

		if track := remap[rawChannel]; track >= 0 {
			events[track][row] = current
		}
	}

	tracks := make([]unitrk.Track, channelCount)
	for ch := 0; ch < channelCount; ch++ {
		var builder unitrk.Builder
		builder.Reset()
		builder.SetArpeggioMemory(true)
		converter := unitrk.S3MITConverter{
			ResolveOrder:   resolver,
			FiltersEnabled: filterEnabled,
			ActiveMacro:    activeMacro,
			FilterMacros:   filterMacros,
			FilterSettings: filterSettings,
		}

		for row := 0; row < rows; row++ {
			event := events[ch][row]
			if err := unitrk.ITEvent(&builder, &converter, event.Note, event.Instrument, event.VolPan, event.Command, event.Info, oldStyle); err != nil {
				return nil, err
			}
			builder.NewLine()
		}
		tracks[ch] = builder.Track()
	}

	return tracks, nil
}

func decodePatternNote(value byte) uint8 {
	if value == 255 {
		return 253
	}
	return value
}

func emptyPattern(rows uint16, channels int) module.Pattern {
	tracks := make([]unitrk.Track, channels)
	for ch := 0; ch < channels; ch++ {
		var builder unitrk.Builder
		builder.Reset()
		for row := 0; row < int(rows); row++ {
			builder.NewLine()
		}
		tracks[ch] = builder.Track()
	}
	return module.Pattern{
		Rows:   rows,
		Tracks: tracks,
	}
}

func applyChannelSettings(mod *module.Module, hdr header, remap [itMaxChannels]int) {
	for raw, track := range remap {
		if track < 0 || track >= module.MaxChannels {
			continue
		}
		mod.ChannelSettings[track].Volume = hdr.VolTable[raw]
		if hdr.Flags&1 != 0 {
			mod.ChannelSettings[track].Panning = module.ImplicitPatternPanning(track)
		} else {
			mod.ChannelSettings[track].Panning = module.PanCenter
		}
	}
}

func scaleITChannelPanning(value byte) (uint16, error) {
	switch {
	case value < 64:
		return uint16(value) << 2, nil
	case value == 64:
		return 255, nil
	case value == 100:
		return module.PanSurround, nil
	case value == 127:
		return module.PanCenter, nil
	default:
		return 0, fmt.Errorf("invalid IT channel panning %d", value)
	}
}

func scaleITSamplePanning(value byte) int16 {
	panning := int16(value & 0x7f)
	if panning == 64 {
		return 255
	}
	return panning << 2
}

func linearTuning(c5speed uint32) (int8, int16) {
	if c5speed == 0 {
		return 0, 0
	}
	target := int(c5speed >> 1)
	ctmp, tmp := 0, 0
	note, fine := 1, 0

	for {
		tmp = linearRawFrequency(note, 0)
		if tmp >= target {
			break
		}
		ctmp = tmp
		note++
	}

	if tmp != target {
		if (tmp - target) < (target - ctmp) {
			for tmp > target {
				fine--
				tmp = linearRawFrequency(note, fine)
			}
		} else {
			note--
			for ctmp < target {
				fine++
				ctmp = linearRawFrequency(note, fine)
			}
		}
	}

	rawPeriod := linearRawPeriod(note, fine)
	const (
		minTotal = -128 * 128
		maxTotal = 127*128 + 127
	)
	low := minTotal
	high := maxTotal
	for high-low > 1 {
		mid := low + (high-low)/2
		if linearCanonicalPeriod(mid) > rawPeriod {
			low = mid
		} else {
			high = mid
		}
	}

	total := high
	if lowDiff := absInt(linearCanonicalPeriod(low) - rawPeriod); lowDiff <= absInt(linearCanonicalPeriod(high)-rawPeriod) {
		total = low
	}
	relativeNote := total / 128
	fineTune := total - relativeNote*128
	if fineTune > 127 {
		fineTune -= 128
		relativeNote++
	}
	if fineTune < -128 {
		fineTune += 128
		relativeNote--
	}
	return int8(clampInt(relativeNote, -128, 127)), int16(fineTune)
}

func linearCanonicalPeriod(total int) int {
	relativeNote := total / 128
	fineTune := total - relativeNote*128
	note := 4*12 + relativeNote
	return pitch.XMPeriod(note, int16(fineTune), true)
}

func linearRawPeriod(note, fine int) int {
	base := ((20+2*2)*12 + 2 - note*2) * 32
	return int(uint16(uint32(base) - (uint32(int32(fine)) >> 1)))
}

func linearRawFrequency(note, fine int) int {
	return pitch.XMFrequencyFromPeriod(linearRawPeriod(note, fine), true)
}

func buildITEnvelopeU8(nodes []byte, ticks []uint16, count, susStart, susEnd, loopStart, loopEnd, flags byte, volume bool) module.Envelope {
	env := module.Envelope{
		SustainStart: susStart,
		SustainEnd:   susEnd,
		LoopStart:    loopStart,
		LoopEnd:      loopEnd,
	}
	if int(count) > itEnvelopePoints {
		count = itEnvelopePoints
	}
	env.Points = make([]module.EnvelopePoint, int(count))
	if flags&1 != 0 {
		env.Flags |= module.EnvelopeEnabled
	}
	if flags&2 != 0 {
		env.Flags |= module.EnvelopeLoop
	}
	if flags&4 != 0 {
		env.Flags |= module.EnvelopeSustain
	}
	if volume {
		env.Flags |= module.EnvelopeVolume
	}
	for i := 0; i < int(count); i++ {
		env.Points[i] = module.EnvelopePoint{Tick: ticks[i], Value: int16(nodes[i]) << 2}
	}
	if env.Flags&module.EnvelopeEnabled != 0 && len(env.Points) < 2 {
		env.Flags &^= module.EnvelopeEnabled
	}
	return env
}

func buildITEnvelopeS8(nodes []int8, ticks []uint16, count, susStart, susEnd, loopStart, loopEnd, flags byte, volume bool, panning bool) module.Envelope {
	env := module.Envelope{
		SustainStart: susStart,
		SustainEnd:   susEnd,
		LoopStart:    loopStart,
		LoopEnd:      loopEnd,
	}
	if int(count) > itEnvelopePoints {
		count = itEnvelopePoints
	}
	env.Points = make([]module.EnvelopePoint, int(count))
	if flags&1 != 0 {
		env.Flags |= module.EnvelopeEnabled
	}
	if flags&2 != 0 {
		env.Flags |= module.EnvelopeLoop
	}
	if flags&4 != 0 {
		env.Flags |= module.EnvelopeSustain
	}
	if volume {
		env.Flags |= module.EnvelopeVolume
	}
	for i := 0; i < int(count); i++ {
		value := int16(nodes[i] + 32)
		if panning {
			if nodes[i] == 32 {
				value = 255
			} else {
				value <<= 2
			}
		}
		env.Points[i] = module.EnvelopePoint{Tick: ticks[i], Value: value}
	}
	if env.Flags&module.EnvelopeEnabled != 0 && len(env.Points) < 2 {
		env.Flags &^= module.EnvelopeEnabled
	}
	return env
}

func trackerDescription(createdWith, compatibleWith uint16) string {
	name := "Impulse Tracker"
	switch {
	case createdWith >= 0x0217 && createdWith <= 0x0219:
		name = "Impulse Tracker 2.14p4"
	case createdWith >= 0x0215:
		name = "Impulse Tracker 2.14p3"
	default:
		name = fmt.Sprintf("Impulse Tracker %d.%02x", createdWith>>8, createdWith&0xff)
	}
	if compatibleWith >= 0x0214 {
		return "Compressed " + name
	}
	return name
}

func createOrders(orig []byte, curious bool) ([]uint16, []int) {
	orders := make([]uint16, 0, len(orig))
	lookup := make([]int, len(orig))
	for i := range lookup {
		lookup[i] = -1
	}
	remainingEnds := 0
	if curious {
		remainingEnds = 1
	}
	for i, raw := range orig {
		order := uint16(raw)
		if raw == 255 {
			order = module.LastPattern
		}
		lookup[i] = len(orders)
		if raw < 254 {
			orders = append(orders, order)
			continue
		}
		if order == module.LastPattern {
			if remainingEnds == 0 {
				break
			}
			remainingEnds--
		}
	}
	return orders, lookup
}

func defaultInstrument() module.Instrument {
	instrument := module.Instrument{
		GlobalVolume: 64,
		Panning:      -1,
	}
	fillInvalidMap(&instrument)
	return instrument
}

func fillInvalidMap(instrument *module.Instrument) {
	if instrument == nil {
		return
	}
	for i := range instrument.NoteMap {
		instrument.NoteMap[i] = module.NoteSample{Note: 255}
	}
}

func copyFilterSettings(dst *[module.MaxFilters]module.FilterSetting, src [module.MaxFilters]unitrk.FilterSetting) {
	for i := range src {
		dst[i] = module.FilterSetting{Filter: src[i].Filter, Info: src[i].Info}
	}
}

func cleanText(value string) string {
	return strings.TrimRight(value, "\x00 ")
}

func clampByte(value int) uint8 {
	return uint8(clampInt(value, 0, 255))
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func le16(data []byte) uint16 {
	return uint16(data[0]) | uint16(data[1])<<8
}

func le32(data []byte) uint32 {
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
}
