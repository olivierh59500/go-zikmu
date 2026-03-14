package mixer

import (
	"fmt"
	"math"

	modmodel "github.com/olivierh59500/go-zikmu/internal/module"
	"github.com/olivierh59500/go-zikmu/internal/pitch"
	"github.com/olivierh59500/go-zikmu/internal/replay"
)

const (
	xmReferenceC5Hz = 8363.0

	bitShift                 = 9
	fpShift                  = 4
	clickShift               = 6
	clickBuffer              = 1 << clickShift
	fractionBits             = 11
	experimentalFractionBits = 28
	floatMixScale            = (1.0 / 32768.0) / (1 << fpShift)
)
const fractionMask int64 = (int64(1) << fractionBits) - 1

var oldPeriods = [...]uint32{
	0x6b00, 0x6800, 0x6500, 0x6220, 0x5f50, 0x5c80,
	0x5a00, 0x5740, 0x54d0, 0x5260, 0x5010, 0x4dc0,
	0x4b90, 0x4960, 0x4750, 0x4540, 0x4350, 0x4160,
	0x3f90, 0x3dc0, 0x3c10, 0x3a40, 0x38b0, 0x3700,
}

type Config struct {
	SampleRate                int
	OutputChannels            int
	Interpolation             bool
	MasterVolume              float32
	ExperimentalXMHighQuality bool
}

type Mixer struct {
	module                    *modmodel.Module
	sampleRate                int
	outputChannels            int
	interpolation             bool
	masterVolume              float32
	experimentalXMHighQuality bool
	voices                    []voice
	tickBuf                   []int32
}

type voice struct {
	active         bool
	trigger        uint64
	sample         int
	current        int64
	increment      int64
	rawVolume      int
	rawPanning     int
	leftSel        int
	rightSel       int
	oldLeftSel     int
	oldRightSel    int
	rampRemaining  int
	clickRemaining int
	lastLeftValue  int32
	lastRightValue int32
	keyOn          bool
}

func New(module *modmodel.Module, cfg Config) (*Mixer, error) {
	if module == nil {
		return nil, fmt.Errorf("mixer: nil module")
	}
	if cfg.SampleRate <= 0 {
		return nil, fmt.Errorf("mixer: invalid sample rate %d", cfg.SampleRate)
	}
	if cfg.OutputChannels != 1 && cfg.OutputChannels != 2 {
		return nil, fmt.Errorf("mixer: invalid output channel count %d", cfg.OutputChannels)
	}

	master := cfg.MasterVolume
	if master == 0 {
		master = 1
	}

	return &Mixer{
		module:                    module,
		sampleRate:                cfg.SampleRate,
		outputChannels:            cfg.OutputChannels,
		interpolation:             cfg.Interpolation,
		masterVolume:              master,
		experimentalXMHighQuality: cfg.ExperimentalXMHighQuality,
		voices:                    make([]voice, maxInt(module.Channels, module.Voices)),
	}, nil
}

func (m *Mixer) Reset(snapshot replay.Snapshot) {
	for i := range m.voices {
		m.voices[i] = voice{rawPanning: initialPanning(i)}
	}
	m.ApplySnapshot(snapshot)
}

func (m *Mixer) ApplySnapshot(snapshot replay.Snapshot) {
	states := snapshot.Channels
	if len(snapshot.Voices) > 0 {
		states = snapshot.Voices
	}
	for i := range m.voices {
		if i >= len(states) {
			m.voices[i].active = false
			continue
		}
		m.applyChannel(i, states[i])
	}
}

func (m *Mixer) Render(dst []float32) (int, error) {
	return m.renderFloat(dst)
}

func (m *Mixer) RenderPCM16(dst []int16) (int, error) {
	return m.renderPCM16(dst)
}

