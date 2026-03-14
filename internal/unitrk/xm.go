package unitrk

func XMEvent(builder *Builder, note, instrument, volume, effect, data uint8) {
	if note != 0 {
		if note > XMNoteCount {
			builder.KeyFade()
		} else {
			builder.Note(note - 1)
		}
	}
	if instrument != 0 {
		builder.Instrument(uint16(instrument - 1))
	}

	XMVolume(builder, volume)
	XMEffect(builder, effect, data)
}

func XMVolume(builder *Builder, volume uint8) {
	switch volume >> 4 {
	case 0x6:
		if volume&0x0f != 0 {
			builder.Effect(OpXMEffectA, uint16(volume&0x0f))
		}
	case 0x7:
		if volume&0x0f != 0 {
			builder.Effect(OpXMEffectA, uint16(volume<<4))
		}
	case 0x8:
		builder.PTEffect(0x0e, 0xb0|(volume&0x0f))
	case 0x9:
		builder.PTEffect(0x0e, 0xa0|(volume&0x0f))
	case 0xa:
		builder.Effect(OpXMEffect4, uint16(volume<<4))
	case 0xb:
		builder.Effect(OpXMEffect4, uint16(volume&0x0f))
	case 0xc:
		builder.PTEffect(0x08, volume<<4)
	case 0xd:
		if volume&0x0f != 0 {
			builder.Effect(OpXMEffectP, uint16(volume&0x0f))
		}
	case 0xe:
		if volume&0x0f != 0 {
			builder.Effect(OpXMEffectP, uint16(volume<<4))
		}
	case 0xf:
		builder.PTEffect(0x03, volume<<4)
	default:
		if volume >= 0x10 && volume <= 0x50 {
			builder.PTEffect(0x0c, volume-0x10)
		}
	}
}

func XMEffect(builder *Builder, effect, data uint8) {
	switch effect {
	case 0x04:
		builder.Effect(OpXMEffect4, uint16(data))
	case 0x06:
		builder.Effect(OpXMEffect6, uint16(data))
	case 0x0a:
		builder.Effect(OpXMEffectA, uint16(data))
	case 0x0e:
		switch data >> 4 {
		case 0x1:
			builder.Effect(OpXMEffectE1, uint16(data&0x0f))
		case 0x2:
			builder.Effect(OpXMEffectE2, uint16(data&0x0f))
		case 0xa:
			builder.Effect(OpXMEffectEA, uint16(data&0x0f))
		case 0xb:
			builder.Effect(OpXMEffectEB, uint16(data&0x0f))
		default:
			builder.PTEffect(effect, data)
		}
	case 'G' - 55:
		if data > 64 {
			builder.Effect(OpXMEffectG, 128)
		} else {
			builder.Effect(OpXMEffectG, uint16(data<<1))
		}
	case 'H' - 55:
		builder.Effect(OpXMEffectH, uint16(data))
	case 'K' - 55:
		builder.Effect(OpKeyFade, uint16(data))
	case 'L' - 55:
		builder.Effect(OpXMEffectL, uint16(data))
	case 'P' - 55:
		builder.Effect(OpXMEffectP, uint16(data))
	case 'R' - 55:
		builder.Effect(OpS3MEffectQ, uint16(data))
	case 'T' - 55:
		builder.Effect(OpS3MEffectI, uint16(data))
	case 'X' - 55:
		switch data >> 4 {
		case 1:
			builder.Effect(OpXMEffectX1, uint16(data&0x0f))
		case 2:
			builder.Effect(OpXMEffectX2, uint16(data&0x0f))
		}
	default:
		if effect <= 0x0f {
			if effect == 0x0d && ((data >> 4) <= 9) && ((data & 0x0f) <= 9) {
				data = ((data >> 4) * 10) + (data & 0x0f)
			}
			builder.PTEffect(effect, data)
		}
	}
}
