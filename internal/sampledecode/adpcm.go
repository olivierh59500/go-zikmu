package sampledecode

import "fmt"

func decodeADPCM4(data []byte, sampleCount int) ([]int16, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("sampledecode: ADPCM4 sample missing 16-byte table")
	}

	table := data[:16]
	payload := data[16:]
	neededBytes := (sampleCount + 1) / 2
	if len(payload) < neededBytes {
		return nil, fmt.Errorf("sampledecode: ADPCM4 payload truncated: have=%d want=%d", len(payload), neededBytes)
	}

	samples := make([]int16, 0, sampleCount)
	var delta int16
	for i := 0; i < neededBytes && len(samples) < sampleCount; i++ {
		b := payload[i]

		delta = int16(uint16(delta) + uint16(int16(int8(table[b&0x0f]))))
		samples = append(samples, delta<<8)

		if len(samples) == sampleCount {
			break
		}

		delta = int16(uint16(delta) + uint16(int16(int8(table[(b>>4)&0x0f]))))
		samples = append(samples, delta<<8)
	}

	return samples, nil
}
