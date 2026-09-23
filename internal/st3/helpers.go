package st3

func (s *state) setSpeed(val uint8) {
	if val > 0 {
		s.musicmax = val
	}
}

func (s *state) setTempo(bpm uint8) {
	if bpm <= 32 {
		return
	}
	s.samplesPerTick = (s.audioRate * 125) / (uint32(bpm) * 50)
}

func (s *state) setGlobalVol(vol int8) {
	s.globalvol = int16(vol)
	if vol > 64 {
		vol = 64
	}
	if vol < 0 {
		vol = 0
	}
	s.useglobalvol = uint16(vol)
}
