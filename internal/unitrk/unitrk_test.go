package unitrk

import "testing"

func TestBuilderProducesTrackRows(t *testing.T) {
	var b Builder

	b.Reset()
	ProTrackerEvent(&b, 49, 3, 0x0c, 32)
	b.VolumeEffect(VolPortamento, 0x0f)
	b.NewLine()

	track := b.Track()
	if len(track.Rows) != 1 {
		t.Fatalf("unexpected row count: %d", len(track.Rows))
	}

	row := track.Rows[0]
	if len(row.Commands) != 4 {
		t.Fatalf("unexpected command count: %d", len(row.Commands))
	}
	if row.Commands[0].Op != OpNote || row.Commands[0].Param != 49 {
		t.Fatalf("unexpected note command: %#v", row.Commands[0])
	}
	if row.Commands[1].Op != OpInstrument || row.Commands[1].Param != 3 {
		t.Fatalf("unexpected instrument command: %#v", row.Commands[1])
	}
	if row.Commands[2].Op != OpPTEffectC || row.Commands[2].Param != 32 {
		t.Fatalf("unexpected PT effect command: %#v", row.Commands[2])
	}

	effect, data := DecodePair(row.Commands[3].Param)
	if row.Commands[3].Op != OpVolumeEffects || effect != uint8(VolPortamento) || data != 0x0f {
		t.Fatalf("unexpected volume command: %#v", row.Commands[3])
	}
}

func TestBuilderWritesEmptyArpeggioWithMemory(t *testing.T) {
	var b Builder

	b.Reset()
	b.SetArpeggioMemory(true)
	b.PTEffect(0x00, 0x00)
	b.NewLine()

	track := b.Track()
	if len(track.Rows[0].Commands) != 1 {
		t.Fatalf("unexpected command count: %d", len(track.Rows[0].Commands))
	}
	if track.Rows[0].Commands[0].Op != OpPTEffect0 {
		t.Fatalf("unexpected opcode: %#v", track.Rows[0].Commands[0])
	}
}

func TestBuilderPreservesZeroValuedNotes(t *testing.T) {
	var b Builder

	b.Reset()
	b.Note(0)
	b.NewLine()

	row := b.Track().Rows[0]
	if len(row.Commands) != 1 {
		t.Fatalf("unexpected command count: %d", len(row.Commands))
	}
	if row.Commands[0].Op != OpNote || row.Commands[0].Param != 0 {
		t.Fatalf("unexpected zero note command: %#v", row.Commands[0])
	}
}

func TestS3MITConverterEncodesJumpAndFilters(t *testing.T) {
	var b Builder
	converter := S3MITConverter{
		ResolveOrder: func(order uint8) (uint8, bool) {
			if order == 7 {
				return 3, true
			}
			return 0, false
		},
		FiltersEnabled: true,
	}
	converter.FilterMacros[1] = 0x80
	converter.FilterSettings[0x92] = FilterSetting{Filter: 0x81, Info: 0x33}

	b.Reset()
	converter.Process(&b, 0x02, 0x07, 0)
	converter.Process(&b, 0x13, 0xf1, 0)
	converter.Process(&b, 0x1a, 0x92, 0)
	b.NewLine()

	row := b.Track().Rows[0]
	if len(row.Commands) != 2 {
		t.Fatalf("unexpected command count: %d", len(row.Commands))
	}
	if row.Commands[0].Op != OpPTEffectB || row.Commands[0].Param != 3 {
		t.Fatalf("unexpected jump command: %#v", row.Commands[0])
	}
	if row.Commands[1].Op != OpITEffectZ {
		t.Fatalf("unexpected filter command: %#v", row.Commands[1])
	}

	filter, info := DecodePair(row.Commands[1].Param)
	if filter != 0x81 || info != 0x33 {
		t.Fatalf("unexpected filter payload: %02x %02x", filter, info)
	}
}

func TestXMHelpersEncodeVolumeAndEffects(t *testing.T) {
	var b Builder

	b.Reset()
	XMEvent(&b, XMNoteCount+1, 2, 0x6f, 'G'-55, 64)
	b.NewLine()

	row := b.Track().Rows[0]
	if len(row.Commands) != 4 {
		t.Fatalf("unexpected command count: %d", len(row.Commands))
	}
	if row.Commands[0].Op != OpKeyFade {
		t.Fatalf("unexpected key fade command: %#v", row.Commands[0])
	}
	if row.Commands[1].Op != OpInstrument || row.Commands[1].Param != 1 {
		t.Fatalf("unexpected instrument command: %#v", row.Commands[1])
	}
	if row.Commands[2].Op != OpXMEffectA || row.Commands[2].Param != 0x0f {
		t.Fatalf("unexpected XM volume command: %#v", row.Commands[2])
	}
	if row.Commands[3].Op != OpXMEffectG || row.Commands[3].Param != 128 {
		t.Fatalf("unexpected XM effect command: %#v", row.Commands[3])
	}

	var fx Builder
	fx.Reset()
	XMEffect(&fx, 'G'-55, 64)
	fx.NewLine()

	if fx.Track().Rows[0].Commands[0].Op != OpXMEffectG || fx.Track().Rows[0].Commands[0].Param != 128 {
		t.Fatalf("unexpected XM effect command: %#v", fx.Track().Rows[0].Commands[0])
	}
}

func TestITHelpersEncodeAndValidate(t *testing.T) {
	var b Builder

	if err := ITVolumeColumn(&b, 126); err == nil {
		t.Fatal("expected invalid IT volume column error")
	}

	b.Reset()
	if err := ITEvent(&b, nil, 253, 2, 200, 0x14, 0x30, false); err != nil {
		t.Fatalf("ITEvent failed: %v", err)
	}
	b.NewLine()

	row := b.Track().Rows[0]
	if len(row.Commands) != 4 {
		t.Fatalf("unexpected command count: %d", len(row.Commands))
	}
	if row.Commands[0].Op != OpKeyOff {
		t.Fatalf("unexpected keyoff command: %#v", row.Commands[0])
	}
	if row.Commands[1].Op != OpInstrument || row.Commands[1].Param != 1 {
		t.Fatalf("unexpected instrument command: %#v", row.Commands[1])
	}
	if row.Commands[2].Op != OpVolumeEffects {
		t.Fatalf("unexpected volume effect command: %#v", row.Commands[2])
	}
	effect, data := DecodePair(row.Commands[2].Param)
	if effect != uint8(VolPortamento) || data != 96 {
		t.Fatalf("unexpected IT volume payload: effect=%d data=%d", effect, data)
	}
	if row.Commands[3].Op != OpS3MEffectT || row.Commands[3].Param != 0x30 {
		t.Fatalf("unexpected IT effect command: %#v", row.Commands[3])
	}
}
