package zikmu

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"sync"
	"time"

	"github.com/olivierh59500/go-zikmu/internal/mixer"
	"github.com/olivierh59500/go-zikmu/internal/replay"
)

type player struct {
	module          *Module
	config          Config
	engine          *replay.Engine
	mixer           *mixer.Mixer
	pcmScratch      []int16
	framesUntilTick int
	positionFrames  int64
	volume          float32
	playing         bool
	stream          *pcmStream
	mu              sync.Mutex
}

func NewPlayer(module *Module, cfg Config) (Player, error) {
	if module == nil || module.decoded == nil {
		return nil, ErrNilModule
	}
	if cfg.SampleRate <= 0 {
		return nil, fmt.Errorf("zikmu: invalid sample rate %d", cfg.SampleRate)
	}
	if cfg.Channels != 1 && cfg.Channels != 2 {
		return nil, fmt.Errorf("zikmu: invalid channel count %d", cfg.Channels)
	}
	if cfg.BufferSamples <= 0 {
		return nil, fmt.Errorf("zikmu: invalid buffer size %d", cfg.BufferSamples)
	}

	engine, err := replay.New(module.decoded, replay.Config{SampleRate: cfg.SampleRate})
	if err != nil {
		return nil, err
	}
	softwareMixer, err := mixer.New(module.decoded, mixer.Config{
		SampleRate:     cfg.SampleRate,
		OutputChannels: cfg.Channels,
		Interpolation:  cfg.Interpolation,
		MasterVolume:   1,
	})
	if err != nil {
		return nil, err
	}
	softwareMixer.Reset(engine.Snapshot())

	return &player{
		module:          module,
		config:          cfg,
		engine:          engine,
		mixer:           softwareMixer,
		framesUntilTick: maxInt(engine.CurrentTickFrames(), 1),
		volume:          1,
		playing:         true,
	}, nil
}

func (p *player) Module() *Module {
	return p.module
}

func (p *player) Config() Config {
	return p.config
}

func (p *player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playing = true
}

func (p *player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playing = false
}

func (p *player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.resetLocked(); err != nil {
		return err
	}
	p.playing = false
	return nil
}

func (p *player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing
}

func (p *player) Reset() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.resetLocked()
}

func (p *player) Seek(offset time.Duration) error {
	if offset < 0 {
		return fmt.Errorf("zikmu: invalid seek offset %s", offset)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	target := int64(offset) * int64(p.config.SampleRate) / int64(time.Second)
	return p.seekFramesLocked(target)
}

func (p *player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Duration(p.positionFrames) * time.Second / time.Duration(p.config.SampleRate)
}

func (p *player) SetVolume(volume float64) error {
	if math.IsNaN(volume) || math.IsInf(volume, 0) || volume < 0 {
		return fmt.Errorf("zikmu: invalid volume %v", volume)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = float32(volume)
	return nil
}

func (p *player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return float64(p.volume)
}

func (p *player) Stream() io.ReadSeeker {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stream == nil {
		p.stream = &pcmStream{
			player: p,
		}
	}
	return p.stream
}

func (p *player) Render(dst []float32) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.renderLocked(dst, false)
}

func (p *player) RenderPCM16(dst []int16) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.renderPCM16Locked(dst, false)
}

func (p *player) resetLocked() error {
	if err := p.engine.Reset(); err != nil {
		return err
	}
	p.mixer.Reset(p.engine.Snapshot())
	p.framesUntilTick = maxInt(p.engine.CurrentTickFrames(), 1)
	p.positionFrames = 0
	if p.stream != nil {
		p.stream.resetLocked()
	}
	return nil
}

func (p *player) seekFramesLocked(target int64) error {
	if target < 0 {
		return fmt.Errorf("zikmu: invalid seek frame offset %d", target)
	}
	if err := p.resetLocked(); err != nil {
		return err
	}
	if target == 0 {
		return nil
	}

	bufferFrames := maxInt(p.config.BufferSamples, 256)
	buffer := make([]float32, bufferFrames*p.config.Channels)
	for p.positionFrames < target {
		remaining := target - p.positionFrames
		frames := int64(bufferFrames)
		if remaining < frames {
			frames = remaining
		}
		_, err := p.renderLocked(buffer[:int(frames)*p.config.Channels], true)
		if err != nil {
			return err
		}
	}
	if p.stream != nil {
		p.stream.resetLocked()
	}
	return nil
}

func (p *player) renderLocked(dst []float32, forceAdvance bool) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}

	for i := range dst {
		dst[i] = 0
	}

	frames := len(dst) / p.config.Channels
	if frames == 0 {
		return 0, nil
	}
	used := frames * p.config.Channels
	p.ensurePCMScratch(used)

	written, err := p.renderPCM16Locked(p.pcmScratch[:used], forceAdvance)
	if err != nil {
		return written, err
	}
	for i := 0; i < written; i++ {
		dst[i] = float32(p.pcmScratch[i]) / 32768.0
	}

	return used, nil
}

