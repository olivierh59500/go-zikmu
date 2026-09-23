package st3

func (s *state) random32() uint32 {
	s.randSeed = int32(s.randSeed*134775813 + 1)
	return uint32(s.randSeed)
}

func (s *state) mixAudio(out []int16, sampleBlockLength int32) {
	if sampleBlockLength <= 0 {
		return
	}
	if s.musicPaused {
		for i := 0; i < int(sampleBlockLength)*2 && i < len(out); i++ {
			out[i] = 0
		}
		return
	}

	if int(sampleBlockLength) > len(s.mixBufferL) {
		sampleBlockLength = int32(len(s.mixBufferL))
	}

	for i := 0; i < int(sampleBlockLength); i++ {
		s.mixBufferL[i] = 0
		s.mixBufferR[i] = 0
	}

	for i := 0; i < 32; i++ {
		v := &s.voice[i]
		if v.mMixfunc != nil && v.mPos < v.mEnd {
			v.mMixfunc(s, v, sampleBlockLength)
		}
	}

	mixLen := int(sampleBlockLength)
	if mixLen*2 > len(out) {
		mixLen = len(out) / 2
	}

	for i := 0; i < mixLen; i++ {
		prng := int32(s.random32())
		out32 := (int64(s.mixBufferL[i])*int64(s.mixingVol) + int64(prng) - int64(s.prngStateL)) >> 32
		s.prngStateL = prng
		out32 = int64(clamp16(int32(out32)))
		out32 = (out32 * int64(s.mastervol)) >> 8
		out[i*2] = int16(out32)

		prng = int32(s.random32())
		out32 = (int64(s.mixBufferR[i])*int64(s.mixingVol) + int64(prng) - int64(s.prngStateR)) >> 32
		s.prngStateR = prng
		out32 = int64(clamp16(int32(out32)))
		out32 = (out32 * int64(s.mastervol)) >> 8
		out[i*2+1] = int16(out32)
	}
}

func (s *state) fillAudioBuffer(out []int16) bool {
	if len(out)%2 != 0 {
		out = out[:len(out)-1]
	}
	samples := len(out) / 2
	if samples > 65535 {
		return false
	}

	a := samples
	offset := 0
	for a > 0 {
		if s.totalSampleCount == 0 && !s.musicPaused {
			s.insertSampleMarker()
		}
		if s.samplesLeft == 0 {
			if !s.musicPaused {
				s.doRow()
				s.insertSampleMarker()
			}
			s.samplesLeft = int32(s.samplesPerTick)
		}

		b := a
		if int32(b) > s.samplesLeft {
			b = int(s.samplesLeft)
		}

		s.mixAudio(out[offset*2:offset*2+b*2], int32(b))
		offset += b
		s.totalSampleCount += uint32(b)
		a -= b
		s.samplesLeft -= int32(b)
	}

	s.sampleCounter += uint32(samples)
	return true
}

func (s *state) vol0OptimizationNoLoop(v *voice, numSamples uint32) {
	pos := v.mPosFrac + ((v.mSpeed & 0xFFFF) * numSamples)
	realPos := v.mPos + ((v.mSpeed >> 16) * numSamples) + (pos >> 16)
	pos &= 0xFFFF
	if realPos >= v.mEnd {
		v.mMixfunc = nil
		return
	}
	v.mPosFrac = pos
	v.mPos = realPos
}

func (s *state) vol0OptimizationLoop(v *voice, numSamples uint32) {
	pos := v.mPosFrac + ((v.mSpeed & 0xFFFF) * numSamples)
	realPos := v.mPos + ((v.mSpeed >> 16) * numSamples) + (pos >> 16)
	pos &= 0xFFFF
	for realPos >= v.mEnd {
		realPos -= v.mLoopLen
	}
	v.mPosFrac = pos
	v.mPos = realPos
}

