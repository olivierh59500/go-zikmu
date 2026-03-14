package mod

import (
	"fmt"
	"io"
	"strings"

	"github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/sampledecode"
	"github.com/olivierh59500/go-zikmu/internal/unitrk"
)

const (
	moduleHeaderSize = 0x438
	rowsPerPattern   = 64
)

type sampleHeader struct {
	Name            string
	LengthWords     uint16
	FineTune        byte
	Volume          byte
	LoopStartWords  uint16
	LoopLengthWords uint16
}

type note struct {
	A byte
	B byte
	C byte
	D byte
}

type trackerInfo struct {
	Description string
	Channels    int
	ModType     int
	Trekker     bool
}

var finetuneTable = [16]uint32{
	8363, 8413, 8463, 8529, 8581, 8651, 8723, 8757,
	7895, 7941, 7985, 8046, 8107, 8169, 8232, 8280,
}

var notePeriodTable = [7 * 12]uint16{
	0x6b0, 0x650, 0x5f4, 0x5a0, 0x54c, 0x500, 0x4b8, 0x474, 0x434, 0x3f8, 0x3c0, 0x38a,
	0x358, 0x328, 0x2fa, 0x2d0, 0x2a6, 0x280, 0x25c, 0x23a, 0x21a, 0x1fc, 0x1e0, 0x1c5,
	0x1ac, 0x194, 0x17d, 0x168, 0x153, 0x140, 0x12e, 0x11d, 0x10d, 0x0fe, 0x0f0, 0x0e2,
	0x0d6, 0x0ca, 0x0be, 0x0b4, 0x0aa, 0x0a0, 0x097, 0x08f, 0x087, 0x07f, 0x078, 0x071,
	0x06b, 0x065, 0x05f, 0x05a, 0x055, 0x050, 0x04b, 0x047, 0x043, 0x03f, 0x03c, 0x038,
	0x035, 0x032, 0x02f, 0x02d, 0x02a, 0x028, 0x025, 0x023, 0x021, 0x01f, 0x01e, 0x01c,
	0x01b, 0x019, 0x018, 0x016, 0x015, 0x014, 0x013, 0x012, 0x011, 0x010, 0x00f, 0x00e,
}

func Load(r io.ReaderAt, size int64) (*module.Module, error) {
	if r == nil {
		return nil, fmt.Errorf("mod: nil reader")
	}
	if size < moduleHeaderSize+4 {
		return nil, fmt.Errorf("mod: file too short: %d", size)
	}

	data := make([]byte, size)
	section := io.NewSectionReader(r, 0, size)
	if _, err := io.ReadFull(section, data); err != nil {
		return nil, fmt.Errorf("mod: read module: %w", err)
	}

	headers, songTitle, songLength, restart, positions, magic, sampleBytes, maybeWOW, err := parseHeader(data)
	if err != nil {
		return nil, err
	}

	info, ok := checkType(magic)
	if !ok {
		return nil, fmt.Errorf("mod: unsupported MOD signature %q", string(magic))
	}

	patternOffset := moduleHeaderSize + 4
	if strings.HasPrefix(string(magic), "FA0") {
		patternOffset += 4
	}

	if info.Trekker && info.Channels == 8 {
		for _, pos := range positions {
			if pos&1 == 1 {
				info.Channels = 4
				break
			}
		}
	}
	if info.Trekker && info.Channels == 8 {
		for i := range positions {
			positions[i] >>= 1
		}
	}

	numPositions := int(songLength)
	numPatterns := countPatterns(positions[:numPositions], positions[numPositions:])
	if info.ModType == 0 && maybeWOW {
		wowLength := uint64(moduleHeaderSize + 4 + sampleBytes + uint32(numPatterns)*(rowsPerPattern*4*8*4))
		if (uint64(size) &^ 1) == wowLength {
			info.ModType = 1
			info.Description = "Mod's Grave"
			info.Channels = 8
		}
	}

	patterns, usedPanning, nextOffset, err := loadPatterns(data, patternOffset, numPatterns, info.Channels, info.Trekker && info.Channels == 8, headers, info.ModType)
	if err != nil {
		return nil, err
	}

	mod := module.New()
	mod.Metadata = module.Metadata{
		Title:   cleanText(songTitle),
		Tracker: info.Description,
		Format:  module.FormatMOD,
	}
	mod.Channels = info.Channels
	mod.Voices = info.Channels
	mod.InitialSpeed = 6
	mod.InitialTempo = 125
	mod.RestartPosition = uint16(restart)
	mod.Orders = make([]uint16, numPositions)
	for i := 0; i < numPositions; i++ {
		mod.Orders[i] = uint16(positions[i])
	}
	mod.Patterns = patterns
	mod.Samples = make([]module.Sample, 31)

	if usedPanning {
		mod.Flags |= module.FlagUsesPanning
	}

	description := info.Description
	offset := nextOffset
	for i, header := range headers {
		sample, consumed, sampleDesc, decodeErr := loadSample(data, offset, header, info.ModType)
		if decodeErr != nil {
			return nil, fmt.Errorf("mod: load sample %d: %w", i, decodeErr)
		}
		if sampleDesc != "" {
			description = sampleDesc
		}
		mod.Samples[i] = sample
		offset += consumed
	}

	mod.Metadata.Tracker = description
	if mod.Flags&module.FlagUsesPanning == 0 {
		mod.ApplyImplicitPanning()
	}

	if err := mod.Validate(); err != nil {
		return nil, fmt.Errorf("mod: validate module: %w", err)
	}

	return mod, nil
}

