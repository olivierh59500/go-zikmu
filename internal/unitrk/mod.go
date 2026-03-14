package unitrk

func ProTrackerEvent(builder *Builder, note uint8, instrument uint16, effect, data uint8) {
	if note != 0 {
		builder.Note(note)
	}
	if instrument != 0 {
		builder.Instrument(instrument)
	}
	builder.PTEffect(effect, data)
}
