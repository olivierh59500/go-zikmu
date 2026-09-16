package zikmu

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"
	"time"

	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.SampleRate != 44100 {
		t.Fatalf("unexpected sample rate: %d", cfg.SampleRate)
	}
	if cfg.Channels != 2 {
		t.Fatalf("unexpected channel count: %d", cfg.Channels)
	}
	if !cfg.Interpolation {
		t.Fatal("expected interpolation to be enabled")
	}
	if cfg.BufferSamples <= 0 {
		t.Fatalf("unexpected buffer size: %d", cfg.BufferSamples)
	}
}

func TestNewPlayerRejectsInvalidConfig(t *testing.T) {
	_, err := NewPlayer(&Module{}, Config{
		SampleRate:    0,
		Channels:      2,
		Interpolation: true,
		BufferSamples: 2048,
	})
	if err == nil {
		t.Fatal("expected an error for invalid config")
	}
}

func TestNewPlayerRendersAudioWhileAdvancingReplay(t *testing.T) {
	modData := testfixtures.MinimalMOD()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	player, err := NewPlayer(mod, DefaultConfig())
	if err != nil {
		t.Fatalf("NewPlayer failed: %v", err)
	}

	buffer := make([]float32, 1024)
	written, err := player.Render(buffer)
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if written != len(buffer) {
		t.Fatalf("unexpected write count: got=%d want=%d", written, len(buffer))
	}
	hasAudio := false
	for _, sample := range buffer {
		if sample != 0 {
			hasAudio = true
			break
		}
	}
	if !hasAudio {
		t.Fatal("expected non-zero PCM output")
	}

	if err := player.Reset(); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}
}

func TestPlayerTransportControls(t *testing.T) {
	modData := testfixtures.MinimalMOD()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	player, err := NewPlayer(mod, DefaultConfig())
	if err != nil {
		t.Fatalf("NewPlayer failed: %v", err)
	}

	if !player.IsPlaying() {
		t.Fatal("expected player to start in playing state")
	}

	player.Pause()
	if player.IsPlaying() {
		t.Fatal("expected paused player")
	}

	silent := make([]float32, 256)
	if _, err := player.Render(silent); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	for i, sample := range silent {
		if sample != 0 {
			t.Fatalf("expected paused output to be silent at %d, got %f", i, sample)
		}
	}
	if player.Position() != 0 {
		t.Fatalf("expected paused render to preserve position, got %s", player.Position())
	}

	player.Play()
	if !player.IsPlaying() {
		t.Fatal("expected playing player")
	}

	loud := make([]float32, 256)
	if _, err := player.Render(loud); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	if !hasNonZeroFloat32(loud) {
		t.Fatal("expected audio after Play")
	}
	if player.Position() <= 0 {
		t.Fatalf("expected positive playback position, got %s", player.Position())
	}

	if err := player.Seek(0); err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	if player.Position() != 0 {
		t.Fatalf("expected position reset after Seek(0), got %s", player.Position())
	}

	if err := player.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if player.IsPlaying() {
		t.Fatal("expected stopped player to be paused")
	}
	if player.Position() != 0 {
		t.Fatalf("expected stopped player to rewind, got %s", player.Position())
	}
}

func TestPlayerOptionalPCM16Render(t *testing.T) {
	modData := testfixtures.MinimalMOD()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	player, err := NewPlayer(mod, DefaultConfig())
	if err != nil {
		t.Fatalf("NewPlayer failed: %v", err)
	}

	renderer, ok := player.(interface {
		RenderPCM16(dst []int16) (int, error)
	})
	if !ok {
		t.Fatal("expected player to expose optional PCM16 renderer")
	}

	buffer := make([]int16, 1024)
	written, err := renderer.RenderPCM16(buffer)
	if err != nil {
		t.Fatalf("RenderPCM16 failed: %v", err)
	}
	if written != len(buffer) {
		t.Fatalf("unexpected write count: got=%d want=%d", written, len(buffer))
	}
	if !hasNonZeroInt16(buffer) {
		t.Fatal("expected non-zero PCM16 output")
	}
}