func (m *Mixer) renderFloat(dst []float32) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}

	frames := len(dst) / m.outputChannels
	if frames == 0 {
		return 0, nil
	}
	used := frames * m.outputChannels
	mixFrames := frames * m.mixSamplingFactor()
	mixSamples := mixFrames * m.outputChannels

	m.ensureTickBufSize(mixSamples)
	for i := 0; i < mixSamples; i++ {
		m.tickBuf[i] = 0
	}

	for i := range m.voices {
		if !m.voices[i].active {
			continue
		}
		m.addChannel(&m.voices[i], m.tickBuf[:mixSamples], mixFrames)
	}

	m.mix32ToFloat(dst[:used], m.tickBuf[:mixSamples], frames)
	return used, nil
}

func (m *Mixer) renderPCM16(dst []int16) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}

	frames := len(dst) / m.outputChannels
	if frames == 0 {
		return 0, nil
	}
	used := frames * m.outputChannels
	mixFrames := frames * m.mixSamplingFactor()
	mixSamples := mixFrames * m.outputChannels

	m.ensureTickBufSize(mixSamples)
	for i := 0; i < mixSamples; i++ {
		m.tickBuf[i] = 0
	}

	for i := range m.voices {
		if !m.voices[i].active {
			continue
		}
		m.addChannel(&m.voices[i], m.tickBuf[:mixSamples], mixFrames)
	}

	m.mix32To16(dst[:used], m.tickBuf[:mixSamples], frames)
	return used, nil
}

func (m *Mixer) applyChannel(index int, state replay.ChannelSnapshot) {
	v := &m.voices[index]
	if !state.Active || state.Sample < 0 || state.Sample >= len(m.module.Samples) {
		v.active = false
		v.lastLeftValue = 0
		v.lastRightValue = 0
		return
	}

	retrigger := v.trigger != state.Trigger
	if !retrigger && !v.active {
		v.keyOn = state.KeyOn
		return
	}

	sampleIndex := state.Sample
	if !retrigger && v.sample >= 0 && v.sample < len(m.module.Samples) {
		sampleIndex = v.sample
	}
	sample := &m.module.Samples[sampleIndex]
	increment := int64(0)
	if state.Frequency > 0 {
		increment = m.quantizedIncrement(float64(state.Frequency))
	} else {
		increment = m.incrementFor(state.Note, state.PitchDelta, sampleIndex)
	}
	if increment <= 0 {
		v.active = false
		v.lastLeftValue = 0
		v.lastRightValue = 0
		return
	}
	if !retrigger && v.increment < 0 {
		increment = -increment
	}

	v.trigger = state.Trigger
	v.keyOn = state.KeyOn
	v.increment = increment

	m.updateMixState(v, state.Volume, state.Panning)

	if retrigger {
		v.sample = state.Sample
		v.current = sampleStartOffset(sample, state.SampleOffset, state.KeyOn, m.mixFractionBits())
		v.clickRemaining = 0
		if sample.Flags&modmodel.SampleReverse != 0 {
			v.current = int64(maxInt(len(sample.Data)-1, 0)) << m.mixFractionBits()
			v.increment = -abs64(v.increment)
		} else if state.SampleOffset == 0 {
			v.increment = abs64(v.increment)
		}
	}

	if !retrigger && v.sample < 0 {
		v.sample = sampleIndex
	}
	v.active = true
}

func (m *Mixer) updateMixState(v *voice, volume, panning int) {
	if v == nil {
		return
	}
	volume = clampInt(volume, 0, 256)
	panning = clampInt(panning, 0, int(modmodel.PanSurround))
	nextLeftSel, nextRightSel := mixSelectors(panning, volume, m.outputChannels == 2)
	if absInt(v.rawVolume-volume) > 32 || absInt(v.rawPanning-panning) > 48 {
		v.rampRemaining = m.mixClickBuffer()
		v.oldLeftSel = v.leftSel
		v.oldRightSel = v.rightSel
	} else if v.rampRemaining == 0 {
		v.oldLeftSel = nextLeftSel
		v.oldRightSel = nextRightSel
	}
	v.rawVolume = volume
	v.rawPanning = panning
	v.leftSel = nextLeftSel
	v.rightSel = nextRightSel
}