func parseHeader(data []byte) ([]sampleHeader, string, byte, byte, []byte, []byte, uint32, bool, error) {
	if len(data) < moduleHeaderSize+4 {
		return nil, "", 0, 0, nil, nil, 0, false, fmt.Errorf("mod: truncated header")
	}

	headers := make([]sampleHeader, 31)
	sampleBytes := uint32(0)
	maybeWOW := true

	for i := 0; i < 31; i++ {
		offset := 20 + i*30
		lengthWords := be16(data[offset+22 : offset+24])
		volume := data[offset+25]
		fineTune := data[offset+24]

		headers[i] = sampleHeader{
			Name:            cleanText(string(data[offset : offset+22])),
			LengthWords:     lengthWords,
			FineTune:        fineTune,
			Volume:          volume,
			LoopStartWords:  be16(data[offset+26 : offset+28]),
			LoopLengthWords: be16(data[offset+28 : offset+30]),
		}

		sampleBytes += uint32(lengthWords) * 2
		if lengthWords != 0 && (fineTune != 0x00 || volume != 0x40) {
			maybeWOW = false
		}
	}

	songLength := data[950]
	if songLength > 128 {
		songLength = 128
	}
	restart := data[951]
	if restart != 0x00 {
		maybeWOW = false
	}

	positions := append([]byte(nil), data[952:1080]...)
	magic := append([]byte(nil), data[1080:1084]...)

	return headers, string(data[:20]), songLength, restart, positions, magic, sampleBytes, maybeWOW, nil
}

func checkType(id []byte) (trackerInfo, bool) {
	info := trackerInfo{}

	switch string(id) {
	case "M.K.", "M!K!", "M&K!":
		info.Description = "Protracker"
		info.Channels = 4
		return info, true
	case "OKTA":
		info.Description = "Oktalyzer"
		info.Channels = 8
		info.ModType = 1
		return info, true
	case "CD81", "CD61":
		info.Description = "Octalyser"
		info.Channels = int(id[2] - '0')
		info.ModType = 1
		return info, true
	case "LARD", "NSMS":
		info.Description = "Unknown tracker MOD"
		info.Channels = 4
		return info, true
	}

	if (string(id[:3]) == "FLT" || string(id[:3]) == "EXO") && id[3] >= '0' && id[3] <= '9' {
		channels := int(id[3] - '0')
		if channels == 4 || channels == 8 {
			return trackerInfo{Description: "Startrekker", Channels: channels, ModType: 1, Trekker: true}, true
		}
		return trackerInfo{}, false
	}

	if id[0] >= '1' && id[0] <= '9' && string(id[1:4]) == "CHN" {
		return trackerInfo{Description: "Fasttracker", Channels: int(id[0] - '0'), ModType: 1}, true
	}

	if id[0] >= '0' && id[0] <= '9' && id[1] >= '0' && id[1] <= '9' && (string(id[2:4]) == "CH" || string(id[2:4]) == "CN") {
		channels := int(id[0]-'0')*10 + int(id[1]-'0')
		if id[3] == 'H' {
			return trackerInfo{Description: "Fasttracker", Channels: channels, ModType: 2}, true
		}
		return trackerInfo{Description: "TakeTracker", Channels: channels, ModType: 1}, true
	}

	if string(id[:3]) == "TDZ" && id[3] >= '1' && id[3] <= '3' {
		return trackerInfo{Description: "TakeTracker", Channels: int(id[3] - '0')}, true
	}

	if string(id[:3]) == "FA0" && (id[3] == '4' || id[3] == '6' || id[3] == '8') {
		return trackerInfo{Description: "Digital Tracker MOD", Channels: int(id[3] - '0')}, true
	}

	return trackerInfo{}, false
}