func TestPlayerFloatRenderMatchesPCM16Path(t *testing.T) {
	modData := testfixtures.MinimalMOD()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	player, err := NewPlayer(mod, DefaultConfig())
	if err != nil {
		t.Fatalf("NewPlayer failed: %v", err)
	}

	floatBuf := make([]float32, 512)
	if _, err := player.Render(floatBuf); err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if err := player.Reset(); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	renderer, ok := player.(interface {
		RenderPCM16(dst []int16) (int, error)
	})
	if !ok {
		t.Fatal("expected player to expose optional PCM16 renderer")
	}
	intBuf := make([]int16, len(floatBuf))
	if _, err := renderer.RenderPCM16(intBuf); err != nil {
		t.Fatalf("RenderPCM16 failed: %v", err)
	}

	for i := range floatBuf {
		want := float32(intBuf[i]) / 32768.0
		if math.Abs(float64(floatBuf[i]-want)) > 1e-7 {
			t.Fatalf("unexpected float render sample at %d: got=%f want=%f", i, floatBuf[i], want)
		}
	}
}

func TestPlayerVolumeControlAndStreamSeek(t *testing.T) {
	modData := testfixtures.MinimalMOD()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	player, err := NewPlayer(mod, DefaultConfig())
	if err != nil {
		t.Fatalf("NewPlayer failed: %v", err)
	}

	base := make([]float32, 256)
	if _, err := player.Render(base); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	basePeak := peakFloat32(base)
	if basePeak == 0 {
		t.Fatal("expected non-zero baseline audio")
	}

	if err := player.Seek(0); err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	if err := player.SetVolume(0.25); err != nil {
		t.Fatalf("SetVolume failed: %v", err)
	}
	if math.Abs(player.Volume()-0.25) > 1e-9 {
		t.Fatalf("unexpected player volume: %f", player.Volume())
	}

	scaled := make([]float32, 256)
	if _, err := player.Render(scaled); err != nil {
		t.Fatalf("Render failed: %v", err)
	}
	scaledPeak := peakFloat32(scaled)
	if scaledPeak >= basePeak {
		t.Fatalf("expected scaled output to be quieter: base=%f scaled=%f", basePeak, scaledPeak)
	}

	if err := player.Seek(0); err != nil {
		t.Fatalf("Seek failed: %v", err)
	}

	stream := player.Stream()
	first := make([]byte, 64)
	if _, err := io.ReadFull(stream, first); err != nil {
		t.Fatalf("stream Read failed: %v", err)
	}

	if _, err := stream.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("stream Seek failed: %v", err)
	}
	again := make([]byte, 64)
	if _, err := io.ReadFull(stream, again); err != nil {
		t.Fatalf("stream Read failed: %v", err)
	}
	if !bytes.Equal(first, again) {
		t.Fatalf("expected deterministic stream rewind")
	}

	target := 10 * time.Millisecond
	if err := player.Seek(target); err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	if player.Position() < target-time.Millisecond || player.Position() > target+time.Millisecond {
		t.Fatalf("unexpected seek position: got=%s target=%s", player.Position(), target)
	}
}

func TestPlayerSeekMatchesRenderedPosition(t *testing.T) {
	modData := testfixtures.MinimalXM()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	cfg := DefaultConfig()

	reference, err := NewPlayer(mod, cfg)
	if err != nil {
		t.Fatalf("NewPlayer(reference) failed: %v", err)
	}
	const seekDuration = 20 * time.Millisecond
	seekFrames := int(seekDuration) * cfg.SampleRate / int(time.Second)
	discard := make([]float32, seekFrames*cfg.Channels)
	if _, err := reference.Render(discard); err != nil {
		t.Fatalf("reference Render failed: %v", err)
	}
	want := make([]float32, 2048)
	if _, err := reference.Render(want); err != nil {
		t.Fatalf("reference continuation failed: %v", err)
	}

	seeker, err := NewPlayer(mod, cfg)
	if err != nil {
		t.Fatalf("NewPlayer(seeker) failed: %v", err)
	}
	if err := seeker.Seek(seekDuration); err != nil {
		t.Fatalf("Seek failed: %v", err)
	}
	got := make([]float32, len(want))
	if _, err := seeker.Render(got); err != nil {
		t.Fatalf("Render after Seek failed: %v", err)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("post-seek sample %d differs: got=%f want=%f", i, got[i], want[i])
		}
	}
}

func TestPCMStreamReadDoesNotAllocateAfterWarmup(t *testing.T) {
	modData := testfixtures.MinimalXM()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	player, err := NewPlayer(mod, DefaultConfig())
	if err != nil {
		t.Fatalf("NewPlayer failed: %v", err)
	}
	stream := player.Stream()
	buffer := make([]byte, 4093)
	if _, err := stream.Read(buffer); err != nil {
		t.Fatalf("warmup Read failed: %v", err)
	}

	var readErr error
	allocs := testing.AllocsPerRun(100, func() {
		_, readErr = stream.Read(buffer)
	})
	if readErr != nil {
		t.Fatalf("Read failed: %v", readErr)
	}
	if allocs != 0 {
		t.Fatalf("stream Read allocated after warmup: %f allocs/run", allocs)
	}
}