func (p *player) renderPCM16Locked(dst []int16, forceAdvance bool) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}

	for i := range dst {
		dst[i] = 0
	}

	frames := len(dst) / p.config.Channels
	if frames == 0 {
		return 0, nil
	}
	used := frames * p.config.Channels

	if !p.playing && !forceAdvance {
		return used, nil
	}

	remaining := frames
	offset := 0
	for remaining > 0 {
		if p.framesUntilTick <= 0 {
			if err := p.engine.AdvanceTicks(1); err != nil {
				return offset, err
			}
			p.framesUntilTick = maxInt(p.engine.CurrentTickFrames(), 1)
			p.mixer.ApplySnapshot(p.engine.Snapshot())
		}
		step := minInt(remaining, p.framesUntilTick)
		written, err := p.mixer.RenderPCM16(dst[offset : offset+step*p.config.Channels])
		if err != nil {
			return offset + written, err
		}
		offset += written
		p.framesUntilTick -= step
		remaining -= step
		p.positionFrames += int64(step)
	}

	if p.volume != 1 {
		for i := 0; i < used; i++ {
			scaled := float32(dst[i]) * p.volume
			switch {
			case scaled >= 32767:
				dst[i] = 32767
			case scaled <= -32768:
				dst[i] = -32768
			default:
				dst[i] = int16(scaled)
			}
		}
	}

	return used, nil
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

func (p *player) ensurePCMScratch(size int) {
	if cap(p.pcmScratch) >= size {
		p.pcmScratch = p.pcmScratch[:size]
		return
	}
	p.pcmScratch = make([]int16, size)
}

type pcmStream struct {
	player  *player
	pending []byte
	scratch []float32
}

func (s *pcmStream) Read(p []byte) (int, error) {
	s.player.mu.Lock()
	defer s.player.mu.Unlock()

	if len(p) == 0 {
		return 0, nil
	}

	n := 0
	if len(s.pending) > 0 {
		copied := copy(p, s.pending)
		s.pending = s.pending[copied:]
		n += copied
		if n == len(p) {
			return n, nil
		}
	}

	frameBytes := s.player.config.Channels * 4
	needBytes := len(p) - n
	frames := (needBytes + frameBytes - 1) / frameBytes
	if frames <= 0 {
		return n, nil
	}

	s.ensureScratch(frames * s.player.config.Channels)
	written, err := s.player.renderLocked(s.scratch[:frames*s.player.config.Channels], false)
	if err != nil {
		return n, err
	}

	bytes := make([]byte, written*4)
	for i := 0; i < written; i++ {
		binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(s.scratch[i]))
	}

	copied := copy(p[n:], bytes)
	n += copied
	if copied < len(bytes) {
		s.pending = append(s.pending[:0], bytes[copied:]...)
	}
	return n, nil
}

func (s *pcmStream) Seek(offset int64, whence int) (int64, error) {
	s.player.mu.Lock()
	defer s.player.mu.Unlock()

	const bytesPerSample = 4
	frameBytes := int64(s.player.config.Channels * bytesPerSample)
	current := s.player.positionFrames * frameBytes
	if len(s.pending) > 0 {
		current -= int64(len(s.pending))
		if current < 0 {
			current = 0
		}
	}

	var target int64
	switch whence {
	case io.SeekStart:
		target = offset
	case io.SeekCurrent:
		target = current + offset
	default:
		return current, fmt.Errorf("zikmu: unsupported seek whence %d", whence)
	}
	if target < 0 {
		return current, fmt.Errorf("zikmu: invalid seek target %d", target)
	}

	frameTarget := target / frameBytes
	if err := s.player.seekFramesLocked(frameTarget); err != nil {
		return current, err
	}
	s.pending = s.pending[:0]
	remainder := int(target % frameBytes)
	if remainder != 0 {
		s.ensureScratch(s.player.config.Channels)
		buffer := make([]byte, int(frameBytes))
		written, err := s.player.renderLocked(s.scratch[:s.player.config.Channels], true)
		if err != nil {
			return frameTarget * frameBytes, err
		}
		for i := 0; i < written; i++ {
			binary.LittleEndian.PutUint32(buffer[i*bytesPerSample:], math.Float32bits(s.scratch[i]))
		}
		s.pending = append(s.pending[:0], buffer[remainder:written*bytesPerSample]...)
	}
	return target, nil
}

func (s *pcmStream) ensureScratch(size int) {
	if cap(s.scratch) >= size {
		s.scratch = s.scratch[:size]
		return
	}
	s.scratch = make([]float32, size)
}

func (s *pcmStream) resetLocked() {
	s.pending = s.pending[:0]
}
