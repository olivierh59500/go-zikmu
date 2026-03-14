package sampledecode

import (
	"fmt"

	"github.com/olivierh59500/go-zikmu/internal/module"
)

type Options struct {
	DownmixToMono bool
	ScaleFactor   int
}

func Decode(data []byte, sampleCount uint32, flags module.SampleFlags, opts Options) ([]int16, error) {
	if sampleCount == 0 {
		return nil, nil
	}

	if opts.ScaleFactor < 0 {
		return nil, fmt.Errorf("sampledecode: invalid scale factor %d", opts.ScaleFactor)
	}

	samples, err := decodePCM(data, int(sampleCount), flags)
	if err != nil {
		return nil, err
	}

	if flags&module.SampleDelta != 0 {
		applyDelta(samples)
	}

	if flags&module.SampleSigned == 0 {
		applyUnsignedToSigned(samples)
	}

	if opts.DownmixToMono && flags&module.SampleStereo != 0 {
		var downmixErr error
		samples, downmixErr = downmixStereoToMono(samples)
		if downmixErr != nil {
			return nil, downmixErr
		}
	}

	if opts.ScaleFactor > 1 {
		samples = scaleDown(samples, opts.ScaleFactor)
	}

	return samples, nil
}

func DecodeIntoSample(sample *module.Sample, data []byte, opts Options) error {
	if sample == nil {
		return fmt.Errorf("sampledecode: nil sample")
	}

	decoded, err := Decode(data, sample.Length, sample.Flags, opts)
	if err != nil {
		return err
	}

	if opts.DownmixToMono && sample.Flags&module.SampleStereo != 0 {
		sample.Flags &^= module.SampleStereo
		sample.Length /= 2
		sample.LoopStart /= 2
		sample.LoopEnd /= 2
		sample.SustainStart /= 2
		sample.SustainEnd /= 2
	}

	if opts.ScaleFactor > 1 {
		factor := uint32(opts.ScaleFactor)
		sample.DivFactor = uint8(opts.ScaleFactor)
		sample.Length /= factor
		sample.LoopStart /= factor
		sample.LoopEnd /= factor
		sample.SustainStart /= factor
		sample.SustainEnd /= factor
	}

	sample.Data = decoded
	sanitizeLoops(sample)
	return nil
}

func sanitizeLoops(sample *module.Sample) {
	if sample == nil {
		return
	}

	if sample.LoopStart >= sample.Length || sample.LoopEnd > sample.Length || sample.LoopEnd <= sample.LoopStart {
		sample.Flags &^= module.SampleLoop | module.SampleBidiLoop
		sample.LoopStart = 0
		sample.LoopEnd = 0
	}

	if sample.SustainStart >= sample.Length || sample.SustainEnd > sample.Length || sample.SustainEnd <= sample.SustainStart {
		sample.Flags &^= module.SampleSustainLoop | module.SampleSustainBidiLoop
		sample.SustainStart = 0
		sample.SustainEnd = 0
	}
}

func applyUnsignedToSigned(samples []int16) {
	for i := range samples {
		samples[i] = int16(uint16(samples[i]) ^ 0x8000)
	}
}

func applyDelta(samples []int16) {
	var prev int16
	for i, sample := range samples {
		prev = int16(uint16(prev) + uint16(sample))
		samples[i] = prev
	}
}

func downmixStereoToMono(samples []int16) ([]int16, error) {
	if len(samples)%2 != 0 {
		return nil, fmt.Errorf("sampledecode: stereo sample count must be even, got %d", len(samples))
	}

	mono := make([]int16, len(samples)/2)
	for i := 0; i < len(samples); i += 2 {
		mono[i/2] = int16((int32(samples[i]) + int32(samples[i+1])) / 2)
	}
	return mono, nil
}

func scaleDown(samples []int16, factor int) []int16 {
	if factor <= 1 || len(samples) == 0 {
		return append([]int16(nil), samples...)
	}

	out := make([]int16, 0, (len(samples)+factor-1)/factor)
	for i := 0; i < len(samples); i += factor {
		end := i + factor
		if end > len(samples) {
			end = len(samples)
		}

		var sum int32
		for _, sample := range samples[i:end] {
			sum += int32(sample)
		}
		out = append(out, int16(sum/int32(end-i)))
	}

	return out
}
