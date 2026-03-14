package xm

import (
	"fmt"
	"io"
	"strings"

	"github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/sampledecode"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

const (
	xmHeaderPrefix     = "Extended Module: "
	xmHeaderBaseSize   = 60
	xmMinHeaderSize    = 20
	xmMaxOrderCount    = 256
	xmNoteCount        = 8 * 12
	xmEnvelopePoints   = 12
	xmPatternHeader102 = 8
	xmPatternHeader104 = 9
	xmSampleHeaderSize = 40
)

type header struct {
	Title           string
	TrackerName     string
	Version         uint16
	HeaderSize      uint32
	SongLength      uint16
	RestartPosition uint16
	Channels        uint16
	PatternCount    uint16
	InstrumentCount uint16
	Flags           uint16
	Tempo           uint16
	BPM             uint16
	Orders          []byte
}

type note struct {
	Note uint8
	Ins  uint8
	Vol  uint8
	Eff  uint8
	Dat  uint8
}

type patchHeader struct {
	SampleNumbers  [xmNoteCount]byte
	VolumeEnv      [xmEnvelopePoints * 2]uint16
	PanningEnv     [xmEnvelopePoints * 2]uint16
	VolumePoints   byte
	PanningPoints  byte
	VolumeSustain  byte
	VolumeBegin    byte
	VolumeEnd      byte
	PanningSustain byte
	PanningBegin   byte
	PanningEnd     byte
	VolumeFlags    byte
	PanningFlags   byte
	VibratoType    byte
	VibratoSweep   byte
	VibratoDepth   byte
	VibratoRate    byte
	FadeOut        uint16
}

type sampleHeader struct {
	Name       string
	Length     uint32
	LoopStart  uint32
	LoopLength uint32
	Volume     uint8
	FineTune   int8
	Type       uint8
	Panning    uint8
	Relative   int8
	Reserved   uint8
	Vibrato    module.AutoVibrato
}

type pendingSample struct {
	Header     sampleHeader
	DataOffset int64
}

func Load(r io.ReaderAt, size int64) (*module.Module, error) {
	if r == nil {
		return nil, fmt.Errorf("xm: nil reader")
	}
	if size < xmHeaderBaseSize+xmMinHeaderSize {
		return nil, fmt.Errorf("xm: file too short: %d", size)
	}

	data := make([]byte, size)
	section := io.NewSectionReader(r, 0, size)
	if _, err := io.ReadFull(section, data); err != nil {
		return nil, fmt.Errorf("xm: read module: %w", err)
	}

	hdr, err := parseHeader(data)
	if err != nil {
		return nil, err
	}

	headerEnd := int64(xmHeaderBaseSize + hdr.HeaderSize)
	if headerEnd > int64(len(data)) {
		return nil, fmt.Errorf("xm: truncated module header")
	}

	orders, dummyPattern := buildOrders(hdr.Orders, hdr.PatternCount)

	var (
		patterns    []module.Pattern
		samples     []pendingSample
		instruments []module.Instrument
		offset      int64
	)

	if hdr.Version < 0x0104 {
		instruments, samples, offset, err = parseInstrumentsLegacy(data, headerEnd, hdr.InstrumentCount)
		if err != nil {
			return nil, err
		}

		patterns, offset, err = parsePatterns(data, offset, hdr.Version, hdr.PatternCount, int(hdr.Channels), dummyPattern)
		if err != nil {
			return nil, err
		}

		base := offset
		for i := range samples {
			samples[i].DataOffset += base
		}
	} else {
		patterns, offset, err = parsePatterns(data, headerEnd, hdr.Version, hdr.PatternCount, int(hdr.Channels), dummyPattern)
		if err != nil {
			return nil, err
		}

		instruments, samples, offset, err = parseInstruments104(data, offset, hdr.InstrumentCount)
		if err != nil {
			return nil, err
		}
	}

	decodedSamples := make([]module.Sample, len(samples))
	for i, pending := range samples {
		sample, sampleErr := decodeSample(data, pending)
		if sampleErr != nil {
			return nil, fmt.Errorf("xm: load sample %d: %w", i, sampleErr)
		}
		decodedSamples[i] = sample
	}

	mod := module.New()
	mod.Metadata = module.Metadata{
		Title:   hdr.Title,
		Tracker: formatTrackerName(hdr.TrackerName, hdr.Version),
		Format:  module.FormatXM,
	}
	mod.Flags = module.FlagXMPeriods | module.FlagUsesInstruments | module.FlagNoWrapPatternBreak | module.FlagFT2Quirks | module.FlagUsesPanning
	if hdr.Flags&1 != 0 {
		mod.Flags |= module.FlagLinearPeriods
	}
	mod.Channels = int(hdr.Channels)
	mod.Voices = int(hdr.Channels)
	if hdr.RestartPosition < hdr.SongLength {
		mod.RestartPosition = hdr.RestartPosition
	}
	mod.InitialSpeed = uint8(hdr.Tempo)
	mod.InitialTempo = hdr.BPM
	mod.BPMThreshold = 32
	mod.Orders = orders
	mod.Patterns = patterns
	mod.Instruments = instruments
	mod.Samples = decodedSamples

	if err := mod.Validate(); err != nil {
		return nil, fmt.Errorf("xm: validate module: %w", err)
	}

	return mod, nil
}

func parseHeader(data []byte) (header, error) {
	if len(data) < xmHeaderBaseSize+xmMinHeaderSize {
		return header{}, fmt.Errorf("xm: truncated header")
	}
	if string(data[:17]) != xmHeaderPrefix || data[37] != 0x1a {
		return header{}, fmt.Errorf("xm: invalid header signature")
	}

	hdr := header{
		Title:           cleanText(string(data[17:37])),
		TrackerName:     cleanText(string(data[38:58])),
		Version:         le16(data[58:60]),
		HeaderSize:      le32(data[60:64]),
		SongLength:      le16(data[64:66]),
		RestartPosition: le16(data[66:68]),
		Channels:        le16(data[68:70]),
		PatternCount:    le16(data[70:72]),
		InstrumentCount: le16(data[72:74]),
		Flags:           le16(data[74:76]),
		Tempo:           le16(data[76:78]),
		BPM:             le16(data[78:80]),
	}

	switch {
	case hdr.Version < 0x0102 || hdr.Version > 0x0104:
		return header{}, fmt.Errorf("xm: unsupported version 0x%04x", hdr.Version)
	case hdr.Channels == 0 || hdr.Channels > module.MaxChannels:
		return header{}, fmt.Errorf("xm: invalid channel count %d", hdr.Channels)
	case hdr.Tempo > 32 || hdr.BPM < 32 || hdr.BPM > 255:
		return header{}, fmt.Errorf("xm: invalid tempo/speed pair speed=%d bpm=%d", hdr.Tempo, hdr.BPM)
	case hdr.SongLength > xmMaxOrderCount:
		return header{}, fmt.Errorf("xm: invalid song length %d", hdr.SongLength)
	case hdr.HeaderSize < xmMinHeaderSize || hdr.HeaderSize > xmMinHeaderSize+xmMaxOrderCount:
		return header{}, fmt.Errorf("xm: invalid header size %d", hdr.HeaderSize)
	}

	orderBase := 80
	orderEnd := orderBase + int(hdr.SongLength)
	if orderEnd > len(data) {
		return header{}, fmt.Errorf("xm: truncated order table")
	}
	hdr.Orders = append([]byte(nil), data[orderBase:orderEnd]...)
	return hdr, nil
}

func buildOrders(raw []byte, patternCount uint16) ([]uint16, bool) {
	orders := make([]uint16, len(raw))
	dummyPattern := false
	for i, order := range raw {
		if order >= byte(patternCount) {
			orders[i] = patternCount
			dummyPattern = true
			continue
		}
		orders[i] = uint16(order)
	}
	return orders, dummyPattern
}

func parsePatterns(data []byte, offset int64, version uint16, patternCount uint16, channels int, dummyPattern bool) ([]module.Pattern, int64, error) {
	patterns := make([]module.Pattern, 0, int(patternCount)+1)
	for i := 0; i < int(patternCount); i++ {
		if offset+9 > int64(len(data)) {
			return nil, offset, fmt.Errorf("xm: truncated pattern %d header", i)
		}

		headerSize := int64(le32(data[offset : offset+4]))
		minHeader := int64(xmPatternHeader104)
		if version == 0x0102 {
			minHeader = xmPatternHeader102
		}
		if headerSize < minHeader {
			return nil, offset, fmt.Errorf("xm: invalid pattern %d header size %d", i, headerSize)
		}

		packing := data[offset+4]
		if packing != 0 {
			return nil, offset, fmt.Errorf("xm: unsupported pattern %d packing type %d", i, packing)
		}

		var rows uint16
		var packedSize uint16
		if version == 0x0102 {
			rows = uint16(data[offset+5]) + 1
			packedSize = le16(data[offset+6 : offset+8])
		} else {
			rows = le16(data[offset+5 : offset+7])
			packedSize = le16(data[offset+7 : offset+9])
		}
		if rows == 0 {
			return nil, offset, fmt.Errorf("xm: pattern %d has zero rows", i)
		}

		headerEnd := offset + headerSize
		dataEnd := headerEnd + int64(packedSize)
		if headerEnd > int64(len(data)) || dataEnd > int64(len(data)) {
			return nil, offset, fmt.Errorf("xm: truncated pattern %d data", i)
		}

		pattern, err := decodePattern(data[headerEnd:dataEnd], rows, channels)
		if err != nil {
			return nil, offset, fmt.Errorf("xm: decode pattern %d: %w", i, err)
		}
		patterns = append(patterns, pattern)
		offset = dataEnd
	}

	if dummyPattern {
		patterns = append(patterns, emptyPattern(64, channels))
	}

	return patterns, offset, nil
}

func decodePattern(data []byte, rows uint16, channels int) (module.Pattern, error) {
	events := make([][]note, channels)
	for ch := 0; ch < channels; ch++ {
		events[ch] = make([]note, rows)
	}

	cursor := 0
	for row := 0; row < int(rows); row++ {
		for ch := 0; ch < channels; ch++ {
			if cursor >= len(data) {
				break
			}

			event, used, err := readNote(data[cursor:])
			if err != nil {
				return module.Pattern{}, err
			}
			cursor += used
			events[ch][row] = event
		}
	}

	tracks := make([]unitrk.Track, channels)
	for ch := 0; ch < channels; ch++ {
		tracks[ch] = convertTrack(events[ch], rows)
	}

	return module.Pattern{
		Rows:   rows,
		Tracks: tracks,
	}, nil
}

func readNote(data []byte) (note, int, error) {
	if len(data) == 0 {
		return note{}, 0, fmt.Errorf("truncated note")
	}

	first := data[0]
	if first&0x80 == 0 {
		if len(data) < 5 {
			return note{}, 0, fmt.Errorf("truncated uncompressed note")
		}
		return note{
			Note: first,
			Ins:  data[1],
			Vol:  data[2],
			Eff:  data[3],
			Dat:  data[4],
		}, 5, nil
	}

	cursor := 1
	event := note{}
	if first&0x01 != 0 {
		if cursor >= len(data) {
			return note{}, 0, fmt.Errorf("truncated note value")
		}
		event.Note = data[cursor]
		cursor++
	}
	if first&0x02 != 0 {
		if cursor >= len(data) {
			return note{}, 0, fmt.Errorf("truncated instrument value")
		}
		event.Ins = data[cursor]
		cursor++
	}
	if first&0x04 != 0 {
		if cursor >= len(data) {
			return note{}, 0, fmt.Errorf("truncated volume value")
		}
		event.Vol = data[cursor]
		cursor++
	}
	if first&0x08 != 0 {
		if cursor >= len(data) {
			return note{}, 0, fmt.Errorf("truncated effect value")
		}
		event.Eff = data[cursor]
		cursor++
	}
	if first&0x10 != 0 {
		if cursor >= len(data) {
			return note{}, 0, fmt.Errorf("truncated effect data")
		}
		event.Dat = data[cursor]
		cursor++
	}

	return event, cursor, nil
}

func convertTrack(events []note, rows uint16) unitrk.Track {
	var builder unitrk.Builder
	builder.Reset()
	for i := 0; i < int(rows); i++ {
		event := note{}
		if i < len(events) {
			event = events[i]
		}
		unitrk.XMEvent(&builder, event.Note, event.Ins, event.Vol, event.Eff, event.Dat)
		builder.NewLine()
	}
	return builder.Track()
}

func emptyPattern(rows uint16, channels int) module.Pattern {
	tracks := make([]unitrk.Track, channels)
	for ch := 0; ch < channels; ch++ {
		tracks[ch] = convertTrack(nil, rows)
	}
	return module.Pattern{
		Rows:   rows,
		Tracks: tracks,
	}
}

func parseInstrumentsLegacy(data []byte, offset int64, count uint16) ([]module.Instrument, []pendingSample, int64, error) {
	instruments := make([]module.Instrument, count)
	samples := make([]pendingSample, 0)
	var relativeDataOffset int64

	for i := 0; i < int(count); i++ {
		instrument, parsed, next, dataSize, err := parseInstrument(data, offset, false, len(samples))
		if err != nil {
			return nil, nil, offset, fmt.Errorf("xm: read instrument %d: %w", i, err)
		}
		instruments[i] = instrument
		for j := range parsed {
			parsed[j].DataOffset = relativeDataOffset
			relativeDataOffset += encodedSize(parsed[j].Header)
		}
		_ = dataSize
		samples = append(samples, parsed...)
		offset = next
	}

	return instruments, samples, offset, nil
}

func parseInstruments104(data []byte, offset int64, count uint16) ([]module.Instrument, []pendingSample, int64, error) {
	instruments := make([]module.Instrument, count)
	samples := make([]pendingSample, 0)

	for i := 0; i < int(count); i++ {
		instrument, parsed, next, dataSize, err := parseInstrument(data, offset, true, len(samples))
		if err != nil {
			return nil, nil, offset, fmt.Errorf("xm: read instrument %d: %w", i, err)
		}
		instruments[i] = instrument
		samples = append(samples, parsed...)
		offset = next + dataSize
	}

	return instruments, samples, offset, nil
}

func parseInstrument(data []byte, offset int64, inlineData bool, baseSample int) (module.Instrument, []pendingSample, int64, int64, error) {
	if offset+29 > int64(len(data)) {
		return module.Instrument{}, nil, offset, 0, fmt.Errorf("truncated instrument header")
	}

	size := int64(le32(data[offset : offset+4]))
	headEnd := offset + size
	if size < 29 || headEnd > int64(len(data)) {
		return module.Instrument{}, nil, offset, 0, fmt.Errorf("invalid instrument header size %d", size)
	}

	instrument := defaultInstrument()
	instrument.Name = cleanText(string(data[offset+4 : offset+26]))
	numSamples := le16(data[offset+27 : offset+29])
	cursor := offset + 29

	var patch patchHeader
	hasPatch := false
	if size > 29 {
		if cursor+4 > int64(len(data)) {
			return module.Instrument{}, nil, offset, 0, fmt.Errorf("truncated sample header size")
		}

		_ = le32(data[cursor : cursor+4])
		cursor += 4

		if numSamples > 0 {
			if numSamples > xmNoteCount {
				return module.Instrument{}, nil, offset, 0, fmt.Errorf("invalid sample count %d", numSamples)
			}
			if cursor+96+96+16 > int64(len(data)) {
				return module.Instrument{}, nil, offset, 0, fmt.Errorf("truncated patch header")
			}

			copy(patch.SampleNumbers[:], data[cursor:cursor+96])
			cursor += 96
			for i := range patch.VolumeEnv {
				patch.VolumeEnv[i] = le16(data[cursor : cursor+2])
				cursor += 2
			}
			for i := range patch.PanningEnv {
				patch.PanningEnv[i] = le16(data[cursor : cursor+2])
				cursor += 2
			}
			patch.VolumePoints = data[cursor]
			patch.PanningPoints = data[cursor+1]
			patch.VolumeSustain = data[cursor+2]
			patch.VolumeBegin = data[cursor+3]
			patch.VolumeEnd = data[cursor+4]
			patch.PanningSustain = data[cursor+5]
			patch.PanningBegin = data[cursor+6]
			patch.PanningEnd = data[cursor+7]
			patch.VolumeFlags = data[cursor+8]
			patch.PanningFlags = data[cursor+9]
			patch.VibratoType = data[cursor+10]
			patch.VibratoSweep = data[cursor+11]
			patch.VibratoDepth = data[cursor+12]
			patch.VibratoRate = data[cursor+13]
			patch.FadeOut = le16(data[cursor+14 : cursor+16])
			cursor += 16
			hasPatch = true
		}
	}

	if headEnd < cursor {
		return module.Instrument{}, nil, offset, 0, fmt.Errorf("instrument header overlaps sample headers")
	}
	cursor = headEnd

	parsed := make([]pendingSample, numSamples)
	var dataSize int64
	if numSamples > 0 {
		if cursor+int64(numSamples)*xmSampleHeaderSize > int64(len(data)) {
			return module.Instrument{}, nil, offset, 0, fmt.Errorf("truncated sample headers")
		}

		for i := 0; i < int(numSamples); i++ {
			parsed[i] = pendingSample{
				Header: parseSampleHeader(data[cursor:cursor+xmSampleHeaderSize], patch),
			}
			cursor += xmSampleHeaderSize
		}

		if inlineData {
			start := cursor
			running := int64(0)
			for i := range parsed {
				parsed[i].DataOffset = start + running
				running += encodedSize(parsed[i].Header)
			}
			dataSize = running
			if start+dataSize > int64(len(data)) {
				return module.Instrument{}, nil, offset, 0, fmt.Errorf("truncated sample data")
			}
		}

		buildInstrumentMap(&instrument, patch, parsed, baseSample)
	} else {
		fillInvalidMap(&instrument)
	}

	if hasPatch {
		instrument.FadeOut = patch.FadeOut
		instrument.VolumeEnvelope = buildEnvelope(patch.VolumeEnv[:], patch.VolumePoints, patch.VolumeSustain, patch.VolumeBegin, patch.VolumeEnd, patch.VolumeFlags, true)
		instrument.PanningEnvelope = buildEnvelope(patch.PanningEnv[:], patch.PanningPoints, patch.PanningSustain, patch.PanningBegin, patch.PanningEnd, patch.PanningFlags, false)
	}

	return instrument, parsed, cursor, dataSize, nil
}

func parseSampleHeader(data []byte, patch patchHeader) sampleHeader {
	return sampleHeader{
		Length:     le32(data[0:4]),
		LoopStart:  le32(data[4:8]),
		LoopLength: le32(data[8:12]),
		Volume:     data[12],
		FineTune:   int8(data[13]),
		Type:       data[14],
		Panning:    data[15],
		Relative:   int8(data[16]),
		Reserved:   data[17],
		Name:       cleanText(string(data[18:40])),
		Vibrato: module.AutoVibrato{
			Waveform: patch.VibratoType,
			Sweep:    patch.VibratoSweep,
			Depth:    patch.VibratoDepth * 4,
			Rate:     patch.VibratoRate,
		},
	}
}

func buildInstrumentMap(instrument *module.Instrument, patch patchHeader, samples []pendingSample, baseSample int) {
	if instrument == nil {
		return
	}

	fillInvalidMap(instrument)
	for i := 0; i < xmNoteCount && i < module.InstrumentNotes; i++ {
		localSample := int(patch.SampleNumbers[i])
		if localSample < 0 || localSample >= len(samples) {
			continue
		}

		note := i + int(samples[localSample].Header.Relative)
		if note < 0 {
			note = 0
		}
		if note > 255 {
			note = 255
		}

		instrument.NoteMap[i] = module.NoteSample{
			Note:   uint8(note),
			Sample: uint16(baseSample + localSample),
		}
	}
}

func buildEnvelope(raw []uint16, count, sustain, loopStart, loopEnd, flags byte, isVolume bool) module.Envelope {
	if int(count) > xmEnvelopePoints {
		count = xmEnvelopePoints
	}

	env := module.Envelope{
		SustainStart: sustain,
		SustainEnd:   sustain,
		LoopStart:    loopStart,
		LoopEnd:      loopEnd,
		Points:       make([]module.EnvelopePoint, int(count)),
	}
	if flags&0x01 != 0 {
		env.Flags |= module.EnvelopeEnabled
	}
	if flags&0x02 != 0 {
		env.Flags |= module.EnvelopeSustain
	}
	if flags&0x04 != 0 {
		env.Flags |= module.EnvelopeLoop
	}
	if isVolume {
		env.Flags |= module.EnvelopeVolume
	}

	for i := 0; i < int(count); i++ {
		env.Points[i] = module.EnvelopePoint{
			Tick:  raw[i*2],
			Value: int16(raw[i*2+1] << 2),
		}
	}
	fixEnvelope(env.Points)

	if env.Flags&module.EnvelopeEnabled != 0 && len(env.Points) < 2 {
		env.Flags &^= module.EnvelopeEnabled
	}

	return env
}

func fixEnvelope(points []module.EnvelopePoint) {
	if len(points) < 2 {
		return
	}

	old := int(points[0].Tick)
	for i := 1; i < len(points); i++ {
		prev := int(points[i-1].Tick)
		cur := int(points[i].Tick)
		if cur < prev {
			if cur < 0x100 {
				var fixed int
				if cur > old {
					fixed = cur + (prev - old)
				} else {
					fixed = cur | ((prev + 0x100) & 0xff00)
				}
				old = cur
				points[i].Tick = uint16(fixed)
				continue
			}
			old = cur
			continue
		}
		old = cur
	}
}

func decodeSample(data []byte, pending pendingSample) (module.Sample, error) {
	header := pending.Header
	sample := module.Sample{
		Name:         header.Name,
		Panning:      int16(header.Panning),
		C5Speed:      8363,
		Volume:       header.Volume,
		GlobalVolume: 64,
		RelativeNote: header.Relative,
		FineTune:     int16(header.FineTune),
		Flags:        module.SampleOwnPanning | module.SampleDelta | module.SampleSigned,
		Length:       header.Length,
		LoopStart:    header.LoopStart,
		LoopEnd:      header.LoopStart + header.LoopLength,
		Vibrato:      header.Vibrato,
	}

	if header.Type&0x03 != 0 {
		sample.Flags |= module.SampleLoop
	}
	if header.Type&0x02 != 0 {
		sample.Flags |= module.SampleBidiLoop
	}
	if header.Type&0x10 != 0 {
		sample.Flags |= module.Sample16Bits
		sample.Length >>= 1
		sample.LoopStart >>= 1
		sample.LoopEnd >>= 1
	}
	if isPackedSample(header) {
		sample.Flags &^= module.SampleDelta
		sample.Flags |= module.SampleADPCM4
	}

	rawSize := encodedSize(header)
	if sample.Length == 0 || rawSize == 0 {
		return sample, nil
	}
	if pending.DataOffset < 0 || pending.DataOffset+rawSize > int64(len(data)) {
		return sample, fmt.Errorf("sample data out of range")
	}

	if err := sampledecode.DecodeIntoSample(&sample, data[pending.DataOffset:pending.DataOffset+rawSize], sampledecode.Options{}); err != nil {
		return sample, err
	}

	return sample, nil
}

func encodedSize(header sampleHeader) int64 {
	if header.Length == 0 {
		return 0
	}
	if isPackedSample(header) {
		return int64(((header.Length + 1) >> 1) + 16)
	}
	return int64(header.Length)
}

func isPackedSample(header sampleHeader) bool {
	return header.Reserved == 0xad && header.Type&0x30 == 0
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

func formatTrackerName(name string, version uint16) string {
	if name == "" {
		name = "Unknown tracker"
	}
	return fmt.Sprintf("%s (XM format %d.%02d)", name, version>>8, version&0xff)
}

func cleanText(value string) string {
	return strings.TrimRight(value, "\x00 ")
}

func le16(data []byte) uint16 {
	return uint16(data[0]) | uint16(data[1])<<8
}

func le32(data []byte) uint32 {
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
}
