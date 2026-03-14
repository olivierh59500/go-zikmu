package zikmu

import (
	"fmt"
	"io"
	"time"

	itloader "github.com/olivierh59500/go-zikmu/internal/loader/it"
	modloader "github.com/olivierh59500/go-zikmu/internal/loader/mod"
	s3mloader "github.com/olivierh59500/go-zikmu/internal/loader/s3m"
	xmloader "github.com/olivierh59500/go-zikmu/internal/loader/xm"
	modmodel "github.com/olivierh59500/go-zikmu/internal/module"
)

type Format string

const (
	FormatUnknown Format = ""
	FormatMOD     Format = "mod"
	FormatS3M     Format = "s3m"
	FormatXM      Format = "xm"
	FormatIT      Format = "it"
)

type Metadata struct {
	Title        string
	Tracker      string
	Format       Format
	Channels     int
	Orders       int
	Patterns     int
	Instruments  int
	Samples      int
	InitialSpeed int
	InitialTempo int
}

type Module struct {
	Metadata Metadata
	decoded  *modmodel.Module
}

type Config struct {
	SampleRate    int
	Channels      int
	Interpolation bool
	BufferSamples int
}

func DefaultConfig() Config {
	return Config{
		SampleRate:    44100,
		Channels:      2,
		Interpolation: true,
		BufferSamples: 2048,
	}
}

type Player interface {
	Module() *Module
	Config() Config
	Play()
	Pause()
	Stop() error
	IsPlaying() bool
	Reset() error
	Seek(offset time.Duration) error
	Position() time.Duration
	SetVolume(volume float64) error
	Volume() float64
	Stream() io.ReadSeeker
	Render(dst []float32) (int, error)
}

func Load(r io.ReaderAt, size int64) (*Module, error) {
	format, err := Detect(r, size)
	if err != nil {
		return nil, err
	}
	if format == FormatUnknown {
		return nil, errorf(CodeUnsupported, "Load", FormatUnknown, -1, fmt.Errorf("no known module signature found"))
	}

	var decoded *modmodel.Module
	switch format {
	case FormatMOD:
		decoded, err = modloader.Load(r, size)
	case FormatS3M:
		decoded, err = s3mloader.Load(r, size)
	case FormatXM:
		decoded, err = xmloader.Load(r, size)
	case FormatIT:
		decoded, err = itloader.Load(r, size)
	default:
		return nil, errorf(CodeNotImplemented, "Load", format, -1, fmt.Errorf("loader not implemented yet"))
	}
	if err != nil {
		return nil, errorf(CodeReadFailure, "Load", format, -1, err)
	}

	return wrapModule(decoded), nil
}

func wrapModule(decoded *modmodel.Module) *Module {
	if decoded == nil {
		return nil
	}

	return &Module{
		Metadata: Metadata{
			Title:        decoded.Metadata.Title,
			Tracker:      decoded.Metadata.Tracker,
			Format:       Format(decoded.Metadata.Format),
			Channels:     decoded.Channels,
			Orders:       len(decoded.Orders),
			Patterns:     len(decoded.Patterns),
			Instruments:  len(decoded.Instruments),
			Samples:      len(decoded.Samples),
			InitialSpeed: int(decoded.InitialSpeed),
			InitialTempo: int(decoded.InitialTempo),
		},
		decoded: decoded,
	}
}
