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
	commands           []Command
	rowEnds            []int
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
	b.commands = b.commands[:0]
	b.rowEnds = b.rowEnds[:0]
	b.current = b.current[:0]
	b.allowEmptyArpeggio = false
}

func (b *Builder) Grow(rows int) {
	if rows > cap(b.rowEnds) {
		b.rowEnds = make([]int, 0, rows)
	}
	if cap(b.current) == 0 {
		b.current = make([]Command, 0, 8)
	}
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
	b.commands = append(b.commands, b.current...)
	b.rowEnds = append(b.rowEnds, len(b.commands))
	b.current = b.current[:0]
}

func (b *Builder) Track() Track {
	commands := append([]Command(nil), b.commands...)
	return Track{Rows: buildRows(commands, b.rowEnds)}
}

func (b *Builder) TakeTrack() Track {
	track := Track{Rows: buildRows(b.commands, b.rowEnds)}
	*b = Builder{}
	return track
}

func buildRows(commands []Command, ends []int) []Row {
	rows := make([]Row, len(ends))
	start := 0
	for i, end := range ends {
		rows[i].Commands = commands[start:end:end]
		start = end
	}
	return rows
}
