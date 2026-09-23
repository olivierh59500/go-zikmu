package st3

func (s *state) getLastNfo(ch *channel) {
	if ch.info == 0 {
		ch.info = ch.alastnfo
	}
}

func (s *state) setSpd(ch *channel) {
	v := &s.voice[ch.channelnum]
	ch.achannelused |= 0x80

	if s.amigalimits {
		if uint16(ch.aorgspd) > uint16(s.aspdmax) {
			ch.aorgspd = s.aspdmax
		}
		if ch.aorgspd < s.aspdmin {
			ch.aorgspd = s.aspdmin
		}
	}

	tmpspd := ch.aspd
	if uint16(tmpspd) > uint16(s.aspdmax) {
		tmpspd = s.aspdmax
		if s.amigalimits {
			ch.aspd = tmpspd
		}
	}

	if tmpspd == 0 {
		v.mSpeed = 0
		v.mSpeedRev = ^uint32(0)
		return
	}

	if tmpspd < s.aspdmin {
		tmpspd = s.aspdmin
		if s.amigalimits {
			ch.aspd = tmpspd
		}
	}

	v.mSpeed = uint32((s.dPer2HzDiv / float64(tmpspd)) + 0.5)
	if v.mSpeed == 0 {
		v.mSpeed = 1
	}
	v.mSpeedRev = ^uint32(0) / v.mSpeed
}

func (s *state) setVol(ch *channel, volFlag bool) {
	if volFlag {
		ch.achannelused |= 0x80
	}
	s.voiceSetVolume(ch.channelnum, int32(ch.avol)*int32(s.useglobalvol), int32(ch.apanpos))
}

func (s *state) stnote2herz(note uint8) int16 {
	if note == 254 {
		return 0
	}
	noteVal := notespd[note&0x0F]
	shiftVal := octavediv[note>>4]
	if shiftVal > 0 {
		noteVal >>= shiftVal & 0x1F
	}
	return noteVal
}

func (s *state) scalec2spd(ch *channel, spd int16) int16 {
	tmpspd := int32(spd) * c2Freq
	if (tmpspd >> 16) >= int32(ch.ac2spd) {
		return 32767
	}
	tmpspd /= int32(ch.ac2spd)
	if tmpspd > 32767 {
		tmpspd = 32767
	}
	return int16(tmpspd)
}

func (s *state) roundspd(ch *channel, spd int16) int16 {
	newspd := int32(spd) * int32(ch.ac2spd)
	if (newspd >> 16) >= c2Freq {
		return spd
	}
	newspd /= c2Freq

	var octa int8
	lastspd := (notespd[12] + notespd[11]) >> 1
	for lastspd >= int16(newspd) {
		octa++
		lastspd >>= 1
	}

	newnote := int8(0)
	notemin := int16(32767)
	for i := int8(0); i < 11; i++ {
		note := notespd[i]
		if octa > 0 {
			note >>= octa
		}
		diff := note - int16(newspd)
		if diff < 0 {
			diff = -diff
		}
		if diff < notemin {
			notemin = diff
			newnote = i
		}
	}

	newspd = int32(s.stnote2herz(uint8((octa<<4)|(newnote&0x0F)))) * c2Freq
	if (newspd >> 16) >= int32(ch.ac2spd) {
		return spd
	}
	newspd /= int32(ch.ac2spd)
	return int16(newspd)
}

