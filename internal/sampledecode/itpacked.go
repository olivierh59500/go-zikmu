package sampledecode

import (
	"encoding/binary"
	"fmt"

	"github.com/olivierh59500/go-zikmu/internal/module"
)

func decodeITPacked(data []byte, sampleCount int, flags module.SampleFlags) ([]int16, error) {
	samples := make([]int16, sampleCount)
	cursor := 0
	written := 0

	for written < sampleCount {
		if cursor+2 > len(data) {
			return nil, fmt.Errorf("sampledecode: missing IT block header: unexpected end of data")
		}
		compressedLen := int(binary.LittleEndian.Uint16(data[cursor:]))
		cursor += 2
		if compressedLen > len(data)-cursor {
			return nil, fmt.Errorf("sampledecode: truncated IT block: have=%d want=%d", len(data)-cursor, compressedLen)
		}
		block := data[cursor : cursor+compressedLen]
		cursor += compressedLen

		remaining := sampleCount - written
		if flags&module.Sample16Bits != 0 {
			if remaining > 0x4000 {
				remaining = 0x4000
			}
			if err := decodeITPacked16Block(block, samples[written:written+remaining]); err != nil {
				return nil, err
			}
		} else {
			if remaining > 0x8000 {
				remaining = 0x8000
			}
			if err := decodeITPacked8Block(block, samples[written:written+remaining]); err != nil {
				return nil, err
			}
		}
		written += remaining
	}

	return samples, nil
}

func decodeITPacked8Block(block []byte, out []int16) error {
	br := bitReader{data: block}

	bits := 9
	newCount := false
	var last int8
	written := 0

	for written < len(out) {
		needBits := bits
		if newCount {
			needBits = 3
		}

		x, err := br.readBits(needBits)
		if err != nil {
			return fmt.Errorf("sampledecode: invalid IT 8-bit data: %w", err)
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
			return fmt.Errorf("sampledecode: invalid IT 8-bit bit width %d", bits)
		}

		out[written] = int16(last) << 8
		written++
	}

	return nil
}

func decodeITPacked16Block(block []byte, out []int16) error {
	br := bitReader{data: block}

	bits := 17
	newCount := false
	var last int16
	written := 0

	for written < len(out) {
		needBits := bits
		if newCount {
			needBits = 4
		}

		x, err := br.readBits(needBits)
		if err != nil {
			return fmt.Errorf("sampledecode: invalid IT 16-bit data: %w", err)
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
			return fmt.Errorf("sampledecode: invalid IT 16-bit bit width %d", bits)
		}

		out[written] = last
		written++
	}

	return nil
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
