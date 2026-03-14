package sampledecode

import (
	"fmt"

	"github.com/olivierh59500/go-zikmu/internal/binary"
	"github.com/olivierh59500/go-zikmu/internal/module"
)

func decodeITPacked(data []byte, sampleCount int, flags module.SampleFlags) ([]int16, error) {
	reader := binary.NewBuffer(data)
	samples := make([]int16, 0, sampleCount)

	for len(samples) < sampleCount {
		compressedLen, err := reader.Uint16LE()
		if err != nil {
			return nil, fmt.Errorf("sampledecode: missing IT block header: %w", err)
		}

		block, err := reader.Bytes(int64(compressedLen))
		if err != nil {
			return nil, fmt.Errorf("sampledecode: truncated IT block: %w", err)
		}

		remaining := sampleCount - len(samples)
		if flags&module.Sample16Bits != 0 {
			if remaining > 0x4000 {
				remaining = 0x4000
			}
			decoded, decodeErr := decodeITPacked16Block(block, remaining)
			if decodeErr != nil {
				return nil, decodeErr
			}
			samples = append(samples, decoded...)
		} else {
			if remaining > 0x8000 {
				remaining = 0x8000
			}
			decoded, decodeErr := decodeITPacked8Block(block, remaining)
			if decodeErr != nil {
				return nil, decodeErr
			}
			samples = append(samples, decoded...)
		}
	}

	return samples, nil
}

func decodeITPacked8Block(block []byte, count int) ([]int16, error) {
	br := bitReader{data: block}
	out := make([]int16, 0, count)

	bits := 9
	newCount := false
	var last int8

	for len(out) < count {
		needBits := bits
		if newCount {
			needBits = 3
		}

		x, err := br.readBits(needBits)
		if err != nil {
			return nil, fmt.Errorf("sampledecode: invalid IT 8-bit data: %w", err)
		}

		if newCount {
			newCount = false
			x++
			if x >= uint32(bits) {
				x++
			}
			bits = int(x)
			continue
		}

		switch {
		case bits < 7:
			if x == 1<<uint(bits-1) {
				newCount = true
				continue
			}
			last = int8(uint8(last) + uint8(signExtend(x, bits)))
		case bits < 9:
			y := (0xff >> uint(9-bits)) - 4
			if x > uint32(y) && x <= uint32(y+8) {
				x -= uint32(y)
				if x >= uint32(bits) {
					x++
				}
				bits = int(x)
				continue
			}
			last = int8(uint8(last) + uint8(x))
		case bits < 10:
			if x >= 0x100 {
				bits = int(x - 0x100 + 1)
				continue
			}
			last = int8(uint8(last) + uint8(x))
		default:
			return nil, fmt.Errorf("sampledecode: invalid IT 8-bit bit width %d", bits)
		}

		out = append(out, int16(last)<<8)
	}

	return out, nil
}

func decodeITPacked16Block(block []byte, count int) ([]int16, error) {
	br := bitReader{data: block}
	out := make([]int16, 0, count)

	bits := 17
	newCount := false
	var last int16

	for len(out) < count {
		needBits := bits
		if newCount {
			needBits = 4
		}

		x, err := br.readBits(needBits)
		if err != nil {
			return nil, fmt.Errorf("sampledecode: invalid IT 16-bit data: %w", err)
		}

		if newCount {
			newCount = false
			x++
			if x >= uint32(bits) {
				x++
			}
			bits = int(x)
			continue
		}

		switch {
		case bits < 7:
			if x == 1<<uint(bits-1) {
				newCount = true
				continue
			}
			last = int16(uint16(last) + uint16(signExtend(x, bits)))
		case bits < 17:
			y := (0xffff >> uint(17-bits)) - 8
			if x > uint32(y) && x <= uint32(y+16) {
				x -= uint32(y)
				if x >= uint32(bits) {
					x++
				}
				bits = int(x)
				continue
			}
			last = int16(uint16(last) + uint16(x))
		case bits < 18:
			if x >= 0x10000 {
				bits = int(x - 0x10000 + 1)
				continue
			}
			last = int16(uint16(last) + uint16(x))
		default:
			return nil, fmt.Errorf("sampledecode: invalid IT 16-bit bit width %d", bits)
		}

		out = append(out, last)
	}

	return out, nil
}

type bitReader struct {
	data []byte
	pos  int
	bits uint
	buf  uint32
}

func (br *bitReader) readBits(n int) (uint32, error) {
	for br.bits < uint(n) {
		if br.pos >= len(br.data) {
			return 0, fmt.Errorf("unexpected end of block")
		}
		br.buf |= uint32(br.data[br.pos]) << br.bits
		br.bits += 8
		br.pos++
	}

	mask := uint32((1 << uint(n)) - 1)
	value := br.buf & mask
	br.buf >>= uint(n)
	br.bits -= uint(n)
	return value, nil
}

func signExtend(value uint32, bits int) int16 {
	shift := 16 - bits
	return int16(value<<uint(shift)) >> uint(shift)
}