func (s *state) triggerVoice(v *voice) {
	inst := v.insPtr
	if inst == nil {
		return
	}

	hasLoop := inst.flags&1 != 0
	is16bit := (inst.flags>>2)&1 != 0

	v.mLoopBeg = inst.lbeg
	v.mLoopLen = inst.lend512 - inst.lbeg

	if hasLoop && v.mLoopLen > 0 {
		v.mOrigEnd = inst.lend - inst.lbeg
		v.mEnd = inst.lend512
		v.mLoopFlag = true
	} else {
		v.mOrigEnd = inst.length
		v.mEnd = inst.length + 32
		v.mLoopFlag = false
	}

	if is16bit {
		v.mBase16 = inst.raw16
		v.mBase8 = nil
		v.mBaseOffset = samplePad16
	} else {
		v.mBase8 = inst.raw8
		v.mBase16 = nil
		v.mBaseOffset = samplePad8
	}

	v.lastMixFuncOffset = uint32((boolToInt(is16bit) << 2) + (boolToInt(s.interpolationFlag) << 1) + boolToInt(v.mLoopFlag))
	v.mMixfunc = mixRoutineTable[v.lastMixFuncOffset]
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *state) doAmiga(channelIdx uint8) {
	ch := &s.chn[channelIdx]
	v := &s.voice[channelIdx]

	if ch.ins > 0 {
		ch.astartoffset = 0
		ch.lastins = ch.ins
		if ch.ins <= 99 {
			inst := &s.ins[ch.ins-1]
			if inst.typ != 0 {
				if inst.typ == 1 {
					ch.ac2spd = inst.c2spd
					ch.avol = inst.vol
					ch.aorgvol = ch.avol
					s.setVol(ch, true)

					v.insPtr = inst
					if s.soundcardtype == soundcardSBPro {
						s.triggerVoice(v)
					}
				} else {
					ch.lastins = 0
				}
			}
		}
	}

	if ch.lastins == 0 {
		return
	}

	if ch.cmd == 'O'-64 {
		if ch.info == 0 {
			ch.astartoffset = ch.astartoffset00
		} else {
			ch.astartoffset00 = uint16(ch.info) << 8
			ch.astartoffset = ch.astartoffset00
		}
	}

	if ch.note != 255 {
		if ch.note == 254 {
			ch.aspd = 0
			s.setSpd(ch)
			ch.avol = 0
			s.setVol(ch, true)
			s.voiceCut(channelIdx)
			ch.asldspd = -1
		} else {
			if ch.cmd != 'G'-64 && ch.cmd != 'L'-64 {
				v.mPos = uint32(ch.astartoffset)
				v.mPosFrac = 0

				if s.soundcardtype == soundcardGUS {
					s.triggerVoice(v)
					if v.mPos >= v.mEnd && ch.astartoffset > 0 && v.mLoopFlag && v.mLoopLen > 0 {
						v.mPos = v.mLoopBeg + ((v.mPos - v.mEnd) % v.mLoopLen)
					}
				} else {
					if v.mPos > v.mOrigEnd && ch.astartoffset > 0 && v.mLoopFlag {
						v.mMixfunc = nil
					}
				}
			}

			if ch.ins > 0 && ch.ins <= 99 && s.ins[ch.ins-1].typ == 0 {
				s.voiceCut(channelIdx)
			}

			ch.lastnote = ch.note
			newspd := s.scalec2spd(ch, s.stnote2herz(ch.note))
			if ch.aorgspd == 0 || (ch.cmd != 'G'-64 && ch.cmd != 'L'-64) {
				ch.aspd = newspd
				s.setSpd(ch)
				ch.avibcnt = 0
				ch.aorgspd = newspd
			}
			ch.asldspd = newspd
		}
	}

	if ch.vol != 255 {
		ch.avol = int8(ch.vol)
		s.setVol(ch, true)
		ch.aorgvol = int8(ch.vol)
	}
}

func (s *state) donewNote(channelIdx uint8, notedelayflag bool) {
	ch := &s.chn[channelIdx]

	if notedelayflag {
		ch.achannelused = 0x81
	} else {
		if ch.channelnum > uint8(s.lastachannelused) {
			s.lastachannelused = int8(ch.channelnum + 1)
			if s.lastachannelused > 31 {
				s.lastachannelused = 31
			}
		}
		ch.achannelused = 0x01
		if ch.cmd == 'S'-64 {
			if (ch.info & 0xF0) == 0xD0 {
				return
			}
		}
	}

	s.doAmiga(channelIdx)
}

func (s *state) doNotes() {
	for i := 0; i < 32; i++ {
		ch := &s.chn[i]
		ch.note = 255
		ch.vol = 255
		ch.ins = 0
		ch.cmd = 0
		ch.info = 0
	}

	if s.npPatoff == -1 {
		if int(s.npPat) < len(s.patdata) {
			s.npPatseg = s.patdata[int(s.npPat)]
		} else {
			s.npPatseg = nil
		}
		if s.npPatseg != nil {
			j := 0
			if s.npRow > 0 {
				i := s.npRow
				for i > 0 {
					dat := s.readPatByte(&j)
					if dat == 0 {
						i--
					} else {
						if dat&0x20 != 0 {
							j += 2
						}
						if dat&0x40 != 0 {
							j++
						}
						if dat&0x80 != 0 {
							j += 2
						}
					}
				}
			}
			s.npPatoff = int16(j)
		}
	}

	for {
		channel := s.getNote1()
		if channel == 255 {
			break
		}
		if (s.chnsettings[channel] & 0x7F) < 16 {
			s.donewNote(channel, false)
		}
	}
}

