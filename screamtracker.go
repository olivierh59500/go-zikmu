package zikmu

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/olivierh59500/go-zikmu/internal/st3"
)

// ScreamTracker3Options selects the integer replay used by timing-sensitive
// productions. PackedPatterns enables the pattern-byte encoding used by FC
// soundtracks. Ordinary S3M files leave it false.
type ScreamTracker3Options struct {
	SampleRate     int
	Interpolation  bool
	StartOrder     int
	PackedPatterns bool
}

// TrackerPosition identifies the tracker tick at a PCM frame. PlusFlags is the
// signed distance to the nearest order separator, as used by classic timelines.
type TrackerPosition struct {
	Order, Row uint16
	Frame      uint32
	PlusFlags  int16
}

// ScreamTracker3 preserves the integer mixer and a bounded history of tracker
// markers. Audio callbacks and marker lookups are serialized by the replay core.
// Unlike the default Player, this compatibility player emits stereo int16 PCM.
// The compatibility core's license is in internal/st3/LICENSE.
type ScreamTracker3 struct {
	player *st3.Player
	title  string
}

// NewScreamTracker3 opens an S3M soundtrack without an audio device. Playback
// follows the tracker's native order flow until Close; callers may impose an
// explicit duration. StartOrder is zero-based.
func NewScreamTracker3(data []byte, options ScreamTracker3Options) (*ScreamTracker3, error) {
	if options.SampleRate == 0 {
		options.SampleRate = 48000
	}
	if options.SampleRate < 8000 || options.SampleRate > 192000 {
		return nil, fmt.Errorf("zikmu: invalid Scream Tracker sample rate")
	}
	if options.StartOrder < 0 || options.StartOrder > 255 {
		return nil, fmt.Errorf("zikmu: invalid Scream Tracker starting order")
	}
	if len(data) < 0x70 || string(data[44:48]) != "SCRM" {
		return nil, fmt.Errorf("zikmu: invalid Scream Tracker module")
	}
	orders := int(binary.LittleEndian.Uint16(data[32:]))
	instruments := int(binary.LittleEndian.Uint16(data[34:]))
	patterns := int(binary.LittleEndian.Uint16(data[36:]))
	if orders == 0 || orders > 256 || instruments > 100 || patterns > 100 || 96+orders+2*(instruments+patterns) > len(data) {
		return nil, fmt.Errorf("zikmu: unsupported or truncated Scream Tracker tables")
	}
	if options.StartOrder >= orders {
		return nil, fmt.Errorf("zikmu: starting order exceeds the song")
	}
	p, err := st3.New(data, st3.Config{SampleRate: options.SampleRate, Interpolation: options.Interpolation, StartOrder: options.StartOrder, PackedPatterns: options.PackedPatterns})
	if err != nil {
		return nil, err
	}
	return &ScreamTracker3{player: p, title: strings.TrimRight(string(data[:28]), "\x00 ")}, nil
}

// Title returns the module's title without retaining the caller's data.
func (p *ScreamTracker3) Title() string { return p.title }

// Fill renders complete stereo frames. The final odd sample, if any, is ignored.
// It allocates no memory. Small internal chunks bound mixer working storage.
func (p *ScreamTracker3) Fill(out []int16) int {
	frames := 0
	for len(out) >= 2 {
		count := min(len(out)/2, 1024)
		n := p.player.Fill(out[:count*2])
		frames += n
		if n != count {
			break
		}
		out = out[n*2:]
	}
	return frames
}

// PositionAt returns the marker at an audible PCM frame. History is bounded;
// requests older than retained history clamp to its oldest available marker.
func (p *ScreamTracker3) PositionAt(samplePosition uint64) TrackerPosition {
	position := uint32(min(samplePosition, uint64(math.MaxUint32)))
	order, row, frame, flags := p.player.SnapshotAt(position)
	return TrackerPosition{Order: order, Row: row, Frame: frame, PlusFlags: flags}
}

// Close is idempotent and safe during concurrent callback reads.
func (p *ScreamTracker3) Close() error { p.player.Close(); return nil }
