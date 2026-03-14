package pitch

const xmOctave = 12

// XMPeriod mirrors libmikmod's XM note-to-period conversion.
func XMPeriod(note int, fineTune int16, linear bool) int {
	return XMPeriodFromNoteParam(note*2, fineTune, linear)
}

// XMPeriodFromNoteParam mirrors libmikmod's doubled-note XM period path.
func XMPeriodFromNoteParam(noteParam int, fineTune int16, linear bool) int {
	if linear {
		return xmLinearPeriodFromNoteParam(noteParam, fineTune)
	}
	return xmLogPeriodFromNoteParam(noteParam, fineTune)
}

// XMLinearPeriod mirrors libmikmod's XM linear period path.
func XMLinearPeriod(note int, fineTune int16) int {
	return xmLinearPeriodFromNoteParam(note*2, fineTune)
}

// XMLogPeriod mirrors libmikmod's XM logarithmic period path.
func XMLogPeriod(note int, fineTune int16) int {
	return xmLogPeriodFromNoteParam(note*2, fineTune)
}

func xmLinearPeriodFromNoteParam(noteParam int, fineTune int16) int {
	fine := clampInt(int(fineTune)+128, 0, 255)
	return ((20+2*xmHighOctave)*xmOctave + 2 - noteParam) * 32 - (fine >> 1)
}

func xmLogPeriodFromNoteParam(noteParam int, fineTune int16) int {
	fine := clampInt(int(fineTune)+128, 0, 255)
	n := positiveMod(noteParam, 2*xmOctave)
	o := noteParam / (2 * xmOctave)
	index := (n << 2) + (fine >> 4)
	p1 := int(xmLogTab[index])
	p2 := int(xmLogTab[index+1])
	return interpolateInt(fine>>4, 0, 15, p1, p2) >> o
}

// XMFrequencyFromPeriod mirrors libmikmod's XM period-to-frequency conversion.
func XMFrequencyFromPeriod(period int, linear bool) int {
	if period <= 0 {
		return 0
	}
	if linear {
		shift := period/768 - xmHighOctave
		value := uint64(xmLinearTab[positiveMod(period, len(xmLinearTab))])
		if shift >= 0 {
			value >>= shift
		} else {
			value <<= -shift
		}
		return int(value)
	}
	return (8363 * 1712) / period
}

func interpolateInt(pos, p1, p2, v1, v2 int) int {
	if p1 == p2 || pos == p1 {
		return v1
	}
	return v1 + (pos-p1)*(v2-v1)/(p2-p1)
}

var xmLogTab = [...]uint16{
	907, 900, 894, 887, 881, 875, 868, 862,
	856, 850, 844, 838, 832, 826, 820, 814,
	808, 802, 796, 791, 785, 779, 774, 768,
	762, 757, 752, 746, 741, 736, 730, 725,
	720, 715, 709, 704, 699, 694, 689, 684,
	678, 675, 670, 665, 660, 655, 651, 646,
	640, 636, 632, 628, 623, 619, 614, 610,
	604, 601, 597, 592, 588, 584, 580, 575,
	570, 567, 563, 559, 555, 551, 547, 543,
	538, 535, 532, 528, 524, 520, 516, 513,
	508, 505, 502, 498, 494, 491, 487, 484,
	480, 477, 474, 470, 467, 463, 460, 457,
	453, 450, 447, 443, 440, 437, 434, 431,
}