func (s *state) doCmd1() {
	oldvolslidetype := s.volslidetype
	for i := uint8(0); i < uint8(s.lastachannelused)+1; i++ {
		ch := &s.chn[i]
		if ch.achannelused != 0 {
			if ch.info > 0 {
				ch.alastnfo = ch.info
			}
			if ch.cmd > 0 {
				ch.achannelused |= 0x80

				if ch.cmd == 'D'-64 {
					ch.atrigcnt = 0
					if ch.aspd != ch.aorgspd {
						ch.aspd = ch.aorgspd
						s.setSpd(ch)
					}
				} else {
					if ch.cmd != 'I'-64 {
						ch.atremor = 0
						ch.atreon = true
					}

					if ch.cmd != 'H'-64 && ch.cmd != 'U'-64 && ch.cmd != 'K'-64 && ch.cmd != 'R'-64 {
						ch.avibcnt |= 128
					}
				}

				if ch.cmd < 27 {
					s.volslidetype = 0
					sonceJump[ch.cmd](s, ch)
				}
			} else {
				ch.atrigcnt = 0
				if ch.aspd != ch.aorgspd {
					ch.aspd = ch.aorgspd
					s.setSpd(ch)
				}
				if !s.amigalimits && ch.cmd < 27 {
					s.volslidetype = 0
					sonceJump[ch.cmd](s, ch)
				}
			}
		}
	}
	s.volslidetype = oldvolslidetype
}

func (s *state) doCmd2() {
	oldvolslidetype := s.volslidetype
	for i := uint8(0); i < uint8(s.lastachannelused)+1; i++ {
		ch := &s.chn[i]
		if ch.achannelused != 0 && ch.cmd > 0 {
			ch.achannelused |= 0x80
			if ch.cmd < 27 {
				s.volslidetype = 0
				sotherJump[ch.cmd](s, ch)
			}
		}
	}
	s.volslidetype = oldvolslidetype
}

func (s *state) doRow() {
	s.npZframe++
	s.patmusicrand = uint16((((uint32(s.patmusicrand) * 0xCDEF) >> 16) + 0x1727) & 0xFFFF)

	if s.npPat == 255 {
		return
	}

	if s.musiccount == 0 {
		if s.patterndelay > 0 {
			s.npRow--
			s.doCmd1()
			s.patterndelay--
		} else {
			s.doNotes()
			s.doCmd1()
		}
	} else {
		s.doCmd2()
	}

	s.musiccount++
	if s.musiccount >= s.musicmax {
		s.npRow++
		if s.jumptorow != -1 {
			s.npRow = s.jumptorow
			s.jumptorow = -1
		}
		if s.npRow >= 64 || (s.patloopcount == 0 && s.breakpat > 0) {
			if s.breakpat == 255 {
				s.breakpat = 0
				return
			}
			s.breakpat = 0
			if s.jmptoord != -1 {
				s.npOrd = s.jmptoord
				s.jmptoord = -1
			}
			s.npRow = s.newOrder()
		}
		s.musiccount = 0
	}
}

func (s *state) sRet(ch *channel) {
	_ = ch
}

func (s *state) sSetGliss(ch *channel) {
	ch.aglis = ch.info & 0x0F
}

func (s *state) sSetFinetune(ch *channel) {
	_ = ch
}

func (s *state) sSetVibWave(ch *channel) {
	ch.avibtretype = (ch.avibtretype & 0xF0) | ((ch.info << 1) & 0x0F)
}

func (s *state) sSetTreWave(ch *channel) {
	ch.avibtretype = ((ch.info << 5) & 0xF0) | (ch.avibtretype & 0x0F)
}