func (m *Mixer) useExperimentalXMHighQuality() bool {
	return m != nil &&
		m.experimentalXMHighQuality &&
		m.interpolation &&
		m.module != nil &&
		m.module.Metadata.Format == modmodel.FormatXM
}

func (m *Mixer) mixFractionBits() int {
	if m.useExperimentalXMHighQuality() {
		return experimentalFractionBits
	}
	return fractionBits
}

func (m *Mixer) mixFractionScale() int64 {
	return int64(1) << m.mixFractionBits()
}

func (m *Mixer) mixFractionMask() int64 {
	if m.useExperimentalXMHighQuality() {
		return m.mixFractionScale() - 1
	}
	return fractionMask
}

func (m *Mixer) mixSamplingShift() int {
	if m.useExperimentalXMHighQuality() {
		return 2
	}
	return 0
}

func (m *Mixer) mixSamplingFactor() int {
	return 1 << m.mixSamplingShift()
}

func (m *Mixer) mixClickShift() int {
	return clickShift + m.mixSamplingShift()
}

func (m *Mixer) mixClickBuffer() int {
	return 1 << m.mixClickShift()
}

func (m *Mixer) quantizedIncrement(freq float64) int64 {
	if freq <= 0 || m.sampleRate <= 0 {
		return 0
	}
	increment := int64(freq)
	increment = (increment << (m.mixFractionBits() - m.mixSamplingShift())) / int64(m.sampleRate)
	if increment <= 0 {
		return 0
	}
	return increment
}

func (m *Mixer) quantizedStep(freq float64) float64 {
	increment := m.quantizedIncrement(freq)
	if increment <= 0 {
		return 0
	}
	return float64(increment*int64(m.mixSamplingFactor())) / float64(m.mixFractionScale())
}

func (m *Mixer) stepFor(note, pitchDelta, sampleIndex int) float64 {
	increment := m.incrementFor(note, pitchDelta, sampleIndex)
	if increment <= 0 {
		return 0
	}
	return float64(increment*int64(m.mixSamplingFactor())) / float64(m.mixFractionScale())
}

func (m *Mixer) incrementFor(note, pitchDelta, sampleIndex int) int64 {
	if sampleIndex < 0 || sampleIndex >= len(m.module.Samples) || note < 0 {
		return 0
	}

	sample := m.module.Samples[sampleIndex]
	if m.module.Flags&modmodel.FlagXMPeriods != 0 {
		pitchedNote, pitchedFineTune := offsetNoteFineTune(note, sample.FineTune, pitchDelta)
		freq := xmFrequency(pitchedNote, pitchedFineTune, m.module.Flags&modmodel.FlagLinearPeriods != 0)
		if sample.DivFactor > 0 {
			freq /= float64(sample.DivFactor)
		}
		if freq <= 0 {
			return 0
		}
		return m.quantizedIncrement(freq)
	}

	pitchOffset := math.Pow(2, float64(pitchDelta)/1536.0)
	freq := oldPeriodFrequency(note, sample.C5Speed)
	if sample.DivFactor > 0 {
		freq /= float64(sample.DivFactor)
	}
	if freq <= 0 {
		return 0
	}
	return m.quantizedIncrement(freq * pitchOffset)
}

func (m *Mixer) ensureTickBufSize(samples int) {
	if len(m.tickBuf) >= samples {
		return
	}
	m.tickBuf = make([]int32, samples)
}