func TestPCMStreamHandlesUnalignedReads(t *testing.T) {
	modData := testfixtures.MinimalXM()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	const outputSize = 257

	referencePlayer, err := NewPlayer(mod, DefaultConfig())
	if err != nil {
		t.Fatalf("NewPlayer(reference) failed: %v", err)
	}
	want := make([]byte, outputSize)
	if _, err := io.ReadFull(referencePlayer.Stream(), want); err != nil {
		t.Fatalf("reference Read failed: %v", err)
	}

	for _, chunkSize := range []int{1, 2, 3, 5, 7, 11} {
		player, err := NewPlayer(mod, DefaultConfig())
		if err != nil {
			t.Fatalf("NewPlayer(chunk=%d) failed: %v", chunkSize, err)
		}
		stream := player.Stream()
		got := make([]byte, outputSize)
		for offset := 0; offset < len(got); {
			end := minInt(offset+chunkSize, len(got))
			if _, err := io.ReadFull(stream, got[offset:end]); err != nil {
				t.Fatalf("Read(chunk=%d) failed: %v", chunkSize, err)
			}
			offset = end
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("unaligned stream differs for chunk size %d", chunkSize)
		}
	}
}

func hasNonZeroFloat32(values []float32) bool {
	for _, value := range values {
		if value != 0 {
			return true
		}
	}
	return false
}

func hasNonZeroInt16(values []int16) bool {
	for _, value := range values {
		if value != 0 {
			return true
		}
	}
	return false
}

func peakFloat32(values []float32) float32 {
	var peak float32
	for _, value := range values {
		abs := value
		if abs < 0 {
			abs = -abs
		}
		if abs > peak {
			peak = abs
		}
	}
	return peak
}

func TestDetectRecognizesKnownFormats(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want Format
	}{
		{name: "mod", data: testfixtures.MOD("M.K."), want: FormatMOD},
		{name: "s3m", data: testfixtures.S3M(), want: FormatS3M},
		{name: "xm", data: testfixtures.XM(), want: FormatXM},
		{name: "it", data: testfixtures.IT(), want: FormatIT},
		{name: "unknown", data: testfixtures.Unknown(), want: FormatUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Detect(bytes.NewReader(tt.data), int64(len(tt.data)))
			if err != nil {
				t.Fatalf("Detect failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected format: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestLoadReturnsTypedErrors(t *testing.T) {
	modData := testfixtures.MinimalMOD()
	mod, err := Load(bytes.NewReader(modData), int64(len(modData)))
	if err != nil {
		t.Fatalf("expected MOD load success, got %v", err)
	}
	if mod.Metadata.Format != FormatMOD || mod.Metadata.Channels != 4 || mod.Metadata.Samples != 31 {
		t.Fatalf("unexpected MOD metadata: %+v", mod.Metadata)
	}

	s3mData := testfixtures.MinimalS3M()
	s3m, err := Load(bytes.NewReader(s3mData), int64(len(s3mData)))
	if err != nil {
		t.Fatalf("expected S3M load success, got %v", err)
	}
	if s3m.Metadata.Format != FormatS3M || s3m.Metadata.Channels != 2 || s3m.Metadata.Orders != 2 || s3m.Metadata.Samples != 1 {
		t.Fatalf("unexpected S3M metadata: %+v", s3m.Metadata)
	}

	xmData := testfixtures.MinimalXM()
	xm, err := Load(bytes.NewReader(xmData), int64(len(xmData)))
	if err != nil {
		t.Fatalf("expected XM load success, got %v", err)
	}
	if xm.Metadata.Format != FormatXM || xm.Metadata.Channels != 2 || xm.Metadata.Orders != 1 || xm.Metadata.Instruments != 1 || xm.Metadata.Samples != 1 {
		t.Fatalf("unexpected XM metadata: %+v", xm.Metadata)
	}

	itData := testfixtures.MinimalIT()
	it, err := Load(bytes.NewReader(itData), int64(len(itData)))
	if err != nil {
		t.Fatalf("expected IT load success, got %v", err)
	}
	if it.Metadata.Format != FormatIT || it.Metadata.Channels != 2 || it.Metadata.Orders != 1 || it.Metadata.Instruments != 1 || it.Metadata.Samples != 1 {
		t.Fatalf("unexpected IT metadata: %+v", it.Metadata)
	}

	_, err = Load(bytes.NewReader(testfixtures.Unknown()), int64(len(testfixtures.Unknown())))
	if err == nil {
		t.Fatal("expected an error for unsupported format")
	}
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}