func (s *state) sSetPanPos(ch *channel) {
	if s.soundcardtype == soundcardGUS {
		ch.surround = false
		ch.apanpos = ((ch.info & 0xF) << 4) | (ch.info & 0xF)
		s.setVol(ch, false)
	}
}

func (s *state) sSoundCntr(ch *channel) {
	if s.soundcardtype == soundcardGUS {
		info := ch.info & 0xF
		if info == 0 {
			ch.surround = false
			s.setVol(ch, false)
		} else if info == 1 {
			ch.surround = true
			ch.apanpos = 128
			s.setVol(ch, false)
		}
	}
}

func (s *state) sStereoCntr(ch *channel) {
	if s.soundcardtype == soundcardSBPro && (ch.info&0x0F) < 8 {
		ch.amixtype = ch.info & 0x0F
		s.setVol(ch, false)
	}
}

func (s *state) sPatLoop(ch *channel) {
	if (ch.info & 0x0F) == 0 {
		s.patloopstart = s.npRow
		return
	}

	if s.patloopcount == 0 {
		s.patloopcount = int8((ch.info & 0x0F) + 1)
		if s.patloopstart == -1 {
			s.patloopstart = 0
		}
	}

	if s.patloopcount > 1 {
		s.patloopcount--
		s.jumptorow = s.patloopstart
		s.npPatoff = -1
	} else {
		s.patloopcount = 0
		s.patloopstart = s.npRow + 1
	}
}

func (s *state) sNoteCut(ch *channel) {
	ch.anotecutcnt = ch.info & 0x0F
}

func (s *state) sNoteCutB(ch *channel) {
	if ch.anotecutcnt > 0 {
		ch.anotecutcnt--
		if ch.anotecutcnt == 0 {
			s.voice[ch.channelnum].mSpeed = 0
		}
	}
}

func (s *state) sNoteDelay(ch *channel) {
	ch.anotedelaycnt = ch.info & 0x0F
}

func (s *state) sNoteDelayB(ch *channel) {
	if ch.anotedelaycnt > 0 {
		ch.anotedelaycnt--
		if ch.anotedelaycnt == 0 {
			s.donewNote(ch.channelnum, true)
		}
	}
}

func (s *state) sPatternDelay(ch *channel) {
	if s.patterndelay == 0 {
		s.patterndelay = int8(ch.info & 0x0F)
	}
}

func (s *state) sSetSpeed(ch *channel) {
	s.setSpeed(ch.info)
}

func (s *state) sJumpTo(ch *channel) {
	if ch.info == 0xFF {
		s.breakpat = 255
	} else {
		s.breakpat = 1
		s.jmptoord = int16(ch.info)
	}
}

func (s *state) sBreak(ch *channel) {
	hi := ch.info >> 4
	lo := ch.info & 0x0F
	if hi <= 9 && lo <= 9 {
		s.startrow = uint8((hi * 10) + lo)
		s.breakpat = 1
	}
}

func (s *state) sVibVol(ch *channel) {
	s.volslidetype = 2
	s.sVolSlide(ch)
}

func (s *state) sToneVol(ch *channel) {
	s.volslidetype = 1
	s.sVolSlide(ch)
}

func (s *state) sVolSlide(ch *channel) {
	s.getLastNfo(ch)
	infohi := ch.info >> 4
	infolo := ch.info & 0x0F

	avol := int(ch.avol)
	if infolo == 0x0F {
		if infohi == 0 {
			avol -= int(infolo)
		} else if s.musiccount == 0 {
			avol += int(infohi)
		}
	} else if infohi == 0x0F {
		if infolo == 0 {
			avol += int(infohi)
		} else if s.musiccount == 0 {
			avol -= int(infolo)
		}
	} else if s.fastvolslide || s.musiccount > 0 {
		if infolo == 0 {
			avol += int(infohi)
		} else {
			avol -= int(infolo)
		}
	} else {
		return
	}

	avol = clamp(avol, 0, 63)
	ch.avol = int8(avol)
	s.setVol(ch, true)

	if s.volslidetype == 1 {
		s.sToneSlide(ch)
	} else if s.volslidetype == 2 {
		s.sVibrato(ch)
	}
}