func (m *Mixer) addChannel(v *voice, dest []int32, frames int) {
	if v.sample < 0 || v.sample >= len(m.module.Samples) {
		v.active = false
		return
	}

	sample := &m.module.Samples[v.sample]
	if len(sample.Data) == 0 || v.increment == 0 {
		v.active = false
		v.lastLeftValue = 0
		v.lastRightValue = 0
		return
	}

	todo := frames
	ptr := 0
	fracBits := m.mixFractionBits()
	for todo > 0 {
		loopStart, loopEnd, bidi, looped := activeLoop(sample, v.keyOn)
		idxsize := (int64(len(sample.Data)) << fracBits) - 1
		var idxlpos, idxlend int64
		if looped {
			idxlpos = int64(loopStart) << fracBits
			idxlend = (int64(loopEnd) << fracBits) - 1
		}

		if v.increment < 0 {
			if looped && v.current < idxlpos {
				if bidi {
					v.current = idxlpos + (idxlpos - v.current)
					v.increment = -v.increment
				} else {
					v.current = idxlend - (idxlpos - v.current)
				}
			} else if !looped && v.current < 0 {
				v.current = 0
				v.active = false
				break
			}
		} else {
			if looped && v.current >= idxlend {
				if bidi {
					v.increment = -v.increment
					v.current = idxlend - (v.current - idxlend)
				} else {
					v.current = idxlpos + (v.current - idxlend)
				}
			} else if !looped && v.current >= idxsize {
				v.current = 0
				v.active = false
				break
			}
		}

		var end int64
		switch {
		case v.increment < 0 && looped:
			end = idxlpos
		case v.increment < 0:
			end = 0
		case looped:
			end = idxlend
		default:
			end = idxsize
		}

		done := 0
		if !((v.increment > 0 && v.current >= end) || (v.increment < 0 && v.current <= end) || v.increment == 0) {
			span := (end-v.current)/v.increment + 1
			if span > 0 {
				done = minInt(int(span), todo)
			}
		}
		if done <= 0 {
			v.active = false
			break
		}

		endPos := v.current + int64(done)*v.increment
		if v.rawVolume != 0 {
			v.current = m.mixVoice(sample, v, dest, ptr, done)
		} else {
			v.lastLeftValue = 0
			v.lastRightValue = 0
			v.current = endPos
		}

		todo -= done
		if m.outputChannels == 2 {
			ptr += done << 1
		} else {
			ptr += done
		}
	}
}

func (m *Mixer) mixVoice(sample *modmodel.Sample, v *voice, dest []int32, ptr, count int) int64 {
	current := v.current
	lastMixedLeft := v.lastLeftValue
	lastMixedRight := v.lastRightValue
	clickShift := m.mixClickShift()
	clickBuffer := m.mixClickBuffer()
	for i := 0; i < count; i++ {
		value := m.sampleAt(sample, current, v.keyOn)
		leftMix, rightMix, ramping := rampMixSelectors(v, clickShift)
		clicking := v.clickRemaining > 0 && !ramping

		if m.outputChannels == 1 {
			if ramping {
				dest[ptr+i] += int32((int64(leftMix) * int64(value)) >> clickShift)
			} else if clicking {
				dest[ptr+i] += int32(((int64(v.leftSel*(clickBuffer-v.clickRemaining)) * int64(value)) + int64(lastMixedLeft)*int64(v.clickRemaining)) >> clickShift)
			} else {
				dest[ptr+i] += int32(v.leftSel) * value
			}
		} else if v.rawPanning == int(modmodel.PanSurround) {
			vol := v.leftSel
			if v.rightSel > vol {
				vol = v.rightSel
			}
			base := ptr + i*2
			if ramping {
				rampVol := leftMix
				if rightMix > rampVol {
					rampVol = rightMix
				}
				mixed := int32((int64(rampVol) * int64(value)) >> clickShift)
				dest[base] += mixed
				dest[base+1] -= mixed
			} else if clicking {
				mixed := int32(((int64(vol*(clickBuffer-v.clickRemaining)) * int64(value)) + int64(lastMixedLeft)*int64(v.clickRemaining)) >> clickShift)
				dest[base] += mixed
				dest[base+1] -= mixed
			} else {
				mixed := int32(vol) * value
				dest[base] += mixed
				dest[base+1] -= mixed
			}
		} else {
			base := ptr + i*2
			if ramping {
				dest[base] += int32((int64(leftMix) * int64(value)) >> clickShift)
				dest[base+1] += int32((int64(rightMix) * int64(value)) >> clickShift)
			} else if clicking {
				dest[base] += int32(((int64(v.leftSel*(clickBuffer-v.clickRemaining)) * int64(value)) + int64(lastMixedLeft)*int64(v.clickRemaining)) >> clickShift)
				dest[base+1] += int32(((int64(v.rightSel*(clickBuffer-v.clickRemaining)) * int64(value)) + int64(lastMixedRight)*int64(v.clickRemaining)) >> clickShift)
			} else {
				dest[base] += int32(v.leftSel) * value
				dest[base+1] += int32(v.rightSel) * value
			}
		}

		current += v.increment
		if v.rampRemaining > 0 {
			v.rampRemaining--
		} else if v.clickRemaining > 0 {
			v.clickRemaining--
		}
		lastMixedLeft = int32(v.leftSel) * value
		if m.outputChannels == 1 || v.rawPanning == int(modmodel.PanSurround) {
			lastMixedRight = lastMixedLeft
		} else {
			lastMixedRight = int32(v.rightSel) * value
		}
	}
	v.lastLeftValue = lastMixedLeft
	v.lastRightValue = lastMixedRight
	return current
}

