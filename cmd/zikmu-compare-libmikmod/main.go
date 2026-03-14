package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/olivierh59500/go-zikmu"
	"github.com/olivierh59500/go-zikmu/internal/validation"
)

type input struct {
	Name   string
	Format zikmu.Format
	Data   []byte
	Path   string
}

func main() {
	var renderer string
	var frames int
	var fixture string
	var dump int
	var dumpStart int
	var goMixer string
	var startFrame int
	var startSecond float64

	flag.StringVar(&renderer, "renderer", "", "path to the compiled libmikmod renderer helper")
	flag.IntVar(&frames, "frames", validation.DefaultFrames, "number of stereo frames to compare")
	flag.StringVar(&fixture, "fixture", "", "single regression fixture name to compare")
	flag.IntVar(&dump, "dump", 0, "number of PCM16 samples to dump for both renderers")
	flag.IntVar(&dumpStart, "dump-start", 0, "starting PCM16 sample index for both dumps")
	flag.StringVar(&goMixer, "go-mixer", "stable", "go-zikmu mixer variant: stable or xm-hq")
	flag.IntVar(&startFrame, "start-frame", 0, "starting stereo frame offset for the comparison window")
	flag.Float64Var(&startSecond, "start-second", 0, "starting second offset for the comparison window")
	flag.Parse()

	if renderer == "" {
		exitf("missing -renderer")
	}
	if frames <= 0 {
		exitf("invalid -frames value %d", frames)
	}
	if startSecond < 0 {
		exitf("invalid -start-second value %f", startSecond)
	}
	if startSecond > 0 {
		if startFrame != 0 {
			exitf("use either -start-frame or -start-second, not both")
		}
		startFrame = int(math.Round(startSecond * float64(validation.DefaultConfig().SampleRate)))
	}
	if startFrame < 0 {
		exitf("invalid -start-frame value %d", startFrame)
	}

	opts := validation.RenderOptions{}
	switch goMixer {
	case "stable":
	case "xm-hq":
		opts.ExperimentalXMHighQuality = true
	default:
		exitf("invalid -go-mixer value %q", goMixer)
	}

	inputs, err := collectInputs(flag.Args(), fixture)
	if err != nil {
		exitf("%v", err)
	}
	if len(inputs) == 0 {
		exitf("no module to compare")
	}

	tempDir, err := os.MkdirTemp("", "zikmu-libmikmod-compare-*")
	if err != nil {
		exitf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	totalFrames := startFrame + frames
	fmt.Printf("renderer=%s go_mixer=%s start_frame=%d frames=%d total_frames=%d\n", renderer, goMixer, startFrame, frames, totalFrames)
	for _, item := range inputs {
		result, err := validation.RenderPCM16WithOptions(item.Data, validation.DefaultConfig(), totalFrames, opts)
		if err != nil {
			exitf("%s: go-zikmu render failed: %v", item.Name, err)
		}
		result.PCM16 = slicePCMWindow(result.PCM16, startFrame, frames, validation.DefaultConfig().Channels)
		result.Frames = len(result.PCM16) / (validation.DefaultConfig().Channels * 2)
		result.SHA256 = hashPCM(result.PCM16)

		modulePath := item.Path
		if modulePath == "" {
			modulePath = filepath.Join(tempDir, item.Name+validation.FileSuffix(item.Format))
			if err := os.WriteFile(modulePath, item.Data, 0o600); err != nil {
				exitf("%s: writing temp module: %v", item.Name, err)
			}
		}

		wavPath := filepath.Join(tempDir, item.Name+".wav")
		if err := renderWithLibMikMod(renderer, modulePath, wavPath, totalFrames); err != nil {
			exitf("%s: libmikmod render failed: %v", item.Name, err)
		}
		refPCM, err := readWAVPCM16(wavPath)
		if err != nil {
			exitf("%s: decoding wav: %v", item.Name, err)
		}
		refPCM = slicePCMWindow(refPCM, startFrame, frames, validation.DefaultConfig().Channels)

		comparison, err := validation.ComparePCM16(result.PCM16, refPCM)
		if err != nil {
			exitf("%s: compare failed: %v", item.Name, err)
		}

		refHash := hashPCM(refPCM)
		fmt.Printf(
			"%s format=%s go=%s mikmod=%s compared=%d left=%d right=%d diff=%d first_diff=%d max=%d mean=%.3f\n",
			item.Name,
			item.Format,
			result.SHA256,
			refHash,
			comparison.ComparedSamples,
			comparison.LeftSamples,
			comparison.RightSamples,
			comparison.Different,
			comparison.FirstDifferent,
			comparison.MaxAbsDelta,
			comparison.MeanAbsDelta,
		)
		if dump > 0 {
			dumpSamples("go-zikmu", result.PCM16, dumpStart, dump)
			dumpSamples("libmikmod", refPCM, dumpStart, dump)
		}
	}
}

func collectInputs(paths []string, fixture string) ([]input, error) {
	if len(paths) == 0 {
		var inputs []input
		for _, entry := range validation.RegressionCorpus() {
			if fixture != "" && entry.Name != fixture {
				continue
			}
			inputs = append(inputs, input{
				Name:   entry.Name,
				Format: entry.Format,
				Data:   entry.Data,
			})
		}
		if fixture != "" && len(inputs) == 0 {
			return nil, fmt.Errorf("unknown fixture %q", fixture)
		}
		return inputs, nil
	}

	inputs := make([]input, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		module, err := zikmu.Load(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", path, err)
		}
		inputs = append(inputs, input{
			Name:   filepath.Base(path),
			Format: module.Metadata.Format,
			Data:   data,
			Path:   path,
		})
	}

	return inputs, nil
}

func renderWithLibMikMod(renderer, modulePath, wavPath string, frames int) error {
	absModulePath, err := filepath.Abs(modulePath)
	if err != nil {
		return err
	}

	outputDir := filepath.Dir(wavPath)
	outputName := filepath.Base(wavPath)
	cmd := exec.Command(renderer, "-input", absModulePath, "-output", outputName, "-frames", fmt.Sprintf("%d", frames))
	cmd.Dir = outputDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	if _, err := os.Stat(wavPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	fallback := filepath.Join(outputDir, "music.wav")
	if _, err := os.Stat(fallback); err != nil {
		return err
	}
	return os.Rename(fallback, wavPath)
}

func readWAVPCM16(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errors.New("invalid wav header")
	}

	var bitsPerSample uint16
	var pcmData []byte
	for offset := 12; offset+8 <= len(data); {
		chunkID := string(data[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(data[offset+4:]))
		offset += 8
		if offset+chunkSize > len(data) {
			return nil, errors.New("truncated wav chunk")
		}

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return nil, errors.New("short fmt chunk")
			}
			audioFormat := binary.LittleEndian.Uint16(data[offset:])
			channels := binary.LittleEndian.Uint16(data[offset+2:])
			sampleRate := binary.LittleEndian.Uint32(data[offset+4:])
			bitsPerSample = binary.LittleEndian.Uint16(data[offset+14:])
			if audioFormat != 1 || channels != 2 || sampleRate != 44100 {
				return nil, fmt.Errorf("unexpected wav format pcm=%d channels=%d rate=%d", audioFormat, channels, sampleRate)
			}
		case "data":
			pcmData = append([]byte(nil), data[offset:offset+chunkSize]...)
		}

		offset += chunkSize
		if chunkSize%2 != 0 {
			offset++
		}
	}

	if bitsPerSample != 16 {
		return nil, fmt.Errorf("unexpected wav bit depth %d", bitsPerSample)
	}
	if len(pcmData) == 0 {
		return nil, errors.New("missing data chunk")
	}
	return pcmData, nil
}

func hashPCM(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func dumpSamples(label string, data []byte, start, count int) {
	if count <= 0 {
		return
	}
	if start < 0 {
		start = 0
	}
	samples := len(data) / 2
	if start > samples {
		start = samples
	}
	limit := count
	if remaining := samples - start; remaining < limit {
		limit = remaining
	}
	fmt.Printf("%s samples[%d:%d]:", label, start, start+limit)
	for i := 0; i < limit; i++ {
		index := start + i
		value := int16(binary.LittleEndian.Uint16(data[index*2:]))
		fmt.Printf(" %d", value)
	}
	fmt.Println()
}

func slicePCMWindow(data []byte, startFrame, frames, channels int) []byte {
	if startFrame < 0 {
		startFrame = 0
	}
	if frames < 0 {
		frames = 0
	}
	if channels <= 0 {
		return nil
	}
	bytesPerFrame := channels * 2
	start := startFrame * bytesPerFrame
	if start >= len(data) {
		return nil
	}
	end := start + frames*bytesPerFrame
	if end > len(data) {
		end = len(data)
	}
	return append([]byte(nil), data[start:end]...)
}

func exitf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