func (s *state) sSlideDown(ch *channel) {
	if ch.aorgspd <= 0 {
		return
	}
	s.getLastNfo(ch)

	if s.musiccount > 0 {
		if ch.info >= 0xE0 {
			return
		}
		ch.aspd += int16(ch.info) << 2
		if uint16(ch.aspd) > 32767 {
			ch.aspd = 32767
		}
	} else {
		if ch.info <= 0xE0 {
			return
		}
		if ch.info <= 0xF0 {
			ch.aspd += int16(ch.info & 0x0F)
			if uint16(ch.aspd) > 32767 {
				ch.aspd = 32767
			}
		} else {
			ch.aspd += int16(ch.info&0x0F) << 2
			if uint16(ch.aspd) > 32767 {
				ch.aspd = 32767
			}
		}
	}

	ch.aorgspd = ch.aspd
	s.setSpd(ch)
}

func (s *state) sSlideUp(ch *channel) {
	if ch.aorgspd <= 0 {
		return
	}
	s.getLastNfo(ch)

	if s.musiccount > 0 {
		if ch.info >= 0xE0 {
			return
		}
		ch.aspd -= int16(ch.info) << 2
		if ch.aspd < 0 {
			ch.aspd = 0
		}
	} else {
		if ch.info <= 0xE0 {
			return
		}
		if ch.info <= 0xF0 {
			ch.aspd -= int16(ch.info & 0x0F)
			if ch.aspd < 0 {
				ch.aspd = 0
			}
		} else {
			ch.aspd -= int16(ch.info&0x0F) << 2
			if ch.aspd < 0 {
				ch.aspd = 0
			}
		}
	}

	ch.aorgspd = ch.aspd
	s.setSpd(ch)
}

func (s *state) sToneSlide(ch *channel) {
	var toneinfo uint8
	if s.volslidetype == 1 {
		toneinfo = ch.alasteff1
	} else {
		if ch.aorgspd == 0 {
			if ch.asldspd == 0 {
				return
			}
			ch.aorgspd = ch.asldspd
			ch.aspd = ch.asldspd
		}
		if ch.info == 0 {
			ch.info = ch.alasteff1
		} else {
			ch.alasteff1 = ch.info
		}
		toneinfo = ch.info
	}

	if ch.aorgspd != ch.asldspd {
		if ch.aorgspd < ch.asldspd {
			ch.aorgspd += int16(toneinfo) << 2
			if uint16(ch.aorgspd) > uint16(ch.asldspd) {
				ch.aorgspd = ch.asldspd
			}
		} else {
			ch.aorgspd -= int16(toneinfo) << 2
			if ch.aorgspd < ch.asldspd {
				ch.aorgspd = ch.asldspd
			}
		}

		if ch.aglis != 0 {
			ch.aspd = s.roundspd(ch, ch.aorgspd)
		} else {
			ch.aspd = ch.aorgspd
		}
		s.setSpd(ch)
	}
}

func (s *state) sVibrato(ch *channel) {
	var vibinfo uint8
	if s.volslidetype == 2 {
		vibinfo = ch.alasteff
	} else {
		if ch.info == 0 {
			ch.info = ch.alasteff
		}
		if (ch.info & 0xF0) == 0 {
			ch.info = (ch.alasteff & 0xF0) | (ch.info & 0x0F)
		}
		ch.alasteff = ch.info
		vibinfo = ch.alasteff
	}

	if ch.aorgspd > 0 {
		cnt := ch.avibcnt
		typ := (ch.avibtretype & 0x0E) >> 1
		var dat int32

		if typ == 0 || typ == 4 {
			if typ == 4 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int32(vibsin[cnt>>1])
		} else if typ == 1 || typ == 5 {
			if typ == 5 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int32(vibramp[cnt>>1])
		} else if typ == 2 || typ == 6 {
			if typ == 6 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int32(vibsqu[cnt>>1])
		} else if typ == 3 || typ == 7 {
			if typ == 7 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int32(vibsin[cnt>>1])
			cnt += int16(s.patmusicrand & 0x1E)
		}

		if s.oldstvib {
			ch.aspd = ch.aorgspd + int16((dat*int32(vibinfo&0x0F))>>4)
		} else {
			ch.aspd = ch.aorgspd + int16((dat*int32(vibinfo&0x0F))>>5)
		}
		s.setSpd(ch)
		ch.avibcnt = (cnt + int16((vibinfo>>4)<<1)) & 126
	}
}

