package st3

import (
	"errors"
	"sync"
)

type Config struct {
	SampleRate     int
	Interpolation  bool
	StartOrder     int
	PackedPatterns bool
}

type Player struct {
	mu     sync.Mutex
	st     *state
	closed bool
}

func New(moduleData []byte, cfg Config) (*Player, error) {
	if len(moduleData) == 0 {
		return nil, errors.New("st3: empty module")
	}
	if cfg.SampleRate == 0 {
		cfg.SampleRate = 44100
	}
	st := newState(cfg.SampleRate, cfg.Interpolation)
	st.packedPatterns = cfg.PackedPatterns
	if err := st.loadS3M(moduleData, uint32(cfg.StartOrder)); err != nil {
		return nil, err
	}
	st.musicPaused = false
	return &Player{st: st}, nil
}

func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
}

// Fill writes interleaved stereo samples into out and returns the number of frames.
func (p *Player) Fill(out []int16) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed || p.st == nil {
		return 0
	}

	if len(out)%2 != 0 {
		out = out[:len(out)-1]
	}
	if !p.st.fillAudioBuffer(out) {
		return 0
	}
	return len(out) / 2
}

func (p *Player) Order() uint16 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st == nil {
		return 0
	}
	order, _, _ := p.st.orderRowFrame()
	return order
}

func (p *Player) OrderRowFrame() (uint16, uint16, uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st == nil {
		return 0, 0, 0
	}
	return p.st.orderRowFrame()
}

func (p *Player) OrderRowFrameAt(samplePosition uint32) (uint16, uint16, uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st == nil {
		return 0, 0, 0
	}
	return p.st.orderRowFrameAt(samplePosition)
}

func (p *Player) Row() uint16 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st == nil {
		return 0
	}
	_, row, _ := p.st.orderRowFrame()
	return row
}

func (p *Player) Frame() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st == nil {
		return 0
	}
	_, _, frame := p.st.orderRowFrame()
	return frame
}

func (p *Player) PlusFlags() int16 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st == nil {
		return 0
	}
	return p.st.plusFlags()
}

func (p *Player) PlusFlagsAt(samplePosition uint32) int16 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st == nil {
		return 0
	}
	return p.st.plusFlagsAt(samplePosition)
}

// SnapshotAt reads the musical position and separator flags under one lock.
func (p *Player) SnapshotAt(samplePosition uint32) (uint16, uint16, uint32, int16) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.st == nil || p.closed {
		return 0, 0, 0, 0
	}
	order, row, frame := p.st.orderRowFrameAt(samplePosition)
	return order, row, frame, p.st.plusFlagsAt(samplePosition)
}
