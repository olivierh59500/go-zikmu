package st3

func (s *state) optimizeSampleDatasForMixer() bool {
	for i := 0; i < len(s.ins); i++ {
		inst := &s.ins[i]
		if inst.length == 0 {
			continue
		}

		hasLoop := inst.flags&1 != 0
		is16bit := inst.flags&4 != 0

		loopLen := int(inst.lend - inst.lbeg)
		if loopLen > 0 && loopLen < 500 {
			loopLen *= 1 + (500 / loopLen)
		}
		inst.lend512 = inst.lbeg + uint32(loopLen)

		if is16bit {
			if inst.data16 == nil {
				continue
			}
			required := samplePad16 + int(inst.length) + sampleUnrollBytes
			if len(inst.raw16) < required {
				newRaw := make([]int16, required)
				copy(newRaw, inst.raw16)
				inst.raw16 = newRaw
				inst.data16 = newRaw[samplePad16:]
			}
			for j := 0; j < samplePad16; j++ {
				inst.raw16[j] = 0
			}

			if hasLoop && loopLen > 0 {
				loopEnd := inst.data16[int(inst.lend):]
				loopBeg := inst.data16[int(inst.lbeg):]
				for j := 0; j < sampleUnrollBytes; j++ {
					loopEnd[j] = loopBeg[j%loopLen]
				}
				if inst.lbeg == 0 && inst.lend > 0 {
					inst.raw16[samplePad16-1] = inst.data16[int(inst.lend)-1]
				}
			} else {
				if inst.length == 0 {
					continue
				}
				last := inst.data16[int(inst.length)-1]
				end := inst.data16[int(inst.length):]
				for j := 0; j < sampleUnrollBytes; j++ {
					end[j] = last
					if last > 0 {
						last -= 32768 / 32
						if last < 0 {
							last = 0
						}
					} else if last < 0 {
						last += 32768 / 32
						if last > 0 {
							last = 0
						}
					}
				}
			}
		} else {
			if inst.data8 == nil {
				continue
			}
			required := samplePad8 + int(inst.length) + sampleUnrollBytes
			if len(inst.raw8) < required {
				newRaw := make([]int8, required)
				copy(newRaw, inst.raw8)
				inst.raw8 = newRaw
				inst.data8 = newRaw[samplePad8:]
			}
			for j := 0; j < samplePad8; j++ {
				inst.raw8[j] = 0
			}

			if hasLoop && loopLen > 0 {
				loopEnd := inst.data8[int(inst.lend):]
				loopBeg := inst.data8[int(inst.lbeg):]
				for j := 0; j < sampleUnrollBytes; j++ {
					loopEnd[j] = loopBeg[j%loopLen]
				}
				if inst.lbeg == 0 && inst.lend > 0 {
					inst.raw8[samplePad8-1] = inst.data8[int(inst.lend)-1]
				}
			} else {
				if inst.length == 0 {
					continue
				}
				last := inst.data8[int(inst.length)-1]
				end := inst.data8[int(inst.length):]
				for j := 0; j < sampleUnrollBytes; j++ {
					end[j] = last
					if last > 0 {
						last -= 128 / 32
						if last < 0 {
							last = 0
						}
					} else if last < 0 {
						last += 128 / 32
						if last > 0 {
							last = 0
						}
					}
				}
			}
		}
	}

	return true
}