func (s *state) sTremor(ch *channel) {
	s.getLastNfo(ch)

	if ch.atremor > 0 {
		ch.atremor--
		return
	}

	if ch.atreon {
		ch.atreon = false
		ch.avol = 0
		s.setVol(ch, true)
		ch.atremor = ch.info & 0x0F
	} else {
		ch.atreon = true
		ch.avol = ch.aorgvol
		s.setVol(ch, true)
		ch.atremor = ch.info >> 4
	}
}

func (s *state) sArp(ch *channel) {
	s.getLastNfo(ch)
	tick := s.musiccount % 3
	var noteadd uint8
	if tick == 1 {
		noteadd = ch.info >> 4
	} else if tick == 2 {
		noteadd = ch.info & 0x0F
	} else {
		noteadd = 0
	}

	octa := ch.lastnote & 0xF0
	note := (ch.lastnote & 0x0F) + noteadd
	for note >= 12 {
		note -= 12
		octa += 16
	}

	ch.aspd = s.scalec2spd(ch, s.stnote2herz(octa|note))
	s.setSpd(ch)
}

func (s *state) sRetrig(ch *channel) {
	v := &s.voice[ch.channelnum]
	s.getLastNfo(ch)
	infohi := ch.info >> 4

	if (ch.info&0x0F) == 0 || (ch.info&0x0F) > ch.atrigcnt {
		ch.atrigcnt++
		return
	}

	ch.atrigcnt = 0
	v.mPos = 0

	if s.soundcardtype != soundcardGUS {
		v.mPosFrac = 0
		if v.mMixfunc == nil && (v.mBase8 != nil || v.mBase16 != nil) {
			v.mMixfunc = mixRoutineTable[v.lastMixFuncOffset]
		}
	}

	if retrigvoladd[infohi+16] == 0 {
		ch.avol += retrigvoladd[infohi]
	} else {
		ch.avol = int8((int16(ch.avol) * int16(retrigvoladd[infohi+16])) >> 4)
	}

	ch.avol = int8(clamp(int(ch.avol), 0, 63))
	s.setVol(ch, true)
	ch.atrigcnt++
}

func (s *state) sTremolo(ch *channel) {
	s.getLastNfo(ch)
	if (ch.info & 0xF0) == 0 {
		ch.info = (ch.alastnfo & 0xF0) | (ch.info & 0x0F)
	}
	ch.alastnfo = ch.info

	if ch.aorgvol > 0 {
		cnt := ch.avibcnt
		typ := ch.avibtretype >> 5
		var dat int16

		if typ == 0 || typ == 4 {
			if typ == 4 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = vibsin[cnt>>1]
		} else if typ == 1 || typ == 5 {
			if typ == 5 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = vibramp[cnt>>1]
		} else if typ == 2 || typ == 6 {
			if typ == 6 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int16(vibsqu[cnt>>1])
		} else if typ == 3 || typ == 7 {
			if typ == 7 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = vibsin[cnt>>1]
			cnt += int16(s.patmusicrand & 0x1E)
		}

		d := int16(ch.aorgvol) + int16((int32(dat)*int32(ch.info&0x0F))>>7)
		d = int16(clamp(int(d), 0, 63))
		ch.avol = int8(d)
		s.setVol(ch, true)
		ch.avibcnt = (cnt + int16((ch.info&0xF0)>>3)) & 126
	}
}

func (s *state) sSet7BitPan(ch *channel) {
	if s.soundcardtype == soundcardGUS {
		if ch.info == 0xA4 {
			ch.surround = true
			ch.apanpos = 128
			s.setVol(ch, false)
		} else if ch.info <= 0x7F {
			ch.surround = false
			ch.apanpos = ch.info << 1
			s.setVol(ch, false)
		}
	}
}

func (s *state) sSCommand1(ch *channel) {
	s.getLastNfo(ch)
	ssonceJump[ch.info>>4](s, ch)
}

func (s *state) sSCommand2(ch *channel) {
	s.getLastNfo(ch)
	ssotherJump[ch.info>>4](s, ch)
}

func (s *state) sSetTempo(ch *channel) {
	if s.musiccount == 0 && ch.info >= 0x20 {
		s.setTempo(ch.info)
	}
}