func countPatterns(used []byte, scan []byte) int {
	maxPattern := byte(0)
	for _, pos := range used {
		if pos > maxPattern {
			maxPattern = pos
		}
	}

	validScan := true
	for _, pos := range scan {
		if pos >= 0x80 {
			validScan = false
			break
		}
	}
	if validScan {
		for _, pos := range scan {
			if pos > maxPattern {
				maxPattern = pos
			}
		}
	}

	return int(maxPattern) + 1
}

func loadPatterns(data []byte, offset, numPatterns, channels int, dualPattern bool, headers []sampleHeader, modType int) ([]module.Pattern, bool, int, error) {
	patterns := make([]module.Pattern, numPatterns)
	usedPanning := false
	cursor := offset

	for p := 0; p < numPatterns; p++ {
		pattern := module.Pattern{
			Rows:   rowsPerPattern,
			Tracks: make([]unitrk.Track, channels),
		}

		if dualPattern {
			first, next, err := readPatternNotes(data, cursor, 4)
			if err != nil {
				return nil, false, 0, err
			}
			cursor = next
			second, next, err := readPatternNotes(data, cursor, 4)
			if err != nil {
				return nil, false, 0, err
			}
			cursor = next

			for ch := 0; ch < 4; ch++ {
				track, panning := convertTrack(first, ch, 4, headers, modType)
				pattern.Tracks[ch] = track
				usedPanning = usedPanning || panning
			}
			for ch := 0; ch < 4; ch++ {
				track, panning := convertTrack(second, ch, 4, headers, modType)
				pattern.Tracks[ch+4] = track
				usedPanning = usedPanning || panning
			}
		} else {
			notes, next, err := readPatternNotes(data, cursor, channels)
			if err != nil {
				return nil, false, 0, err
			}
			cursor = next

			for ch := 0; ch < channels; ch++ {
				track, panning := convertTrack(notes, ch, channels, headers, modType)
				pattern.Tracks[ch] = track
				usedPanning = usedPanning || panning
			}
		}

		patterns[p] = pattern
	}

	return patterns, usedPanning, cursor, nil
}

func readPatternNotes(data []byte, offset, channels int) ([]note, int, error) {
	count := rowsPerPattern * channels
	size := count * 4
	if offset+size > len(data) {
		return nil, 0, fmt.Errorf("mod: truncated pattern data at offset %d", offset)
	}

	notes := make([]note, count)
	for i := 0; i < count; i++ {
		base := offset + i*4
		notes[i] = note{
			A: data[base],
			B: data[base+1],
			C: data[base+2],
			D: data[base+3],
		}
	}

	return notes, offset + size, nil
}

func convertTrack(notes []note, channel, channels int, headers []sampleHeader, modType int) (unitrk.Track, bool) {
	var builder unitrk.Builder
	builder.Reset()

	lastEffect := byte(0x10)
	usedPanning := false
	for row := 0; row < rowsPerPattern; row++ {
		effect, rowPanning := convertNote(&builder, notes[row*channels+channel], lastEffect, headers, modType)
		lastEffect = effect
		usedPanning = usedPanning || rowPanning
		builder.NewLine()
	}

	return builder.Track(), usedPanning
}

