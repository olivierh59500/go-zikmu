package validation

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/olivierh59500/go-zikmu"
	itloader "github.com/olivierh59500/go-zikmu/internal/loader/it"
	modloader "github.com/olivierh59500/go-zikmu/internal/loader/mod"
	s3mloader "github.com/olivierh59500/go-zikmu/internal/loader/s3m"
	xmloader "github.com/olivierh59500/go-zikmu/internal/loader/xm"
	"github.com/olivierh59500/go-zikmu/internal/mixer"
	modmodel "github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/replay"
	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
)

const (
	DefaultFrames = 4096
)

type CorpusEntry struct {
	Name   string
	Format zikmu.Format
	Data   []byte
}

type RenderResult struct {
	PCM16          []byte
	Frames         int
	SHA256         string
	Peak           int
	NonZeroSamples int
}

type PCM16Comparison struct {
	ComparedSamples int
	LeftSamples     int
	RightSamples    int
	Different       int
	FirstDifferent  int
	MaxAbsDelta     int
	MeanAbsDelta    float64
}

type RenderOptions struct {
	ExperimentalXMHighQuality bool
}

type pcm16Renderer interface {
	RenderPCM16(dst []int16) (int, error)
}

func DefaultConfig() zikmu.Config {
	return zikmu.Config{
		SampleRate:    44100,
		Channels:      2,
		Interpolation: true,
		BufferSamples: 1024,
	}
}

func RegressionCorpus() []CorpusEntry {
	return []CorpusEntry{
		{Name: "mod-minimal", Format: zikmu.FormatMOD, Data: testfixtures.MinimalMOD()},
		{Name: "mod-adpcm", Format: zikmu.FormatMOD, Data: testfixtures.MinimalMODADPCM()},
		{Name: "mod-flt8", Format: zikmu.FormatMOD, Data: testfixtures.MinimalFLT8MOD()},
		{Name: "s3m-minimal", Format: zikmu.FormatS3M, Data: testfixtures.MinimalS3M()},
		{Name: "s3m-adpcm", Format: zikmu.FormatS3M, Data: testfixtures.MinimalS3MADPCM()},
		{Name: "xm-minimal", Format: zikmu.FormatXM, Data: testfixtures.MinimalXM()},
		{Name: "xm-adpcm", Format: zikmu.FormatXM, Data: testfixtures.MinimalXMADPCM()},
		{Name: "xm-103", Format: zikmu.FormatXM, Data: testfixtures.MinimalXM103()},
		{Name: "it-minimal", Format: zikmu.FormatIT, Data: testfixtures.MinimalIT()},
		{Name: "it-packed", Format: zikmu.FormatIT, Data: testfixtures.MinimalITPacked()},
	}
}

func RenderPCM16(data []byte, cfg zikmu.Config, frames int) (RenderResult, error) {
	return RenderPCM16WithOptions(data, cfg, frames, RenderOptions{})
}

func RenderPCM16WithOptions(data []byte, cfg zikmu.Config, frames int, opts RenderOptions) (RenderResult, error) {
	if frames <= 0 {
		return RenderResult{}, fmt.Errorf("validation: invalid frame count %d", frames)
	}

	var (
		pcm16   []byte
		written int
		err     error
	)
	if opts.ExperimentalXMHighQuality {
		written, pcm16, err = renderPCM16Experimental(data, cfg, frames, opts)
	} else {
		module, loadErr := zikmu.Load(bytes.NewReader(data), int64(len(data)))
		if loadErr != nil {
			return RenderResult{}, loadErr
		}
		player, playerErr := zikmu.NewPlayer(module, cfg)
		if playerErr != nil {
			return RenderResult{}, playerErr
		}
		renderer, ok := player.(pcm16Renderer)
		if ok {
			samples := make([]int16, frames*cfg.Channels)
			written, err = renderer.RenderPCM16(samples)
			if err != nil {
				return RenderResult{}, err
			}
			pcm16 = Int16ToPCM16LE(samples[:written])
		} else {
			samples := make([]float32, frames*cfg.Channels)
			written, err = player.Render(samples)
			if err != nil {
				return RenderResult{}, err
			}
			pcm16 = Float32ToPCM16LE(samples[:written])
		}
	}
	result := RenderResult{
		PCM16:  pcm16,
		Frames: written / cfg.Channels,
		SHA256: hashBytes(pcm16),
	}

	for i := 0; i+1 < len(pcm16); i += 2 {
		value := int16(binary.LittleEndian.Uint16(pcm16[i:]))
		if value != 0 {
			result.NonZeroSamples++
		}
		abs := int(value)
		if abs < 0 {
			abs = -abs
		}
		if abs > result.Peak {
			result.Peak = abs
		}
	}

	return result, nil
}