func (m *Mixer) sampleAt(sample *modmodel.Sample, current int64, keyOn bool) int32 {
	fracBits := m.mixFractionBits()
	index := int(current >> fracBits)
	primary, ok := sampleValueAt(sample, index)
	if !ok {
		return 0
	}
	if !m.interpolation {
		return int32(primary)
	}

	secondary := interpolationSampleAt(sample, index, keyOn)
	frac := current & m.mixFractionMask()
	weightA := m.mixFractionScale() - frac
	weightB := frac
	return int32(((int64(primary) * weightA) + (int64(secondary) * weightB)) >> fracBits)
}

func sampleValueAt(sample *modmodel.Sample, index int) (int16, bool) {
	if sample == nil || index < 0 || index >= len(sample.Data) {
		return 0, false
	}
	return sample.Data[index], true
}

func interpolationSampleAt(sample *modmodel.Sample, index int, keyOn bool) int16 {
	nextIndex := index + 1
	start, end, bidi, looped := activeLoop(sample, keyOn)
	if looped && index >= start && nextIndex >= end {
		if bidi {
			guard := end - 1
			if value, ok := sampleValueAt(sample, guard); ok {
				return value
			}
			return 0
		}
		if value, ok := sampleValueAt(sample, start); ok {
			return value
		}
		return 0
	}

	value, ok := sampleValueAt(sample, nextIndex)
	if !ok {
		return 0
	}
	return value
}

func readSampleAt(sample *modmodel.Sample, index int, keyOn bool) (int16, bool) {
	mapped, ok := mapSampleIndex(sample, index, keyOn)
	if !ok {
		return 0, false
	}
	return sampleValueAt(sample, mapped)
}

func activeLoop(sample *modmodel.Sample, keyOn bool) (int, int, bool, bool) {
	length := len(sample.Data)
	if keyOn && sample.Flags&modmodel.SampleSustainLoop != 0 {
		start := clampInt(int(sample.SustainStart), 0, length)
		end := clampInt(int(sample.SustainEnd), 0, length)
		if end > start {
			return start, end, sample.Flags&modmodel.SampleSustainBidiLoop != 0, true
		}
	}
	if sample.Flags&modmodel.SampleLoop != 0 {
		start := clampInt(int(sample.LoopStart), 0, length)
		end := clampInt(int(sample.LoopEnd), 0, length)
		if end > start {
			return start, end, sample.Flags&modmodel.SampleBidiLoop != 0, true
		}
	}
	return 0, 0, false, false
}

func mapSampleIndex(sample *modmodel.Sample, index int, keyOn bool) (int, bool) {
	length := len(sample.Data)
	if length == 0 {
		return 0, false
	}
	if index >= 0 && index < length {
		return index, true
	}

	start, end, bidi, looped := activeLoop(sample, keyOn)
	if !looped {
		return 0, false
	}

	if bidi {
		span := end - start
		if span <= 1 {
			return start, true
		}
		period := 2 * (span - 1)
		rel := positiveMod(index-start, period)
		if rel >= span {
			rel = period - rel
		}
		return start + rel, true
	}

	loopLen := end - start
	if loopLen <= 0 {
		return 0, false
	}
	return start + positiveMod(index-start, loopLen), true
}