func (s *state) mix8bNoLoop(v *voice, numSamples int32) {
	if v.mVolL == 0 && v.mVolR == 0 {
		s.vol0OptimizationNoLoop(v, uint32(numSamples))
		return
	}

	audioMixL := s.mixBufferL
	audioMixR := s.mixBufferR
	realPos := v.mPos
	pos := v.mPosFrac
	delta := v.mSpeed
	base := v.mBase8
	baseOffset := v.mBaseOffset
	smpIndex := baseOffset + int(realPos)
	volL := v.mVolL
	volR := v.mVolR

	samplesToRender := numSamples
	mixIndex := 0
	for samplesToRender > 0 {
		i := (v.mEnd - 1) - realPos
		if i > 65535 {
			i = 65535
		}
		samplesToMix := (((((uint64(i) << 16) | uint64(pos^0xFFFF)) * uint64(v.mSpeedRev)) >> 32) + 1)
		if samplesToMix > uint64(samplesToRender) {
			samplesToMix = uint64(samplesToRender)
		}
		samplesToRender -= int32(samplesToMix)

		if samplesToMix&1 != 0 {
			sample := int32(base[smpIndex]) << 20
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}
		samplesToMix >>= 1
		for i := uint64(0); i < samplesToMix; i++ {
			sample := int32(base[smpIndex]) << 20
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF

			sample = int32(base[smpIndex]) << 20
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}

		realPos = uint32(smpIndex - baseOffset)
		if realPos >= v.mEnd {
			v.mMixfunc = nil
			return
		}
	}

	v.mPosFrac = pos
	v.mPos = realPos
}

func (s *state) mix8bLoop(v *voice, numSamples int32) {
	if v.mVolL == 0 && v.mVolR == 0 {
		s.vol0OptimizationLoop(v, uint32(numSamples))
		return
	}

	audioMixL := s.mixBufferL
	audioMixR := s.mixBufferR
	realPos := v.mPos
	pos := v.mPosFrac
	delta := v.mSpeed
	base := v.mBase8
	baseOffset := v.mBaseOffset
	smpIndex := baseOffset + int(realPos)
	volL := v.mVolL
	volR := v.mVolR

	samplesToRender := numSamples
	mixIndex := 0
	for samplesToRender > 0 {
		i := (v.mEnd - 1) - realPos
		if i > 65535 {
			i = 65535
		}
		samplesToMix := (((((uint64(i) << 16) | uint64(pos^0xFFFF)) * uint64(v.mSpeedRev)) >> 32) + 1)
		if samplesToMix > uint64(samplesToRender) {
			samplesToMix = uint64(samplesToRender)
		}
		samplesToRender -= int32(samplesToMix)

		if samplesToMix&1 != 0 {
			sample := int32(base[smpIndex]) << 20
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}
		samplesToMix >>= 1
		for i := uint64(0); i < samplesToMix; i++ {
			sample := int32(base[smpIndex]) << 20
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF

			sample = int32(base[smpIndex]) << 20
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}

		realPos = uint32(smpIndex - baseOffset)
		for realPos >= v.mEnd {
			realPos -= v.mLoopLen
		}
		smpIndex = baseOffset + int(realPos)
	}

	v.mPosFrac = pos
	v.mPos = realPos
}

func interpolate8(s0, s1, s2, s3 int32, frac uint32) int32 {
	idx := int((frac >> 6) & 0x3FC)
	t := fastSincTable[idx:]
	s0 = ((s0*int32(t[0]) + s1*int32(t[1]) + s2*int32(t[2]) + s3*int32(t[3])) >> (14 - 8))
	return s0
}

func interpolate16(s0, s1, s2, s3 int32, frac uint32) int32 {
	idx := int((frac >> 6) & 0x3FC)
	t := fastSincTable[idx:]
	s0 = ((s0*int32(t[0]) + s1*int32(t[1]) + s2*int32(t[2]) + s3*int32(t[3])) >> 14)
	return s0
}

