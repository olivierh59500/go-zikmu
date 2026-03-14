package sampledecode

import (
	"fmt"

	"github.com/olivierh59500/go-zikmu/internal/binary"
	"github.com/olivierh59500/go-zikmu/internal/module"
)

func decodePCM(data []byte, sampleCount int, flags module.SampleFlags) ([]int16, error) {
	switch {
	case flags&module.SampleITPacked != 0:
		return decodeITPacked(data, sampleCount, flags)
	case flags&module.SampleADPCM4 != 0:
		return decodeADPCM4(data, sampleCount)
	case flags&module.Sample16Bits != 0:
		return decodeRaw16(data, sampleCount, flags)
	default:
		return decodeRaw8(data, sampleCount)
	}
}

func decodeRaw8(data []byte, sampleCount int) ([]int16, error) {
	if len(data) < sampleCount {
		return nil, fmt.Errorf("sampledecode: raw 8-bit sample truncated: have=%d want=%d", len(data), sampleCount)
	}

	samples := make([]int16, sampleCount)
	for i := 0; i < sampleCount; i++ {
		samples[i] = int16(int8(data[i])) << 8
	}
	return samples, nil
}

func decodeRaw16(data []byte, sampleCount int, flags module.SampleFlags) ([]int16, error) {
	if len(data) < sampleCount*2 {
		return nil, fmt.Errorf("sampledecode: raw 16-bit sample truncated: have=%d want=%d", len(data), sampleCount*2)
	}

	reader := binary.NewBuffer(data)
	samples := make([]int16, sampleCount)
	for i := 0; i < sampleCount; i++ {
		var value uint16
		var err error
		if flags&module.SampleBigEndian != 0 {
			value, err = reader.Uint16BE()
		} else {
			value, err = reader.Uint16LE()
		}
		if err != nil {
			return nil, err
		}
		samples[i] = int16(value)
	}
	return samples, nil
}
