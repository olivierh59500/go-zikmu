package unitrk

type Opcode uint8

const (
	OpInvalid Opcode = iota
	OpNote
	OpInstrument
	OpPTEffect0
	OpPTEffect1
	OpPTEffect2
	OpPTEffect3
	OpPTEffect4
	OpPTEffect5
	OpPTEffect6
	OpPTEffect7
	OpPTEffect8
	OpPTEffect9
	OpPTEffectA
	OpPTEffectB
	OpPTEffectC
	OpPTEffectD
	OpPTEffectE
	OpPTEffectF
	OpS3MEffectA
	OpS3MEffectD
	OpS3MEffectE
	OpS3MEffectF
	OpS3MEffectI
	OpS3MEffectQ
	OpS3MEffectR
	OpS3MEffectT
	OpS3MEffectU
	OpKeyOff
	OpKeyFade
	OpVolumeEffects
	OpXMEffect4
	OpXMEffect6
	OpXMEffectA
	OpXMEffectE1
	OpXMEffectE2
	OpXMEffectEA
	OpXMEffectEB
	OpXMEffectG
	OpXMEffectH
	OpXMEffectL
	OpXMEffectP
	OpXMEffectX1
	OpXMEffectX2
	OpITEffectG
	OpITEffectH
	OpITEffectI
	OpITEffectM
	OpITEffectN
	OpITEffectP
	OpITEffectT
	OpITEffectU
	OpITEffectW
	OpITEffectY
	OpITEffectZ
	OpITEffectS0
	OpUltEffect9
	OpMEDSpeed
	OpMEDEffectF1
	OpMEDEffectF2
	OpMEDEffectF3
	OpOktArp
	OpFormatLast
	OpS3MEffectH
	OpITEffectHOld
	OpITEffectUOld
	OpGDMEffect4
	OpGDMEffect7
	OpGDMEffect14
	OpMEDEffectVibrato
	OpMEDEffectFD
	OpMEDEffect16
	OpMEDEffect18
	OpMEDEffect1E
	OpMEDEffect1F
	OpFarEffect1
	OpFarEffect2
	OpFarEffect3
	OpFarEffect4
	OpFarEffect6
	OpFarEffectD
	OpFarEffectE
	OpFarEffectF
	OpLast
)

type VolumeEffect uint8

const (
	VolNone VolumeEffect = iota
	VolSetVolume
	VolSetPanning
	VolSlide
	VolPitchSlideDown
	VolPitchSlideUp
	VolPortamento
	VolVibrato
)

type SpecialEffect uint8

const (
	SpecialGlissando SpecialEffect = iota + 1
	SpecialFineTune
	SpecialVibratoWave
	SpecialTremoloWave
	SpecialPanbrelloWave
	SpecialFrameDelay
	SpecialS7Effects
	SpecialPanning
	SpecialSurround
	SpecialHighOffset
	SpecialPatternLoop
	SpecialNoteCut
	SpecialNoteDelay
	SpecialPatternDelay
)

type S3MITFlags uint8

const (
	S3MITOldStyle S3MITFlags = 1 << iota
	S3MITImpulseTracker
	S3MITScreamTracker
)

type FilterSetting struct {
	Filter uint8
	Info   uint8
}

const XMNoteCount = 8 * 12

var operandCounts = [...]uint8{
	0,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	0,
	1,
	2,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	2,
	1,
	2,
	2,
	0,
	0,
	0,
	2,
	0,
	1,
	1,
	1,
	1,
	1,
	1,
	2,
	0,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
	1,
}

func (op Opcode) Valid() bool {
	return op > OpInvalid && op < OpLast && op != OpFormatLast
}

func OperandCount(op Opcode) int {
	if !op.Valid() {
		return 0
	}
	return int(operandCounts[op])
}