func sampleStartOffset(sample *modmodel.Sample, offset int, keyOn bool, fractionBits int) int64 {
	if sample == nil {
		return 0
	}
	if offset < 0 {
		offset = 0
	}
	length := len(sample.Data)
	if offset > length {
		start, _, _, looped := activeLoop(sample, keyOn)
		if looped {
			offset = start
		} else {
			offset = length
		}
	}
	return int64(offset) << fractionBits
}

func mixSelectors(panning int, volume int, stereo bool) (int, int) {
	if volume < 0 {
		volume = 0
	}
	if volume > 256 {
		volume = 256
	}
	if !stereo {
		return volume, volume
	}
	if panning == int(modmodel.PanSurround) {
		gain := volume / 2
		return gain, gain
	}
	if panning < 0 {
		panning = 0
	}
	if panning > 255 {
		panning = 255
	}
	left := (volume * (255 - panning)) >> 8
	right := (volume * panning) >> 8
	return left, right
}

func rampMixSelectors(v *voice, clickShift int) (int, int, bool) {
	if v.rampRemaining <= 0 {
		return v.leftSel, v.rightSel, false
	}
	left := (v.leftSel << clickShift) + (v.oldLeftSel-v.leftSel)*v.rampRemaining
	right := (v.rightSel << clickShift) + (v.oldRightSel-v.rightSel)*v.rampRemaining
	return left, right, true
}

func (m *Mixer) mix32ToFloat(dst []float32, src []int32, frames int) {
	if m.mixSamplingFactor() == 1 {
		for i, value := range src[:len(dst)] {
			mixed := float32(value>>(bitShift-fpShift)) * floatMixScale
			mixed *= m.masterVolume
			dst[i] = clampSample(mixed)
		}
		return
	}

	factor := m.mixSamplingFactor()
	if m.outputChannels == 1 {
		for frame := 0; frame < frames; frame++ {
			sum := float32(0)
			base := frame * factor
			for i := 0; i < factor; i++ {
				sum += clampSample(float32(src[base+i]>>(bitShift-fpShift)) * floatMixScale)
			}
			dst[frame] = clampSample((sum / float32(factor)) * m.masterVolume)
		}
		return
	}

	for frame := 0; frame < frames; frame++ {
		sumLeft := float32(0)
		sumRight := float32(0)
		base := frame * factor * 2
		for i := 0; i < factor; i++ {
			sumLeft += clampSample(float32(src[base+i*2]>>(bitShift-fpShift)) * floatMixScale)
			sumRight += clampSample(float32(src[base+i*2+1]>>(bitShift-fpShift)) * floatMixScale)
		}
		dst[frame*2] = clampSample((sumLeft / float32(factor)) * m.masterVolume)
		dst[frame*2+1] = clampSample((sumRight / float32(factor)) * m.masterVolume)
	}
}

func (m *Mixer) mix32To16(dst []int16, src []int32, frames int) {
	if m.mixSamplingFactor() == 1 {
		for i, value := range src[:len(dst)] {
			dst[i] = m.mix16Sample(value >> bitShift)
		}
		return
	}

	factor := int32(m.mixSamplingFactor())
	if m.outputChannels == 1 {
		for frame := 0; frame < frames; frame++ {
			sum := int32(0)
			base := frame * int(factor)
			for i := 0; i < int(factor); i++ {
				sum += clampPCM16(src[base+i] >> bitShift)
			}
			dst[frame] = m.mix16Sample(sum / factor)
		}
		return
	}

	for frame := 0; frame < frames; frame++ {
		sumLeft := int32(0)
		sumRight := int32(0)
		base := frame * int(factor) * 2
		for i := 0; i < int(factor); i++ {
			sumLeft += clampPCM16(src[base+i*2] >> bitShift)
			sumRight += clampPCM16(src[base+i*2+1] >> bitShift)
		}
		dst[frame*2] = m.mix16Sample(sumLeft / factor)
		dst[frame*2+1] = m.mix16Sample(sumRight / factor)
	}
}

