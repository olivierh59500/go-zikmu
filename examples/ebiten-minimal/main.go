package main

import (
	"bytes"
	"fmt"
	"image/color"
	"log"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/olivierh59500/go-zikmu"
	"github.com/olivierh59500/go-zikmu/ebitenaudio"
)

type game struct {
	player    *ebitenaudio.Player
	module    *zikmu.Module
	spacePrev bool
	rPrev     bool
	sPrev     bool
	leftPrev  bool
	rightPrev bool
	upPrev    bool
	downPrev  bool
}

func (g *game) Update() error {
	g.handleToggle(ebiten.KeySpace, &g.spacePrev, func() {
		if g.player.IsPlaying() {
			g.player.Pause()
		} else {
			g.player.Play()
		}
	})
	g.handleToggle(ebiten.KeyR, &g.rPrev, func() {
		if err := g.player.Reset(); err != nil {
			log.Printf("reset failed: %v", err)
		}
	})
	g.handleToggle(ebiten.KeyS, &g.sPrev, func() {
		if err := g.player.Stop(); err != nil {
			log.Printf("stop failed: %v", err)
		}
	})
	g.handleToggle(ebiten.KeyLeft, &g.leftPrev, func() {
		target := g.player.Position() - 5*time.Second
		if target < 0 {
			target = 0
		}
		if err := g.player.Seek(target); err != nil {
			log.Printf("seek failed: %v", err)
		}
	})
	g.handleToggle(ebiten.KeyRight, &g.rightPrev, func() {
		if err := g.player.Seek(g.player.Position() + 5*time.Second); err != nil {
			log.Printf("seek failed: %v", err)
		}
	})
	g.handleToggle(ebiten.KeyUp, &g.upPrev, func() {
		volume := g.player.Volume() + 0.1
		if err := g.player.SetVolume(volume); err != nil {
			log.Printf("volume failed: %v", err)
		}
	})
	g.handleToggle(ebiten.KeyDown, &g.downPrev, func() {
		volume := g.player.Volume() - 0.1
		if volume < 0 {
			volume = 0
		}
		if err := g.player.SetVolume(volume); err != nil {
			log.Printf("volume failed: %v", err)
		}
	})

	ebiten.SetWindowTitle(fmt.Sprintf(
		"go-zikmu | %s | %s | pos=%s | vol=%.2f",
		g.module.Metadata.Title,
		playState(g.player.IsPlaying()),
		g.player.Position().Truncate(100*time.Millisecond),
		g.player.Volume(),
	))
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 18, G: 21, B: 27, A: 255})
}

func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return 640, 120
}

func (g *game) handleToggle(key ebiten.Key, prev *bool, fn func()) {
	pressed := ebiten.IsKeyPressed(key)
	if pressed && !*prev {
		fn()
	}
	*prev = pressed
}

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <module.mod|module.s3m|module.xm|module.it>", os.Args[0])
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	module, err := zikmu.Load(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		log.Fatal(err)
	}

	cfg := zikmu.DefaultConfig()
	source, err := zikmu.NewPlayer(module, cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx := audio.NewContext(cfg.SampleRate)
	player, err := ebitenaudio.NewPlayer(ctx, source, ebitenaudio.Options{
		BufferSize: 100 * time.Millisecond,
		AutoPlay:   true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer player.Close()

	ebiten.SetWindowSize(640, 120)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetRunnableOnUnfocused(true)
	log.Printf("controls: space play/pause, r reset, s stop, left/right seek, up/down volume")

	if err := ebiten.RunGame(&game{
		player: player,
		module: module,
	}); err != nil {
		log.Fatal(err)
	}
}

func playState(playing bool) string {
	if playing {
		return "playing"
	}
	return "paused"
}
