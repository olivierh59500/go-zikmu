# go-zikmu

`go-zikmu` is a pure-Go library for loading, replaying, and streaming tracker modules.

It currently supports:

- `MOD`
- `S3M`
- `XM`
- `IT`

The package is designed for software playback in Go applications. You can use it to:

- detect and load tracker modules from any `io.ReaderAt`
- inspect module metadata
- render interleaved `float32` PCM samples
- control playback with play, pause, stop, reset, seek, and volume
- expose an `io.ReadSeeker` stream for audio backends
- plug directly into Ebiten through the `ebitenaudio` helper package

## Installation

Use the module path from `go.mod`:

```bash
go get github.com/olivierh59500/go-zikmu
```

Use the Go toolchain version declared in [`go.mod`](go.mod).

## Quick Start

### Load a module

```go
package main

import (
	"fmt"
	"os"

	"github.com/olivierh59500/go-zikmu"
)

func main() {
	f, err := os.Open("music.xm")
	if err != nil {
		panic(err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		panic(err)
	}

	module, err := zikmu.Load(f, info.Size())
	if err != nil {
		panic(err)
	}

	fmt.Printf(
		"%s | format=%s | channels=%d | patterns=%d\n",
		module.Metadata.Title,
		module.Metadata.Format,
		module.Metadata.Channels,
		module.Metadata.Patterns,
	)
}
```

`Load` auto-detects the module format. If you only want detection, use `zikmu.Detect`.

### Render PCM in memory

```go
cfg := zikmu.DefaultConfig()

player, err := zikmu.NewPlayer(module, cfg)
if err != nil {
	panic(err)
}

buffer := make([]float32, cfg.BufferSamples*cfg.Channels)
written, err := player.Render(buffer)
if err != nil {
	panic(err)
}

pcm := buffer[:written]
_ = pcm
```

`Render` writes interleaved `float32` PCM samples in the usual `[-1.0, 1.0]` range. The default config uses:

- `44100` Hz
- `2` output channels
- interpolation enabled

Custom configs currently support `1` or `2` output channels.

The player API also exposes:

- `Play()` / `Pause()`
- `Stop()` / `Reset()`
- `Seek(time.Duration)`
- `Position()`
- `SetVolume(float64)` / `Volume()`
- `Stream() io.ReadSeeker`

`Stream()` produces little-endian `float32` PCM, which is useful for backends that pull audio data.

## Ebiten Integration

The repository includes an `ebitenaudio` package that wraps a `zikmu.Player` as an Ebiten audio player.

```go
package main

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/olivierh59500/go-zikmu/ebitenaudio"
)

ctx := audio.NewContext(cfg.SampleRate)

ebitenPlayer, err := ebitenaudio.NewPlayer(ctx, player, ebitenaudio.Options{
	BufferSize: 100 * time.Millisecond,
	AutoPlay:   true,
})
if err != nil {
	panic(err)
}
defer ebitenPlayer.Close()
```

The Ebiten audio context sample rate must match `zikmu.Config.SampleRate`.

A runnable example is available in [`examples/ebiten-minimal`](examples/ebiten-minimal):

```bash
go run ./examples/ebiten-minimal /path/to/module.it
```

Keyboard controls in the example:

- `Space`: play/pause
- `R`: reset
- `S`: stop
- `Left` / `Right`: seek backward or forward by 5 seconds
- `Up` / `Down`: volume

## Errors

Unknown files return a typed error, so you can use `errors.Is`:

```go
if errors.Is(err, zikmu.ErrUnsupportedFormat) {
	// not a supported tracker module
}
```

The package also exposes `*zikmu.Error`, which carries:

- an error code
- the operation name
- the detected format
- a byte offset when available

## Development

Run the full test suite:

```bash
go test ./...
```

Run benchmarks:

```bash
go test -bench . ./...
```

The repository also contains:

- `examples/ebiten-minimal` for manual playback testing
- `cmd/zikmu-compare-libmikmod` for renderer comparisons against a libmikmod-based reference helper
- `tools/` for the small C utilities used by the comparison workflow

## License

MIT. See [`LICENSE`](LICENSE).

### Timing-sensitive Scream Tracker playback

`NewScreamTracker3(data, ScreamTracker3Options{SampleRate: 48000,
Interpolation: true})` selects the integer Scream Tracker compatibility mixer.
`Fill` renders interleaved `int16` stereo without callback allocations;
`PositionAt` maps an audible sample frame to its order, row, tracker frame and
order-separator flags. Its marker history is bounded. `StartOrder` starts at a
specific order; `PackedPatterns` supports the pattern encoding in FC soundtracks.
Ordinary S3M files leave `PackedPatterns` false. The default `NewPlayer` behavior
and supported formats are unchanged.

The compatibility core is distributed under the GNU General Public License in
[`internal/st3/LICENSE`](internal/st3/LICENSE); the surrounding library retains
its existing license. No soundtrack assets are included with the core.
