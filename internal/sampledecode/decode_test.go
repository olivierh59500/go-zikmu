package sampledecode

import (
	"reflect"
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/module"
)

func TestDecodeRaw8Unsigned(t *testing.T) {
	got, err := Decode([]byte{0x00, 0x80, 0xff}, 3, 0, Options{})
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	want := []int16{-32768, 0, 32512}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected samples: got=%v want=%v", got, want)
	}
}

func TestDecodeRaw16BigEndianSigned(t *testing.T) {
	got, err := Decode([]byte{0x12, 0x34, 0xff, 0xfe}, 2, module.Sample16Bits|module.SampleSigned|module.SampleBigEndian, Options{})
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	want := []int16{0x1234, -2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected samples: got=%v want=%v", got, want)
	}
}

func TestDecodeDelta8Signed(t *testing.T) {
	got, err := Decode([]byte{0x01, 0x02, 0xff}, 3, module.SampleSigned|module.SampleDelta, Options{})
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	want := []int16{256, 768, 512}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected samples: got=%v want=%v", got, want)
	}
}

func TestDecodeADPCM4(t *testing.T) {
	data := make([]byte, 18)
	data[0] = 1
	data[1] = 2
	data[16] = 0x10

	got, err := Decode(data, 2, module.SampleADPCM4|module.SampleSigned, Options{})
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	want := []int16{256, 768}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected samples: got=%v want=%v", got, want)
	}
}

func TestDecodeITPacked8Zeros(t *testing.T) {
	data := append([]byte{9, 0}, make([]byte, 9)...)

	got, err := Decode(data, 8, module.SampleITPacked|module.SampleSigned, Options{})
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	want := make([]int16, 8)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected samples: got=%v want=%v", got, want)
	}
}

func TestDecodeITPacked16Zeros(t *testing.T) {
	data := append([]byte{9, 0}, make([]byte, 9)...)

	got, err := Decode(data, 4, module.SampleITPacked|module.Sample16Bits|module.SampleSigned, Options{})
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	want := make([]int16, 4)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected samples: got=%v want=%v", got, want)
	}
}

func TestDecodeIntoSampleAppliesDownmixScaleAndLoopSanitizing(t *testing.T) {
	sample := &module.Sample{
		Flags:     module.SampleSigned | module.SampleStereo | module.SampleLoop | module.SampleBidiLoop | module.SampleSustainLoop,
		Length:    8,
		LoopStart: 2,
		LoopEnd:   6,
	}

	data := []byte{0x00, 0x00, 0x40, 0x40, 0x7f, 0x7f, 0x40, 0x40}

	err := DecodeIntoSample(sample, data, Options{
		DownmixToMono: true,
		ScaleFactor:   2,
	})
	if err != nil {
		t.Fatalf("DecodeIntoSample failed: %v", err)
	}

	wantData := []int16{8192, 24448}
	if !reflect.DeepEqual(sample.Data, wantData) {
		t.Fatalf("unexpected sample data: got=%v want=%v", sample.Data, wantData)
	}
	if sample.Flags&module.SampleStereo != 0 {
		t.Fatal("expected stereo flag to be cleared after downmix")
	}
	if sample.DivFactor != 2 {
		t.Fatalf("unexpected div factor: %d", sample.DivFactor)
	}
	if sample.Length != 2 || sample.LoopStart != 0 || sample.LoopEnd != 1 {
		t.Fatalf("unexpected loop-adjusted sample metadata: length=%d loop=%d..%d", sample.Length, sample.LoopStart, sample.LoopEnd)
	}
	if sample.Flags&module.SampleLoop == 0 || sample.Flags&module.SampleBidiLoop == 0 {
		t.Fatalf("expected loop flags to remain set: %#x", sample.Flags)
	}
	if sample.Flags&module.SampleSustainLoop != 0 {
		t.Fatalf("expected invalid sustain loop to be cleared: %#x", sample.Flags)
	}
}

func TestDecodeRejectsOddStereoDownmix(t *testing.T) {
	_, err := Decode([]byte{0x00, 0x10, 0x20}, 3, module.SampleSigned|module.SampleStereo, Options{DownmixToMono: true})
	if err == nil {
		t.Fatal("expected an error for odd stereo sample count")
	}
}
