package st3

import (
	"encoding/binary"
	"errors"
)

const (
	sampleUnrollBytes = 512 + 2
	samplePad8        = 4
	samplePad16       = 2
)

func (s *state) loadS3M(dat []byte, startingOrder uint32) error {
	if len(dat) < 0x70 {
		return errors.New("st3: invalid s3m")
	}
	if dat[0x1D] != 16 {
		return errors.New("st3: invalid s3m")
	}
	if string(dat[0x2C:0x30]) != "SCRM" {
		return errors.New("st3: invalid s3m")
	}

	copy(s.songName[:], dat[:28])
	s.songName[28] = 0

	signedSamples := readU16(dat, 0x2A) == 1

	s.ordNum = readU16(dat, 0x20)
	if s.ordNum > 256 {
		s.ordNum = 256
	}
	s.insNum = readU16(dat, 0x22)
	if s.insNum > 100 {
		s.insNum = 100
	}
	s.patNum = readU16(dat, 0x24)
	if s.patNum > 100 {
		s.patNum = 100
	}

	clear(s.order[:])
	clear(s.chnsettings[:])
	for i := range s.patdata {
		s.patdata[i] = nil
	}
	for i := range s.ins {
		s.ins[i] = instrument{}
	}

	orderStart := 0x60
	orderEnd := orderStart + int(s.ordNum)
	if orderEnd > len(dat) {
		return errors.New("st3: invalid s3m")
	}
	copy(s.order[:], dat[orderStart:orderEnd])

	chnStart := 0x40
	chnEnd := chnStart + 32
	if chnEnd > len(dat) {
		return errors.New("st3: invalid s3m")
	}
	copy(s.chnsettings[:], dat[chnStart:chnEnd])

	// load instrument headers
	for i := 0; i < int(s.insNum); i++ {
		offs := uint32(readU16(dat, orderStart+int(s.ordNum)+i*2)) << 4
		if offs == 0 {
			continue
		}
		if int(offs)+0x24 > len(dat) {
			return errors.New("st3: invalid s3m")
		}
		ptr := dat[offs:]
		inst := &s.ins[i]

		inst.typ = ptr[0x00]
		inst.length = readU32(ptr, 0x10)
		inst.lbeg = readU32(ptr, 0x14)
		inst.lend = readU32(ptr, 0x18)
		inst.vol = int8(clamp(int(ptr[0x1C]), 0, 63))
		inst.flags = ptr[0x1F]

		c2spd := readU32(ptr, 0x20)
		if c2spd > 65535 {
			c2spd = 65535
		}
		inst.c2spd = uint16(c2spd)

		offsSample := uint32(ptr[0x0D])<<16 | uint32(ptr[0x0F])<<8 | uint32(ptr[0x0E])
		offsSample <<= 4

		bytesPerSample := uint32(1)
		if inst.flags&4 != 0 {
			bytesPerSample = 2
		}

		if offsSample > 0 && uint64(offsSample)+uint64(inst.length)*uint64(bytesPerSample) >= uint64(len(dat)) {
			if offsSample < uint32(len(dat)) {
				inst.length = (uint32(len(dat)) - offsSample) / bytesPerSample
			} else {
				inst.length = 0
			}
		}

		if inst.lend == inst.lbeg {
			inst.flags &^= 1
		}
		if inst.lend < inst.lbeg {
			inst.lend = inst.lbeg + 1
		}
		if inst.lend > inst.length {
			inst.lend = inst.length
		}
		if inst.lend <= inst.lbeg {
			inst.flags &^= 1
		}
	}

	// load pattern data
	patTableOff := orderStart + int(s.ordNum) + int(s.insNum)*2
	for i := 0; i < int(s.patNum); i++ {
		offs := uint32(readU16(dat, patTableOff+i*2)) << 4
		if offs == 0 {
			continue
		}
		if int(offs)+2 > len(dat) {
			return errors.New("st3: invalid s3m")
		}
		patDataLen := readU16(dat, int(offs))
		if patDataLen == 0 {
			continue
		}
		if int(offs)+2+int(patDataLen) > len(dat) {
			return errors.New("st3: invalid s3m")
		}
		buf := make([]byte, patDataLen)
		copy(buf, dat[int(offs)+2:int(offs)+2+int(patDataLen)])
		s.patdata[i] = buf
	}

	// load sample data
	for i := 0; i < int(s.insNum); i++ {
		inst := &s.ins[i]
		if inst.length == 0 || inst.typ != 1 {
			continue
		}

		offsHeader := uint32(readU16(dat, orderStart+int(s.ordNum)+i*2)) << 4
		if offsHeader == 0 || int(offsHeader)+0x1F >= len(dat) {
			continue
		}
		if dat[offsHeader+0x1E] != 0 {
			continue
		}

		offsSample := (uint32(dat[offsHeader+0x0D])<<16 | uint32(dat[offsHeader+0x0F])<<8 | uint32(dat[offsHeader+0x0E])) << 4
		if offsSample == 0 || int(offsSample) >= len(dat) {
			continue
		}

		hasLoop := inst.flags&1 != 0
		is16bit := inst.flags&4 != 0

		if hasLoop && inst.length > inst.lend {
			inst.length = inst.lend
		}

		if is16bit {
			raw := make([]int16, samplePad16+int(inst.length)+sampleUnrollBytes)
			inst.raw16 = raw
			inst.data16 = raw[samplePad16:]
			for j := 0; j < int(inst.length); j++ {
				idx := int(offsSample) + j*2
				if idx+2 > len(dat) {
					break
				}
				u := binary.LittleEndian.Uint16(dat[idx : idx+2])
				if signedSamples {
					inst.data16[j] = int16(u)
				} else {
					inst.data16[j] = int16(int32(u) - 32768)
				}
			}
		} else {
			raw := make([]int8, samplePad8+int(inst.length)+sampleUnrollBytes)
			inst.raw8 = raw
			inst.data8 = raw[samplePad8:]
			for j := 0; j < int(inst.length); j++ {
				idx := int(offsSample) + j
				if idx >= len(dat) {
					break
				}
				if signedSamples {
					inst.data8[j] = int8(dat[idx])
				} else {
					inst.data8[j] = int8(int16(dat[idx]) - 128)
				}
			}
		}
	}

	if !s.optimizeSampleDatasForMixer() {
		return errors.New("st3: sample optimize failed")
	}

	// scan for panning/surround commands
	for i := 0; i < int(s.patNum); i++ {
		s.npPatseg = s.patdata[i]
		if s.npPatseg == nil {
			continue
		}
		s.npPatoff = 0
		for row := 0; row < 64; {
			ch := s.getNote1()
			if ch != 255 {
				c := &s.chn[ch]
				if c.cmd == 'S'-64 {
					if (c.info&0xF0) == 0x80 || c.info == 0x90 || c.info == 0x91 {
						s.soundcardtype = soundcardGUS
						i = int(s.patNum)
						break
					}
				} else if c.cmd == 'X'-64 && (c.info == 0xA4 || c.info <= 0x7F) {
					s.soundcardtype = soundcardGUS
					i = int(s.patNum)
					break
				}
			} else {
				row++
			}
		}
	}
	s.npPatoff = 0
	s.npPatseg = nil

	// set up pans
	for i := 0; i < 32; i++ {
		ch := &s.chn[i]
		ch.apanpos = 0x77
		if s.chnsettings[i] != 0xFF {
			if (s.chnsettings[i] & 8) != 0 {
				ch.apanpos = 0xCC
			} else {
				ch.apanpos = 0x33
			}
		}

		if dat[0x35] == 252 {
			s.soundcardtype = soundcardGUS
			panIndex := orderStart + int(s.ordNum) + int(s.insNum)*2 + int(s.patNum)*2 + i
			if panIndex < len(dat) {
				pan := dat[panIndex]
				if pan&32 != 0 {
					ch.apanpos = ((pan & 0x0F) << 4) | (pan & 0x0F)
				}
			}
		}
	}

	s.setSpeed(6)
	s.setTempo(125)
	s.setGlobalVol(64)

	s.amigalimits = (dat[0x26] & 0x10) != 0
	s.oldstvib = (dat[0x26] & 0x01) != 0
	s.mastermul = int32(dat[0x33])
	s.fastvolslide = readU16(dat, 0x28) == 0x1300 || (dat[0x26]&0x40) != 0

	if signedSamples {
		switch s.mastermul {
		case 0:
			s.mastermul = 0x10
		case 1:
			s.mastermul = 0x20
		case 2:
			s.mastermul = 0x30
		case 3:
			s.mastermul = 0x40
		case 4:
			s.mastermul = 0x50
		case 5:
			s.mastermul = 0x60
		case 6:
			s.mastermul = 0x70
		case 7:
			s.mastermul = 0x7F
		}
	}

	if s.mastermul == 2 {
		s.mastermul = 0x20
	}
	if s.mastermul == 2+16 {
		s.mastermul = 0x20 + 128
	}
	s.mastermul &= 127
	if s.mastermul == 0 {
		s.mastermul = 48
	}
	if s.soundcardtype == soundcardGUS {
		s.mastermul = 32
	}

	if dat[0x32] > 0 {
		s.setTempo(dat[0x32])
	}
	if dat[0x30] != 255 {
		s.setGlobalVol(int8(dat[0x30]))
	}
	if dat[0x31] > 0 && dat[0x31] != 255 {
		s.setSpeed(dat[0x31])
	}

	if s.amigalimits {
		s.aspdmin = 907 / 2
		s.aspdmax = 1712 * 2
	} else {
		s.aspdmin = 64
		s.aspdmax = 32767
	}

	for i := 0; i < 32; i++ {
		s.chn[i].channelnum = uint8(i)
		s.chn[i].achannelused = 0x80
	}

	s.npPatseg = nil
	s.musiccount = 0
	s.patterndelay = 0
	s.patloopcount = 0
	s.startrow = 0
	s.breakpat = 0
	s.volslidetype = 0
	s.npZframe = 0
	s.npPatoff = -1
	s.jmptoord = -1

	if startingOrder > uint32(s.ordNum) {
		s.npOrd = 0
	} else {
		s.npOrd = int16(startingOrder)
	}

	_ = s.newOrder()
	s.lastachannelused = 1

	s.mixingVol = s.mastermul * s.mastervol * (512 + 64)
	s.randSeed = initialDitherSeed
	s.prngStateL = 0
	s.prngStateR = 0

	return nil
}

func readU16(b []byte, off int) uint16 {
	if off < 0 || off+2 > len(b) {
		return 0
	}
	return binary.LittleEndian.Uint16(b[off : off+2])
}

func readU32(b []byte, off int) uint32 {
	if off < 0 || off+4 > len(b) {
		return 0
	}
	return binary.LittleEndian.Uint32(b[off : off+4])
}