func renderPCM16Experimental(data []byte, cfg zikmu.Config, frames int, opts RenderOptions) (int, []byte, error) {
	module, err := loadDecodedModule(data)
	if err != nil {
		return 0, nil, err
	}

	engine, err := replay.New(module, replay.Config{SampleRate: cfg.SampleRate})
	if err != nil {
		return 0, nil, err
	}
	softwareMixer, err := mixer.New(module, mixer.Config{
		SampleRate:                cfg.SampleRate,
		OutputChannels:            cfg.Channels,
		Interpolation:             cfg.Interpolation,
		MasterVolume:              1,
		ExperimentalXMHighQuality: opts.ExperimentalXMHighQuality,
	})
	if err != nil {
		return 0, nil, err
	}
	softwareMixer.Reset(engine.Snapshot())

	samples := make([]int16, frames*cfg.Channels)
	remaining := frames
	offset := 0
	framesUntilTick := maxInt(engine.CurrentTickFrames(), 1)
	for remaining > 0 {
		if framesUntilTick <= 0 {
			if err := engine.AdvanceTicks(1); err != nil {
				return 0, nil, err
			}
			framesUntilTick = maxInt(engine.CurrentTickFrames(), 1)
			softwareMixer.ApplySnapshot(engine.Snapshot())
		}
		step := minInt(remaining, framesUntilTick)
		written, err := softwareMixer.RenderPCM16(samples[offset : offset+step*cfg.Channels])
		if err != nil {
			return 0, nil, err
		}
		offset += written
		remaining -= step
		framesUntilTick -= step
	}

	return offset, Int16ToPCM16LE(samples[:offset]), nil
}

func loadDecodedModule(data []byte) (*modmodel.Module, error) {
	reader := bytes.NewReader(data)
	format, err := zikmu.Detect(reader, int64(len(data)))
	if err != nil {
		return nil, err
	}

	switch format {
	case zikmu.FormatMOD:
		return modloader.Load(reader, int64(len(data)))
	case zikmu.FormatS3M:
		return s3mloader.Load(reader, int64(len(data)))
	case zikmu.FormatXM:
		return xmloader.Load(reader, int64(len(data)))
	case zikmu.FormatIT:
		return itloader.Load(reader, int64(len(data)))
	default:
		return nil, fmt.Errorf("validation: unsupported format %q", format)
	}
}

func Int16ToPCM16LE(samples []int16) []byte {
	pcm16 := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(pcm16[i*2:], uint16(sample))
	}
	return pcm16
}

func Float32ToPCM16LE(samples []float32) []byte {
	pcm16 := make([]byte, len(samples)*2)
	for i, sample := range samples {
		value := quantizePCM16(sample)
		binary.LittleEndian.PutUint16(pcm16[i*2:], uint16(value))
	}
	return pcm16
}

func ComparePCM16(left, right []byte) (PCM16Comparison, error) {
	if len(left)%2 != 0 || len(right)%2 != 0 {
		return PCM16Comparison{}, fmt.Errorf("validation: pcm16 buffers must have even lengths")
	}

	leftSamples := len(left) / 2
	rightSamples := len(right) / 2
	compared := leftSamples
	if rightSamples < compared {
		compared = rightSamples
	}

	var totalDelta int64
	comparison := PCM16Comparison{
		ComparedSamples: compared,
		LeftSamples:     leftSamples,
		RightSamples:    rightSamples,
		FirstDifferent:  -1,
	}

	for i := 0; i < compared; i++ {
		lv := int(int16(binary.LittleEndian.Uint16(left[i*2:])))
		rv := int(int16(binary.LittleEndian.Uint16(right[i*2:])))
		delta := lv - rv
		if delta < 0 {
			delta = -delta
		}
		if delta != 0 {
			comparison.Different++
			if comparison.FirstDifferent < 0 {
				comparison.FirstDifferent = i
			}
		}
		if delta > comparison.MaxAbsDelta {
			comparison.MaxAbsDelta = delta
		}
		totalDelta += int64(delta)
	}

	if compared > 0 {
		comparison.MeanAbsDelta = float64(totalDelta) / float64(compared)
	}

	return comparison, nil
}

func FileSuffix(format zikmu.Format) string {
	switch format {
	case zikmu.FormatMOD:
		return ".mod"
	case zikmu.FormatS3M:
		return ".s3m"
	case zikmu.FormatXM:
		return ".xm"
	case zikmu.FormatIT:
		return ".it"
	default:
		return ".bin"
	}
}

func quantizePCM16(sample float32) int16 {
	switch {
	case sample >= 1:
		return math.MaxInt16
	case sample <= -1:
		return math.MinInt16
	default:
		return int16(math.Round(float64(sample) * float64(math.MaxInt16)))
	}
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