func convertNote(builder *unitrk.Builder, raw note, lastEffect byte, headers []sampleHeader, modType int) (byte, bool) {
	instrument := (raw.A & 0x10) | (raw.C >> 4)
	period := uint16(raw.A&0x0f)<<8 | uint16(raw.B)
	effect := raw.C & 0x0f
	effectData := raw.D
	usedPanning := false

	noteValue := byte(0)
	if period != 0 {
		for i, candidate := range notePeriodTable {
			if period >= candidate {
				noteValue = byte(i + 1)
				break
			}
		}
	}

	lastNote := byte(0)
	if instrument != 0 {
		if instrument > 31 || headers[instrument-1].LengthWords == 0 {
			builder.PTEffect(0x0c, 0)
			if effect == 0x0c {
				effect = 0
				effectData = 0
			}
		} else if modType == 0 {
			if noteValue != 0 {
				builder.Instrument(uint16(instrument - 1))
			} else if effect != 0 || effectData != 0 {
				builder.Instrument(uint16(instrument - 1))
				noteValue = lastNote
			} else {
				builder.PTEffect(0x0c, headers[instrument-1].Volume&0x7f)
			}
		} else {
			builder.Instrument(uint16(instrument - 1))
			if noteValue == 0 {
				noteValue = lastNote
			}
		}
	}

	if noteValue != 0 {
		builder.Note(noteValue + 23)
		lastNote = noteValue
	}

	if effect == 0x0d {
		effectData = ((effectData >> 4) * 10) + (effectData & 0x0f)
	}
	if effect == 0x0a && effectData&0x0f != 0 && effectData&0xf0 != 0 {
		effectData &= 0xf0
	}
	if effect == 0x0c && effectData > 0x40 {
		effectData = 0x40
	}
	if effectData == 0 && (effect == 0x01 || effect == 0x02 || effect == 0x03) && lastEffect < 0x10 && effect != lastEffect {
		effect = 0
	}

	builder.PTEffect(effect, effectData)
	if effect == 0x08 {
		usedPanning = true
	}

	return effect, usedPanning
}

func loadSample(data []byte, offset int, header sampleHeader, modType int) (module.Sample, int, string, error) {
	sample := module.Sample{
		Name:         header.Name,
		C5Speed:      finetuneTable[header.FineTune&0x0f],
		Volume:       header.Volume & 0x7f,
		Flags:        module.SampleSigned,
		LoopStart:    uint32(header.LoopStartWords) * 2,
		LoopEnd:      (uint32(header.LoopStartWords) + uint32(header.LoopLengthWords)) * 2,
		Length:       uint32(header.LengthWords) * 2,
		GlobalVolume: 64,
	}
	description := ""

	rawLength := int(header.LengthWords) * 2
	if modType == 2 && header.Volume&0x80 != 0 {
		sample.Flags |= module.Sample16Bits
		sample.Length /= 2
		sample.LoopStart /= 2
		sample.LoopEnd /= 2
		description = "Imago Orpheus (MOD format)"
	}

	if header.LoopLengthWords > 2 {
		sample.Flags |= module.SampleLoop
	}

	if sample.Length == 0 {
		return sample, 0, description, nil
	}

	if offset > len(data) {
		return module.Sample{}, 0, "", fmt.Errorf("sample offset out of range: %d", offset)
	}

	consumed := rawLength
	rawData := data[offset:min(offset+rawLength, len(data))]

	if offset+5 <= len(data) && string(data[offset:offset+5]) == "ADPCM" {
		payloadLength := int(header.LengthWords) + 16
		start := offset + 5
		end := start + payloadLength
		if end > len(data) {
			return module.Sample{}, 0, "", fmt.Errorf("truncated ADPCM sample data")
		}
		sample.Flags |= module.SampleADPCM4
		rawData = data[start:end]
		consumed = 5 + payloadLength
	}

	if err := sampledecode.DecodeIntoSample(&sample, rawData, sampledecode.Options{}); err != nil {
		return module.Sample{}, 0, "", err
	}

	return sample, consumed, description, nil
}

func cleanText(s string) string {
	return strings.TrimRight(s, "\x00 ")
}

func be16(data []byte) uint16 {
	return uint16(data[0])<<8 | uint16(data[1])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
