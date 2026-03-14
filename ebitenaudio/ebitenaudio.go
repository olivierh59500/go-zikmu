package ebitenaudio

import (
	"fmt"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/olivierh59500/go-zikmu"
)

type Options struct {
	BufferSize time.Duration
	AutoPlay   bool
}

type Player struct {
	source zikmu.Player
	audio  *audio.Player
}

func NewPlayer(ctx *audio.Context, source zikmu.Player, opts Options) (*Player, error) {
	if ctx == nil {
		return nil, fmt.Errorf("ebitenaudio: nil context")
	}
	if source == nil {
		return nil, fmt.Errorf("ebitenaudio: nil source player")
	}
	if ctx.SampleRate() != source.Config().SampleRate {
		return nil, fmt.Errorf("ebitenaudio: sample rate mismatch context=%d source=%d", ctx.SampleRate(), source.Config().SampleRate)
	}

	player, err := ctx.NewPlayerF32(source.Stream())
	if err != nil {
		return nil, err
	}
	if opts.BufferSize != 0 {
		player.SetBufferSize(opts.BufferSize)
	}

	wrapped := &Player{
		source: source,
		audio:  player,
	}
	if opts.AutoPlay {
		wrapped.Play()
	}
	return wrapped, nil
}

func (p *Player) Source() zikmu.Player {
	return p.source
}

func (p *Player) Audio() *audio.Player {
	return p.audio
}

func (p *Player) Play() {
	p.source.Play()
	p.audio.Play()
}

func (p *Player) Pause() {
	p.source.Pause()
	p.audio.Pause()
}

func (p *Player) Stop() error {
	p.audio.Pause()
	if err := p.audio.Rewind(); err != nil {
		return err
	}
	return p.source.Stop()
}

func (p *Player) Reset() error {
	if err := p.audio.Rewind(); err != nil {
		return err
	}
	return p.source.Reset()
}

func (p *Player) Seek(offset time.Duration) error {
	if err := p.audio.SetPosition(offset); err != nil {
		return err
	}
	return p.source.Seek(offset)
}

func (p *Player) Position() time.Duration {
	return p.source.Position()
}

func (p *Player) IsPlaying() bool {
	return p.source.IsPlaying() && p.audio.IsPlaying()
}

func (p *Player) SetVolume(volume float64) error {
	return p.source.SetVolume(volume)
}

func (p *Player) Volume() float64 {
	return p.source.Volume()
}

func (p *Player) Close() error {
	p.audio.Pause()
	return p.audio.Close()
}
