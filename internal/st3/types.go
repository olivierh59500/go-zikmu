package st3

type mixRoutine func(s *state, v *voice, numSamples int32)

type effectRoutine func(s *state, ch *channel)

type instrument struct {
	raw8    []int8
	data8   []int8
	raw16   []int16
	data16  []int16
	vol     int8
	flags   uint8
	typ     uint8
	c2spd   uint16
	length  uint32
	lbeg    uint32
	lend    uint32
	lend512 uint32
}

type channel struct {
	aorgvol        int8
	avol           int8
	apanpos        uint8
	atreon         bool
	surround       bool
	channelnum     uint8
	amixtype       uint8
	achannelused   uint8
	aglis          uint8
	atremor        uint8
	atrigcnt       uint8
	anotecutcnt    uint8
	anotedelaycnt  uint8
	avibtretype    uint8
	note           uint8
	ins            uint8
	vol            uint8
	cmd            uint8
	info           uint8
	lastins        uint8
	lastnote       uint8
	alastnfo       uint8
	alasteff       uint8
	alasteff1      uint8
	avibcnt        int16
	asldspd        int16
	aspd           int16
	aorgspd        int16
	astartoffset   uint16
	astartoffset00 uint16
	ac2spd         uint16
}

type voice struct {
	mBase8            []int8
	mBase16           []int16
	mBaseOffset       int
	mLoopFlag         bool
	mVolL             int32
	mVolR             int32
	mPos              uint32
	mEnd              uint32
	mOrigEnd          uint32
	mLoopBeg          uint32
	mLoopLen          uint32
	mPosFrac          uint32
	mSpeed            uint32
	mSpeedRev         uint32
	lastMixFuncOffset uint32
	insPtr            *instrument
	mMixfunc          mixRoutine
}
