package unitrk

import "fmt"

var itPortamentoTable = [10]uint8{0, 1, 4, 8, 16, 32, 64, 96, 128, 255}

func ITVolumeColumn(builder *Builder, volpan uint8) error {
	switch {
	case volpan <= 64:
		builder.VolumeEffect(VolSetVolume, volpan)
	case volpan == 65:
		builder.VolumeEffect(VolSlide, 0)
	case volpan <= 74:
		builder.VolumeEffect(VolSlide, 0x0f+((volpan-65)<<4))
	case volpan == 75:
		builder.VolumeEffect(VolSlide, 0)
	case volpan <= 84:
		builder.VolumeEffect(VolSlide, 0xf0+(volpan-75))
	case volpan <= 94:
		builder.VolumeEffect(VolSlide, (volpan-85)<<4)
	case volpan <= 104:
		builder.VolumeEffect(VolSlide, volpan-95)
	case volpan <= 114:
		builder.VolumeEffect(VolPitchSlideDown, volpan-105)
	case volpan <= 124:
		builder.VolumeEffect(VolPitchSlideUp, volpan-115)
	case volpan <= 127:
		return fmt.Errorf("unitrk: invalid IT volume column value %d", volpan)
	case volpan <= 192:
		panning := volpan - 128
		if panning == 64 {
			panning = 255
		} else {
			panning <<= 2
		}
		builder.VolumeEffect(VolSetPanning, panning)
	case volpan <= 202:
		builder.VolumeEffect(VolPortamento, itPortamentoTable[volpan-193])
	case volpan <= 212:
		builder.VolumeEffect(VolVibrato, volpan-203)
	case volpan != 239 && volpan != 255:
		return fmt.Errorf("unitrk: invalid IT volume column value %d", volpan)
	}

	return nil
}

func ITEvent(builder *Builder, converter *S3MITConverter, note, instrument, volpan, cmd, inf uint8, oldStyle bool) error {
	if note != 255 {
		switch note {
		case 253:
			builder.KeyOff()
		case 254:
			builder.PTEffect(0x0c, 0xff)
			volpan = 255
		default:
			builder.Note(note)
		}
	}

	switch {
	case instrument != 0 && instrument < 253:
		builder.Instrument(uint16(instrument - 1))
	case instrument == 253:
		builder.KeyOff()
	case instrument != 255:
		return fmt.Errorf("unitrk: invalid IT instrument value %d", instrument)
	}

	if err := ITVolumeColumn(builder, volpan); err != nil {
		return err
	}

	if converter == nil {
		converter = &S3MITConverter{}
	}

	flags := S3MITImpulseTracker
	if oldStyle {
		flags |= S3MITOldStyle
	}
	converter.Process(builder, cmd, inf, flags)

	return nil
}
