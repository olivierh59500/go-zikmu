package s3m

import (
	"fmt"
	"io"
	"strings"

	"github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/sampledecode"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

const (
	s3mHeaderSize   = 0x60
	s3mPatternRows  = 64
	s3mSampleHeader = 80
)

type header struct {
	Title        string
	OrderCount   uint16
	SampleCount  uint16
	PatternCount uint16
	Flags        uint16
	Tracker      uint16
	FileFormat   uint16
	MasterVolume byte
	InitialSpeed byte
	InitialTempo byte
	PanTable     byte
	Channels     [32]byte
}

type sampleHeader struct {
	Type      byte
	Name      string
	MemSegHi  byte
	MemSegLo  uint16
	Length    uint32
	LoopStart uint32
	LoopEnd   uint32
	Volume    byte
	Pack      byte
	Flags     byte
	C2Speed   uint32
	SCRS      string
}

type trackerInfo struct {
	Description string
	Index       int
}

type note struct {
	Note uint8
	Ins  uint8
	Vol  uint8
	Cmd  uint8
	Inf  uint8
}

func Load(r io.ReaderAt, size int64) (*module.Module, error) {
	if r == nil {
		return nil, fmt.Errorf("s3m: nil reader")
	}
	if size < s3mHeaderSize {
		return nil, fmt.Errorf("s3m: file too short: %d", size)
	}

	data := make([]byte, size)
	section := io.NewSectionReader(r, 0, size)
	if _, err := io.ReadFull(section, data); err != nil {
		return nil, fmt.Errorf("s3m: read module: %w", err)
	}

	hdr, err := parseHeader(data)
	if err != nil {
		return nil, err
	}
	if hdr.OrderCount == 0 {
		return nil, fmt.Errorf("s3m: module has no orders")
	}
	if hdr.SampleCount > 255 || hdr.PatternCount > 255 {
		return nil, fmt.Errorf("s3m: unsupported counts: samples=%d patterns=%d", hdr.SampleCount, hdr.PatternCount)
	}

	orderOffset := s3mHeaderSize
	parapointerOffset := orderOffset + int(hdr.OrderCount)
	required := parapointerOffset + int(hdr.SampleCount+hdr.PatternCount)*2
	if hdr.PanTable == 252 {
		required += 32
	}
	if required > len(data) {
		return nil, fmt.Errorf("s3m: truncated parapointer table")
	}

	origOrders := append([]byte(nil), data[orderOffset:orderOffset+int(hdr.OrderCount)]...)
	for i := range origOrders {
		if origOrders[i] >= byte(hdr.PatternCount) && origOrders[i] < 254 {
			origOrders[i] = 255
		}
	}

	samplePtrs := make([]uint16, hdr.SampleCount)
	patternPtrs := make([]uint16, hdr.PatternCount)
	for i := 0; i < int(hdr.SampleCount); i++ {
		base := parapointerOffset + i*2
		samplePtrs[i] = le16(data[base : base+2])
	}
	for i := 0; i < int(hdr.PatternCount); i++ {
		base := parapointerOffset + int(hdr.SampleCount)*2 + i*2
		patternPtrs[i] = le16(data[base : base+2])
	}

	var panTable [32]byte
	if hdr.PanTable == 252 {
		copy(panTable[:], data[required-32:required])
	}

	rawUsed, err := scanUsedChannels(data, patternPtrs, hdr.Channels)
	if err != nil {
		return nil, err
	}
	remap, channelCount := buildRemap(rawUsed)
	if channelCount == 0 {
		for ch, value := range hdr.Channels {
			if value < 32 {
				rawUsed[ch] = true
			}
		}
		remap, channelCount = buildRemap(rawUsed)
	}
	if channelCount == 0 {
		return nil, fmt.Errorf("s3m: module has no playable channels")
	}

	orders, poslookup := createOrders(origOrders, false)
	if len(orders) == 0 {
		return nil, fmt.Errorf("s3m: module has no playable orders")
	}
	curiousOrders, curiousLookup := createOrders(origOrders, true)
	resolver := func(order uint8) (uint8, bool) {
		index := int(order)
		if index >= len(poslookup) {
			return 0, false
		}
		if poslookup[index] >= 0 {
			return uint8(poslookup[index]), true
		}
		if index < len(origOrders) && origOrders[index] != 255 && index < len(curiousLookup) && curiousLookup[index] >= 0 {
			_ = curiousOrders
			return uint8(curiousLookup[index]), true
		}
		return 0, false
	}

	tracker := describeTracker(hdr.Tracker)
	trackFlags := unitrk.S3MITOldStyle
	if tracker.Index == 0 {
		trackFlags |= unitrk.S3MITScreamTracker
	}

	patterns := make([]module.Pattern, hdr.PatternCount)
	for i, ptr := range patternPtrs {
		decoded, err := readPattern(data, ptr, remap)
		if err != nil {
			return nil, fmt.Errorf("s3m: read pattern %d: %w", i, err)
		}

		tracks := make([]unitrk.Track, channelCount)
		for ch := 0; ch < channelCount; ch++ {
			track, convErr := convertTrack(decoded[ch], resolver, trackFlags)
			if convErr != nil {
				return nil, fmt.Errorf("s3m: convert pattern %d track %d: %w", i, ch, convErr)
			}
			tracks[ch] = track
		}
		patterns[i] = module.Pattern{
			Rows:   s3mPatternRows,
			Tracks: tracks,
		}
	}

	mod := module.New()
	mod.Metadata = module.Metadata{
		Title:   cleanText(hdr.Title),
		Tracker: tracker.Description,
		Format:  module.FormatS3M,
	}
	mod.Flags = module.FlagArpeggioMemory | module.FlagUsesPanning
	if hdr.Tracker == 0x1300 || hdr.Flags&0x40 != 0 {
		mod.Flags |= module.FlagUsesS3MSlides
	}
	mod.Channels = channelCount
	mod.Voices = channelCount
	mod.InitialSpeed = hdr.InitialSpeed
	mod.InitialTempo = uint16(hdr.InitialTempo)
	mod.InitialGlobalVolume = clampVolume(int(hdr.MasterVolume&0x7f) * 2)
	mod.BPMThreshold = 32
	mod.Orders = orders
	mod.Patterns = patterns
	mod.Samples = make([]module.Sample, hdr.SampleCount)
	mod.Instruments = make([]module.Instrument, hdr.SampleCount)

	applyChannelPanning(mod, hdr.Channels, panTable, remap, hdr.PanTable == 252)

	for i, ptr := range samplePtrs {
		sample, instrument, err := loadSample(data, hdr, ptr, tracker.Index, i)
		if err != nil {
			return nil, fmt.Errorf("s3m: load sample %d: %w", i, err)
		}
		mod.Samples[i] = sample
		mod.Instruments[i] = instrument
	}

	if err := mod.Validate(); err != nil {
		return nil, fmt.Errorf("s3m: validate module: %w", err)
	}

	return mod, nil
}

func parseHeader(data []byte) (header, error) {
	if len(data) < s3mHeaderSize {
		return header{}, fmt.Errorf("s3m: truncated header")
	}
	if string(data[44:48]) != "SCRM" {
		return header{}, fmt.Errorf("s3m: missing SCRM signature")
	}

	hdr := header{
		Title:        string(data[:28]),
		OrderCount:   le16(data[32:34]),
		SampleCount:  le16(data[34:36]),
		PatternCount: le16(data[36:38]),
		Flags:        le16(data[38:40]),
		Tracker:      le16(data[40:42]),
		FileFormat:   le16(data[42:44]),
		MasterVolume: data[48],
		InitialSpeed: data[49],
		InitialTempo: data[50],
		PanTable:     data[53],
	}
	copy(hdr.Channels[:], data[64:96])
	return hdr, nil
}

func describeTracker(code uint16) trackerInfo {
	version := []string{
		"Screamtracker x.xx",
		"Imago Orpheus x.xx (S3M format)",
		"Impulse Tracker x.xx (S3M format)",
		"Unknown tracker x.xx (S3M format)",
		"Impulse Tracker 2.14p3 (S3M format)",
		"Impulse Tracker 2.14p4 (S3M format)",
	}
	numeric := []int{14, 14, 16, 16}

	index := int(code >> 12)
	if index == 0 || index >= 4 {
		index = 3
	} else if code >= 0x3217 {
		index = 5
	} else if code >= 0x3216 {
		index = 4
	} else {
		index--
	}

	description := version[index]
	if index < len(numeric) {
		buf := []byte(description)
		pos := numeric[index]
		buf[pos] = byte(((code >> 8) & 0x0f) + '0')
		buf[pos+2] = byte(((code >> 4) & 0x0f) + '0')
		buf[pos+3] = byte((code & 0x0f) + '0')
		description = string(buf)
	}

	return trackerInfo{
		Description: description,
		Index:       index,
	}
}

func scanUsedChannels(data []byte, patternPtrs []uint16, channelSettings [32]byte) ([32]bool, error) {
	var used [32]bool
	for i, ptr := range patternPtrs {
		stream, err := patternStream(data, ptr)
		if err != nil {
			return used, fmt.Errorf("pattern %d: %w", i, err)
		}

		row := 0
		offset := 0
		for row < s3mPatternRows {
			if offset >= len(stream) {
				return used, fmt.Errorf("pattern %d: truncated packed data", i)
			}

			flag := stream[offset]
			offset++
			if flag == 0 {
				row++
				continue
			}

			channel := int(flag & 31)
			if channel < len(channelSettings) && channelSettings[channel] < 32 {
				used[channel] = true
			}

			advance := 0
			if flag&0x20 != 0 {
				advance += 2
			}
			if flag&0x40 != 0 {
				advance++
			}
			if flag&0x80 != 0 {
				advance += 2
			}
			if offset+advance > len(stream) {
				return used, fmt.Errorf("pattern %d: truncated packed event", i)
			}
			offset += advance
		}
	}
	return used, nil
}

func buildRemap(used [32]bool) ([32]int, int) {
	remap := [32]int{}
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

func readPattern(data []byte, ptr uint16, remap [32]int) ([][]note, error) {
	tracks := make([][]note, 0, 32)
	maxTrack := -1
	for _, track := range remap {
		if track > maxTrack {
			maxTrack = track
		}
	}
	if maxTrack < 0 {
		return nil, fmt.Errorf("no remapped channels")
	}
	tracks = make([][]note, maxTrack+1)
	for i := range tracks {
		tracks[i] = make([]note, s3mPatternRows)
		for row := range tracks[i] {
			tracks[i][row] = note{Note: 255, Ins: 255, Vol: 255, Cmd: 255, Inf: 0}
		}
	}

	stream, err := patternStream(data, ptr)
	if err != nil {
		return nil, err
	}

	row := 0
	offset := 0
	for row < s3mPatternRows {
		if offset >= len(stream) {
			return nil, fmt.Errorf("truncated packed data")
		}

		flag := stream[offset]
		offset++
		if flag == 0 {
			row++
			continue
		}

		channel := int(flag & 31)
		trackIndex := -1
		if channel < len(remap) {
			trackIndex = remap[channel]
		}
		target := note{}
		if trackIndex >= 0 {
			target = tracks[trackIndex][row]
		} else {
			target = note{Note: 255, Ins: 255, Vol: 255, Cmd: 255, Inf: 0}
		}

		if flag&0x20 != 0 {
			if offset+2 > len(stream) {
				return nil, fmt.Errorf("truncated note data")
			}
			target.Note = stream[offset]
			target.Ins = stream[offset+1]
			offset += 2
		}
		if flag&0x40 != 0 {
			if offset >= len(stream) {
				return nil, fmt.Errorf("truncated volume data")
			}
			target.Vol = stream[offset]
			if target.Vol > 64 {
				target.Vol = 64
			}
			offset++
		}
		if flag&0x80 != 0 {
			if offset+2 > len(stream) {
				return nil, fmt.Errorf("truncated effect data")
			}
			target.Cmd = stream[offset]
			target.Inf = stream[offset+1]
			offset += 2
		}

		if trackIndex >= 0 {
			tracks[trackIndex][row] = target
		}
	}

	return tracks, nil
}

func patternStream(data []byte, ptr uint16) ([]byte, error) {
	if ptr == 0 {
		return make([]byte, s3mPatternRows), nil
	}

	offset := int(ptr) << 4
	if offset+2 > len(data) {
		return nil, fmt.Errorf("parapointer out of range")
	}

	length := int(le16(data[offset : offset+2]))
	end := offset + 2 + length
	if length <= 0 || end > len(data) {
		end = len(data)
	}
	return data[offset+2 : end], nil
}

func convertTrack(rows []note, resolver unitrk.OrderResolver, flags unitrk.S3MITFlags) (unitrk.Track, error) {
	var builder unitrk.Builder
	builder.Reset()
	builder.SetArpeggioMemory(true)

	converter := unitrk.S3MITConverter{
		ResolveOrder: resolver,
	}

	for _, row := range rows {
		vol := row.Vol
		if row.Ins != 0 && row.Ins != 255 {
			builder.Instrument(uint16(row.Ins - 1))
		}
		if row.Note != 255 {
			if row.Note == 254 {
				builder.PTEffect(0x0c, 0)
				vol = 255
			} else {
				builder.Note(uint8((row.Note>>4)*12) + (row.Note & 0x0f))
			}
		}
		if vol < 255 {
			builder.PTEffect(0x0c, vol)
		}
		converter.Process(&builder, row.Cmd, row.Inf, flags)
		builder.NewLine()
	}

	return builder.Track(), nil
}

func applyChannelPanning(mod *module.Module, channels [32]byte, panTable [32]byte, remap [32]int, hasPanTable bool) {
	for raw, track := range remap {
		if track < 0 || raw >= len(channels) || channels[raw] >= 32 {
			continue
		}

		panning := uint16(0xc0)
		if channels[raw] < 8 {
			panning = 0x30
		}
		if hasPanTable && panTable[raw]&0x20 != 0 {
			panning = uint16(panTable[raw]&0x0f) << 4
		}

		mod.ChannelSettings[track].Panning = panning
		mod.ChannelSettings[track].Volume = 64
	}
}

func loadSample(data []byte, hdr header, ptr uint16, trackerIndex, sampleIndex int) (module.Sample, module.Instrument, error) {
	sample := module.Sample{
		GlobalVolume: 64,
		Panning:      int16(module.PanCenter),
		Volume:       64,
	}
	instrument := defaultInstrument(sampleIndex)

	if ptr == 0 {
		return sample, instrument, nil
	}

	offset := int(ptr) << 4
	if offset+s3mSampleHeader > len(data) {
		return sample, instrument, fmt.Errorf("sample header out of range")
	}

	sh := parseSampleHeader(data[offset : offset+s3mSampleHeader])
	sample.Name = sh.Name
	sample.Volume = sh.Volume
	if sh.C2Speed != 0 {
		sample.C5Speed = sh.C2Speed
	} else {
		sample.C5Speed = 8363
	}
	instrument.Name = sh.Name

	if sh.Flags&0x01 != 0 {
		sample.Flags |= module.SampleLoop
	}
	if sh.Flags&0x04 != 0 {
		sample.Flags |= module.Sample16Bits
	}
	if hdr.FileFormat == 1 {
		sample.Flags |= module.SampleSigned
	}
	if sh.Pack == 4 {
		sample.Flags &^= module.Sample16Bits
		sample.Flags |= module.SampleADPCM4 | module.SampleSigned
	}

	sample.Length = sh.Length
	sample.LoopStart = sh.LoopStart
	sample.LoopEnd = sh.LoopEnd
	if sample.Flags&module.Sample16Bits != 0 {
		sample.Length /= 2
		sample.LoopStart /= 2
		sample.LoopEnd /= 2
	}

	if trackerIndex == 0 && sh.Length > 64000 {
		sample.Length = 64000
		if sample.Flags&module.Sample16Bits != 0 {
			sample.Length /= 2
		}
	}

	if sh.SCRS != "SCRS" || sh.Type != 1 || sample.Length == 0 {
		sample.Length = 0
		sample.LoopStart = 0
		sample.LoopEnd = 0
		sample.Flags &^= module.SampleLoop
		return sample, instrument, nil
	}

	dataOffset := int((uint32(sh.MemSegHi)<<16 | uint32(sh.MemSegLo)) << 4)
	rawBytes := int(sh.Length)
	if sh.Pack == 4 {
		rawBytes = 16 + int((sample.Length+1)/2)
	}
	if trackerIndex == 0 && sh.Length > 64000 {
		rawBytes = 64000
		if sh.Pack == 4 {
			rawBytes = 16 + int((sample.Length+1)/2)
		}
	}
	if dataOffset < 0 || dataOffset+rawBytes > len(data) {
		return sample, instrument, fmt.Errorf("sample data out of range")
	}

	if err := sampledecode.DecodeIntoSample(&sample, data[dataOffset:dataOffset+rawBytes], sampledecode.Options{}); err != nil {
		return sample, instrument, err
	}

	return sample, instrument, nil
}

func parseSampleHeader(data []byte) sampleHeader {
	return sampleHeader{
		Type:      data[0],
		MemSegHi:  data[13],
		MemSegLo:  le16(data[14:16]),
		Length:    le32(data[16:20]),
		LoopStart: le32(data[20:24]),
		LoopEnd:   le32(data[24:28]),
		Volume:    data[28],
		Pack:      data[30],
		Flags:     data[31],
		C2Speed:   le32(data[32:36]),
		Name:      cleanText(string(data[48:76])),
		SCRS:      string(data[76:80]),
	}
}

func defaultInstrument(sampleIndex int) module.Instrument {
	instrument := module.Instrument{
		GlobalVolume: 64,
		Panning:      -1,
	}
	for i := 0; i < module.InstrumentNotes; i++ {
		instrument.NoteMap[i] = module.NoteSample{
			Note:   uint8(i),
			Sample: uint16(sampleIndex),
		}
	}
	return instrument
}

func cleanText(value string) string {
	return strings.TrimRight(value, "\x00 ")
}

func clampVolume(value int) uint8 {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}

func le16(data []byte) uint16 {
	return uint16(data[0]) | uint16(data[1])<<8
}

func le32(data []byte) uint32 {
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
}