func (s *state) sFineVibrato(ch *channel) {
	if ch.info == 0 {
		ch.info = ch.alasteff
	}
	if (ch.info & 0xF0) == 0 {
		ch.info = (ch.alasteff & 0xF0) | (ch.info & 0x0F)
	}
	ch.alasteff = ch.info

	if ch.aorgspd > 0 {
		cnt := ch.avibcnt
		typ := (ch.avibtretype & 0x0E) >> 1
		var dat int32

		if typ == 0 || typ == 4 {
			if typ == 4 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int32(vibsin[cnt>>1])
		} else if typ == 1 || typ == 5 {
			if typ == 5 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int32(vibramp[cnt>>1])
		} else if typ == 2 || typ == 6 {
			if typ == 6 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int32(vibsqu[cnt>>1])
		} else if typ == 3 || typ == 7 {
			if typ == 7 {
				cnt &= 0x7F
			} else if cnt&0x80 != 0 {
				cnt = 0
			}
			dat = int32(vibsin[cnt>>1])
			cnt += int16(s.patmusicrand & 0x1E)
		}

		if s.oldstvib {
			ch.aspd = ch.aorgspd + int16((dat*int32(ch.info&0x0F))>>6)
		} else {
			ch.aspd = ch.aorgspd + int16((dat*int32(ch.info&0x0F))>>7)
		}

		s.setSpd(ch)
		ch.avibcnt = (cnt + int16((ch.info>>4)<<1)) & 126
	}
}

func (s *state) sSetGVol(ch *channel) {
	if ch.info <= 64 {
		s.setGlobalVol(int8(ch.info))
	}
}

func (s *state) voiceCut(voiceNumber uint8) {
	v := &s.voice[voiceNumber]
	v.mMixfunc = nil
	v.mPos = 0
	v.mPosFrac = 0
}

func (s *state) voiceSetVolume(voiceNumber uint8, vol int32, pan int32) {
	v := &s.voice[voiceNumber]
	ch := &s.chn[voiceNumber]

	panL := pan ^ 0xFF
	panR := pan

	if ch.amixtype > 0 && s.soundcardtype == soundcardSBPro {
		centerPanVal := int32(128)
		if ch.amixtype >= 4 {
			panL = centerPanVal
			panR = centerPanVal
		} else if (ch.amixtype & 1) == 1 {
			tmp := panL
			panL = panR
			panR = tmp
		}
	}

	if ch.surround {
		panR = -panR
	}

	vol <<= 8
	v.mVolL = vol * panL
	v.mVolR = vol * panR
}

var ssonceJump = [...]effectRoutine{
	(*state).sRet,
	(*state).sSetGliss,
	(*state).sSetFinetune,
	(*state).sSetVibWave,
	(*state).sSetTreWave,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sSetPanPos,
	(*state).sSoundCntr,
	(*state).sStereoCntr,
	(*state).sPatLoop,
	(*state).sNoteCut,
	(*state).sNoteDelay,
	(*state).sPatternDelay,
	(*state).sRet,
}

var ssotherJump = [...]effectRoutine{
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sNoteCutB,
	(*state).sNoteDelayB,
	(*state).sRet,
	(*state).sRet,
}

var sonceJump = [...]effectRoutine{
	(*state).sRet,
	(*state).sSetSpeed,
	(*state).sJumpTo,
	(*state).sBreak,
	(*state).sVolSlide,
	(*state).sSlideDown,
	(*state).sSlideUp,
	(*state).sRet,
	(*state).sRet,
	(*state).sTremor,
	(*state).sArp,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRetrig,
	(*state).sRet,
	(*state).sSCommand1,
	(*state).sSetTempo,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sSet7BitPan,
	(*state).sRet,
	(*state).sRet,
}

var sotherJump = [...]effectRoutine{
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sVolSlide,
	(*state).sSlideDown,
	(*state).sSlideUp,
	(*state).sToneSlide,
	(*state).sVibrato,
	(*state).sTremor,
	(*state).sArp,
	(*state).sVibVol,
	(*state).sToneVol,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRetrig,
	(*state).sTremolo,
	(*state).sSCommand2,
	(*state).sRet,
	(*state).sFineVibrato,
	(*state).sSetGVol,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
	(*state).sRet,
}
