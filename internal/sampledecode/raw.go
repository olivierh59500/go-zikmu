package sampledecode

import (
	"encoding/binary"
	"fmt"

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

	samples := make([]int16, sampleCount)
	data = data[:sampleCount*2]
	if flags&module.SampleBigEndian != 0 {
		for i := range samples {
			samples[i] = int16(binary.BigEndian.Uint16(data[i*2:]))
		}
		return samples, nil
	}
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples, nil
}