func clampSample(value float32) float32 {
	switch {
	case value < -1:
		return -1
	case value > 1:
		return 1
	default:
		return value
	}
}

func clampPCM16(sample int32) int32 {
	switch {
	case sample >= 32768:
		return 32767
	case sample < -32768:
		return -32768
	default:
		return sample
	}
}

func (m *Mixer) mix16Sample(sample int32) int16 {
	sample = clampPCM16(sample)
	if m.masterVolume != 1 {
		scaled := float32(sample) * m.masterVolume
		switch {
		case scaled >= 32767:
			sample = 32767
		case scaled <= -32768:
			sample = -32768
		default:
			sample = int32(scaled)
		}
	}
	return int16(sample)
}

func quantizedIncrement(freq float64, sampleRate int) int64 {
	if freq <= 0 || sampleRate <= 0 {
		return 0
	}
	increment := int64(freq)
	increment = (increment << fractionBits) / int64(sampleRate)
	if increment <= 0 {
		return 0
	}
	return increment
}

func quantizedStep(freq float64, sampleRate int) float64 {
	increment := quantizedIncrement(freq, sampleRate)
	if increment <= 0 {
		return 0
	}
	return float64(increment) / float64(1<<fractionBits)
}

func positiveMod(value, mod int) int {
	if mod <= 0 {
		return 0
	}
	value %= mod
	if value < 0 {
		value += mod
	}
	return value
}

func xmFrequency(note int, fineTune int16, linear bool) float64 {
	if linear {
		return xmLinearFrequency(note, fineTune)
	}
	return xmLogFrequency(note, fineTune)
}

func xmLinearFrequency(note int, fineTune int16) float64 {
	return float64(pitch.XMLinearFrequency(note, fineTune))
}

func xmLogFrequency(note int, fineTune int16) float64 {
	period := xmLogPeriod(note, fineTune)
	if period <= 0 {
		return 0
	}
	return float64((8363 * 1712) / period)
}

func xmLogPeriod(note int, fineTune int16) int {
	fine := clampInt(int(fineTune)+128, 0, 255)
	noteParam := note * 2
	n := positiveMod(noteParam, 24)
	o := noteParam / 24
	index := (n << 2) + (fine >> 4)
	p1 := int(xmLogTab[index])
	p2 := int(xmLogTab[index+1])
	return interpolateInt(fine>>4, 0, 15, p1, p2) >> o
}

func oldPeriodFrequency(note int, speed uint32) float64 {
	if speed == 0 {
		speed = uint32(xmReferenceC5Hz)
	}

	noteParam := note * 2
	octave := noteParam / len(oldPeriods)
	index := positiveMod(noteParam, len(oldPeriods))

	numerator := int64(8363) * int64(oldPeriods[index])
	period := (numerator >> octave) / int64(speed)
	if period <= 0 {
		return 0
	}

	return float64(8363*1712) / float64(period)
}

func interpolateInt(pos, p1, p2, v1, v2 int) int {
	if p1 == p2 || pos == p1 {
		return v1
	}
	return v1 + (pos-p1)*(v2-v1)/(p2-p1)
}

func offsetNoteFineTune(note int, fineTune int16, pitchDelta int) (int, int16) {
	total := note*128 + int(fineTune) + pitchDelta
	shiftedNote := total / 128
	shiftedFineTune := total - shiftedNote*128
	if shiftedFineTune < -128 {
		shiftedFineTune += 128
		shiftedNote--
	} else if shiftedFineTune > 127 {
		shiftedFineTune -= 128
		shiftedNote++
	}
	return shiftedNote, int16(shiftedFineTune)
}

func clampInt(value, minValue, maxValue int) int {
	switch {
	case value < minValue:
		return minValue
	case value > maxValue:
		return maxValue
	default:
		return value
	}
}

func initialPanning(index int) int {
	if index&1 != 0 {
		return int(modmodel.PanLeft)
	}
	return int(modmodel.PanRight)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func abs64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
