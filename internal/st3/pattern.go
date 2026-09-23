package st3

func (s *state) newOrder() int16 {
	var numSep uint16
	for {
		s.npOrd++
		if s.npOrd <= 0 || int(s.npOrd-1) >= len(s.order) {
			return 0
		}
		patt := s.order[s.npOrd-1]
		if patt == pattSep {
			numSep++
			if numSep >= s.ordNum {
				return 0
			}
			continue
		}
		if patt == pattEnd {
			s.npOrd = 0
			if s.order[0] == pattEnd {
				return 0
			}
			continue
		}
		s.npPat = int16(patt)
		break
	}

	s.npPatoff = -1
	s.npRow = int16(s.startrow)
	s.startrow = 0
	s.patmusicrand = 0
	s.patloopstart = -1
	s.jumptorow = -1

	return s.npRow
}

func (s *state) readPatByte(i *int) uint8 {
	if s.npPatseg == nil {
		*i = *i + 1
		return 0
	}
	if *i < 0 || *i >= len(s.npPatseg) {
		*i = *i + 1
		return 0
	}
	idx := *i
	val := int(s.npPatseg[idx])
	if s.packedPatterns {
		val ^= (idx + 2) ^ ((idx + 2) * 4)
	}
	*i++
	return uint8(val & 0xFF)
}

func (s *state) getNote1() uint8 {
	if s.npPatseg == nil {
		return 255
	}
	if s.npPat >= int16(s.patNum) {
		return 255
	}
	if s.npPatoff < 0 {
		return 255
	}

	i := int(s.npPatoff)
	var dat uint8

	for {
		dat = s.readPatByte(&i)
		if dat == 0 {
			s.npPatoff = int16(i)
			return 255
		}
		if (s.chnsettings[dat&0x1F] & 0x80) == 0 {
			break
		}
		if dat&0x20 != 0 {
			i += 2
		}
		if dat&0x40 != 0 {
			i++
		}
		if dat&0x80 != 0 {
			i += 2
		}
		if i >= len(s.npPatseg) {
			s.npPatoff = int16(i)
			return 255
		}
	}

	channel := dat & 0x1F
	ch := &s.chn[channel]

	if dat&0x20 != 0 {
		ch.note = s.readPatByte(&i)
		ch.ins = s.readPatByte(&i)
		if ch.note != 255 {
			ch.lastnote = ch.note
		}
		if ch.ins > 0 {
			ch.lastins = ch.ins
		}
	}

	if dat&0x40 != 0 {
		ch.vol = s.readPatByte(&i)
	}

	if dat&0x80 != 0 {
		ch.cmd = s.readPatByte(&i)
		ch.info = s.readPatByte(&i)
	}

	s.npPatoff = int16(i)
	return channel
}
