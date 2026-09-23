package st3

type sampleMarker struct {
	sampleCountStart uint32
	row              uint8
	order            uint8
	frame            uint32
}

func (s *state) insertSampleMarker() {
	if s.sampleMarkerCount < uint16(len(s.sampleMarkers)) {
		idx := s.sampleMarkerCount
		s.sampleMarkers[idx] = sampleMarker{
			sampleCountStart: s.totalSampleCount,
			order:            uint8(s.npOrd),
			row:              uint8(s.npRow),
			frame:            s.npZframe,
		}
		s.sampleMarkerCount++
		return
	}

	s.sampleMarkers[s.sampleMarkerIndex] = sampleMarker{
		sampleCountStart: s.totalSampleCount,
		order:            uint8(s.npOrd),
		row:              uint8(s.npRow),
		frame:            s.npZframe,
	}
	s.sampleMarkerIndex = (s.sampleMarkerIndex + 1) & uint16(len(s.sampleMarkers)-1)
}

func (s *state) markerAt(i int) sampleMarker {
	if len(s.sampleMarkers) == 0 {
		return sampleMarker{}
	}
	idx := (int(s.sampleMarkerIndex) + i) & (len(s.sampleMarkers) - 1)
	return s.sampleMarkers[idx]
}

func (s *state) orderRowFrameAt(samplePosition uint32) (uint16, uint16, uint32) {
	if s.sampleMarkerCount == 0 {
		return 0, 0, 0
	}

	first := s.markerAt(0)
	if s.sampleMarkerCount == 1 {
		return uint16(first.order) - 1, uint16(first.row), first.frame
	}
	last := s.markerAt(int(s.sampleMarkerCount - 1))
	if samplePosition <= first.sampleCountStart {
		return uint16(first.order) - 1, uint16(first.row), first.frame
	}
	if samplePosition >= last.sampleCountStart {
		return uint16(last.order) - 1, uint16(last.row), last.frame
	}

	i0 := 0
	i1 := int(s.sampleMarkerCount - 1)
	for i1-i0 > 1 {
		half := (i1 + i0) / 2
		if s.markerAt(half).sampleCountStart <= samplePosition {
			i0 = half
		} else {
			i1 = half
		}
	}

	m := s.markerAt(i0)
	return uint16(m.order) - 1, uint16(m.row), m.frame
}

func (s *state) orderRowFrame() (uint16, uint16, uint32) {
	return s.orderRowFrameAt(s.totalSampleCount)
}

func (s *state) plusFlags() int16 {
	return s.plusFlagsAt(s.totalSampleCount)
}

func (s *state) plusFlagsAt(samplePosition uint32) int16 {
	order, row, _ := s.orderRowFrameAt(samplePosition)
	flags := uint8(0)

	if int(order)+1 < len(s.order) && s.order[order+1] == pattSep {
		flags |= 1
	}
	if order > 0 && s.order[order-1] == pattSep {
		flags |= 2
	}

	switch flags {
	case 0:
		return -32
	case 1:
		val := int16(row) - 64
		if val < -32 {
			return -32
		}
		return val
	case 2:
		if row >= 32 {
			return -32
		}
		return int16(row)
	case 3:
		val := int16(row)
		if val < -32 {
			return val
		}
		return val - 64
	}

	return 0
}