func (s *state) mix8bNoLoopIntrp(v *voice, numSamples int32) {
	if v.mVolL == 0 && v.mVolR == 0 {
		s.vol0OptimizationNoLoop(v, uint32(numSamples))
		return
	}

	audioMixL := s.mixBufferL
	audioMixR := s.mixBufferR
	realPos := v.mPos
	pos := v.mPosFrac
	delta := v.mSpeed
	base := v.mBase8
	baseOffset := v.mBaseOffset
	smpIndex := baseOffset + int(realPos)
	volL := v.mVolL
	volR := v.mVolR

	samplesToRender := numSamples
	mixIndex := 0
	for samplesToRender > 0 {
		i := (v.mEnd - 1) - realPos
		if i > 65535 {
			i = 65535
		}
		samplesToMix := (((((uint64(i) << 16) | uint64(pos^0xFFFF)) * uint64(v.mSpeedRev)) >> 32) + 1)
		if samplesToMix > uint64(samplesToRender) {
			samplesToMix = uint64(samplesToRender)
		}
		samplesToRender -= int32(samplesToMix)

		if samplesToMix&1 != 0 {
			sample := interpolate8(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}
		samplesToMix >>= 1
		for i := uint64(0); i < samplesToMix; i++ {
			sample := interpolate8(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF

			sample = interpolate8(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}

		realPos = uint32(smpIndex - baseOffset)
		if realPos >= v.mEnd {
			v.mMixfunc = nil
			return
		}
	}

	v.mPosFrac = pos
	v.mPos = realPos
}

func (s *state) mix8bLoopIntrp(v *voice, numSamples int32) {
	if v.mVolL == 0 && v.mVolR == 0 {
		s.vol0OptimizationLoop(v, uint32(numSamples))
		return
	}

	audioMixL := s.mixBufferL
	audioMixR := s.mixBufferR
	realPos := v.mPos
	pos := v.mPosFrac
	delta := v.mSpeed
	base := v.mBase8
	baseOffset := v.mBaseOffset
	smpIndex := baseOffset + int(realPos)
	volL := v.mVolL
	volR := v.mVolR

	samplesToRender := numSamples
	mixIndex := 0
	for samplesToRender > 0 {
		i := (v.mEnd - 1) - realPos
		if i > 65535 {
			i = 65535
		}
		samplesToMix := (((((uint64(i) << 16) | uint64(pos^0xFFFF)) * uint64(v.mSpeedRev)) >> 32) + 1)
		if samplesToMix > uint64(samplesToRender) {
			samplesToMix = uint64(samplesToRender)
		}
		samplesToRender -= int32(samplesToMix)

		if samplesToMix&1 != 0 {
			sample := interpolate8(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}
		samplesToMix >>= 1
		for i := uint64(0); i < samplesToMix; i++ {
			sample := interpolate8(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF

			sample = interpolate8(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}

		realPos = uint32(smpIndex - baseOffset)
		for realPos >= v.mEnd {
			realPos -= v.mLoopLen
		}
		smpIndex = baseOffset + int(realPos)
	}

	v.mPosFrac = pos
	v.mPos = realPos
}

func (s *state) mix16bNoLoop(v *voice, numSamples int32) {
	if v.mVolL == 0 && v.mVolR == 0 {
		s.vol0OptimizationNoLoop(v, uint32(numSamples))
		return
	}

	audioMixL := s.mixBufferL
	audioMixR := s.mixBufferR
	realPos := v.mPos
	pos := v.mPosFrac
	delta := v.mSpeed
	base := v.mBase16
	baseOffset := v.mBaseOffset
	smpIndex := baseOffset + int(realPos)
	volL := v.mVolL
	volR := v.mVolR

	samplesToRender := numSamples
	mixIndex := 0
	for samplesToRender > 0 {
		i := (v.mEnd - 1) - realPos
		if i > 65535 {
			i = 65535
		}
		samplesToMix := (((((uint64(i) << 16) | uint64(pos^0xFFFF)) * uint64(v.mSpeedRev)) >> 32) + 1)
		if samplesToMix > uint64(samplesToRender) {
			samplesToMix = uint64(samplesToRender)
		}
		samplesToRender -= int32(samplesToMix)

		if samplesToMix&1 != 0 {
			sample := int32(base[smpIndex]) << 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}
		samplesToMix >>= 1
		for i := uint64(0); i < samplesToMix; i++ {
			sample := int32(base[smpIndex]) << 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF

			sample = int32(base[smpIndex]) << 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}

		realPos = uint32(smpIndex - baseOffset)
		if realPos >= v.mEnd {
			v.mMixfunc = nil
			return
		}
	}

	v.mPosFrac = pos
	v.mPos = realPos
}

func (s *state) mix16bLoop(v *voice, numSamples int32) {
	if v.mVolL == 0 && v.mVolR == 0 {
		s.vol0OptimizationLoop(v, uint32(numSamples))
		return
	}

	audioMixL := s.mixBufferL
	audioMixR := s.mixBufferR
	realPos := v.mPos
	pos := v.mPosFrac
	delta := v.mSpeed
	base := v.mBase16
	baseOffset := v.mBaseOffset
	smpIndex := baseOffset + int(realPos)
	volL := v.mVolL
	volR := v.mVolR

	samplesToRender := numSamples
	mixIndex := 0
	for samplesToRender > 0 {
		i := (v.mEnd - 1) - realPos
		if i > 65535 {
			i = 65535
		}
		samplesToMix := (((((uint64(i) << 16) | uint64(pos^0xFFFF)) * uint64(v.mSpeedRev)) >> 32) + 1)
		if samplesToMix > uint64(samplesToRender) {
			samplesToMix = uint64(samplesToRender)
		}
		samplesToRender -= int32(samplesToMix)

		if samplesToMix&1 != 0 {
			sample := int32(base[smpIndex]) << 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}
		samplesToMix >>= 1
		for i := uint64(0); i < samplesToMix; i++ {
			sample := int32(base[smpIndex]) << 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF

			sample = int32(base[smpIndex]) << 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}

		realPos = uint32(smpIndex - baseOffset)
		for realPos >= v.mEnd {
			realPos -= v.mLoopLen
		}
		smpIndex = baseOffset + int(realPos)
	}

	v.mPosFrac = pos
	v.mPos = realPos
}

func (s *state) mix16bNoLoopIntrp(v *voice, numSamples int32) {
	if v.mVolL == 0 && v.mVolR == 0 {
		s.vol0OptimizationNoLoop(v, uint32(numSamples))
		return
	}

	audioMixL := s.mixBufferL
	audioMixR := s.mixBufferR
	realPos := v.mPos
	pos := v.mPosFrac
	delta := v.mSpeed
	base := v.mBase16
	baseOffset := v.mBaseOffset
	smpIndex := baseOffset + int(realPos)
	volL := v.mVolL
	volR := v.mVolR

	samplesToRender := numSamples
	mixIndex := 0
	for samplesToRender > 0 {
		i := (v.mEnd - 1) - realPos
		if i > 65535 {
			i = 65535
		}
		samplesToMix := (((((uint64(i) << 16) | uint64(pos^0xFFFF)) * uint64(v.mSpeedRev)) >> 32) + 1)
		if samplesToMix > uint64(samplesToRender) {
			samplesToMix = uint64(samplesToRender)
		}
		samplesToRender -= int32(samplesToMix)

		if samplesToMix&1 != 0 {
			sample := interpolate16(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}
		samplesToMix >>= 1
		for i := uint64(0); i < samplesToMix; i++ {
			sample := interpolate16(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF

			sample = interpolate16(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}

		realPos = uint32(smpIndex - baseOffset)
		if realPos >= v.mEnd {
			v.mMixfunc = nil
			return
		}
	}

	v.mPosFrac = pos
	v.mPos = realPos
}

func (s *state) mix16bLoopIntrp(v *voice, numSamples int32) {
	if v.mVolL == 0 && v.mVolR == 0 {
		s.vol0OptimizationLoop(v, uint32(numSamples))
		return
	}

	audioMixL := s.mixBufferL
	audioMixR := s.mixBufferR
	realPos := v.mPos
	pos := v.mPosFrac
	delta := v.mSpeed
	base := v.mBase16
	baseOffset := v.mBaseOffset
	smpIndex := baseOffset + int(realPos)
	volL := v.mVolL
	volR := v.mVolR

	samplesToRender := numSamples
	mixIndex := 0
	for samplesToRender > 0 {
		i := (v.mEnd - 1) - realPos
		if i > 65535 {
			i = 65535
		}
		samplesToMix := (((((uint64(i) << 16) | uint64(pos^0xFFFF)) * uint64(v.mSpeedRev)) >> 32) + 1)
		if samplesToMix > uint64(samplesToRender) {
			samplesToMix = uint64(samplesToRender)
		}
		samplesToRender -= int32(samplesToMix)

		if samplesToMix&1 != 0 {
			sample := interpolate16(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}
		samplesToMix >>= 1
		for i := uint64(0); i < samplesToMix; i++ {
			sample := interpolate16(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF

			sample = interpolate16(int32(base[smpIndex-1]), int32(base[smpIndex]), int32(base[smpIndex+1]), int32(base[smpIndex+2]), pos)
			sample <<= 12
			audioMixL[mixIndex] += int32((int64(sample) * int64(volL)) >> 32)
			audioMixR[mixIndex] += int32((int64(sample) * int64(volR)) >> 32)
			mixIndex++
			pos += delta
			smpIndex += int(pos >> 16)
			pos &= 0xFFFF
		}

		realPos = uint32(smpIndex - baseOffset)
		for realPos >= v.mEnd {
			realPos -= v.mLoopLen
		}
		smpIndex = baseOffset + int(realPos)
	}

	v.mPosFrac = pos
	v.mPos = realPos
}

var mixRoutineTable = [...]mixRoutine{
	(*state).mix8bNoLoop,
	(*state).mix8bLoop,
	(*state).mix8bNoLoopIntrp,
	(*state).mix8bLoopIntrp,
	(*state).mix16bNoLoop,
	(*state).mix16bLoop,
	(*state).mix16bNoLoopIntrp,
	(*state).mix16bLoopIntrp,
}
