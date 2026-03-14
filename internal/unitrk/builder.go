package unitrk

type Command struct {
	Op    Opcode
	Param uint16
}

type Row struct {
	Commands []Command
}

type Track struct {
	Rows []Row
}

type Builder struct {
	rows               []Row
	current            []Command
	allowEmptyArpeggio bool
}

func EncodePair(a, b uint8) uint16 {
	return uint16(a)<<8 | uint16(b)
}

func DecodePair(v uint16) (uint8, uint8) {
	return uint8(v >> 8), uint8(v)
}

func (b *Builder) Reset() {
	b.rows = b.rows[:0]
	b.current = b.current[:0]
	b.allowEmptyArpeggio = false
}

func (b *Builder) SetArpeggioMemory(enabled bool) {
	b.allowEmptyArpeggio = enabled
}

func (b *Builder) Write(op Opcode, param uint16) {
	b.Effect(op, param)
}

func (b *Builder) Effect(op Opcode, param uint16) {
	if !op.Valid() {
		return
	}
	b.current = append(b.current, Command{Op: op, Param: param})
}

func (b *Builder) Note(note uint8) {
	b.Effect(OpNote, uint16(note))
}

func (b *Builder) Instrument(instrument uint16) {
	b.Effect(OpInstrument, instrument)
}

func (b *Builder) KeyOff() {
	b.Effect(OpKeyOff, 0)
}

func (b *Builder) KeyFade() {
	b.Effect(OpKeyFade, 0)
}

func (b *Builder) PTEffect(effect, data uint8) {
	if effect > 0x0f {
		return
	}
	if effect == 0 && data == 0 && !b.allowEmptyArpeggio {
		return
	}
	b.Effect(OpPTEffect0+Opcode(effect), uint16(data))
}

func (b *Builder) VolumeEffect(effect VolumeEffect, data uint8) {
	if effect == VolNone && data == 0 {
		return
	}
	b.Effect(OpVolumeEffects, EncodePair(uint8(effect), data))
}

func (b *Builder) NewLine() {
	row := Row{Commands: append([]Command(nil), b.current...)}
	b.rows = append(b.rows, row)
	b.current = b.current[:0]
}

func (b *Builder) Track() Track {
	rows := make([]Row, len(b.rows))
	for i, row := range b.rows {
		rows[i] = Row{Commands: append([]Command(nil), row.Commands...)}
	}
	return Track{Rows: rows}
}
