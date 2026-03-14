package unitrk

type OrderResolver func(order uint8) (uint8, bool)

type S3MITConverter struct {
	ResolveOrder   OrderResolver
	FiltersEnabled bool
	ActiveMacro    uint8
	FilterMacros   [0x10]uint8
	FilterSettings [0x100]FilterSetting
}

func (c *S3MITConverter) Process(builder *Builder, cmd, inf uint8, flags S3MITFlags) {
	if cmd == 0 || cmd == 0xff {
		return
	}

	lo := inf & 0x0f

	switch cmd {
	case 0x01:
		builder.Effect(OpS3MEffectA, uint16(inf))
	case 0x02:
		if c.ResolveOrder == nil {
			return
		}
		if order, ok := c.ResolveOrder(inf); ok {
			builder.PTEffect(0x0b, order)
		}
	case 0x03:
		if flags&S3MITOldStyle != 0 && flags&S3MITImpulseTracker == 0 {
			builder.PTEffect(0x0d, ((inf>>4)*10)+(inf&0x0f))
			return
		}
		builder.PTEffect(0x0d, inf)
	case 0x04:
		builder.Effect(OpS3MEffectD, uint16(inf))
	case 0x05:
		builder.Effect(OpS3MEffectE, uint16(inf))
	case 0x06:
		builder.Effect(OpS3MEffectF, uint16(inf))
	case 0x07:
		if flags&S3MITOldStyle != 0 {
			builder.PTEffect(0x03, inf)
			return
		}
		builder.Effect(OpITEffectG, uint16(inf))
	case 0x08:
		if flags&S3MITOldStyle != 0 {
			if flags&S3MITImpulseTracker != 0 {
				builder.Effect(OpITEffectHOld, uint16(inf))
			} else {
				builder.Effect(OpS3MEffectH, uint16(inf))
			}
			return
		}
		builder.Effect(OpITEffectH, uint16(inf))
	case 0x09:
		if flags&S3MITOldStyle != 0 {
			builder.Effect(OpS3MEffectI, uint16(inf))
			return
		}
		builder.Effect(OpITEffectI, uint16(inf))
	case 0x0a:
		builder.PTEffect(0x00, inf)
	case 0x0b:
		if flags&S3MITOldStyle != 0 {
			if flags&S3MITImpulseTracker != 0 {
				builder.Effect(OpITEffectHOld, 0)
			} else {
				builder.Effect(OpS3MEffectH, 0)
			}
		} else {
			builder.Effect(OpITEffectH, 0)
		}
		builder.Effect(OpS3MEffectD, uint16(inf))
	case 0x0c:
		if flags&S3MITOldStyle != 0 {
			builder.PTEffect(0x03, 0)
		} else {
			builder.Effect(OpITEffectG, 0)
		}
		builder.Effect(OpS3MEffectD, uint16(inf))
	case 0x0d:
		if inf <= 0x40 {
			builder.Effect(OpITEffectM, uint16(inf))
		}
	case 0x0e:
		builder.Effect(OpITEffectN, uint16(inf))
	case 0x0f:
		builder.PTEffect(0x09, inf)
	case 0x10:
		builder.Effect(OpITEffectP, uint16(inf))
	case 0x11:
		if inf != 0 && lo == 0 && flags&S3MITOldStyle == 0 {
			builder.Effect(OpS3MEffectQ, 1)
			return
		}
		builder.Effect(OpS3MEffectQ, uint16(inf))
	case 0x12:
		builder.Effect(OpS3MEffectR, uint16(inf))
	case 0x13:
		if inf >= 0xf0 {
			c.activateMacro(inf & 0x0f)
			return
		}
		if flags&S3MITScreamTracker != 0 && (inf&0xf0) == 0xa0 {
			return
		}
		builder.Effect(OpITEffectS0, uint16(inf))
	case 0x14:
		if inf >= 0x20 {
			builder.Effect(OpS3MEffectT, uint16(inf))
			return
		}
		if flags&S3MITOldStyle == 0 {
			builder.Effect(OpITEffectT, uint16(inf))
		}
	case 0x15:
		if flags&S3MITOldStyle != 0 {
			if flags&S3MITImpulseTracker != 0 {
				builder.Effect(OpITEffectUOld, uint16(inf))
			} else {
				builder.Effect(OpS3MEffectU, uint16(inf))
			}
			return
		}
		builder.Effect(OpITEffectU, uint16(inf))
	case 0x16:
		builder.Effect(OpXMEffectG, uint16(inf))
	case 0x17:
		builder.Effect(OpITEffectW, uint16(inf))
	case 0x18:
		if flags&S3MITOldStyle != 0 {
			if inf > 128 {
				builder.Effect(OpITEffectS0, uint16(0x91))
			} else if inf == 128 {
				builder.PTEffect(0x08, 255)
			} else {
				builder.PTEffect(0x08, inf<<1)
			}
			return
		}
		builder.PTEffect(0x08, inf)
	case 0x19:
		builder.Effect(OpITEffectY, uint16(inf))
	case 0x1a:
		setting := c.FilterSettings[inf]
		if setting.Filter != 0 {
			builder.Effect(OpITEffectZ, EncodePair(setting.Filter, setting.Info))
		}
	}
}

func (c *S3MITConverter) activateMacro(macro uint8) {
	if !c.FiltersEnabled || macro == c.ActiveMacro {
		return
	}

	c.ActiveMacro = macro
	for i := 0; i < 0x80; i++ {
		c.FilterSettings[i].Filter = c.FilterMacros[macro]
	}
}
